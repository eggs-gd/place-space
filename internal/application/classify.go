package application

import (
	"fmt"
	"time"

	"github.com/eggs-gd/place-space/internal/domain"
	"github.com/eggs-gd/place-space/internal/id"
	"github.com/eggs-gd/place-space/internal/sources"
	"github.com/eggs-gd/place-space/internal/storage/sqlite"
)

const (
	OutcomeNew       = "new"
	OutcomeChanged   = "changed"
	OutcomeUnchanged = "unchanged"
)

// Item is one listing considered during a poll.
type Item struct {
	Outcome     string
	Changes     []domain.Change
	DuplicateOf string
	Decision    domain.Decision
	Listing     domain.Listing
}

// Classified is the in-memory result of one source check, ready to store.
type Classified struct {
	Run   domain.Run
	Items []Item
	Write sqlite.PollWrite
}

// ClassifyInput is everything classify needs. Normalize reports a bad payload
// without stopping the rest of the batch.
type ClassifyInput struct {
	Now         time.Time
	RunID       string
	Watch       domain.Watch
	Source      domain.Source
	Raw         []sources.RawListing
	Existing    []domain.Listing
	Matches     []domain.Match
	FetchErrors []string
	Normalize   func(sources.RawListing) (domain.Listing, error)
}

// Classify decides what is new, changed, unchanged, rejected, or duplicated.
//
// new + changed + duplicates + failed == received.
// Repeated rows of the same external id in one fetch count as duplicates and
// are not evaluated twice.
func Classify(in ClassifyInput) (Classified, error) {
	if in.Normalize == nil {
		return Classified{}, fmt.Errorf("normalize is nil")
	}
	if in.RunID == "" {
		return Classified{}, fmt.Errorf("run id is empty")
	}
	byExternal := make(map[string]domain.Listing, len(in.Existing))
	byURL := make(map[string]domain.Listing, len(in.Existing))
	for _, listing := range in.Existing {
		byExternal[listing.ExternalID] = listing
		if listing.URL != "" {
			byURL[listing.URL] = listing
		}
	}
	previousMatch := make(map[string]domain.Match, len(in.Matches))
	for _, match := range in.Matches {
		previousMatch[match.ListingID] = match
	}

	run := domain.Run{
		ID:         in.RunID,
		WatchID:    in.Watch.ID,
		SourceID:   in.Source.ID,
		SourceType: in.Source.Type,
		Received:   len(in.Raw),
		Errors:     append([]string{}, in.FetchErrors...),
	}
	seen := map[string]struct{}{}
	var (
		items        []Item
		listings     []sqlite.ListingUpsert
		observations []domain.Observation
		matches      []domain.Match
		links        []domain.DuplicateLink
	)

	for _, raw := range in.Raw {
		listing, err := in.Normalize(raw)
		if err != nil {
			run.Failed++
			label := raw.ExternalID
			if label == "" {
				label = "listing"
			}
			run.Errors = append(run.Errors, fmt.Sprintf("normalize %s: %s", label, err.Error()))
			continue
		}
		listing.Source = in.Source.ID
		if _, ok := seen[listing.ExternalID]; ok {
			run.Duplicates++
			continue
		}
		seen[listing.ExternalID] = struct{}{}

		item := Item{Listing: listing}
		write := sqlite.ListingUpsert{Listing: listing}
		if prev, ok := byExternal[listing.ExternalID]; ok {
			listing.ID = prev.ID
			listing.FirstSeenAt = prev.FirstSeenAt
			listing.LastSeenAt = in.Now
			item.Listing = listing
			write.Listing = listing
			item.Changes = domain.Diff(prev, listing)
			if len(item.Changes) == 0 {
				run.Duplicates++
				item.Outcome = OutcomeUnchanged
			} else {
				run.Changed++
				item.Outcome = OutcomeChanged
				observations = append(observations, observation(in.RunID, listing, in.Now))
			}
		} else {
			listing.ID = id.New()
			listing.FirstSeenAt = in.Now
			listing.LastSeenAt = in.Now
			write.Insert = true
			item.Outcome = OutcomeNew
			item.Listing = listing
			write.Listing = listing
			run.New++
			observations = append(observations, observation(in.RunID, listing, in.Now))
			if other, ok := byURL[listing.URL]; ok && other.ID != listing.ID {
				link := orderedLink(listing.ID, other.ID, in.Now)
				links = append(links, link)
				run.DuplicateLinks++
				item.DuplicateOf = other.ID
			}
		}
		byExternal[listing.ExternalID] = listing
		if listing.URL != "" {
			byURL[listing.URL] = listing
		}

		item.Decision = domain.Apply(in.Watch.Filters, listing)
		if item.Decision.Status == domain.MatchMatched {
			run.Matched++
		} else {
			run.Rejected++
		}
		matches = append(matches, matchFrom(in.Watch.ID, listing.ID, item.Decision, previousMatch[listing.ID], in.Now))
		items = append(items, item)
		listings = append(listings, write)
	}

	return Classified{
		Run:   run,
		Items: items,
		Write: sqlite.PollWrite{
			Run:          run,
			Listings:     listings,
			Observations: observations,
			Matches:      matches,
			Links:        links,
		},
	}, nil
}

func observation(runID string, listing domain.Listing, seenAt time.Time) domain.Observation {
	return domain.Observation{
		ID:          id.New(),
		ListingID:   listing.ID,
		RunID:       runID,
		SeenAt:      seenAt,
		PriceAmount: listing.PriceAmount,
		Currency:    listing.Currency,
		Title:       listing.Title,
		Description: listing.Description,
		Images:      listing.Images,
		Present:     true,
		ContentHash: domain.Fingerprint(listing),
	}
}

func matchFrom(watchID, listingID string, decision domain.Decision, prev domain.Match, now time.Time) domain.Match {
	match := domain.Match{
		WatchID:        watchID,
		ListingID:      listingID,
		Status:         decision.Status,
		Reasons:        decision.Reasons,
		FirstMatchedAt: prev.FirstMatchedAt,
		LastMatchedAt:  prev.LastMatchedAt,
		UpdatedAt:      now,
	}
	if decision.Status != domain.MatchMatched {
		return match
	}
	if match.FirstMatchedAt == nil {
		seen := now
		match.FirstMatchedAt = &seen
	}
	seen := now
	match.LastMatchedAt = &seen
	return match
}

func orderedLink(left, right string, now time.Time) domain.DuplicateLink {
	if right < left {
		left, right = right, left
	}
	return domain.DuplicateLink{
		ListingA:  left,
		ListingB:  right,
		Reason:    domain.DuplicateCanonicalURL,
		CreatedAt: now,
	}
}
