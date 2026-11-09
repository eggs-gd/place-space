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
	f.saveFor(t, f.watch, external, at)
}

// addWatch creates another watch on the same shared source.
func (f retentionFixture) addWatch(t *testing.T, name string) domain.Watch {
	t.Helper()
	watch, err := f.store.CreateWatch(context.Background(), domain.Watch{
		Name: name, Enabled: true, SourceIDs: []string{f.source.ID},
		Query:        domain.Query{City: name, Deal: domain.DealRent},
		PollInterval: 10 * time.Minute,
	})
	if err != nil {
		t.Fatal(err)
	}
	return watch
}

// check records a check that received listings without finding anything new.
func (f retentionFixture) check(t *testing.T, watch domain.Watch, at time.Time, failed int, errs ...string) {
	t.Helper()
	f.source.UpdatedAt = at
	if err := f.store.SavePoll(context.Background(), PollWrite{
		Run: domain.Run{
			ID: id.New(), WatchID: watch.ID, SourceID: f.source.ID, SourceType: f.source.Type,
			StartedAt: at, FinishedAt: at, Received: 5, Failed: failed, Errors: errs,
		},
		Source: f.source,
	}); err != nil {
		t.Fatal(err)
	}
}

func (f retentionFixture) exists(t *testing.T, external string) bool {
	t.Helper()
	found, err := f.store.ListingsByKeys(context.Background(), f.source.ID, []string{external}, nil)
	if err != nil {
		t.Fatal(err)
	}
	return len(found) == 1
}

func (f retentionFixture) saveFor(t *testing.T, watch domain.Watch, external string, at time.Time) {
	t.Helper()
	listingID, runID := id.New(), id.New()
	f.source.UpdatedAt = at
	err := f.store.SavePoll(context.Background(), PollWrite{
		Run: domain.Run{
			ID: runID, WatchID: watch.ID, SourceID: f.source.ID, SourceType: f.source.Type,
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
			WatchID: watch.ID, ListingID: listingID, Status: domain.MatchMatched, UpdatedAt: at,
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
	f.save(t, "edge", now.Add(-30*24*time.Hour-time.Hour))
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
	// "edge" was last missed only 29 days ago: the check that first missed it
	// is the one that found "month".
	kept, err := f.store.ListingsByKeys(ctx, f.source.ID, []string{"edge", "month", "live"}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(kept) != 3 {
		t.Fatalf("kept %+v", kept)
	}
	observations, err := f.store.CountObservations(ctx, "")
	if err != nil {
		t.Fatal(err)
	}
	if observations != 3 {
		t.Fatalf("observations %d", observations)
	}
	runs, err := f.store.RecentRuns(ctx, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(runs) != 3 {
		t.Fatalf("runs %d", len(runs))
	}
}

func TestPruneIgnoresAppDowntime(t *testing.T) {
	f := newRetentionFixture(t)
	ctx := context.Background()
	f.save(t, "old", time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC))
	// The app comes back months later and the listing is no longer shown.
	f.save(t, "live", time.Date(2026, 11, 9, 12, 0, 0, 0, time.UTC))
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

func TestPruneIgnoresChecksOfOtherWatches(t *testing.T) {
	f := newRetentionFixture(t)
	ctx := context.Background()
	now := time.Date(2026, 11, 9, 12, 0, 0, 0, time.UTC)
	lviv := f.addWatch(t, "Львів")
	f.save(t, "kyiv", now.Add(-40*24*time.Hour))
	f.check(t, lviv, now.Add(-35*24*time.Hour), 0)
	f.check(t, lviv, now, 0)

	if err := f.store.Prune(ctx); err != nil {
		t.Fatal(err)
	}
	if !f.exists(t, "kyiv") {
		t.Fatal("another watch's checks erased the listing")
	}
}

func TestPruneWaitsForEveryWatchHoldingTheListing(t *testing.T) {
	f := newRetentionFixture(t)
	ctx := context.Background()
	now := time.Date(2026, 11, 9, 12, 0, 0, 0, time.UTC)
	lviv := f.addWatch(t, "Львів")
	f.save(t, "shared", now.Add(-40*24*time.Hour))
	f.check(t, f.watch, now.Add(-35*24*time.Hour), 0)
	f.check(t, f.watch, now, 0)
	// The second watch holds the listing too, but has been checked only once since.
	err := f.store.SavePoll(ctx, PollWrite{
		Run: domain.Run{
			ID: id.New(), WatchID: lviv.ID, SourceID: f.source.ID, SourceType: f.source.Type,
			StartedAt: now, FinishedAt: now, Received: 5,
		},
		Source:  f.source,
		Matches: []domain.Match{{WatchID: lviv.ID, ListingID: f.listingID(t, "shared"), Status: domain.MatchMatched, UpdatedAt: now}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := f.store.Prune(ctx); err != nil {
		t.Fatal(err)
	}
	if !f.exists(t, "shared") {
		t.Fatal("listing erased while a watch still holds it")
	}
}

func (f retentionFixture) listingID(t *testing.T, external string) string {
	t.Helper()
	found, err := f.store.ListingsByKeys(context.Background(), f.source.ID, []string{external}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(found) != 1 {
		t.Fatalf("listing %s: %+v", external, found)
	}
	return found[0].ID
}

func TestPruneJudgesUnheldListingsBySource(t *testing.T) {
	f := newRetentionFixture(t)
	ctx := context.Background()
	now := time.Date(2026, 11, 9, 12, 0, 0, 0, time.UTC)
	lviv := f.addWatch(t, "Львів")
	f.save(t, "kyiv", now.Add(-40*24*time.Hour))
	f.check(t, lviv, now.Add(-35*24*time.Hour), 0)
	f.check(t, lviv, now, 0)
	// Switching the watch to another city releases what it held.
	if err := f.store.ClearMatches(ctx, f.watch.ID); err != nil {
		t.Fatal(err)
	}
	if err := f.store.Prune(ctx); err != nil {
		t.Fatal(err)
	}
	if f.exists(t, "kyiv") {
		t.Fatal("listing nobody holds was kept")
	}
}

func TestPartialChecksDoNotAdvanceTheClock(t *testing.T) {
	f := newRetentionFixture(t)
	ctx := context.Background()
	now := time.Date(2026, 11, 9, 12, 0, 0, 0, time.UTC)
	f.save(t, "house", now.Add(-40*24*time.Hour))
	f.check(t, f.watch, now.Add(-35*24*time.Hour), 0, "lun: /rent/house: status 500")
	f.check(t, f.watch, now, 0, "lun: /rent/house: status 500")

	if got := f.feedIDs(t); len(got) != 1 {
		t.Fatalf("feed lost a listing to partial checks: %v", got)
	}
	if err := f.store.Prune(ctx); err != nil {
		t.Fatal(err)
	}
	if !f.exists(t, "house") {
		t.Fatal("partial checks erased the listing")
	}
}

func TestMalformedCardDoesNotStopTheClock(t *testing.T) {
	f := newRetentionFixture(t)
	now := time.Date(2026, 11, 9, 12, 0, 0, 0, time.UTC)
	f.save(t, "gone", now.Add(-4*24*time.Hour))
	f.check(t, f.watch, now, 1, "normalize 7: bad card")

	if got := f.feedIDs(t); len(got) != 0 {
		t.Fatalf("feed %v", got)
	}
}

func TestPruneKeepsNewestCompleteRunOfEachWatch(t *testing.T) {
	f := newRetentionFixture(t)
	ctx := context.Background()
	now := time.Date(2026, 11, 9, 12, 0, 0, 0, time.UTC)
	lviv := f.addWatch(t, "Львів")
	f.check(t, f.watch, now.Add(-60*24*time.Hour), 0)
	f.check(t, lviv, now, 0)
	if err := f.store.Prune(ctx); err != nil {
		t.Fatal(err)
	}
	runs, err := f.store.RecentRuns(ctx, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(runs) != 2 {
		t.Fatalf("runs %d", len(runs))
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
