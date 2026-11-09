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

// AreaReader is implemented by adapters that can list a city's areas.
//
// It fails only when the city page itself cannot be read. A source without
// areas for the city returns an empty list.
type AreaReader interface {
	Areas(ctx context.Context, city string) ([]domain.Area, error)
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
