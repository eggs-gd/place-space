package lun

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/eggs-gd/place-space/internal/domain"
	"github.com/eggs-gd/place-space/internal/sources"
)

const cityPage = `<html>
<a data-event-action="internal_linkdistrict" data-event-label="Дарницький" href="/rent/kyiv/flats-darnytskyi-district">Дарницький район</a>
<a data-event-action="card" data-event-label="microdistrict_click" href="/rent/kyiv/flats-obolon"><span>Оболонь</span></a>
<a data-event-action="card" data-event-label="subway_station_click" href="/rent/kyiv/flats-osokorky-subway">Осокорки</a>
<a data-event-action="popular_search" data-event-label="1 room" href="/rent/kyiv/flats-odnokimnatni">1-кімнатні</a>
<a data-event-action="card" data-event-label="district_click" href="/rent/lviv/flats-sykhivskyi-district">Сихівський</a>
</html>`

const districtPage = `<html><script>{"realties":{"totalRealtiesCount":1,"cards":[{"id":1,"geoEntities":[
{"type":"microdistrict","name":"Солом’янка","realtyGeoUrl":{"link":"/rent/kyiv/flats-solomianka"}},
{"type":"subway_station","name":"Позняки","realtyGeoUrl":{"link":"/rent/kyiv/flats-pozniaky-subway"}},
{"type":"microdistrict","name":"Без посилання","realtyGeoUrl":null}
]}]}}</script></html>`

func serve(t *testing.T, handler http.HandlerFunc) *Adapter {
	t.Helper()
	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)
	return New(Options{BaseURL: server.URL, HTTP: server.Client(), DisablePause: true, MaxPages: 1})
}

func TestAreasReadCityAndDistrictPages(t *testing.T) {
	adapter := serve(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/rent/kyiv/flats-darnytskyi-district" {
			_, _ = w.Write([]byte(districtPage))
			return
		}
		_, _ = w.Write([]byte(cityPage))
	})
	areas, err := adapter.Areas(context.Background(), "Київ")
	if err != nil {
		t.Fatal(err)
	}
	want := []domain.Area{
		{Key: "дарницький", Name: "Дарницький", Kind: domain.AreaDistrict, Path: "/rent/kyiv/flats-darnytskyi-district"},
		{Key: "оболонь", Name: "Оболонь", Kind: domain.AreaMicrodistrict, Path: "/rent/kyiv/flats-obolon"},
		{Key: "солом'янка", Name: "Солом’янка", Kind: domain.AreaMicrodistrict, Path: "/rent/kyiv/flats-solomianka"},
	}
	if len(areas) != len(want) {
		t.Fatalf("areas %+v", areas)
	}
	for i := range want {
		if areas[i] != want[i] {
			t.Fatalf("area %d: %+v, want %+v", i, areas[i], want[i])
		}
	}
}

func TestAreasKeepCityPageWhenDistrictPageFails(t *testing.T) {
	adapter := serve(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/rent/kyiv/flats" {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		_, _ = w.Write([]byte(cityPage))
	})
	areas, err := adapter.Areas(context.Background(), "Київ")
	if !errors.Is(err, sources.ErrIncompleteAreas) || len(areas) != 2 {
		t.Fatalf("areas %+v err %v", areas, err)
	}
}

func cardsPage(total int, ids ...int) string {
	cards := make([]string, 0, len(ids))
	for _, id := range ids {
		cards = append(cards, fmt.Sprintf(`{"id":%d,"urlRaw":"https://example.test/%d"}`, id, id))
	}
	return fmt.Sprintf(`<script>{"realties":{"totalRealtiesCount":%d,"cards":[%s]}}</script>`, total, strings.Join(cards, ","))
}

func TestOverlappingAreasStillReadLaterPages(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		page := r.URL.Query().Get("page")
		switch {
		case strings.HasSuffix(r.URL.Path, "-b") && page == "2":
			_, _ = w.Write([]byte(cardsPage(4, 3, 4)))
		case page == "2":
			_, _ = w.Write([]byte(cardsPage(2)))
		default:
			_, _ = w.Write([]byte(cardsPage(4, 1, 2)))
		}
	}))
	defer server.Close()
	adapter := New(Options{BaseURL: server.URL, HTTP: server.Client(), DisablePause: true, MaxPages: 2})
	raws, err := adapter.Fetch(context.Background(), domain.Query{
		City: "Київ", Areas: []domain.AreaRef{{Key: "a"}, {Key: "b"}},
		AreaPaths: []string{"/rent/kyiv/flats-a", "/rent/kyiv/flats-b"},
	})
	if err != nil {
		t.Fatal(err)
	}
	ids := make([]string, 0, len(raws))
	for _, raw := range raws {
		ids = append(ids, raw.ExternalID)
	}
	if got := strings.Join(ids, ","); got != "1,2,3,4" {
		t.Fatalf("ids %s", got)
	}
}

func TestAreasFailWhenCityPageFails(t *testing.T) {
	adapter := serve(t, func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusBadGateway) })
	if _, err := adapter.Areas(context.Background(), "Київ"); err == nil {
		t.Fatal("no error")
	}
}

func oneCard() string {
	return `<script>{"realties":{"totalRealtiesCount":1,"cards":[{"id":1,"urlRaw":"https://example.test/1"}]}}</script>`
}

func TestFetchReadsChosenAreasNotTheCity(t *testing.T) {
	var (
		mu    sync.Mutex
		paths []string
	)
	adapter := serve(t, func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		paths = append(paths, r.URL.Path)
		mu.Unlock()
		_, _ = w.Write([]byte(oneCard()))
	})
	_, err := adapter.Fetch(context.Background(), domain.Query{
		City:       "Київ",
		Properties: []string{domain.PropertyApartment, domain.PropertyHouse},
		Areas:      []domain.AreaRef{{Key: "дарницький"}},
		AreaPaths:  []string{"/rent/kyiv/flats-darnytskyi-district"},
	})
	if err != nil {
		t.Fatal(err)
	}
	want := "/rent/kyiv/flats-darnytskyi-district,/rent/kyiv/houses-darnytskyi-district"
	if got := strings.Join(paths, ","); got != want {
		t.Fatalf("paths %s", got)
	}
}

func TestFetchRefusesAreasWithoutPaths(t *testing.T) {
	adapter := serve(t, func(http.ResponseWriter, *http.Request) { t.Error("requested a page") })
	if _, err := adapter.Fetch(context.Background(), domain.Query{City: "Київ", Areas: []domain.AreaRef{{Key: "x"}}}); err == nil {
		t.Fatal("no error")
	}
}

func TestFetchTreatsAreaRedirectAsFailure(t *testing.T) {
	adapter := serve(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/rent/kyiv/flats" {
			http.Redirect(w, r, "/rent/kyiv/flats", http.StatusFound)
			return
		}
		_, _ = w.Write([]byte(oneCard()))
	})
	raws, err := adapter.Fetch(context.Background(), domain.Query{
		City: "Київ", Areas: []domain.AreaRef{{Key: "x"}}, AreaPaths: []string{"/rent/kyiv/flats-gone"},
	})
	if err == nil || !strings.Contains(err.Error(), "redirected") || len(raws) != 0 {
		t.Fatalf("raws %d err %v", len(raws), err)
	}
}

func TestFetchAsksForNewestFirst(t *testing.T) {
	var query string
	adapter := serve(t, func(w http.ResponseWriter, r *http.Request) {
		query = r.URL.RawQuery
		_, _ = w.Write([]byte(oneCard()))
	})
	if _, err := adapter.Fetch(context.Background(), domain.Query{City: "Київ"}); err != nil {
		t.Fatal(err)
	}
	if query != "sort=insert_time" {
		t.Fatalf("query %q", query)
	}
}

func TestFetchFallsBackWhenSortEmptiesCatalog(t *testing.T) {
	adapter := serve(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("sort") != "" {
			_, _ = w.Write([]byte(`<script>{"realties":{"totalRealtiesCount":0,"cards":[]}}</script>`))
			return
		}
		_, _ = w.Write([]byte(oneCard()))
	})
	raws, err := adapter.Fetch(context.Background(), domain.Query{City: "Київ"})
	if err != nil || len(raws) != 1 {
		t.Fatalf("raws %d err %v", len(raws), err)
	}
}

func TestFetchMarksRefusals(t *testing.T) {
	for _, status := range []int{http.StatusForbidden, http.StatusTooManyRequests} {
		adapter := serve(t, func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(status) })
		_, err := adapter.Fetch(context.Background(), domain.Query{City: "Київ"})
		if !errors.Is(err, sources.ErrBlocked) {
			t.Fatalf("status %d: %v", status, err)
		}
	}
}

func TestCleanPathAcceptsOnlyCatalogPaths(t *testing.T) {
	for _, good := range []string{"/rent/kyiv/flats", "/rent/khmelnytskyi/flats-kamianets-podilskyi"} {
		if _, err := cleanPath(good); err != nil {
			t.Errorf("%s: %v", good, err)
		}
	}
	for _, bad := range []string{
		"", "/rent/", "/sale/kyiv/flats", "/rent/../admin", "/rent/kyiv/flats?x=1", "/rent/kyiv/flats#a",
		"/rent/kyiv/%2e%2e/flats", "/rent/kyiv//flats", "/rent/kyiv/flats@evil.test", `/rent/kyiv\flats`, "//evil.test/rent/a",
	} {
		if _, err := cleanPath(bad); err == nil {
			t.Errorf("%q accepted", bad)
		}
	}
}

func TestAreasPlaceMicrodistrictsInsideDistricts(t *testing.T) {
	page := `<script>{"realties":{"totalRealtiesCount":3,"cards":[
{"id":1,"geoEntities":[{"type":"district","name":"Дарницький","realtyGeoUrl":{"link":"/rent/kyiv/flats-darnytskyi-district"}},{"type":"microdistrict","name":"Осокорки","realtyGeoUrl":{"link":"/rent/kyiv/flats-osokorky"}}]},
{"id":2,"geoEntities":[{"type":"district","name":"Дарницький"},{"type":"microdistrict","name":"Осокорки"}]},
{"id":3,"geoEntities":[{"type":"district","name":"Подільський"},{"type":"microdistrict","name":"Осокорки"}]}
]}}</script>`
	adapter := serve(t, func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write([]byte(page)) })
	areas, err := adapter.Areas(context.Background(), "Київ")
	if err != nil {
		t.Fatal(err)
	}
	if len(areas) != 2 || areas[0].Parent != "" || areas[1].Key != "осокорки" || areas[1].Parent != "дарницький" {
		t.Fatalf("areas %+v", areas)
	}
}
