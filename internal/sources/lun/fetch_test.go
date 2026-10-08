package lun

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/eggs-gd/place-space/internal/domain"
)

func TestFetchReadsEmbeddedPages(t *testing.T) {
	var paths []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		paths = append(paths, r.URL.RequestURI())
		switch {
		case strings.Contains(r.URL.Path, "/houses"):
			_, _ = w.Write([]byte(embedPage([]cardJSON{{ID: "3", Price: 9000}}, 1)))
		case r.URL.Query().Get("page") == "2":
			_, _ = w.Write([]byte(embedPage([]cardJSON{{ID: "2", Price: 24500}}, 2)))
		default:
			_, _ = w.Write([]byte(embedPage([]cardJSON{{ID: "1", Price: 18000}}, 2)))
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
	if !strings.Contains(joined, "/rent/khmelnytskyi/flats-kamianets-podilskyi") || !strings.Contains(joined, "/houses-kamianets-podilskyi") {
		t.Fatalf("paths %v", paths)
	}
}

func TestFetchRejectsUnknownCity(t *testing.T) {
	adapter := New(Options{})
	if _, err := adapter.Fetch(context.Background(), domain.Query{City: "неіснуюче"}); err == nil {
		t.Fatal("expected an unknown-city error")
	}
}

func TestEmbeddedQuotesRoundTrip(t *testing.T) {
	page := embedPage([]cardJSON{{ID: "9", Price: 1000, Text: `say "hi" and a \ slash`}}, 1)
	env, err := extractRealties(page)
	if err != nil {
		t.Fatal(err)
	}
	if len(env.Cards) != 1 {
		t.Fatalf("cards %d", len(env.Cards))
	}
	var got struct {
		Text string `json:"text"`
	}
	if err := json.Unmarshal(env.Cards[0], &got); err != nil {
		t.Fatal(err)
	}
	if got.Text != `say "hi" and a \ slash` {
		t.Fatalf("text %q", got.Text)
	}
}

type cardJSON struct {
	ID    string
	Price int
	Text  string
}

func embedPage(cards []cardJSON, total int) string {
	payload := make([]map[string]any, 0, len(cards))
	for _, card := range cards {
		payload = append(payload, map[string]any{
			"id":       json.Number(card.ID),
			"urlRaw":   "https://example.test/" + card.ID,
			"price":    card.Price,
			"currency": "uah",
			"text":     card.Text,
		})
	}
	body, err := json.Marshal(map[string]any{
		"realties": map[string]any{
			"cards":              payload,
			"totalRealtiesCount": total,
		},
	})
	if err != nil {
		panic(err)
	}
	return `<html><script>self.__next_f.push([1,"` + jsEscape(string(body)) + `"])</script></html>`
}

func jsEscape(value string) string {
	var b strings.Builder
	for _, r := range value {
		switch r {
		case '\\', '"':
			b.WriteByte('\\')
			b.WriteRune(r)
		case '\n':
			b.WriteString(`\n`)
		default:
			b.WriteRune(r)
		}
	}
	return b.String()
}
