package domain

import "testing"

func TestApplyPriceRejection(t *testing.T) {
	price := int64(24500)
	max := int64(20000)
	decision := Apply(Filters{PriceMax: &max}, Listing{PriceAmount: &price, Currency: "UAH"}, nil)
	if decision.Status != MatchRejected {
		t.Fatalf("status %q", decision.Status)
	}
	if len(decision.Reasons) != 1 || decision.Reasons[0].Message != "price: 24500 > 20000" {
		t.Fatalf("reasons %#v", decision.Reasons)
	}
}

func TestApplyExplainsEveryPredicate(t *testing.T) {
	price := int64(18000)
	rooms := 2
	area := 62.5
	max := int64(20000)
	roomsMin := 2
	areaMin := 50.0
	decision := Apply(Filters{
		PriceMax: &max,
		RoomsMin: &roomsMin,
		AreaMin:  &areaMin,
	}, Listing{PriceAmount: &price, Currency: "UAH", Rooms: &rooms, Area: &area}, nil)
	if decision.Status != MatchMatched {
		t.Fatalf("status %q reasons %#v", decision.Status, decision.Reasons)
	}
	want := []string{
		"price: 18000 <= 20000",
		"rooms: 2 >= 2",
		"area: 62.5 >= 50",
	}
	if len(decision.Reasons) != len(want) {
		t.Fatalf("reasons %#v", decision.Reasons)
	}
	for i, message := range want {
		if decision.Reasons[i].Message != message || !decision.Reasons[i].Passed {
			t.Fatalf("reason %d = %#v", i, decision.Reasons[i])
		}
	}
}

func TestApplyUnknownPriceIsRejected(t *testing.T) {
	max := int64(20000)
	decision := Apply(Filters{PriceMax: &max}, Listing{}, nil)
	if decision.Status != MatchRejected || decision.Reasons[0].Message != "price: unknown" {
		t.Fatalf("decision %#v", decision)
	}
}

func TestApplyCurrencyIsNotConverted(t *testing.T) {
	price := int64(500)
	max := int64(20000)
	decision := Apply(Filters{PriceMax: &max}, Listing{PriceAmount: &price, Currency: "USD"}, nil)
	if decision.Status != MatchRejected || decision.Reasons[0].Message != "currency: USD != UAH" {
		t.Fatalf("decision %#v", decision)
	}
}

func TestApplyWithoutFiltersMatches(t *testing.T) {
	decision := Apply(Filters{}, Listing{}, nil)
	if decision.Status != MatchMatched || len(decision.Reasons) != 0 {
		t.Fatalf("decision %#v", decision)
	}
}

func TestApplyConvertsForeignPricesForComparison(t *testing.T) {
	rates := Rates{"USD": 45}
	max := int64(20000)
	cheap := int64(400)
	dear := int64(500)
	if got := Apply(Filters{PriceMax: &max}, Listing{PriceAmount: &cheap, Currency: "USD"}, rates); got.Status != MatchMatched {
		t.Fatalf("cheap: %+v", got)
	}
	got := Apply(Filters{PriceMax: &max}, Listing{PriceAmount: &dear, Currency: "usd"}, rates)
	if got.Status != MatchRejected || got.Reasons[0].Message != "price: 22500 > 20000" {
		t.Fatalf("dear: %+v", got)
	}
}

func TestApplyConvertsThroughHryvniaToFilterCurrency(t *testing.T) {
	rates := Rates{"USD": 40, "EUR": 50}
	max := int64(500)
	price := int64(450)
	hryvnias := int64(18000)
	got := Apply(Filters{PriceMax: &max, Currency: "USD"}, Listing{PriceAmount: &price, Currency: "EUR"}, rates)
	if got.Status != MatchRejected || got.Reasons[0].Message != "price: 563 > 500" {
		t.Fatalf("eur: %+v", got)
	}
	got = Apply(Filters{PriceMax: &max, Currency: "USD"}, Listing{PriceAmount: &hryvnias, Currency: "UAH"}, rates)
	if got.Status != MatchMatched {
		t.Fatalf("uah: %+v", got)
	}
}

func TestApplyRejectsWhenRateOrCurrencyIsUnknown(t *testing.T) {
	max := int64(20000)
	price := int64(400)
	got := Apply(Filters{PriceMax: &max}, Listing{PriceAmount: &price, Currency: "PLN"}, Rates{"USD": 45})
	if got.Status != MatchRejected || got.Reasons[0].Message != "currency: PLN != UAH" {
		t.Fatalf("no rate: %+v", got)
	}
	got = Apply(Filters{PriceMax: &max}, Listing{PriceAmount: &price}, Rates{"USD": 45})
	if got.Status != MatchRejected || got.Reasons[0].Message != "currency: unknown != UAH" {
		t.Fatalf("no currency: %+v", got)
	}
}
