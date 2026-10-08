package domain

import "testing"

func TestApplyPriceRejection(t *testing.T) {
	price := int64(24500)
	max := int64(20000)
	decision := Apply(Filters{PriceMax: &max}, Listing{PriceAmount: &price, Currency: "UAH"})
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
	}, Listing{PriceAmount: &price, Currency: "UAH", Rooms: &rooms, Area: &area})
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
	decision := Apply(Filters{PriceMax: &max}, Listing{})
	if decision.Status != MatchRejected || decision.Reasons[0].Message != "price: unknown" {
		t.Fatalf("decision %#v", decision)
	}
}

func TestApplyCurrencyIsNotConverted(t *testing.T) {
	price := int64(500)
	max := int64(20000)
	decision := Apply(Filters{PriceMax: &max}, Listing{PriceAmount: &price, Currency: "USD"})
	if decision.Status != MatchRejected || decision.Reasons[0].Message != "currency: USD != UAH" {
		t.Fatalf("decision %#v", decision)
	}
}

func TestApplyWithoutFiltersMatches(t *testing.T) {
	decision := Apply(Filters{}, Listing{})
	if decision.Status != MatchMatched || len(decision.Reasons) != 0 {
		t.Fatalf("decision %#v", decision)
	}
}
