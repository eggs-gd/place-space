package rates

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"

	"github.com/eggs-gd/place-space/internal/domain"
	"github.com/eggs-gd/place-space/internal/storage/sqlite"
)

func TestNBUFetchParsesRates(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`[{"cc":"USD","rate":45.32},{"cc":"EUR","rate":50.64},{"cc":"XXX","rate":0}]`))
	}))
	defer server.Close()
	got, err := (&NBU{URL: server.URL, HTTP: server.Client()}).Fetch(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 || got["USD"] != 45.32 || got["EUR"] != 50.64 {
		t.Fatalf("rates %v", got)
	}
}

func TestNBUFetchRejectsBadResponses(t *testing.T) {
	for name, handler := range map[string]http.HandlerFunc{
		"status": func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusBadGateway) },
		"body":   func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write([]byte(`<html>`)) },
		"empty":  func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write([]byte(`[]`)) },
	} {
		t.Run(name, func(t *testing.T) {
			server := httptest.NewServer(handler)
			defer server.Close()
			if _, err := (&NBU{URL: server.URL, HTTP: server.Client()}).Fetch(context.Background()); err == nil {
				t.Fatal("no error")
			}
		})
	}
}

type stubFetcher struct {
	rates domain.Rates
	err   error
	calls int
}

func (s *stubFetcher) Fetch(context.Context) (domain.Rates, error) {
	s.calls++
	return s.rates, s.err
}

func openStore(t *testing.T) *sqlite.Store {
	t.Helper()
	store, err := sqlite.Open(filepath.Join(t.TempDir(), "place.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	if err := store.Migrate(context.Background()); err != nil {
		t.Fatal(err)
	}
	return store
}

func TestCacheReusesRecentRatesAndRefreshesOldOnes(t *testing.T) {
	ctx := context.Background()
	now := time.Date(2026, 11, 9, 12, 0, 0, 0, time.UTC)
	fetcher := &stubFetcher{rates: domain.Rates{"USD": 45}}
	cache := &Cache{Store: openStore(t), Fetcher: fetcher, Now: func() time.Time { return now }}

	if got := cache.Current(ctx); got["USD"] != 45 || fetcher.calls != 1 {
		t.Fatalf("first %v calls %d", got, fetcher.calls)
	}
	fetcher.rates = domain.Rates{"USD": 46}
	if got := cache.Current(ctx); got["USD"] != 45 || fetcher.calls != 1 {
		t.Fatalf("cached %v calls %d", got, fetcher.calls)
	}
	now = now.Add(13 * time.Hour)
	if got := cache.Current(ctx); got["USD"] != 46 || fetcher.calls != 2 {
		t.Fatalf("refreshed %v calls %d", got, fetcher.calls)
	}
}

func TestCacheFallsBackToSavedRatesWhenFetchFails(t *testing.T) {
	ctx := context.Background()
	now := time.Date(2026, 11, 9, 12, 0, 0, 0, time.UTC)
	fetcher := &stubFetcher{rates: domain.Rates{"USD": 45}}
	cache := &Cache{Store: openStore(t), Fetcher: fetcher, Now: func() time.Time { return now }}
	cache.Current(ctx)

	now = now.Add(48 * time.Hour)
	fetcher.rates, fetcher.err = nil, errors.New("down")
	if got := cache.Current(ctx); got["USD"] != 45 {
		t.Fatalf("fallback %v", got)
	}
}
