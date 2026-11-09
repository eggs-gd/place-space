package domria

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/eggs-gd/place-space/internal/domain"
	"github.com/eggs-gd/place-space/internal/sources"
)

func serve(t *testing.T, handler http.HandlerFunc) *Adapter {
	t.Helper()
	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)
	return New(Options{BaseURL: server.URL, HTTP: server.Client(), DisablePause: true, MaxPages: 1})
}

func TestAreasReadRayonLinks(t *testing.T) {
	adapter := serve(t, func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`<html>
<a data-level="area" href="/uk/arenda-kvartir/kiev-rayon-darnytskyi/" class="x">
   Дарницький
</a>
<a href="/uk/arenda-kvartir/kiev-rayon-solomenskyi/"><b>Солом'янський</b></a>
<a href="/uk/arenda-kvartir/kiev-metro-osokorki/">Осокорки</a>
<a href="/uk/arenda-kvartir/lvov-rayon-sykhov/">Сихів</a>
</html>`))
	})
	areas, err := adapter.Areas(context.Background(), "Київ")
	if err != nil {
		t.Fatal(err)
	}
	if len(areas) != 2 || areas[0].Key != "дарницький" || areas[0].Path != "/uk/arenda-kvartir/kiev-rayon-darnytskyi/" ||
		areas[0].Kind != domain.AreaMicrodistrict || areas[1].Key != "солом'янський" {
		t.Fatalf("areas %+v", areas)
	}
}

func TestFetchReadsAreaPagesForEachProperty(t *testing.T) {
	var paths []string
	adapter := serve(t, func(w http.ResponseWriter, r *http.Request) {
		paths = append(paths, r.URL.Path)
		_, _ = w.Write([]byte(catalogPage([]string{cardJSON("1", false)}, 1, false)))
	})
	_, err := adapter.Fetch(context.Background(), domain.Query{
		City:       "Київ",
		Properties: []string{domain.PropertyApartment, domain.PropertyHouse},
		Areas:      []domain.AreaRef{{Key: "дарницький"}},
		AreaPaths:  []string{"/uk/arenda-kvartir/kiev-rayon-darnytskyi/"},
	})
	if err != nil {
		t.Fatal(err)
	}
	want := "/uk/arenda-kvartir/kiev-rayon-darnytskyi/,/uk/arenda-domov/kiev-rayon-darnytskyi/"
	if got := strings.Join(paths, ","); got != want {
		t.Fatalf("paths %s", got)
	}
}

func TestFetchTreatsRedirectToWholeCityAsFailure(t *testing.T) {
	adapter := serve(t, func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "-rayon-") {
			http.Redirect(w, r, "/uk/arenda-kvartir/kiev/", http.StatusMovedPermanently)
			return
		}
		_, _ = w.Write([]byte(catalogPage([]string{cardJSON("1", false)}, 1, false)))
	})
	raws, err := adapter.Fetch(context.Background(), domain.Query{
		City: "Київ", Areas: []domain.AreaRef{{Key: "x"}}, AreaPaths: []string{"/uk/arenda-kvartir/kiev-rayon-gone/"},
	})
	if err == nil || !strings.Contains(err.Error(), "redirected") || len(raws) != 0 {
		t.Fatalf("raws %d err %v", len(raws), err)
	}
}

func TestOverlappingAreasStillReadLaterPages(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		page := r.URL.Query().Get("page")
		switch {
		case strings.Contains(r.URL.Path, "rayon-b") && page == "2":
			_, _ = w.Write([]byte(catalogPage([]string{cardJSON("3", false), cardJSON("4", false)}, 4, false)))
		case page == "2":
			_, _ = w.Write([]byte(catalogPage([]string{cardJSON("1", false), cardJSON("2", false)}, 4, false)))
		default:
			_, _ = w.Write([]byte(catalogPage([]string{cardJSON("1", false), cardJSON("2", false)}, 4, false)))
		}
	}))
	defer server.Close()
	adapter := New(Options{BaseURL: server.URL, HTTP: server.Client(), DisablePause: true, MaxPages: 2})
	raws, err := adapter.Fetch(context.Background(), domain.Query{
		City: "Київ", Areas: []domain.AreaRef{{Key: "a"}, {Key: "b"}},
		AreaPaths: []string{"/uk/arenda-kvartir/kiev-rayon-a/", "/uk/arenda-kvartir/kiev-rayon-b/"},
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

func TestFetchMarksRefusals(t *testing.T) {
	adapter := serve(t, func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusTooManyRequests) })
	_, err := adapter.Fetch(context.Background(), domain.Query{City: "Київ"})
	if !errors.Is(err, sources.ErrBlocked) {
		t.Fatalf("err %v", err)
	}
}

func TestCleanPathAcceptsOnlyCatalogPaths(t *testing.T) {
	for _, good := range []string{"/uk/arenda-kvartir/kiev/", "/uk/arenda-kvartir/kiev-rayon-darnytskyi/"} {
		if _, err := cleanPath(good); err != nil {
			t.Errorf("%s: %v", good, err)
		}
	}
	for _, bad := range []string{
		"", "/uk/arenda-kvartir/", "/uk/prodazha/kiev/", "/uk/arenda-kvartir/../admin", "/uk/arenda-kvartir/kiev/?x=1",
		"/uk/arenda-kvartir/%2e%2e/", "/uk/arenda-kvartir/kiev/extra/", "//evil.test/uk/arenda-kvartir/a/",
	} {
		if _, err := cleanPath(bad); err == nil {
			t.Errorf("%q accepted", bad)
		}
	}
}
