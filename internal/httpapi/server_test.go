package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/eggs-gd/place-space/internal/application"
	"github.com/eggs-gd/place-space/internal/domain"
	"github.com/eggs-gd/place-space/internal/id"
	"github.com/eggs-gd/place-space/internal/storage/sqlite"
)

func TestOverviewFeedAndDetail(t *testing.T) {
	ctx := context.Background()
	store, err := sqlite.Open(filepath.Join(t.TempDir(), "place.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	if err := store.Migrate(ctx); err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 10, 6, 9, 0, 0, 0, time.UTC)
	source, err := store.EnsureSource(ctx, domain.Source{
		ID: "lun", Type: "lun", Enabled: true, Status: domain.SourceUnknown,
		Configuration: []byte(`{}`),
	})
	if err != nil {
		t.Fatal(err)
	}
	max := int64(20000)
	roomsMin := 2
	watch, err := store.CreateWatch(ctx, domain.Watch{
		Name: "Кам'янець", Enabled: true, SourceIDs: []string{source.ID},
		Query:        domain.Query{City: "Кам'янець-Подільський", Deal: domain.DealRent, Properties: []string{domain.PropertyApartment}},
		Filters:      domain.Filters{PriceMax: &max, RoomsMin: &roomsMin},
		PollInterval: 10 * time.Minute,
	})
	if err != nil {
		t.Fatal(err)
	}
	price := int64(18000)
	expensive := int64(24500)
	rooms := 2
	one := 1
	area := 62.0
	matchedID := id.New()
	rejectedID := id.New()
	runID := id.New()
	matched := domain.Listing{
		ID: matchedID, Source: source.ID, ExternalID: "1", URL: "https://example.test/1",
		Title: "Світла", Description: "Повний опис", PriceAmount: &price, Currency: "UAH",
		Location: domain.Location{Name: "Кам'янець-Подільський", Address: "вулиця Шевченка, 1"},
		Rooms:    &rooms, Area: &area, Images: []string{"https://example.test/a.jpg"},
		FirstSeenAt: now, LastSeenAt: now, RawData: []byte(`{}`),
	}
	rejected := matched
	rejected.ID = rejectedID
	rejected.ExternalID = "2"
	rejected.URL = "https://example.test/2"
	rejected.PriceAmount = &expensive
	rejected.Rooms = &one
	rejected.Description = "Дорого"
	source.Status = domain.SourceHealthy
	source.UpdatedAt = now
	source.LastSuccessAt = &now
	matchedDecision := domain.Apply(watch.Filters, matched, nil)
	rejectedDecision := domain.Apply(watch.Filters, rejected, nil)
	if err := store.SavePoll(ctx, sqlite.PollWrite{
		Run: domain.Run{
			ID: runID, WatchID: watch.ID, SourceID: source.ID, SourceType: source.Type,
			StartedAt: now.Add(-time.Minute), FinishedAt: now,
			Received: 2, New: 2, Matched: 1, Rejected: 1,
		},
		Source: source,
		Listings: []sqlite.ListingUpsert{
			{Insert: true, Listing: matched},
			{Insert: true, Listing: rejected},
		},
		Observations: []domain.Observation{{
			ID: id.New(), ListingID: matchedID, RunID: runID, SeenAt: now,
			PriceAmount: &price, Currency: "UAH", Title: matched.Title, Present: true, ContentHash: "a",
		}},
		Matches: []domain.Match{
			{WatchID: watch.ID, ListingID: matchedID, Status: matchedDecision.Status, Reasons: matchedDecision.Reasons, UpdatedAt: now},
			{WatchID: watch.ID, ListingID: rejectedID, Status: rejectedDecision.Status, Reasons: rejectedDecision.Reasons, UpdatedAt: now},
		},
	}); err != nil {
		t.Fatal(err)
	}

	api := httptest.NewServer((&Server{Store: store, Now: func() time.Time { return now }}).Handler())
	t.Cleanup(api.Close)

	var overview overviewBody
	get(t, api.URL+"/api/overview", &overview)
	if overview.Watch == nil || overview.Summary.Places != 1 || overview.Summary.New != 1 || overview.Summary.Rejected != 1 {
		t.Fatalf("overview %+v", overview)
	}
	if overview.Source == nil || overview.Source.Status != domain.SourceHealthy || overview.Run == nil || overview.Run.Received != 2 {
		t.Fatalf("source/run %+v %+v", overview.Source, overview.Run)
	}

	var feed feedBody
	get(t, api.URL+"/api/listings", &feed)
	if feed.Status != "matched" || len(feed.Items) != 1 || feed.Items[0].ID != matchedID || !feed.Items[0].IsNew {
		t.Fatalf("feed %+v", feed)
	}

	var rejectedFeed feedBody
	get(t, api.URL+"/api/listings?status=rejected", &rejectedFeed)
	if len(rejectedFeed.Items) != 1 || rejectedFeed.Items[0].Reasons[0].Message != "price: 24500 > 20000" {
		t.Fatalf("rejected %+v", rejectedFeed.Items)
	}

	var detail detailBody
	get(t, api.URL+"/api/listings/"+matchedID, &detail)
	if detail.Description != "Повний опис" || detail.Address != "вулиця Шевченка, 1" || len(detail.History) != 1 {
		t.Fatalf("detail %+v", detail)
	}
}

func TestCreateAndUpdateWatch(t *testing.T) {
	ctx := context.Background()
	store, err := sqlite.Open(filepath.Join(t.TempDir(), "place.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	if err := store.Migrate(ctx); err != nil {
		t.Fatal(err)
	}
	refreshed := make(chan string, 1)
	api := httptest.NewServer((&Server{Store: store, Refresh: func(id string) { refreshed <- id }}).Handler())
	t.Cleanup(api.Close)

	created := watchJSON{}
	post(t, api.URL+"/api/watches", map[string]any{
		"city": "Львів", "priceMax": 15000, "roomsMin": 2, "properties": []string{"apartment"}, "enabled": true,
	}, http.StatusCreated, &created)
	if created.City != "Львів" || created.PriceMax == nil || *created.PriceMax != 15000 || created.PollIntervalSeconds != 600 {
		t.Fatalf("created %+v", created)
	}
	if got := <-refreshed; got != created.ID {
		t.Fatalf("refresh %s", got)
	}

	updated := watchJSON{}
	sendJSON(t, http.MethodPatch, api.URL+"/api/watches/"+created.ID, map[string]any{
		"city": "Львів", "priceMax": nil, "roomsMin": 3, "areaMin": 40, "properties": []string{"apartment", "house"}, "enabled": false,
	}, http.StatusOK, &updated)
	if updated.PriceMax != nil || updated.RoomsMin == nil || *updated.RoomsMin != 3 || updated.AreaMin == nil || *updated.AreaMin != 40 || updated.Enabled {
		t.Fatalf("updated %+v", updated)
	}
	select {
	case id := <-refreshed:
		t.Fatalf("disabled watch refreshed %s", id)
	default:
	}

	stored, err := store.GetWatch(ctx, created.ID)
	if err != nil {
		t.Fatal(err)
	}
	if stored.Name != "Львів" {
		t.Fatalf("name %s", stored.Name)
	}

	res, err := http.Post(api.URL+"/api/watches", "application/json", strings.NewReader(`{"city":" ","properties":["apartment"]}`))
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusBadRequest {
		t.Fatalf("blank city %d", res.StatusCode)
	}
}

func TestCityChangeClearsPreviousFeed(t *testing.T) {
	ctx := context.Background()
	store, err := sqlite.Open(filepath.Join(t.TempDir(), "place.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	if err := store.Migrate(ctx); err != nil {
		t.Fatal(err)
	}
	source, err := store.EnsureSource(ctx, domain.Source{
		ID: "lun", Type: "lun", Enabled: true, Status: domain.SourceHealthy, Configuration: []byte(`{}`),
	})
	if err != nil {
		t.Fatal(err)
	}
	watch, err := store.CreateWatch(ctx, domain.Watch{
		Name: "Кам'янець-Подільський", Enabled: true, SourceIDs: []string{source.ID},
		Query:        domain.Query{City: "Кам'янець-Подільський", Deal: domain.DealRent, Properties: []string{domain.PropertyApartment}},
		PollInterval: 10 * time.Minute,
	})
	if err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 10, 8, 9, 0, 0, 0, time.UTC)
	listingID := id.New()
	if err := store.SavePoll(ctx, sqlite.PollWrite{
		Run: domain.Run{
			ID: id.New(), WatchID: watch.ID, SourceID: source.ID, SourceType: source.Type,
			StartedAt: now, FinishedAt: now,
		},
		Source: source,
		Listings: []sqlite.ListingUpsert{{Insert: true, Listing: domain.Listing{
			ID: listingID, Source: source.ID, ExternalID: "1", URL: "https://example.test/1",
			Location:    domain.Location{Name: "Кам'янець-Подільський"},
			FirstSeenAt: now, LastSeenAt: now, RawData: []byte(`{}`),
		}}},
		Matches: []domain.Match{{
			WatchID: watch.ID, ListingID: listingID, Status: domain.MatchMatched, UpdatedAt: now,
		}},
	}); err != nil {
		t.Fatal(err)
	}
	api := httptest.NewServer((&Server{Store: store}).Handler())
	t.Cleanup(api.Close)

	sendJSON(t, http.MethodPatch, api.URL+"/api/watches/"+watch.ID, map[string]any{
		"city": "Київ", "properties": []string{"apartment"}, "enabled": true,
	}, http.StatusOK, nil)

	var feed feedBody
	get(t, api.URL+"/api/listings", &feed)
	if len(feed.Items) != 0 {
		t.Fatalf("old city stayed in the feed: %+v", feed.Items)
	}
	stored, err := store.GetWatch(ctx, watch.ID)
	if err != nil {
		t.Fatal(err)
	}
	if stored.Query.City != "Київ" || stored.Name != "Київ" {
		t.Fatalf("watch %+v", stored)
	}
}

func post(t *testing.T, url string, body any, status int, dest any) {
	t.Helper()
	sendJSON(t, http.MethodPost, url, body, status, dest)
}

func sendJSON(t *testing.T, method, url string, body any, status int, dest any) {
	t.Helper()
	raw, err := json.Marshal(body)
	if err != nil {
		t.Fatal(err)
	}
	req, err := http.NewRequest(method, url, bytes.NewReader(raw))
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Content-Type", "application/json")
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	if res.StatusCode != status {
		payload, _ := io.ReadAll(res.Body)
		t.Fatalf("%s %s status %d %s", method, url, res.StatusCode, payload)
	}
	if dest != nil {
		if err := json.NewDecoder(res.Body).Decode(dest); err != nil {
			t.Fatal(err)
		}
	}
}

func get(t *testing.T, url string, dest any) {
	t.Helper()
	res, err := http.Get(url)
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		t.Fatalf("%s status %d", url, res.StatusCode)
	}
	if err := json.NewDecoder(res.Body).Decode(dest); err != nil {
		t.Fatal(err)
	}
}

func TestOriginPolicy(t *testing.T) {
	store, err := sqlite.Open(filepath.Join(t.TempDir(), "place.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	if err := store.Migrate(context.Background()); err != nil {
		t.Fatal(err)
	}
	handler := (&Server{Store: store}).Handler()
	cases := []struct {
		name, method, origin string
		status               int
		allow                string
	}{
		{"no origin", http.MethodGet, "", http.StatusOK, ""},
		{"same origin", http.MethodGet, "http://example.test:9000", http.StatusOK, "http://example.test:9000"},
		{"dev server", http.MethodGet, "http://localhost:5173", http.StatusOK, "http://localhost:5173"},
		{"dev preflight", http.MethodOptions, "http://127.0.0.1:5174", http.StatusNoContent, "http://127.0.0.1:5174"},
		{"foreign read", http.MethodGet, "https://evil.test", http.StatusForbidden, ""},
		{"foreign write", http.MethodPost, "https://evil.test", http.StatusForbidden, ""},
		{"lookalike host", http.MethodPost, "https://localhost.evil.test", http.StatusForbidden, ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			target := "http://example.test:9000/api/overview"
			if tc.method == http.MethodPost {
				target = "http://example.test:9000/api/watches"
			}
			req := httptest.NewRequest(tc.method, target, strings.NewReader(`{"city":"Київ","properties":["apartment"]}`))
			if tc.origin != "" {
				req.Header.Set("Origin", tc.origin)
			}
			rec := httptest.NewRecorder()
			handler.ServeHTTP(rec, req)
			if rec.Code != tc.status {
				t.Fatalf("status %d: %s", rec.Code, rec.Body)
			}
			if got := rec.Header().Get("Access-Control-Allow-Origin"); got != tc.allow {
				t.Fatalf("allow origin %q", got)
			}
		})
	}
	watches, err := store.ListWatches(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(watches) != 0 {
		t.Fatalf("foreign write landed: %+v", watches)
	}
}

type stubAreas struct{}

func (s *stubAreas) List(context.Context, string) ([]application.AreaOption, error) {
	return []application.AreaOption{{Key: "дарницький", Name: "Дарницький", Kind: domain.AreaDistrict, Sources: []string{"lun"}}}, nil
}

func (s *stubAreas) Resolve(_ context.Context, _ string, requested []string) ([]domain.AreaRef, error) {
	if len(requested) == 1 && requested[0] == "дарницький" {
		return []domain.AreaRef{{Key: "дарницький", Name: "Дарницький"}}, nil
	}
	return nil, &application.AreaError{Message: "район не знайдено"}
}

func areaServer(t *testing.T) (http.Handler, *sqlite.Store) {
	t.Helper()
	store, err := sqlite.Open(filepath.Join(t.TempDir(), "place.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	if err := store.Migrate(context.Background()); err != nil {
		t.Fatal(err)
	}
	return (&Server{Store: store, Areas: &stubAreas{}}).Handler(), store
}

func call(t *testing.T, handler http.Handler, method, target, body string) *httptest.ResponseRecorder {
	t.Helper()
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest(method, target, strings.NewReader(body)))
	return rec
}

func TestAreasEndpoint(t *testing.T) {
	handler, _ := areaServer(t)
	rec := call(t, handler, http.MethodGet, "/api/areas?city=%D0%9A%D0%B8%D1%97%D0%B2", "")
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"key":"дарницький"`) {
		t.Fatalf("%d %s", rec.Code, rec.Body)
	}
}

func TestWatchKeepsChosenAreas(t *testing.T) {
	handler, store := areaServer(t)
	rec := call(t, handler, http.MethodPost, "/api/watches", `{"city":"Київ","properties":["apartment"],"areas":["дарницький"]}`)
	if rec.Code != http.StatusCreated || !strings.Contains(rec.Body.String(), `"areas":[{"key":"дарницький","name":"Дарницький"}]`) {
		t.Fatalf("%d %s", rec.Code, rec.Body)
	}
	watches, err := store.ListWatches(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(watches) != 1 || len(watches[0].Query.Areas) != 1 {
		t.Fatalf("stored %+v", watches)
	}
}

func TestWatchRejectsUnknownArea(t *testing.T) {
	handler, store := areaServer(t)
	rec := call(t, handler, http.MethodPost, "/api/watches", `{"city":"Київ","properties":["apartment"],"areas":["марс"]}`)
	if rec.Code != http.StatusBadRequest || !strings.Contains(rec.Body.String(), "не знайдено") {
		t.Fatalf("%d %s", rec.Code, rec.Body)
	}
	watches, err := store.ListWatches(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(watches) != 0 {
		t.Fatalf("saved %+v", watches)
	}
}

func TestChangingAreasChangesTheSearch(t *testing.T) {
	base := domain.Query{City: "Київ", Deal: domain.DealRent, Areas: []domain.AreaRef{{Key: "а"}, {Key: "б"}}}
	same := domain.Query{City: "Київ", Deal: domain.DealRent, Areas: []domain.AreaRef{{Key: "б"}, {Key: "а"}}}
	other := domain.Query{City: "Київ", Deal: domain.DealRent, Areas: []domain.AreaRef{{Key: "а"}, {Key: "в"}}}
	none := domain.Query{City: "Київ", Deal: domain.DealRent}
	if queryChanged(base, same) {
		t.Fatal("reordering counted as a change")
	}
	if !queryChanged(base, other) || !queryChanged(base, none) {
		t.Fatal("different areas not noticed")
	}
}
