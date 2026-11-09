package sqlite

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/eggs-gd/place-space/internal/domain"
	"github.com/eggs-gd/place-space/internal/id"
)

type retentionFixture struct {
	store  *Store
	source domain.Source
	watch  domain.Watch
}

func newRetentionFixture(t *testing.T) retentionFixture {
	t.Helper()
	ctx := context.Background()
	store := openTestStore(t)
	source, err := store.EnsureSource(ctx, domain.Source{
		ID: "lun", Type: "lun", Enabled: true, Status: domain.SourceUnknown,
		Configuration: json.RawMessage(`{}`),
	})
	if err != nil {
		t.Fatal(err)
	}
	watch, err := store.CreateWatch(ctx, domain.Watch{
		Name: "Київ", Enabled: true, SourceIDs: []string{source.ID},
		Query:        domain.Query{City: "Київ", Deal: domain.DealRent},
		PollInterval: 10 * time.Minute,
	})
	if err != nil {
		t.Fatal(err)
	}
	return retentionFixture{store: store, source: source, watch: watch}
}

// save records one check at the given time that found one new matched listing.
func (f retentionFixture) save(t *testing.T, external string, at time.Time) {
	t.Helper()
	listingID, runID := id.New(), id.New()
	f.source.UpdatedAt = at
	err := f.store.SavePoll(context.Background(), PollWrite{
		Run: domain.Run{
			ID: runID, WatchID: f.watch.ID, SourceID: f.source.ID, SourceType: f.source.Type,
			StartedAt: at, FinishedAt: at, Received: 1, New: 1, Matched: 1,
		},
		Source: f.source,
		Listings: []ListingUpsert{{Insert: true, Listing: domain.Listing{
			ID: listingID, Source: f.source.ID, ExternalID: external,
			URL: "https://example.test/" + external, FirstSeenAt: at, LastSeenAt: at,
		}}},
		Observations: []domain.Observation{{
			ID: id.New(), ListingID: listingID, RunID: runID, SeenAt: at, Present: true, ContentHash: external,
		}},
		Matches: []domain.Match{{
			WatchID: f.watch.ID, ListingID: listingID, Status: domain.MatchMatched, UpdatedAt: at,
		}},
	})
	if err != nil {
		t.Fatal(err)
	}
}

func (f retentionFixture) feedIDs(t *testing.T) []string {
	t.Helper()
	rows, err := f.store.Feed(context.Background(), f.watch.ID, domain.MatchMatched)
	if err != nil {
		t.Fatal(err)
	}
	out := make([]string, 0, len(rows))
	for _, row := range rows {
		out = append(out, row.Listing.ExternalID)
	}
	return out
}

func TestFeedHidesListingsTheSourceStoppedShowing(t *testing.T) {
	f := newRetentionFixture(t)
	now := time.Date(2026, 11, 9, 12, 0, 0, 0, time.UTC)
	f.save(t, "gone", now.Add(-4*24*time.Hour))
	f.save(t, "recent", now.Add(-2*24*time.Hour))
	f.save(t, "live", now)

	got := f.feedIDs(t)
	if len(got) != 2 || got[0] != "live" || got[1] != "recent" {
		t.Fatalf("feed %v", got)
	}
	matched, rejected, err := f.store.MatchCounts(context.Background(), f.watch.ID)
	if err != nil {
		t.Fatal(err)
	}
	if matched != 2 || rejected != 0 {
		t.Fatalf("counts %d %d", matched, rejected)
	}
}

func TestFeedKeepsListingsWhileAppWasClosed(t *testing.T) {
	f := newRetentionFixture(t)
	// The latest check itself is months old; nothing is measured against the clock.
	f.save(t, "old", time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC))
	if got := f.feedIDs(t); len(got) != 1 {
		t.Fatalf("feed %v", got)
	}
}

func TestPruneRemovesOldListingsAndKeepsRecent(t *testing.T) {
	f := newRetentionFixture(t)
	ctx := context.Background()
	now := time.Date(2026, 11, 9, 12, 0, 0, 0, time.UTC)
	f.save(t, "ancient", now.Add(-31*24*time.Hour))
	f.save(t, "month", now.Add(-29*24*time.Hour))
	f.save(t, "live", now)

	if err := f.store.Prune(ctx); err != nil {
		t.Fatal(err)
	}
	gone, err := f.store.ListingsByKeys(ctx, f.source.ID, []string{"ancient"}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(gone) != 0 {
		t.Fatalf("ancient listing kept: %+v", gone)
	}
	kept, err := f.store.ListingsByKeys(ctx, f.source.ID, []string{"month", "live"}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(kept) != 2 {
		t.Fatalf("kept %+v", kept)
	}
	observations, err := f.store.CountObservations(ctx, "")
	if err != nil {
		t.Fatal(err)
	}
	if observations != 2 {
		t.Fatalf("observations %d", observations)
	}
	runs, err := f.store.RecentRuns(ctx, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(runs) != 2 {
		t.Fatalf("runs %d", len(runs))
	}
}

func TestPruneIgnoresAppDowntime(t *testing.T) {
	f := newRetentionFixture(t)
	ctx := context.Background()
	f.save(t, "old", time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC))
	if err := f.store.Prune(ctx); err != nil {
		t.Fatal(err)
	}
	kept, err := f.store.ListingsByKeys(ctx, f.source.ID, []string{"old"}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(kept) != 1 {
		t.Fatalf("kept %+v", kept)
	}
}

func TestSavePollRefusesOutdatedWatch(t *testing.T) {
	f := newRetentionFixture(t)
	ctx := context.Background()
	outdated := f.watch.UpdatedAt
	time.Sleep(time.Millisecond)
	if err := f.store.UpdateWatch(ctx, f.watch); err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 11, 9, 12, 0, 0, 0, time.UTC)
	err := f.store.SavePoll(ctx, PollWrite{
		Run: domain.Run{
			ID: id.New(), WatchID: f.watch.ID, SourceID: f.source.ID, SourceType: f.source.Type,
			StartedAt: now, FinishedAt: now,
		},
		Source:         f.source,
		WatchUpdatedAt: outdated,
	})
	if !errors.Is(err, ErrWatchChanged) {
		t.Fatalf("err %v", err)
	}
	runs, err := f.store.RecentRuns(ctx, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(runs) != 0 {
		t.Fatalf("runs written: %d", len(runs))
	}
}

func TestExchangeRatesRoundTrip(t *testing.T) {
	f := newRetentionFixture(t)
	ctx := context.Background()
	at := time.Date(2026, 11, 9, 12, 0, 0, 0, time.UTC)
	if err := f.store.SaveExchangeRates(ctx, domain.Rates{"USD": 45.5}, at); err != nil {
		t.Fatal(err)
	}
	if err := f.store.SaveExchangeRates(ctx, domain.Rates{"USD": 46}, at.Add(time.Hour)); err != nil {
		t.Fatal(err)
	}
	rates, fetchedAt, err := f.store.ExchangeRates(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if rates["USD"] != 46 || !fetchedAt.Equal(at.Add(time.Hour)) {
		t.Fatalf("rates %v at %s", rates, fetchedAt)
	}
}
