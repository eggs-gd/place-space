package otodim

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/eggs-gd/place-space/internal/domain"
	"github.com/eggs-gd/place-space/internal/sources"
)

const kyivPath = "/uk/rezultaty/orenda/kvartyry/kyiv/kyiv/kyiv/kyiv"

func cardJSON(id int) string {
	return fmt.Sprintf(`{"id":%d,"title":"Квартира %d","slug":"kvartira-ID%d","hidePrice":false,
		"totalPrice":{"value":15000,"currency":"UAH"},"areaInSquareMeters":45,"roomsNumber":"TWO",
		"floorNumber":"THIRD","dateCreated":"2026-11-10T08:00:00.000000Z","createdAtFirst":"2026-11-01T10:00:00.000000+02:00",
		"images":[{"small":"s","medium":"m","large":"https://img.test/%d"}],
		"location":{"reverseGeocoding":{"locations":[{"locationLevel":"region","name":"Область"},{"locationLevel":"city_village","name":"Київ"}]}}}`, id, id, id, id)
}

func page(totalPages int, ids ...int) string {
	cards := make([]string, 0, len(ids))
	for _, id := range ids {
		cards = append(cards, cardJSON(id))
	}
	return fmt.Sprintf(`<html><script id="__NEXT_DATA__" type="application/json" crossorigin="anonymous">
		{"props":{"pageProps":{"data":{"searchAds":{"items":[%s],"pagination":{"totalPages":%d}}}}}}</script></html>`,
		strings.Join(cards, ","), totalPages)
}

func newAdapter(server *httptest.Server, maxPages int) *Adapter {
	return New(Options{BaseURL: server.URL, HTTP: server.Client(), DisablePause: true, MaxPages: maxPages})
}

func TestFetchReadsNewestFirstAcrossCatalogs(t *testing.T) {
	var requests []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests = append(requests, r.URL.RequestURI())
		switch {
		case strings.Contains(r.URL.Path, "/budynky/"):
			_, _ = w.Write([]byte(page(1, 2, 3)))
		case r.URL.Query().Get("page") == "2":
			_, _ = w.Write([]byte(page(2, 2)))
		default:
			_, _ = w.Write([]byte(page(2, 1)))
		}
	}))
	defer server.Close()

	raws, err := newAdapter(server, 5).Fetch(context.Background(), domain.Query{
		City: "Київ", Deal: domain.DealRent, Properties: []string{domain.PropertyApartment, domain.PropertyHouse},
	})
	if err != nil {
		t.Fatal(err)
	}
	var ids []string
	for _, raw := range raws {
		ids = append(ids, raw.ExternalID)
	}
	if strings.Join(ids, ",") != "1,2,3" {
		t.Fatalf("ids %v", ids)
	}
	want := []string{
		kyivPath + "?by=LATEST&direction=DESC",
		kyivPath + "?by=LATEST&direction=DESC&page=2",
		"/uk/rezultaty/orenda/budynky/kyiv/kyiv/kyiv/kyiv?by=LATEST&direction=DESC",
	}
	if strings.Join(requests, " ") != strings.Join(want, " ") {
		t.Fatalf("requests %v", requests)
	}
}

func TestFetchStopsAtMaxPages(t *testing.T) {
	calls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		_, _ = w.Write([]byte(page(100, calls)))
	}))
	defer server.Close()
	if _, err := newAdapter(server, 2).Fetch(context.Background(), domain.Query{City: "Київ"}); err != nil {
		t.Fatal(err)
	}
	if calls != 2 {
		t.Fatalf("calls %d", calls)
	}
}

func TestDefaultMaxPagesStaysLow(t *testing.T) {
	if got := New(Options{}).maxPages; got != 2 {
		t.Fatalf("maxPages %d", got)
	}
}

func TestFetchRejectsRedirectToAnotherCatalog(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/vsia-ukraina") {
			_, _ = w.Write([]byte(page(1, 1)))
			return
		}
		http.Redirect(w, r, "/uk/rezultaty/orenda/kvartyry/vsia-ukraina", http.StatusPermanentRedirect)
	}))
	defer server.Close()
	raws, err := newAdapter(server, 1).Fetch(context.Background(), domain.Query{City: "Київ"})
	if err == nil || !strings.Contains(err.Error(), "redirected") {
		t.Fatalf("err %v", err)
	}
	if len(raws) != 0 {
		t.Fatalf("listings from the wrong catalog: %d", len(raws))
	}
}

func TestFetchMarksBlockedResponses(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
	}))
	defer server.Close()
	_, err := newAdapter(server, 1).Fetch(context.Background(), domain.Query{City: "Київ"})
	if !errors.Is(err, sources.ErrBlocked) {
		t.Fatalf("err %v", err)
	}
}

func TestFetchEmptyCatalogIsNotAnError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(page(0)))
	}))
	defer server.Close()
	raws, err := newAdapter(server, 2).Fetch(context.Background(), domain.Query{City: "Київ"})
	if err != nil || len(raws) != 0 {
		t.Fatalf("raws %d err %v", len(raws), err)
	}
}

func TestExtractCatalogNeedsPayload(t *testing.T) {
	if _, _, err := extractCatalog([]byte("<html></html>")); err == nil {
		t.Fatal("page without data was accepted")
	}
	body := `<script id="__NEXT_DATA__">{"props":{"pageProps":{"data":{}}}}</script>`
	if _, _, err := extractCatalog([]byte(body)); err == nil {
		t.Fatal("page without listings was accepted")
	}
}

func TestSearchPaths(t *testing.T) {
	cases := []struct {
		name  string
		query domain.Query
		want  string
		fails bool
	}{
		{name: "apostrophe", query: domain.Query{City: "Кам’янець-Подільський"}, want: "/uk/rezultaty/orenda/kvartyry/khmelnytska-oblast/kamianets--podilskyi-raion/kamianets--podilska-hromada/kamianets--podilskyi"},
		{name: "path", query: domain.Query{City: kyivPath}, want: kyivPath},
		{name: "unknown city", query: domain.Query{City: "Атлантида"}, fails: true},
		{name: "path of another source", query: domain.Query{City: "/rent/kyiv/flats"}, fails: true},
		{name: "path outside the catalog", query: domain.Query{City: "/api/query"}, fails: true},
		{name: "sale", query: domain.Query{City: "Київ", Deal: "sale"}, fails: true},
		{name: "areas", query: domain.Query{City: "Київ", Areas: []domain.AreaRef{{Key: "x", Name: "x"}}}, fails: true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			paths, err := searchPaths(tc.query)
			if tc.fails {
				if err == nil {
					t.Fatalf("paths %v", paths)
				}
				return
			}
			if err != nil || len(paths) != 1 || paths[0].path != tc.want {
				t.Fatalf("paths %v err %v", paths, err)
			}
		})
	}
}

func TestFetchNearAPlace(t *testing.T) {
	var requests []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests = append(requests, r.URL.Path+" "+r.URL.Query().Get("placeId")+" "+r.URL.Query().Get("distanceRadius")+" "+r.URL.Query().Get("by"))
		_, _ = w.Write([]byte(page(1, 1)))
	}))
	defer server.Close()
	_, err := newAdapter(server, 1).Fetch(context.Background(), domain.Query{
		City:       "Київ",
		Properties: []string{domain.PropertyApartment, domain.PropertyHouse},
		Near:       &domain.Near{Name: "Поділ", PlaceID: "ChIJ26L7ANfa1EARS0Dp8jarxS4", RadiusMeters: 1500},
	})
	if err != nil {
		t.Fatal(err)
	}
	want := []string{
		"/uk/rezultaty/orenda/kvartyry/vsia-ukraina ChIJ26L7ANfa1EARS0Dp8jarxS4 1500 LATEST",
		"/uk/rezultaty/orenda/budynky/vsia-ukraina ChIJ26L7ANfa1EARS0Dp8jarxS4 1500 LATEST",
	}
	if strings.Join(requests, "|") != strings.Join(want, "|") {
		t.Fatalf("requests %v", requests)
	}
}

func TestSearchNearRejectsBadInput(t *testing.T) {
	for _, near := range []domain.Near{
		{Name: "x", PlaceID: "short", RadiusMeters: 1000},
		{Name: "x", PlaceID: "ChIJ26L7ANfa1EAR&page=9", RadiusMeters: 1000},
		{Name: "x", PlaceID: "ChIJ26L7ANfa1EARS0Dp8jarxS4", RadiusMeters: 0},
		{Name: "x", PlaceID: "ChIJ26L7ANfa1EARS0Dp8jarxS4", RadiusMeters: 60_000},
	} {
		near := near
		if paths, err := searchPaths(domain.Query{City: "Київ", Near: &near}); err == nil {
			t.Fatalf("%+v accepted: %v", near, paths)
		}
	}
}
