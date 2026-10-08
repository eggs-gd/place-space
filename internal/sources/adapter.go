// Package sources is the boundary every listing provider implements.
package sources

import (
	"context"
	"encoding/json"

	"github.com/eggs-gd/place-space/internal/domain"
)

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
