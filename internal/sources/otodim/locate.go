package otodim

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"regexp"
	"strconv"
	"strings"

	"github.com/eggs-gd/place-space/internal/domain"
)

// adPath is the only shape of listing page requested. The path comes from a
// stored listing URL, which this adapter built from the catalog slug.
var adPath = regexp.MustCompile(`^/uk/oholoshennia/[A-Za-z0-9-]+$`)

// Locate reads the listing's own page for its coordinates; the catalog has none.
func (a *Adapter) Locate(ctx context.Context, listing domain.Listing) (float64, float64, error) {
	parsed, err := url.Parse(listing.URL)
	if err != nil || !adPath.MatchString(parsed.Path) {
		return 0, 0, fmt.Errorf("otodim: not a listing page: %q", listing.URL)
	}
	endpoint, err := url.Parse(a.baseURL + parsed.Path)
	if err != nil {
		return 0, 0, err
	}
	body, err := a.load(ctx, endpoint)
	if err != nil {
		return 0, 0, err
	}
	var page struct {
		Props struct {
			PageProps struct {
				Ad struct {
					Location struct {
						Coordinates *struct {
							Latitude  float64 `json:"latitude"`
							Longitude float64 `json:"longitude"`
						} `json:"coordinates"`
					} `json:"location"`
				} `json:"ad"`
			} `json:"pageProps"`
		} `json:"props"`
	}
	if err := decodeNextData(body, &page); err != nil {
		return 0, 0, fmt.Errorf("otodim: %s: %w", endpoint.Path, err)
	}
	point := page.Props.PageProps.Ad.Location.Coordinates
	if point == nil || (point.Latitude == 0 && point.Longitude == 0) {
		return 0, 0, fmt.Errorf("otodim: %s: no coordinates", endpoint.Path)
	}
	return point.Latitude, point.Longitude, nil
}

// PlaceCenter asks Otodim for the circle it draws around a Google place and
// returns its centre. Google bills coordinates; Otodim resolves them anyway.
func (a *Adapter) PlaceCenter(ctx context.Context, id string) (float64, float64, error) {
	if !placeID.MatchString(id) {
		return 0, 0, fmt.Errorf("otodim: invalid place id")
	}
	endpoint, err := url.Parse(a.baseURL + flatsPrefix + wholeCountry)
	if err != nil {
		return 0, 0, err
	}
	endpoint.RawQuery = url.Values{"placeId": {id}, "limit": {strconv.Itoa(1)}}.Encode()
	body, err := a.load(ctx, endpoint)
	if err != nil {
		return 0, 0, err
	}
	var page struct {
		Props struct {
			PageProps struct {
				FilterLocations struct {
					ByGeometry []struct {
						GeoJSON string `json:"byGeoJson"`
					} `json:"byGeometry"`
				} `json:"filterLocations"`
			} `json:"pageProps"`
		} `json:"props"`
	}
	if err := decodeNextData(body, &page); err != nil {
		return 0, 0, fmt.Errorf("otodim: place: %w", err)
	}
	shapes := page.Props.PageProps.FilterLocations.ByGeometry
	if len(shapes) == 0 {
		return 0, 0, fmt.Errorf("otodim: no shape for the place")
	}
	return ringCenter(shapes[0].GeoJSON)
}

// ringCenter averages the outer ring of a GeoJSON polygon, given as a Feature
// or a bare geometry. For the circle Otodim draws, that is its centre.
func ringCenter(raw string) (float64, float64, error) {
	var shape struct {
		Type        string          `json:"type"`
		Geometry    json.RawMessage `json:"geometry"`
		Coordinates [][][2]float64  `json:"coordinates"`
	}
	if err := json.Unmarshal([]byte(raw), &shape); err != nil {
		return 0, 0, fmt.Errorf("otodim: place shape: %w", err)
	}
	if strings.EqualFold(shape.Type, "Feature") {
		return ringCenter(string(shape.Geometry))
	}
	if len(shape.Coordinates) == 0 || len(shape.Coordinates[0]) < 3 {
		return 0, 0, fmt.Errorf("otodim: place shape has no ring")
	}
	ring := shape.Coordinates[0]
	if ring[0] == ring[len(ring)-1] {
		ring = ring[:len(ring)-1]
	}
	var lat, lng float64
	for _, point := range ring {
		lng += point[0]
		lat += point[1]
	}
	n := float64(len(ring))
	return lat / n, lng / n, nil
}

func decodeNextData(body []byte, into any) error {
	const marker = `id="__NEXT_DATA__"`
	html := string(body)
	index := strings.Index(html, marker)
	if index < 0 {
		return fmt.Errorf("page data not found")
	}
	start := strings.IndexByte(html[index:], '>')
	if start < 0 {
		return fmt.Errorf("page data not found")
	}
	return json.NewDecoder(strings.NewReader(html[index+start+1:])).Decode(into)
}
