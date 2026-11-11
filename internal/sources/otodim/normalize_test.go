package otodim

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/eggs-gd/place-space/internal/sources"
)

func normalize(t *testing.T, payload string) (listing, error) {
	t.Helper()
	got, err := New(Options{}).Normalize(context.Background(), sources.RawListing{ExternalID: "7", Payload: []byte(payload)})
	return listing{got.ExternalID, got.URL, got.Title, got.Location.Name, got.Location.Address, got.Currency, got.PriceAmount, got.Rooms, got.Floor, got.Area, got.Images, got.PublishedAt, string(got.RawData)}, err
}

type listing struct {
	id, url, title, city, address, currency string
	price                                   *int64
	rooms, floor                            *int
	area                                    *float64
	images                                  []string
	published                               *time.Time
	raw                                     string
}

func TestNormalizeMapsCard(t *testing.T) {
	got, err := normalize(t, cardJSON(7))
	if err != nil {
		t.Fatal(err)
	}
	if got.id != "7" || got.url != "https://www.otodim.com.ua/uk/oholoshennia/kvartira-ID7" || got.title != "Квартира 7" {
		t.Fatalf("identity %+v", got)
	}
	if got.city != "Київ" || got.address != "" {
		t.Fatalf("place %+v", got)
	}
	if got.price == nil || *got.price != 15000 || got.currency != "UAH" {
		t.Fatalf("price %+v", got)
	}
	if got.rooms == nil || *got.rooms != 2 || got.floor == nil || *got.floor != 3 || got.area == nil || *got.area != 45 {
		t.Fatalf("facts %+v", got)
	}
	if len(got.images) != 1 || got.images[0] != "https://img.test/7" {
		t.Fatalf("images %v", got.images)
	}
	want := time.Date(2026, 11, 1, 8, 0, 0, 0, time.UTC)
	if got.published == nil || !got.published.Equal(want) {
		t.Fatalf("published %v, want the first appearance %v", got.published, want)
	}
	if got.raw == "" {
		t.Fatal("raw payload was dropped")
	}
}

func TestNormalizeLeavesUnknownValuesEmpty(t *testing.T) {
	payload := strings.NewReplacer(`"roomsNumber":"TWO"`, `"roomsNumber":"MORE"`, `"floorNumber":"THIRD"`, `"floorNumber":"ABOVE_TENTH"`,
		`"hidePrice":false`, `"hidePrice":true`, `"areaInSquareMeters":45`, `"areaInSquareMeters":0`).Replace(cardJSON(8))
	got, err := normalize(t, payload)
	if err != nil {
		t.Fatal(err)
	}
	if got.rooms != nil || got.floor != nil || got.price != nil || got.area != nil || got.currency != "" {
		t.Fatalf("invented values %+v", got)
	}
}

func TestNormalizeNeedsIDAndSlug(t *testing.T) {
	if _, err := normalize(t, `{"slug":"a"}`); err == nil {
		t.Fatal("card without id was accepted")
	}
	if _, err := normalize(t, `{"id":1}`); err == nil {
		t.Fatal("card without slug was accepted")
	}
}
