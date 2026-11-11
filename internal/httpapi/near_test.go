package httpapi

import (
	"context"
	"net/http"
	"path/filepath"
	"strings"
	"testing"

	"github.com/eggs-gd/place-space/internal/domain"
	"github.com/eggs-gd/place-space/internal/places"
	"github.com/eggs-gd/place-space/internal/storage/sqlite"
)

const secretKey = "AIzaSecretValue9876"

type stubPlaces struct {
	key     string
	lookups []string
}

func (s *stubPlaces) State(context.Context) (places.KeyState, error) {
	if s.key == "" {
		return places.KeyState{}, nil
	}
	return places.KeyState{Set: true, Hint: s.key[len(s.key)-4:]}, nil
}

func (s *stubPlaces) SetKey(_ context.Context, key string) error {
	if key == "bad" {
		return places.ErrKeyRejected
	}
	s.key = key
	return nil
}

func (s *stubPlaces) Resolve(_ context.Context, text string) (string, error) {
	if s.key == "" {
		return "", places.ErrNoKey
	}
	s.lookups = append(s.lookups, text)
	if strings.HasPrefix(text, "нікуди") {
		return "", places.ErrNotFound
	}
	return "id:" + text, nil
}

func nearServer(t *testing.T, key string) (http.Handler, *sqlite.Store, *stubPlaces) {
	t.Helper()
	store, err := sqlite.Open(filepath.Join(t.TempDir(), "place.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	if err := store.Migrate(context.Background()); err != nil {
		t.Fatal(err)
	}
	stub := &stubPlaces{key: key}
	return (&Server{Store: store, Places: stub}).Handler(), store, stub
}

func TestSettingsNeverReturnTheKey(t *testing.T) {
	handler, _, stub := nearServer(t, "")
	rec := call(t, handler, http.MethodPut, "/api/settings", `{"googleKey":"`+secretKey+`"}`)
	if rec.Code != http.StatusOK || stub.key != secretKey {
		t.Fatalf("%d %s", rec.Code, rec.Body)
	}
	for _, body := range []string{rec.Body.String(), call(t, handler, http.MethodGet, "/api/settings", "").Body.String()} {
		if strings.Contains(body, secretKey) || !strings.Contains(body, `"hint":"9876"`) {
			t.Fatalf("settings body %s", body)
		}
	}
	rec = call(t, handler, http.MethodPut, "/api/settings", `{"googleKey":"bad"}`)
	if rec.Code != http.StatusBadRequest || stub.key != secretKey {
		t.Fatalf("%d %s, key %q", rec.Code, rec.Body, stub.key)
	}
}

func TestWatchNearAPlace(t *testing.T) {
	handler, store, stub := nearServer(t, secretKey)
	rec := call(t, handler, http.MethodPost, "/api/watches",
		`{"city":"Київ","properties":["apartment"],"near":{"name":"Дніпровська  набережна","radiusMeters":1000}}`)
	if rec.Code != http.StatusCreated || !strings.Contains(rec.Body.String(), `"near":{"name":"Дніпровська набережна","radiusMeters":1000}`) {
		t.Fatalf("%d %s", rec.Code, rec.Body)
	}
	if strings.Contains(rec.Body.String(), "placeId") {
		t.Fatalf("place id leaked into the view: %s", rec.Body)
	}
	watches, err := store.ListWatches(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	near := watches[0].Query.Near
	if near == nil || near.PlaceID != "id:Дніпровська набережна, Київ" || near.RadiusMeters != 1000 {
		t.Fatalf("stored %+v", near)
	}
	id := watches[0].ID

	rec = call(t, handler, http.MethodPatch, "/api/watches/"+id,
		`{"city":"Київ","properties":["apartment"],"near":{"name":"дніпровська набережна","radiusMeters":2000}}`)
	if rec.Code != http.StatusOK || len(stub.lookups) != 1 {
		t.Fatalf("radius change looked the place up again: %d %v", rec.Code, stub.lookups)
	}
	rec = call(t, handler, http.MethodPatch, "/api/watches/"+id,
		`{"city":"Львів","properties":["apartment"],"near":{"name":"дніпровська набережна","radiusMeters":2000}}`)
	if rec.Code != http.StatusOK || len(stub.lookups) != 2 || stub.lookups[1] != "дніпровська набережна, Львів" {
		t.Fatalf("city change kept the old place: %d %v", rec.Code, stub.lookups)
	}
	rec = call(t, handler, http.MethodPatch, "/api/watches/"+id, `{"city":"Львів","properties":["apartment"],"near":null}`)
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"near":null`) {
		t.Fatalf("%d %s", rec.Code, rec.Body)
	}
}

func TestWatchNearExplainsProblems(t *testing.T) {
	cases := []struct {
		name, key, body, want string
	}{
		{"no key", "", `{"name":"Поділ"}`, "ключ Google"},
		{"not found", secretKey, `{"name":"нікуди"}`, "не знайшов"},
		{"radius", secretKey, `{"name":"Поділ","radiusMeters":50}`, "радіус"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			handler, store, _ := nearServer(t, tc.key)
			rec := call(t, handler, http.MethodPost, "/api/watches", `{"city":"Київ","properties":["apartment"],"near":`+tc.body+`}`)
			if rec.Code != http.StatusBadRequest || !strings.Contains(rec.Body.String(), tc.want) {
				t.Fatalf("%d %s", rec.Code, rec.Body)
			}
			if watches, _ := store.ListWatches(context.Background()); len(watches) != 0 {
				t.Fatal("watch saved")
			}
		})
	}
}

func TestChangingNearChangesTheSearch(t *testing.T) {
	base := domain.Query{City: "Київ", Near: &domain.Near{Name: "a", PlaceID: "1", RadiusMeters: 1000}}
	same := domain.Query{City: "Київ", Near: &domain.Near{Name: "A", PlaceID: "1", RadiusMeters: 1000}}
	wider := domain.Query{City: "Київ", Near: &domain.Near{Name: "a", PlaceID: "1", RadiusMeters: 2000}}
	none := domain.Query{City: "Київ"}
	if queryChanged(base, same) {
		t.Fatal("renaming the same place counted as a change")
	}
	if !queryChanged(base, wider) || !queryChanged(base, none) || !queryChanged(none, base) {
		t.Fatal("place change not noticed")
	}
}
