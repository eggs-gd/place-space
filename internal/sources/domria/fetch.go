package domria

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/eggs-gd/place-space/internal/domain"
	"github.com/eggs-gd/place-space/internal/sources"
)

const maxBody = 8 << 20

// Fetch loads every requested property path, following ?page= until a short
// page, a repeated page, or maxPages. An empty catalog is a successful fetch:
// DOM.RIA still embeds listings from another city when the city itself has none.
func (a *Adapter) Fetch(ctx context.Context, query domain.Query) ([]sources.RawListing, error) {
	paths, err := a.searchPaths(query)
	if err != nil {
		return nil, err
	}
	var (
		out  []sources.RawListing
		errs []error
		seen = map[string]struct{}{}
	)
	for _, path := range paths {
		listings, fetchErr := a.fetchPath(ctx, path, seen)
		out = append(out, listings...)
		if fetchErr != nil {
			errs = append(errs, fetchErr)
		}
	}
	return out, errors.Join(errs...)
}

func (a *Adapter) searchPaths(query domain.Query) ([]string, error) {
	deal := query.Deal
	if deal == "" {
		deal = domain.DealRent
	}
	if deal != domain.DealRent {
		return nil, fmt.Errorf("domria: unsupported deal %q", query.Deal)
	}
	base, err := a.cityPath(query.City)
	if err != nil {
		return nil, err
	}
	properties := query.Properties
	if len(properties) == 0 {
		properties = []string{domain.PropertyApartment}
	}
	var paths []string
	seen := map[string]struct{}{}
	for _, property := range properties {
		path, err := propertyPath(base, property)
		if err != nil {
			return nil, err
		}
		if _, ok := seen[path]; ok {
			continue
		}
		seen[path] = struct{}{}
		paths = append(paths, path)
	}
	if len(paths) == 0 {
		return nil, fmt.Errorf("domria: no search paths for %q", query.City)
	}
	return paths, nil
}

func (a *Adapter) cityPath(city string) (string, error) {
	city = strings.TrimSpace(city)
	if city == "" {
		return "", fmt.Errorf("domria: city is empty")
	}
	if strings.HasPrefix(city, "/") {
		return cleanPath(city)
	}
	path, ok := a.paths[normCity(city)]
	if !ok {
		return "", fmt.Errorf("domria: unknown city %q; pass a DOM.RIA path such as /uk/arenda-kvartir/kiev/", city)
	}
	return cleanPath(path)
}

func cleanPath(path string) (string, error) {
	if strings.Contains(path, "..") || strings.Contains(path, "://") || strings.Contains(path, "?") {
		return "", fmt.Errorf("domria: invalid search path %q", path)
	}
	if !strings.HasPrefix(path, "/uk/arenda-kvartir/") {
		return "", fmt.Errorf("domria: search path %q must start with /uk/arenda-kvartir/", path)
	}
	return path, nil
}

func propertyPath(base, property string) (string, error) {
	switch strings.ToLower(strings.TrimSpace(property)) {
	case "", domain.PropertyApartment, "flat", "flats", "квартира", "квартири":
		return base, nil
	case domain.PropertyHouse, "houses", "будинок", "будинки":
		if !strings.Contains(base, "/uk/arenda-kvartir/") {
			return "", fmt.Errorf("domria: cannot derive a house search from %s", base)
		}
		return strings.Replace(base, "/uk/arenda-kvartir/", "/uk/arenda-domov/", 1), nil
	default:
		return "", fmt.Errorf("domria: unsupported property %q", property)
	}
}

func (a *Adapter) fetchPath(ctx context.Context, path string, seen map[string]struct{}) ([]sources.RawListing, error) {
	var (
		out      []sources.RawListing
		pageSize int
		errs     []error
	)
	for page := 1; page <= a.maxPages; page++ {
		if page > 1 && a.pause > 0 {
			timer := time.NewTimer(a.pause)
			select {
			case <-ctx.Done():
				timer.Stop()
				return out, ctx.Err()
			case <-timer.C:
			}
		}
		cards, total, err := a.fetchPage(ctx, path, page)
		if err != nil {
			errs = append(errs, err)
			break
		}
		if len(cards) == 0 {
			break
		}
		if page == 1 {
			pageSize = len(cards)
		}
		added := 0
		for _, raw := range cards {
			card, err := decodeCard(raw)
			if err != nil {
				errs = append(errs, err)
				continue
			}
			externalID := strconv.FormatInt(card.ID, 10)
			if _, ok := seen[externalID]; ok {
				continue
			}
			seen[externalID] = struct{}{}
			out = append(out, sources.RawListing{ExternalID: externalID, Payload: raw})
			added++
		}
		if added == 0 || (pageSize > 0 && len(cards) < pageSize) || (total > 0 && len(out) >= total) {
			break
		}
	}
	return out, errors.Join(errs...)
}

func (a *Adapter) fetchPage(ctx context.Context, path string, page int) ([]json.RawMessage, int, error) {
	endpoint, err := url.Parse(a.baseURL + path)
	if err != nil {
		return nil, 0, fmt.Errorf("domria: build url: %w", err)
	}
	if page > 1 {
		query := endpoint.Query()
		query.Set("page", strconv.Itoa(page))
		endpoint.RawQuery = query.Encode()
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint.String(), nil)
	if err != nil {
		return nil, 0, fmt.Errorf("domria: request: %w", err)
	}
	req.Header.Set("User-Agent", userAgent)
	req.Header.Set("Accept", "text/html,application/xhtml+xml")
	req.Header.Set("Accept-Language", "uk")

	resp, err := a.http.Do(req)
	if err != nil {
		return nil, 0, fmt.Errorf("domria: get %s: %w", endpoint.Path, err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, maxBody))
	if err != nil {
		return nil, 0, fmt.Errorf("domria: read %s: %w", endpoint.Path, err)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, 0, fmt.Errorf("domria: %s: status %d", endpoint.Path, resp.StatusCode)
	}
	cards, total, err := extractCatalog(body)
	if err != nil {
		return nil, 0, fmt.Errorf("domria: %s: %w", endpoint.Path, err)
	}
	return cards, total, nil
}

func extractCatalog(body []byte) ([]json.RawMessage, int, error) {
	const marker = "window.__INITIAL_STATE__="
	html := string(body)
	index := strings.Index(html, marker)
	if index < 0 {
		return nil, 0, fmt.Errorf("embedded listings payload not found")
	}
	var state struct {
		Catalog struct {
			Items []json.RawMessage `json:"realtyForCatalog"`
			Count int               `json:"realtyCountCatalog"`
			Empty struct {
				IsEmpty bool `json:"isEmpty"`
			} `json:"isEmptyCatalog"`
		} `json:"catalog"`
	}
	decoder := json.NewDecoder(strings.NewReader(html[index+len(marker):]))
	if err := decoder.Decode(&state); err != nil {
		return nil, 0, fmt.Errorf("decode listings: %w", err)
	}
	if state.Catalog.Empty.IsEmpty || state.Catalog.Count <= 0 {
		return nil, 0, nil
	}
	return state.Catalog.Items, state.Catalog.Count, nil
}
