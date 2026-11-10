package core

import (
	"context"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/eggs-gd/place-space/internal/domain"
	"github.com/eggs-gd/place-space/internal/sources"
)

type stubAdapter struct {
	kind  string
	count int
}

func (a stubAdapter) Type() string { return a.kind }

func (a stubAdapter) Fetch(context.Context, domain.Query) ([]sources.RawListing, error) {
	out := make([]sources.RawListing, 0, a.count)
	for i := 0; i < a.count; i++ {
		id := a.kind + "-" + string(rune('a'+i))
		out = append(out, sources.RawListing{ExternalID: id, Payload: []byte(`{}`)})
	}
	return out, nil
}

func (a stubAdapter) Normalize(_ context.Context, raw sources.RawListing) (domain.Listing, error) {
	price := int64(15000)
	rooms := 2
	return domain.Listing{
		Source:      a.kind,
		ExternalID:  raw.ExternalID,
		URL:         "https://example.test/" + raw.ExternalID,
		Title:       "квартира",
		PriceAmount: &price,
		Currency:    "UAH",
		Rooms:       &rooms,
	}, nil
}

type noRates struct{}

func (noRates) Current(context.Context) domain.Rates { return domain.Rates{} }

type clock struct {
	mu  sync.Mutex
	now time.Time
}

func (c *clock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.now
}

func (c *clock) Advance(d time.Duration) {
	c.mu.Lock()
	c.now = c.now.Add(d)
	c.mu.Unlock()
}

func openTestCore(t *testing.T, now *clock) *Core {
	t.Helper()
	c, err := Open(context.Background(), Options{
		DBPath: filepath.Join(t.TempDir(), DBName),
		Adapters: map[string]sources.Adapter{
			"lun":    stubAdapter{kind: "lun", count: 2},
			"domria": stubAdapter{kind: "domria", count: 1},
		},
		Rates: noRates{},
		Now:   now.Now,
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = c.Close() })
	return c
}

func createWatch(t *testing.T, c *Core, name string, enabled bool) domain.Watch {
	t.Helper()
	watch, err := c.Store.CreateWatch(context.Background(), domain.Watch{
		Name:         name,
		Enabled:      enabled,
		SourceIDs:    c.SourceIDs,
		Query:        domain.Query{City: "Київ", Deal: domain.DealRent, Properties: []string{domain.PropertyApartment}},
		PollInterval: 10 * time.Minute,
	})
	if err != nil {
		t.Fatal(err)
	}
	return watch
}

func TestCheckDueRespectsInterval(t *testing.T) {
	ctx := context.Background()
	now := &clock{now: time.Date(2026, 11, 10, 9, 0, 0, 0, time.UTC)}
	c := openTestCore(t, now)
	createWatch(t, c, "Київ", true)
	createWatch(t, c, "Вимкнений", false)

	first, err := c.CheckDue(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if first != (Summary{Checked: 1, Fresh: 3}) {
		t.Fatalf("first round %+v", first)
	}

	now.Advance(9 * time.Minute)
	early, err := c.CheckDue(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if early != (Summary{}) {
		t.Fatalf("round before the interval %+v", early)
	}

	now.Advance(2 * time.Minute)
	again, err := c.CheckDue(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if again != (Summary{Checked: 1}) {
		t.Fatalf("round after the interval %+v", again)
	}
}

func TestCheckDueSkipsWatchBeingChecked(t *testing.T) {
	now := &clock{now: time.Date(2026, 11, 10, 9, 0, 0, 0, time.UTC)}
	c := openTestCore(t, now)
	watch := createWatch(t, c, "Київ", true)
	c.polling[watch.ID] = struct{}{}

	summary, err := c.CheckDue(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if summary != (Summary{}) {
		t.Fatalf("busy watch was checked: %+v", summary)
	}
}
