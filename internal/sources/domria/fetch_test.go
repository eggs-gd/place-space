package domria

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/eggs-gd/place-space/internal/domain"
)

func TestFetchReadsCatalogPages(t *testing.T) {
	var paths []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		paths = append(paths, r.URL.RequestURI())
		switch {
		case strings.Contains(r.URL.Path, "/arenda-domov/"):
			_, _ = w.Write([]byte(catalogPage([]string{cardJSON("3", false)}, 1, false)))
		case r.URL.Query().Get("page") == "2":
			_, _ = w.Write([]byte(catalogPage([]string{cardJSON("2", false)}, 2, false)))
		default:
			_, _ = w.Write([]byte(catalogPage([]string{cardJSON("1", false)}, 2, false)))
		}
	}))
	defer server.Close()

	adapter := New(Options{BaseURL: server.URL, HTTP: server.Client(), DisablePause: true, MaxPages: 5})
	raws, err := adapter.Fetch(context.Background(), domain.Query{
		City:       "Кам'янець-Подільський",
		Deal:       domain.DealRent,
		Properties: []string{domain.PropertyApartment, domain.PropertyHouse},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(raws) != 3 {
		t.Fatalf("raws %d, paths %v", len(raws), paths)
	}
	ids := []string{raws[0].ExternalID, raws[1].ExternalID, raws[2].ExternalID}
	if ids[0] != "1" || ids[1] != "2" || ids[2] != "3" {
		t.Fatalf("ids %v paths %v", ids, paths)
	}
	joined := strings.Join(paths, " ")
	if !strings.Contains(joined, "/uk/arenda-kvartir/kamenets-podolskyi/") || !strings.Contains(joined, "/uk/arenda-domov/kamenets-podolskyi/") {
		t.Fatalf("paths %v", paths)
	}
}

func TestFetchIgnoresFallbackWhenCityIsEmpty(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(catalogPage([]string{cardJSON("9", false)}, 0, true)))
	}))
	defer server.Close()

	adapter := New(Options{BaseURL: server.URL, HTTP: server.Client(), DisablePause: true})
	raws, err := adapter.Fetch(context.Background(), domain.Query{City: "Кам'янець-Подільський", Deal: domain.DealRent})
	if err != nil {
		t.Fatal(err)
	}
	if len(raws) != 0 {
		t.Fatalf("fallback listings were kept: %d", len(raws))
	}
}

func TestFetchRejectsUnknownCity(t *testing.T) {
	adapter := New(Options{})
	if _, err := adapter.Fetch(context.Background(), domain.Query{City: "неіснуюче"}); err == nil {
		t.Fatal("expected an unknown-city error")
	}
}

func catalogPage(cards []string, count int, empty bool) string {
	items := strings.Join(cards, ",")
	emptyJSON := "false"
	if empty {
		emptyJSON = "true"
	}
	return `<html><script>window.__INITIAL_STATE__={"catalog":{"realtyForCatalog":[` + items + `],"realtyCountCatalog":` +
		itoa(count) + `,"isEmptyCatalog":{"isEmpty":` + emptyJSON + `}}}</script></html>`
}

func cardJSON(id string, hiddenStreet bool) string {
	show := "1"
	if hiddenStreet {
		show = "0"
	}
	return `{"realty_id":` + id + `,"beautiful_url":"realty-` + id + `.html","price_total":14000,"currency_type":"грн","rooms_count":2,"is_show_street":` + show + `}`
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var digits [20]byte
	i := len(digits)
	for n > 0 {
		i--
		digits[i] = byte('0' + n%10)
		n /= 10
	}
	return string(digits[i:])
}
