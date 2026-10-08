package domria

import (
	"context"
	"testing"
	"time"

	"github.com/eggs-gd/place-space/internal/sources"
)

func TestNormalizeCard(t *testing.T) {
	payload := []byte(`{
		"realty_id": 35012551,
		"beautiful_url": "realty-example.html",
		"realtyTitle": "вул. Львівське шосе",
		"description_uk": "Опис",
		"price_total": 14000,
		"currency_type": "грн",
		"rooms_count": 2,
		"total_square_meters": 49.5,
		"floor": 6,
		"floors_count": 10,
		"city_name_uk": "Хмельницький",
		"street_name_uk": "Львівське шосе",
		"building_number_str": "53/3",
		"is_show_street": 1,
		"latitude": 49.4117151,
		"longitude": 26.9409804,
		"publishing_date": "2026-10-07 16:02:56",
		"photos": [{"file": "dom/photo/1.jpg"}]
	}`)
	listing, err := New(Options{}).Normalize(context.Background(), sources.RawListing{Payload: payload})
	if err != nil {
		t.Fatal(err)
	}
	if listing.ExternalID != "35012551" || listing.Source != "domria" {
		t.Fatalf("identity %+v", listing)
	}
	if listing.URL != "https://dom.ria.com/uk/realty-example.html" {
		t.Fatalf("url %s", listing.URL)
	}
	if listing.PriceAmount == nil || *listing.PriceAmount != 14000 || listing.Currency != "UAH" {
		t.Fatalf("price %+v %s", listing.PriceAmount, listing.Currency)
	}
	if listing.Rooms == nil || *listing.Rooms != 2 || listing.Area == nil || *listing.Area != 49.5 {
		t.Fatalf("facts rooms=%v area=%v", listing.Rooms, listing.Area)
	}
	if listing.Location.Address != "Львівське шосе, 53/3" || listing.Location.Name != "Хмельницький" {
		t.Fatalf("place %+v", listing.Location)
	}
	if listing.Location.Latitude == nil || *listing.Location.Longitude != 26.9409804 {
		t.Fatalf("coordinates %+v", listing.Location)
	}
	if len(listing.Images) != 1 || listing.Images[0] != "https://cdn.riastatic.com/photos/dom/photo/1xl.jpg" {
		t.Fatalf("images %v", listing.Images)
	}
	if listing.PublishedAt == nil || !listing.PublishedAt.Equal(time.Date(2026, 10, 7, 13, 2, 56, 0, time.UTC)) {
		t.Fatalf("published %v", listing.PublishedAt)
	}
}

func TestDisplayPhotoUsesLargeSize(t *testing.T) {
	payload := []byte(`{
		"realty_id": 34397832,
		"beautiful_url": "realty-example.html",
		"photos": [
			{"file": "dom/photo/33628/3362898/336289840/336289840.jpg"},
			{"file": "https://cdn.riastatic.com/photos/dom/photo/33628/3362898/336289840/336289840b.jpg"}
		]
	}`)
	listing, err := New(Options{}).Normalize(context.Background(), sources.RawListing{Payload: payload})
	if err != nil {
		t.Fatal(err)
	}
	want := []string{
		"https://cdn.riastatic.com/photos/dom/photo/33628/3362898/336289840/336289840xl.jpg",
		"https://cdn.riastatic.com/photos/dom/photo/33628/3362898/336289840/336289840b.jpg",
	}
	if len(listing.Images) != len(want) {
		t.Fatalf("images %v", listing.Images)
	}
	for i := range want {
		if listing.Images[i] != want[i] {
			t.Fatalf("images %v", listing.Images)
		}
	}
}

func TestNormalizeDropsMissingNumbers(t *testing.T) {
	payload := []byte(`{"realty_id":7,"beautiful_url":"realty-7.html","price_total":0,"rooms_count":0,"latitude":0,"longitude":0,"is_show_street":0,"street_name_uk":"Прихована"}`)
	listing, err := New(Options{}).Normalize(context.Background(), sources.RawListing{Payload: payload})
	if err != nil {
		t.Fatal(err)
	}
	if listing.PriceAmount != nil || listing.Rooms != nil || listing.Location.Latitude != nil {
		t.Fatalf("invented values %+v", listing)
	}
	if listing.Location.Address != "" {
		t.Fatalf("hidden street leaked: %q", listing.Location.Address)
	}
}
