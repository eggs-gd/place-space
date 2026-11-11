package otodim

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/eggs-gd/place-space/internal/domain"
	"github.com/eggs-gd/place-space/internal/sources"
)

const (
	maxBody      = 8 << 20
	flatsPrefix  = "/uk/rezultaty/orenda/kvartyry/"
	housesPrefix = "/uk/rezultaty/orenda/budynky/"
	// wholeCountry is the location a search around a place goes with: the
	// place's own circle does the narrowing.
	wholeCountry = "vsia-ukraina"
	// Otodim's form offers radii up to a few kilometres.
	maxRadiusMeters = 50_000
)

// placeID is the shape of a Google Places id.
var placeID = regexp.MustCompile(`^[A-Za-z0-9_-]{10,400}$`)

// SearchesNear marks the adapter as able to search around a place.
func (a *Adapter) SearchesNear() {}

// target is one catalog to read and the filter it carries.
type target struct {
	path   string
	params url.Values
}

// catalogPath is the only shape of path requested from Otodim. A city can be
// typed by a person as a path.
var catalogPath = regexp.MustCompile(`^/uk/rezultaty/orenda/kvartyry(/[a-z0-9-]+)+$`)

// Fetch reads each property catalog, newest first, until the last page, a
// repeated page, or maxPages.
func (a *Adapter) Fetch(ctx context.Context, query domain.Query) ([]sources.RawListing, error) {
	paths, err := searchPaths(query)
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

func searchPaths(query domain.Query) ([]target, error) {
	if query.Deal != "" && query.Deal != domain.DealRent {
		return nil, fmt.Errorf("otodim: unsupported deal %q", query.Deal)
	}
	params := url.Values{}
	var base string
	switch {
	case query.Near != nil:
		near := query.Near
		if !placeID.MatchString(near.PlaceID) {
			return nil, fmt.Errorf("otodim: invalid place id for %q", near.Name)
		}
		if near.RadiusMeters <= 0 || near.RadiusMeters > maxRadiusMeters {
			return nil, fmt.Errorf("otodim: radius %d m is out of range", near.RadiusMeters)
		}
		base = flatsPrefix + wholeCountry
		params.Set("placeId", near.PlaceID)
		params.Set("distanceRadius", strconv.Itoa(near.RadiusMeters))
	case len(query.Areas) > 0:
		return nil, fmt.Errorf("otodim: areas are not supported")
	default:
		var err error
		if base, err = cityPath(query.City); err != nil {
			return nil, err
		}
	}
	properties := query.Properties
	if len(properties) == 0 {
		properties = []string{domain.PropertyApartment}
	}
	var paths []target
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
		paths = append(paths, target{path: path, params: params})
	}
	return paths, nil
}

func cityPath(city string) (string, error) {
	city = strings.TrimSpace(city)
	if city == "" {
		return "", fmt.Errorf("otodim: city is empty")
	}
	if strings.HasPrefix(city, "/") {
		if !catalogPath.MatchString(city) {
			return "", fmt.Errorf("otodim: invalid search path %q; it must look like %skyiv/kyiv/kyiv/kyiv", city, flatsPrefix)
		}
		return city, nil
	}
	location, ok := cityLocations[normCity(city)]
	if !ok {
		return "", fmt.Errorf("otodim: unknown city %q; pass an Otodim path such as %skyiv/kyiv/kyiv/kyiv", city, flatsPrefix)
	}
	return flatsPrefix + location, nil
}

func propertyPath(base, property string) (string, error) {
	switch strings.ToLower(strings.TrimSpace(property)) {
	case "", domain.PropertyApartment, "flat", "flats", "квартира", "квартири":
		return base, nil
	case domain.PropertyHouse, "houses", "будинок", "будинки":
		return housesPrefix + strings.TrimPrefix(base, flatsPrefix), nil
	default:
		return "", fmt.Errorf("otodim: unsupported property %q", property)
	}
}

func (a *Adapter) fetchPath(ctx context.Context, path target, seen map[string]struct{}) ([]sources.RawListing, error) {
	var (
		out     []sources.RawListing
		errs    []error
		catalog = map[string]struct{}{}
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
		cards, totalPages, err := a.fetchPage(ctx, path, page)
		if err != nil {
			errs = append(errs, err)
			break
		}
		added := 0
		for _, raw := range cards {
			card, err := decodeCard(raw)
			if err != nil {
				errs = append(errs, err)
				continue
			}
			externalID := strconv.FormatInt(card.ID, 10)
			if _, ok := catalog[externalID]; ok {
				continue
			}
			catalog[externalID] = struct{}{}
			added++
			if _, ok := seen[externalID]; ok {
				continue
			}
			seen[externalID] = struct{}{}
			out = append(out, sources.RawListing{ExternalID: externalID, Payload: raw})
		}
		if added == 0 || page >= totalPages {
			break
		}
	}
	return out, errors.Join(errs...)
}

func (a *Adapter) fetchPage(ctx context.Context, path target, page int) ([]json.RawMessage, int, error) {
	endpoint, err := url.Parse(a.baseURL + path.path)
	if err != nil {
		return nil, 0, fmt.Errorf("otodim: build url: %w", err)
	}
	query := url.Values{"by": {"LATEST"}, "direction": {"DESC"}}
	for key, values := range path.params {
		query[key] = values
	}
	if page > 1 {
		query.Set("page", strconv.Itoa(page))
	}
	endpoint.RawQuery = query.Encode()
	body, err := a.load(ctx, endpoint)
	if err != nil {
		return nil, 0, err
	}
	cards, totalPages, err := extractCatalog(body)
	if err != nil {
		return nil, 0, fmt.Errorf("otodim: %s: %w", endpoint.Path, err)
	}
	return cards, totalPages, nil
}

func (a *Adapter) load(ctx context.Context, endpoint *url.URL) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint.String(), nil)
	if err != nil {
		return nil, fmt.Errorf("otodim: request: %w", err)
	}
	req.Header.Set("User-Agent", userAgent)
	req.Header.Set("Accept", "text/html,application/xhtml+xml")
	req.Header.Set("Accept-Language", "uk")

	resp, err := a.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("otodim: get %s: %w", endpoint.Path, err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, maxBody))
	if err != nil {
		return nil, fmt.Errorf("otodim: read %s: %w", endpoint.Path, err)
	}
	if resp.StatusCode == http.StatusForbidden || resp.StatusCode == http.StatusTooManyRequests {
		return nil, fmt.Errorf("otodim: %s: status %d: %w", endpoint.Path, resp.StatusCode, sources.ErrBlocked)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("otodim: %s: status %d", endpoint.Path, resp.StatusCode)
	}
	// An unknown location redirects to the whole of Ukraine.
	if resp.Request != nil && resp.Request.URL.Path != endpoint.Path {
		return nil, fmt.Errorf("otodim: %s redirected to %s", endpoint.Path, resp.Request.URL.Path)
	}
	return body, nil
}

func extractCatalog(body []byte) ([]json.RawMessage, int, error) {
	var state struct {
		Props struct {
			PageProps struct {
				Data struct {
					SearchAds *struct {
						Items      []json.RawMessage `json:"items"`
						Pagination struct {
							TotalPages int `json:"totalPages"`
						} `json:"pagination"`
					} `json:"searchAds"`
				} `json:"data"`
			} `json:"pageProps"`
		} `json:"props"`
	}
	if err := decodeNextData(body, &state); err != nil {
		return nil, 0, fmt.Errorf("decode listings: %w", err)
	}
	ads := state.Props.PageProps.Data.SearchAds
	if ads == nil {
		return nil, 0, fmt.Errorf("listings are missing from the page data")
	}
	return ads.Items, ads.Pagination.TotalPages, nil
}
