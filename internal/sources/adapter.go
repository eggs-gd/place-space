// Package sources is the boundary every listing provider implements.
package sources

import (
	"context"
	"encoding/json"
	"errors"

	"github.com/eggs-gd/place-space/internal/domain"
)

// ErrBlocked marks a response that says the site refuses or limits this client.
var ErrBlocked = errors.New("source refused the request")

// ErrIncompleteAreas accompanies a usable but partial list of areas.
var ErrIncompleteAreas = errors.New("some area pages could not be read")

// AreaReader is implemented by adapters that can list a city's areas.
//
// A source without areas for the city returns an empty list. When only part
// of the areas could be read, the list comes with ErrIncompleteAreas; any other
// error means there is no list.
type AreaReader interface {
	Areas(ctx context.Context, city string) ([]domain.Area, error)
}

// NearSearcher is implemented by adapters that can search around a place, given
// its Google Places id and a radius.
type NearSearcher interface {
	SearchesNear()
}

// Locator is implemented by adapters whose catalog omits coordinates that the
// listing's own page has.
type Locator interface {
	Locate(ctx context.Context, listing domain.Listing) (lat, lng float64, err error)
}

// PlacePointer is implemented by adapters that can tell where a Google place is.
type PlacePointer interface {
	PlaceCenter(ctx context.Context, placeID string) (lat, lng float64, err error)
}

// RawListing is one provider payload before normalization.
type RawListing struct {
	ExternalID string
	Payload    json.RawMessage
}

// Adapter fetches and normalizes one provider.
//
// Fetch may return listings together with an error when a later page failed
// after an earlier page succeeded. Callers keep the listings either way.
type Adapter interface {
	Type() string
	Fetch(ctx context.Context, query domain.Query) ([]RawListing, error)
	Normalize(ctx context.Context, raw RawListing) (domain.Listing, error)
}
