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
	matchedDecision := domain.Apply(watch.Filters, matched)
	rejectedDecision := domain.Apply(watch.Filters, rejected)
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

	res, err := http.Post(api.URL+"/api/watches", "application/json", strings.NewReader(`{"city":" ","properties":["apartment"]}`))
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusBadRequest {
		t.Fatalf("blank city %d", res.StatusCode)
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
