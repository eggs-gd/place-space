package application

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/eggs-gd/place-space/internal/domain"
	"github.com/eggs-gd/place-space/internal/sources"
)

func TestClassifyNewChangedAndRejection(t *testing.T) {
	now := time.Date(2026, 10, 6, 9, 0, 0, 0, time.UTC)
	max := int64(20000)
	watch := domain.Watch{ID: "w", Filters: domain.Filters{PriceMax: &max}}
	source := domain.Source{ID: "lun", Type: "lun"}

	first, err := Classify(ClassifyInput{
		Now: now, RunID: "run-1", Watch: watch, Source: source,
		Raw:       []sources.RawListing{priceRaw("1", 18000), priceRaw("2", 24500), priceRaw("2", 24500)},
		Normalize: decodePrice,
	})
	if err != nil {
		t.Fatal(err)
	}
	assertPartition(t, first.Run)
	if first.Run.Received != 3 || first.Run.New != 2 || first.Run.Duplicates != 1 || first.Run.Matched != 1 || first.Run.Rejected != 1 {
		t.Fatalf("first run %+v", first.Run)
	}
	if len(first.Items) != 2 {
		t.Fatalf("items %d", len(first.Items))
	}
	if first.Items[1].Decision.Reasons[0].Message != "price: 24500 > 20000" {
		t.Fatalf("reason %#v", first.Items[1].Decision.Reasons)
	}

	existing := make([]domain.Listing, 0, len(first.Items))
	for _, item := range first.Items {
		existing = append(existing, item.Listing)
	}
	second, err := Classify(ClassifyInput{
		Now: now.Add(time.Minute), RunID: "run-2", Watch: watch, Source: source,
		Raw:       []sources.RawListing{priceRaw("1", 18000), priceRaw("2", 24500)},
		Existing:  existing,
		Normalize: decodePrice,
	})
	if err != nil {
		t.Fatal(err)
	}
	assertPartition(t, second.Run)
	if second.Run.New != 0 || second.Run.Changed != 0 || second.Run.Duplicates != 2 {
		t.Fatalf("second run %+v", second.Run)
	}
	if len(second.Write.Observations) != 0 {
		t.Fatal("unchanged listings stored another observation")
	}

	third, err := Classify(ClassifyInput{
		Now: now.Add(2 * time.Minute), RunID: "run-3", Watch: watch, Source: source,
		Raw:       []sources.RawListing{priceRaw("1", 17000), priceRaw("2", 24500)},
		Existing:  existing,
		Normalize: decodePrice,
	})
	if err != nil {
		t.Fatal(err)
	}
	assertPartition(t, third.Run)
	if third.Run.Changed != 1 || third.Run.New != 0 || third.Run.Duplicates != 1 {
		t.Fatalf("third run %+v", third.Run)
	}
	if third.Items[0].Changes[0].From != "18000 UAH" || third.Items[0].Changes[0].To != "17000 UAH" {
		t.Fatalf("change %#v", third.Items[0].Changes)
	}
	if len(third.Write.Observations) != 1 {
		t.Fatalf("observations %d", len(third.Write.Observations))
	}
}

func TestClassifyLinksCanonicalURLWithoutMerging(t *testing.T) {
	now := time.Date(2026, 10, 6, 9, 0, 0, 0, time.UTC)
	result, err := Classify(ClassifyInput{
		Now: now, RunID: "run", Watch: domain.Watch{ID: "w"}, Source: domain.Source{ID: "lun", Type: "lun"},
		Raw: []sources.RawListing{priceRaw("1", 10000), priceRaw("2", 10000)},
		Normalize: func(raw sources.RawListing) (domain.Listing, error) {
			listing, err := decodePrice(raw)
			if err != nil {
				return domain.Listing{}, err
			}
			listing.URL = "https://example.test/same"
			return listing, nil
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.Run.New != 2 || result.Run.DuplicateLinks != 1 {
		t.Fatalf("run %+v", result.Run)
	}
	if result.Items[1].DuplicateOf == "" || result.Items[1].DuplicateOf == result.Items[1].Listing.ID {
		t.Fatalf("duplicate link %#v", result.Items[1])
	}
	if len(result.Write.Listings) != 2 || !result.Write.Listings[0].Insert || !result.Write.Listings[1].Insert {
		t.Fatal("both listings should be inserted")
	}
	link := result.Write.Links[0]
	if link.ListingA == link.ListingB || link.Reason != domain.DuplicateCanonicalURL {
		t.Fatalf("link %+v", link)
	}
	if link.ListingA > link.ListingB {
		t.Fatalf("link ids are not ordered: %+v", link)
	}
}

func priceRaw(id string, price int64) sources.RawListing {
	payload, _ := json.Marshal(map[string]any{"id": id, "price": price})
	return sources.RawListing{ExternalID: id, Payload: payload}
}

func decodePrice(raw sources.RawListing) (domain.Listing, error) {
	var payload struct {
		ID    string `json:"id"`
		Price int64  `json:"price"`
	}
	if err := json.Unmarshal(raw.Payload, &payload); err != nil {
		return domain.Listing{}, err
	}
	return domain.Listing{
		ExternalID:  payload.ID,
		URL:         "https://example.test/" + payload.ID,
		PriceAmount: &payload.Price,
		Currency:    "UAH",
		Images:      []string{},
	}, nil
}

func assertPartition(t *testing.T, run domain.Run) {
	t.Helper()
	if run.New+run.Changed+run.Duplicates+run.Failed != run.Received {
		t.Fatalf("partition new=%d changed=%d duplicates=%d failed=%d received=%d",
			run.New, run.Changed, run.Duplicates, run.Failed, run.Received)
	}
}
