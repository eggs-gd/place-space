package lun

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/eggs-gd/place-space/internal/sources"
)

func TestNormalizeCard(t *testing.T) {
	payload := []byte(`{
		"id": 1001,
		"urlRaw": "http://www.OLX.ua/d/uk/obyavlenie/test-ID1.html?utm=1",
		"price": 18000,
		"currency": "uah",
		"roomCount": 2,
		"areaTotal": 62.5,
		"floor": 3,
		"floorCount": 9,
		"location": [26.59, 48.67],
		"header": "Здам 2-кімнатну",
		"text": "Світла квартира \"центр\"",
		"insertTime": "2026-10-05T19:19:28",
		"images": [{"imageId": 55}, {"imageId": 0}],
		"geoEntities": [
			{"type": "city", "name": "Кам'янець-Подільський"},
			{"type": "street", "name": "проспект Грушевського"},
			{"type": "house", "name": "50"}
		]
	}`)
	listing, err := New(Options{}).Normalize(context.Background(), sources.RawListing{Payload: payload})
	if err != nil {
		t.Fatal(err)
	}
	if listing.ExternalID != "1001" {
		t.Fatalf("id %s", listing.ExternalID)
	}
	if listing.URL != "https://www.olx.ua/d/uk/obyavlenie/test-ID1.html" {
		t.Fatalf("url %s", listing.URL)
	}
	if listing.Title != "Здам 2-кімнатну" || listing.Location.Address != "проспект Грушевського, 50" || listing.Location.Name != "Кам'янець-Подільський" {
		t.Fatalf("address %#v", listing.Location)
	}
	if listing.PriceAmount == nil || *listing.PriceAmount != 18000 || listing.Currency != "UAH" {
		t.Fatalf("price %v %s", listing.PriceAmount, listing.Currency)
	}
	if listing.Rooms == nil || *listing.Rooms != 2 || listing.Area == nil || *listing.Area != 62.5 {
		t.Fatalf("rooms/area %v %v", listing.Rooms, listing.Area)
	}
	if listing.Floor == nil || *listing.Floor != 3 || listing.TotalFloors == nil || *listing.TotalFloors != 9 {
		t.Fatalf("floor %v/%v", listing.Floor, listing.TotalFloors)
	}
	if listing.Location.Latitude == nil || *listing.Location.Latitude != 48.67 || *listing.Location.Longitude != 26.59 {
		t.Fatalf("coordinates %v %v", listing.Location.Latitude, listing.Location.Longitude)
	}
	if len(listing.Images) != 1 || listing.Images[0] != "https://market-images.lunstatic.net/lun-ua/1200/1200/images/55.jpg" {
		t.Fatalf("images %#v", listing.Images)
	}
	if listing.Description != `Світла квартира "центр"` {
		t.Fatalf("description %q", listing.Description)
	}
	if listing.PublishedAt == nil || listing.PublishedAt.Format("2006-01-02T15:04:05") != "2026-10-05T19:19:28" {
		t.Fatalf("published %v", listing.PublishedAt)
	}
	if !json.Valid(listing.RawData) {
		t.Fatal("raw payload was not kept")
	}
}

func TestNormalizeAcceptsOfferImagePath(t *testing.T) {
	payload := []byte(`{"id": 9, "urlRaw": "https://lun.ua/uk/realty/9", "images": [{"imageId": "offers/1878464294220183"}, {"imageId": "../secret"}, {"imageId": 0}]}`)
	listing, err := New(Options{}).Normalize(context.Background(), sources.RawListing{Payload: payload})
	if err != nil {
		t.Fatal(err)
	}
	want := "https://market-images.lunstatic.net/lun-ua/1200/1200/images/offers/1878464294220183.jpg"
	if len(listing.Images) != 1 || listing.Images[0] != want {
		t.Fatalf("images %#v", listing.Images)
	}
}

func TestNormalizeDropsZeroAndMissing(t *testing.T) {
	payload := []byte(`{"id": 7, "urlRaw": "https://lun.ua/uk/realty/7", "price": 0, "roomCount": 0, "areaTotal": null}`)
	listing, err := New(Options{}).Normalize(context.Background(), sources.RawListing{Payload: payload})
	if err != nil {
		t.Fatal(err)
	}
	if listing.PriceAmount != nil || listing.Rooms != nil || listing.Area != nil || listing.Currency != "" {
		t.Fatalf("invented values: %+v", listing)
	}
	if listing.URL != "https://lun.ua/uk/realty/7" {
		t.Fatalf("url %s", listing.URL)
	}
}
