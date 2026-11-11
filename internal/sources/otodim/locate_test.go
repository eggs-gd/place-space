package otodim

import (
	"context"
	"math"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/eggs-gd/place-space/internal/domain"
)

func nextData(json string) string {
	return `<html><script id="__NEXT_DATA__" type="application/json">` + json + `</script></html>`
}

func TestLocateReadsListingPage(t *testing.T) {
	var asked []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		asked = append(asked, r.URL.Path)
		_, _ = w.Write([]byte(nextData(`{"props":{"pageProps":{"ad":{"location":{"coordinates":{"latitude":50.41,"longitude":30.61}}}}}}`)))
	}))
	defer server.Close()
	a := newAdapter(server, 1)
	lat, lng, err := a.Locate(context.Background(), domain.Listing{URL: "https://www.otodim.com.ua/uk/oholoshennia/kvartira-ID7"})
	if err != nil || lat != 50.41 || lng != 30.61 {
		t.Fatalf("%v %v %v", lat, lng, err)
	}
	if strings.Join(asked, ",") != "/uk/oholoshennia/kvartira-ID7" {
		t.Fatalf("asked %v", asked)
	}
	for _, bad := range []string{"https://evil.test/uk/rezultaty/x", "https://www.otodim.com.ua/uk/oholoshennia/../../api/query", "::"} {
		if _, _, err := a.Locate(context.Background(), domain.Listing{URL: bad}); err == nil {
			t.Fatalf("%q accepted", bad)
		}
	}
	if len(asked) != 1 {
		t.Fatalf("bad urls were requested: %v", asked)
	}
}

func TestLocateWithoutCoordinates(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(nextData(`{"props":{"pageProps":{"ad":{"location":{}}}}}`)))
	}))
	defer server.Close()
	if _, _, err := newAdapter(server, 1).Locate(context.Background(), domain.Listing{URL: "https://x.test/uk/oholoshennia/a-ID1"}); err == nil {
		t.Fatal("page without coordinates was accepted")
	}
}

func TestPlaceCenterAveragesTheCircle(t *testing.T) {
	var query string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		query = r.URL.RequestURI()
		ring := `[[30.6,50.4],[30.61,50.41],[30.62,50.4],[30.61,50.39],[30.6,50.4]]`
		shape := `{\"type\":\"Feature\",\"properties\":{},\"geometry\":{\"type\":\"Polygon\",\"coordinates\":[` + ring + `]}}`
		_, _ = w.Write([]byte(nextData(`{"props":{"pageProps":{"filterLocations":{"byGeometry":[{"byGeoJson":"` + shape + `"}]}}}}`)))
	}))
	defer server.Close()
	lat, lng, err := newAdapter(server, 1).PlaceCenter(context.Background(), "ChIJ26L7ANfa1EARS0Dp8jarxS4")
	if err != nil || math.Abs(lat-50.4) > 1e-9 || math.Abs(lng-30.61) > 1e-9 {
		t.Fatalf("%v %v %v", lat, lng, err)
	}
	if !strings.HasPrefix(query, "/uk/rezultaty/orenda/kvartyry/vsia-ukraina?") || !strings.Contains(query, "placeId=ChIJ26L7ANfa1EARS0Dp8jarxS4") {
		t.Fatalf("query %s", query)
	}
	if _, _, err := newAdapter(server, 1).PlaceCenter(context.Background(), "bad&id"); err == nil {
		t.Fatal("bad place id accepted")
	}
}
