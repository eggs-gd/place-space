package lun

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

const (
	maxBody = 8 << 20
	// newestFirst is LUN's sort by publication time. The default order is by
	// relevance, which keeps new listings out of the pages read here.
	newestFirst = "insert_time"
)

// Fetch loads every requested property path, following ?page= until the last
// short page, a repeated page, or maxPages.
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
	for _, target := range paths {
		listings, fetchErr := a.fetchPath(ctx, target, seen)
		out = append(out, listings...)
		if fetchErr != nil {
			errs = append(errs, fetchErr)
		}
	}
	return out, errors.Join(errs...)
}

// target is one catalog to read. A strict target must be served at its own
// address: a redirect means the requested area no longer exists there.
type target struct {
	path   string
	strict bool
}

func (a *Adapter) basePaths(query domain.Query) ([]string, bool, error) {
	if len(query.AreaPaths) > 0 {
		paths := make([]string, 0, len(query.AreaPaths))
		for _, path := range query.AreaPaths {
			clean, err := cleanPath(path)
			if err != nil {
				return nil, false, err
			}
			paths = append(paths, clean)
		}
		return paths, true, nil
	}
	if len(query.Areas) > 0 {
		return nil, false, fmt.Errorf("lun: no catalog pages for the chosen areas")
	}
	base, err := a.cityPath(query.City)
	if err != nil {
		return nil, false, err
	}
	return []string{base}, false, nil
}

func (a *Adapter) searchPaths(query domain.Query) ([]target, error) {
	deal := query.Deal
	if deal == "" {
		deal = domain.DealRent
	}
	if deal != domain.DealRent {
		return nil, fmt.Errorf("lun: unsupported deal %q", query.Deal)
	}
	bases, strict, err := a.basePaths(query)
	if err != nil {
		return nil, err
	}
	properties := query.Properties
	if len(properties) == 0 {
		properties = []string{domain.PropertyApartment}
	}
	var paths []target
	seen := map[string]struct{}{}
	for _, base := range bases {
		for _, property := range properties {
			path, err := propertyPath(base, property)
			if err != nil {
				return nil, err
			}
			if _, ok := seen[path]; ok {
				continue
			}
			seen[path] = struct{}{}
			paths = append(paths, target{path: path, strict: strict})
		}
	}
	if len(paths) == 0 {
		return nil, fmt.Errorf("lun: no search paths for %q", query.City)
	}
	return paths, nil
}

func (a *Adapter) cityPath(city string) (string, error) {
	city = strings.TrimSpace(city)
	if city == "" {
		return "", fmt.Errorf("lun: city is empty")
	}
	if strings.HasPrefix(city, "/") {
		return cleanPath(city)
	}
	path, ok := a.paths[normCity(city)]
	if !ok {
		return "", fmt.Errorf("lun: unknown city %q; pass a LUN path such as /rent/kyiv/flats", city)
	}
	return cleanPath(path)
}

func cleanPath(path string) (string, error) {
	if strings.Contains(path, "..") || strings.Contains(path, "://") || strings.Contains(path, "?") {
		return "", fmt.Errorf("lun: invalid search path %q", path)
	}
	if !strings.HasPrefix(path, "/rent/") {
		return "", fmt.Errorf("lun: search path %q must start with /rent/", path)
	}
	return path, nil
}

func propertyPath(base, property string) (string, error) {
	switch strings.ToLower(strings.TrimSpace(property)) {
	case "", domain.PropertyApartment, "flat", "flats", "квартира", "квартири":
		return base, nil
	case domain.PropertyHouse, "houses", "будинок", "будинки":
		if !strings.Contains(base, "/flats") {
			return "", fmt.Errorf("lun: cannot derive a house search from %s", base)
		}
		return strings.Replace(base, "/flats", "/houses", 1), nil
	default:
		return "", fmt.Errorf("lun: unsupported property %q", property)
	}
}

func (a *Adapter) fetchPath(ctx context.Context, path target, seen map[string]struct{}) ([]sources.RawListing, error) {
	var (
		out      []sources.RawListing
		pageSize int
		errs     []error
		sorted   = true
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
		cards, total, err := a.fetchPage(ctx, path, page, sorted)
		if err == nil && page == 1 && len(cards) == 0 {
			// LUN answers a sort it does not know with an empty catalog. Without
			// this, a renamed value would look like a city with no listings.
			sorted = false
			cards, total, err = a.fetchPage(ctx, path, page, sorted)
		}
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

func (a *Adapter) fetchPage(ctx context.Context, path target, page int, sorted bool) ([]json.RawMessage, int, error) {
	endpoint, err := url.Parse(a.baseURL + path.path)
	if err != nil {
		return nil, 0, fmt.Errorf("lun: build url: %w", err)
	}
	query := endpoint.Query()
	if sorted {
		query.Set("sort", newestFirst)
	}
	if page > 1 {
		query.Set("page", strconv.Itoa(page))
	}
	endpoint.RawQuery = query.Encode()
	body, err := a.load(ctx, endpoint, path.strict)
	if err != nil {
		return nil, 0, err
	}
	payload, err := extractRealties(string(body))
	if err != nil {
		return nil, 0, fmt.Errorf("lun: %s: %w", endpoint.Path, err)
	}
	return payload.Cards, payload.Total, nil
}

func (a *Adapter) load(ctx context.Context, endpoint *url.URL, strict bool) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint.String(), nil)
	if err != nil {
		return nil, fmt.Errorf("lun: request: %w", err)
	}
	req.Header.Set("User-Agent", userAgent)
	req.Header.Set("Accept", "text/html,application/xhtml+xml")
	req.Header.Set("Accept-Language", "uk")

	resp, err := a.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("lun: get %s: %w", endpoint.Path, err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, maxBody))
	if err != nil {
		return nil, fmt.Errorf("lun: read %s: %w", endpoint.Path, err)
	}
	if resp.StatusCode == http.StatusForbidden || resp.StatusCode == http.StatusTooManyRequests {
		return nil, fmt.Errorf("lun: %s: status %d: %w", endpoint.Path, resp.StatusCode, sources.ErrBlocked)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("lun: %s: status %d", endpoint.Path, resp.StatusCode)
	}
	if strict && resp.Request != nil && strings.TrimRight(resp.Request.URL.Path, "/") != strings.TrimRight(endpoint.Path, "/") {
		return nil, fmt.Errorf("lun: %s redirected to %s", endpoint.Path, resp.Request.URL.Path)
	}
	return body, nil
}

type realtyEnvelope struct {
	Cards []json.RawMessage
	Total int
}

func extractRealties(html string) (realtyEnvelope, error) {
	const escaped = `{\"realties\":`
	if i := strings.Index(html, escaped); i >= 0 {
		decoded, err := unescapeJS(html[i:])
		if err != nil {
			return realtyEnvelope{}, err
		}
		return decodeEnvelope(decoded)
	}
	const plain = `{"realties":`
	if i := strings.Index(html, plain); i >= 0 {
		return decodeEnvelope(html[i:])
	}
	return realtyEnvelope{}, fmt.Errorf("embedded listings payload not found")
}

func decodeEnvelope(payload string) (realtyEnvelope, error) {
	var env struct {
		Realties struct {
			Cards []json.RawMessage `json:"cards"`
			Total int               `json:"totalRealtiesCount"`
		} `json:"realties"`
	}
	dec := json.NewDecoder(strings.NewReader(payload))
	if err := dec.Decode(&env); err != nil {
		return realtyEnvelope{}, fmt.Errorf("decode listings: %w", err)
	}
	return realtyEnvelope{Cards: env.Realties.Cards, Total: env.Realties.Total}, nil
}

// unescapeJS reverses the encoding LUN uses for the JSON embedded in a script
// string. It stops at the first unescaped quote, which closes that string.
func unescapeJS(s string) (string, error) {
	if len(s) > maxBody {
		s = s[:maxBody]
	}
	var b strings.Builder
	b.Grow(len(s))
	for i := 0; i < len(s); {
		c := s[i]
		if c == '"' {
			break
		}
		if c != '\\' {
			b.WriteByte(c)
			i++
			continue
		}
		if i+1 >= len(s) {
			return "", fmt.Errorf("truncated escape")
		}
		switch s[i+1] {
		case '"', '\\', '/':
			b.WriteByte(s[i+1])
			i += 2
		case 'n':
			b.WriteByte('\n')
			i += 2
		case 'r':
			b.WriteByte('\r')
			i += 2
		case 't':
			b.WriteByte('\t')
			i += 2
		case 'u':
			if i+6 > len(s) {
				return "", fmt.Errorf("truncated unicode escape")
			}
			v, err := strconv.ParseUint(s[i+2:i+6], 16, 32)
			if err != nil {
				return "", fmt.Errorf("unicode escape: %w", err)
			}
			b.WriteRune(rune(v))
			i += 6
		default:
			b.WriteByte(s[i+1])
			i += 2
		}
	}
	if b.Len() == 0 {
		return "", fmt.Errorf("empty embedded payload")
	}
	return b.String(), nil
}
