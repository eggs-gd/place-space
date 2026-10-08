// Package domain holds the monitoring model.
//
// Values that a source did not provide stay nil or empty. Nothing in this
// package fills them in from a guess.
package domain

import (
	"encoding/json"
	"time"
)

const (
	DealRent = "rent"

	PropertyApartment = "apartment"
	PropertyHouse     = "house"

	MatchMatched  = "matched"
	MatchRejected = "rejected"

	SourceHealthy = "healthy"
	SourceError   = "error"
	SourceUnknown = "unknown"

	DuplicateCanonicalURL = "canonical_url"
)

// Watch is a saved monitoring request.
type Watch struct {
	ID           string
	Name         string
	Enabled      bool
	SourceIDs    []string
	Query        Query
	Filters      Filters
	PollInterval time.Duration
	CreatedAt    time.Time
	UpdatedAt    time.Time
}

// Query is what to ask a source for. Deterministic limits live in Filters.
type Query struct {
	City       string   `json:"city"`
	Deal       string   `json:"deal"`
	Properties []string `json:"properties,omitempty"`
}

// Filters are deterministic predicates applied after normalization.
type Filters struct {
	PriceMin *int64   `json:"priceMin,omitempty"`
	PriceMax *int64   `json:"priceMax,omitempty"`
	Currency string   `json:"currency,omitempty"`
	RoomsMin *int     `json:"roomsMin,omitempty"`
	AreaMin  *float64 `json:"areaMin,omitempty"`
}

// Source is one configured external provider.
type Source struct {
	ID            string
	Type          string
	Enabled       bool
	Configuration json.RawMessage
	Status        string
	LastSuccessAt *time.Time
	LastErrorAt   *time.Time
	LastError     string
	CreatedAt     time.Time
	UpdatedAt     time.Time
}

// Listing is the canonical normalized entity.
//
// PriceAmount is whole currency units as published (hryvnias, not kopiykas).
// A nil pointer means the source did not give that fact.
type Listing struct {
	ID          string
	Source      string
	ExternalID  string
	URL         string
	Title       string
	Description string

	PriceAmount *int64
	Currency    string

	Location Location

	Rooms       *int
	Area        *float64
	Floor       *int
	TotalFloors *int

	Images []string

	PublishedAt *time.Time
	FirstSeenAt time.Time
	LastSeenAt  time.Time

	RawData json.RawMessage
}

// Location is whatever the source actually named.
type Location struct {
	Name      string
	Address   string
	Latitude  *float64
	Longitude *float64
}

// Observation is one snapshot of a listing. A new row is stored when the
// listing is first seen and whenever an important field changes.
type Observation struct {
	ID          string
	ListingID   string
	RunID       string
	SeenAt      time.Time
	PriceAmount *int64
	Currency    string
	Title       string
	Description string
	Images      []string
	Present     bool
	ContentHash string
}

// Match is the explainable relationship between one watch and one listing.
type Match struct {
	WatchID        string
	ListingID      string
	Status         string
	Reasons        []Reason
	FirstMatchedAt *time.Time
	LastMatchedAt  *time.Time
	UpdatedAt      time.Time
}

// Reason is one predicate result. Message is the form a person can read,
// for example "price: 24500 > 20000".
type Reason struct {
	Field   string `json:"field"`
	Passed  bool   `json:"passed"`
	Message string `json:"message"`
}

// Run is one source check performed for one watch.
type Run struct {
	ID             string
	WatchID        string
	SourceID       string
	SourceType     string
	StartedAt      time.Time
	FinishedAt     time.Time
	Received       int
	New            int
	Changed        int
	Matched        int
	Rejected       int
	Duplicates     int
	DuplicateLinks int
	Failed         int
	Errors         []string
}

// DuplicateLink connects two listings that are the same place by an exact
// key. Both listings stay stored.
type DuplicateLink struct {
	ListingA  string
	ListingB  string
	Reason    string
	CreatedAt time.Time
}

// Duration is how long the run took. It is zero until the run finishes.
func (r Run) Duration() time.Duration {
	if r.FinishedAt.IsZero() || r.StartedAt.IsZero() {
		return 0
	}
	return r.FinishedAt.Sub(r.StartedAt)
}
