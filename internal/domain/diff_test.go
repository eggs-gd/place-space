package domain

import "testing"

func TestDiffPriceAndPhotos(t *testing.T) {
	prevPrice := int64(20000)
	nextPrice := int64(18000)
	prev := Listing{
		PriceAmount: &prevPrice,
		Currency:    "UAH",
		Images:      []string{"a", "b"},
		Title:       "same",
	}
	next := prev
	next.PriceAmount = &nextPrice
	next.Images = []string{"a"}

	changes := Diff(prev, next)
	if len(changes) != 2 {
		t.Fatalf("changes %#v", changes)
	}
	if changes[0].Field != "price" || changes[0].From != "20000 UAH" || changes[0].To != "18000 UAH" {
		t.Fatalf("price change %#v", changes[0])
	}
	if changes[1].Field != "photos" || changes[1].From != "2" || changes[1].To != "1" {
		t.Fatalf("photo change %#v", changes[1])
	}
	if Fingerprint(prev) == Fingerprint(next) {
		t.Fatal("fingerprint ignored the price change")
	}
}

func TestDiffIgnoresSeenAtAndRawPayload(t *testing.T) {
	price := int64(18000)
	listing := Listing{PriceAmount: &price, Currency: "UAH", Title: "вулиця", RawData: []byte(`{"a":1}`)}
	other := listing
	other.RawData = []byte(`{"a":2,"downloadTime":"later"}`)
	if changes := Diff(listing, other); len(changes) != 0 {
		t.Fatalf("changes %#v", changes)
	}
	if Fingerprint(listing) != Fingerprint(other) {
		t.Fatal("fingerprint included raw payload")
	}
}
