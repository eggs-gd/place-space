package application

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/eggs-gd/place-space/internal/domain"
	"github.com/eggs-gd/place-space/internal/sources"
	"github.com/eggs-gd/place-space/internal/sources/lun"
	"github.com/eggs-gd/place-space/internal/storage/sqlite"
)

func TestPipelineFetchNormalizeObservationDiff(t *testing.T) {
	var (
		mu    sync.Mutex
		price = 18000
	)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		defer mu.Unlock()
		if r.URL.Query().Get("page") != "" {
			http.NotFound(w, r)
			return
		}
		_, _ = w.Write([]byte(embedRealties(price)))
	}))
	defer server.Close()

	ctx := context.Background()
	store := openStore(t)
	source, err := store.EnsureSource(ctx, domain.Source{
		ID: "lun", Type: "lun", Enabled: true, Status: domain.SourceUnknown,
		Configuration: json.RawMessage(`{}`),
	})
	if err != nil {
		t.Fatal(err)
	}
	max := int64(20000)
	rooms := 2
	watch, err := store.CreateWatch(ctx, domain.Watch{
		Name: "Кам'янець", Enabled: true, SourceIDs: []string{source.ID},
		Query:        domain.Query{City: "Кам'янець-Подільський", Deal: domain.DealRent, Properties: []string{domain.PropertyApartment}},
		Filters:      domain.Filters{PriceMax: &max, RoomsMin: &rooms},
		PollInterval: 10 * time.Minute,
	})
	if err != nil {
		t.Fatal(err)
	}
	pipeline := &Pipeline{
		Store: store,
		Adapters: map[string]sources.Adapter{
			"lun": lun.New(lun.Options{BaseURL: server.URL, HTTP: server.Client(), DisablePause: true, MaxPages: 2}),
		},
	}

	first, err := pipeline.PollWatch(ctx, watch)
	if err != nil {
		t.Fatal(err)
	}
	if len(first.Runs) != 1 || first.Runs[0].New != 2 || first.Runs[0].Matched != 1 || first.Runs[0].Rejected != 1 {
		t.Fatalf("first %+v", first.Runs)
	}
	observations, err := store.CountObservations(ctx, "")
	if err != nil {
		t.Fatal(err)
	}
	if observations != 2 {
		t.Fatalf("observations %d", observations)
	}

	second, err := pipeline.PollWatch(ctx, watch)
	if err != nil {
		t.Fatal(err)
	}
	if second.Runs[0].New != 0 || second.Runs[0].Changed != 0 || second.Runs[0].Duplicates != 2 {
		t.Fatalf("second %+v", second.Runs[0])
	}
	observations, err = store.CountObservations(ctx, "")
	if err != nil {
		t.Fatal(err)
	}
	if observations != 2 {
		t.Fatalf("observations grew on an unchanged poll: %d", observations)
	}

	mu.Lock()
	price = 17000
	mu.Unlock()
	third, err := pipeline.PollWatch(ctx, watch)
	if err != nil {
		t.Fatal(err)
	}
	if third.Runs[0].Changed != 1 || third.Runs[0].New != 0 || third.Runs[0].Duplicates != 1 {
		t.Fatalf("third %+v", third.Runs[0])
	}
	var priceChange domain.Change
	for _, item := range third.Items {
		if item.Outcome == OutcomeChanged {
			priceChange = item.Changes[0]
		}
	}
	if priceChange.Field != "price" || priceChange.From != "18000 UAH" || priceChange.To != "17000 UAH" {
		t.Fatalf("price change %#v", priceChange)
	}
	observations, err = store.CountObservations(ctx, "")
	if err != nil {
		t.Fatal(err)
	}
	if observations != 3 {
		t.Fatalf("observations %d", observations)
	}

	stored, err := store.ListingsByKeys(ctx, "lun", []string{"1001"}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(stored) != 1 || stored[0].PriceAmount == nil || *stored[0].PriceAmount != 17000 {
		t.Fatalf("stored %+v", stored)
	}
	if !stored[0].FirstSeenAt.Equal(stored[0].FirstSeenAt) || stored[0].FirstSeenAt.After(stored[0].LastSeenAt) {
		t.Fatalf("seen window %s %s", stored[0].FirstSeenAt, stored[0].LastSeenAt)
	}
}

func openStore(t *testing.T) *sqlite.Store {
	t.Helper()
	store, err := sqlite.Open(filepath.Join(t.TempDir(), "place-space.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	if err := store.Migrate(context.Background()); err != nil {
		t.Fatal(err)
	}
	return store
}

func embedRealties(matchedPrice int) string {
	body, err := json.Marshal(map[string]any{
		"realties": map[string]any{
			"totalRealtiesCount": 2,
			"cards": []map[string]any{
				card(1001, matchedPrice, 2),
				card(1002, 24500, 1),
			},
		},
	})
	if err != nil {
		panic(err)
	}
	return "<html><script>" + jsEscape(string(body)) + "</script></html>"
}

func card(id, price, rooms int) map[string]any {
	return map[string]any{
		"id":         id,
		"urlRaw":     "https://example.test/" + strconv.Itoa(id),
		"price":      price,
		"currency":   "uah",
		"roomCount":  rooms,
		"text":       "квартира",
		"insertTime": "2026-10-05T19:19:28",
		"geoEntities": []map[string]string{
			{"type": "city", "name": "Кам'янець-Подільський"},
			{"type": "street", "name": "вулиця Шевченка"},
			{"type": "house", "name": strconv.Itoa(id)},
		},
	}
}

func jsEscape(value string) string {
	var b strings.Builder
	for _, r := range value {
		switch r {
		case '\\', '"':
			b.WriteByte('\\')
			b.WriteRune(r)
		default:
			b.WriteRune(r)
		}
	}
	return b.String()
}
