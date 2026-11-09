package domain

import (
	"fmt"
	"math"
	"strconv"
	"strings"
)

// Rates is the hryvnia value of one unit of each currency, keyed by ISO code.
type Rates map[string]float64

// Apply evaluates every predicate. A listing is rejected when any predicate
// fails. Unknown facts fail a predicate that needs them; they are not treated
// as a match. Prices in another currency are compared through rates; the
// listing keeps the price as published.
func Apply(filters Filters, listing Listing, rates Rates) Decision {
	reasons := make([]Reason, 0, 4)
	reasons = append(reasons, priceReasons(filters, listing, rates)...)
	if filters.RoomsMin != nil {
		reasons = append(reasons, compareInt("rooms", listing.Rooms, *filters.RoomsMin, ">="))
	}
	if filters.AreaMin != nil {
		reasons = append(reasons, compareFloat("area", listing.Area, *filters.AreaMin, ">="))
	}
	status := MatchMatched
	for _, reason := range reasons {
		if !reason.Passed {
			status = MatchRejected
			break
		}
	}
	return Decision{Status: status, Reasons: reasons}
}

// Decision is the explainable result of Apply.
type Decision struct {
	Status  string
	Reasons []Reason
}

func priceReasons(filters Filters, listing Listing, rates Rates) []Reason {
	bounded := filters.PriceMin != nil || filters.PriceMax != nil
	if !bounded && filters.Currency == "" {
		return nil
	}
	currency := filters.Currency
	if currency == "" && bounded {
		currency = "UAH"
	}
	if listing.PriceAmount == nil {
		return []Reason{{
			Field:   "price",
			Passed:  false,
			Message: "price: unknown",
		}}
	}
	price, ok := rates.convert(*listing.PriceAmount, listing.Currency, currency)
	if !ok {
		got := listing.Currency
		if got == "" {
			got = "unknown"
		}
		return []Reason{{
			Field:   "currency",
			Passed:  false,
			Message: fmt.Sprintf("currency: %s != %s", got, strings.ToUpper(currency)),
		}}
	}
	var reasons []Reason
	if filters.PriceMin != nil {
		reasons = append(reasons, compareInt64("price", &price, *filters.PriceMin, ">="))
	}
	if filters.PriceMax != nil {
		reasons = append(reasons, compareInt64("price", &price, *filters.PriceMax, "<="))
	}
	return reasons
}

// convert expresses amount in another currency. It reports false when the
// source currency is unknown or a rate is missing.
func (r Rates) convert(amount int64, from, to string) (int64, bool) {
	from, to = strings.ToUpper(from), strings.ToUpper(to)
	if from == "" || to == "" {
		return 0, false
	}
	if from == to {
		return amount, true
	}
	fromRate, ok := r.hryvnias(from)
	if !ok {
		return 0, false
	}
	toRate, ok := r.hryvnias(to)
	if !ok {
		return 0, false
	}
	return int64(math.Round(float64(amount) * fromRate / toRate)), true
}

func (r Rates) hryvnias(currency string) (float64, bool) {
	if currency == "UAH" {
		return 1, true
	}
	rate, ok := r[currency]
	return rate, ok && rate > 0
}

func compareInt64(field string, value *int64, limit int64, op string) Reason {
	if value == nil {
		return Reason{Field: field, Passed: false, Message: field + ": unknown"}
	}
	ok := false
	switch op {
	case ">=":
		ok = *value >= limit
	case "<=":
		ok = *value <= limit
	}
	symbol := map[bool]string{true: op, false: opposite(op)}[ok]
	return Reason{
		Field:   field,
		Passed:  ok,
		Message: fmt.Sprintf("%s: %d %s %d", field, *value, symbol, limit),
	}
}

func compareInt(field string, value *int, limit int, op string) Reason {
	if value == nil {
		return Reason{Field: field, Passed: false, Message: field + ": unknown"}
	}
	v := int64(*value)
	return compareInt64(field, &v, int64(limit), op)
}

func compareFloat(field string, value *float64, limit float64, op string) Reason {
	if value == nil {
		return Reason{Field: field, Passed: false, Message: field + ": unknown"}
	}
	ok := false
	switch op {
	case ">=":
		ok = *value >= limit
	case "<=":
		ok = *value <= limit
	}
	symbol := map[bool]string{true: op, false: opposite(op)}[ok]
	return Reason{
		Field:   field,
		Passed:  ok,
		Message: fmt.Sprintf("%s: %s %s %s", field, formatFloat(*value), symbol, formatFloat(limit)),
	}
}

func opposite(op string) string {
	switch op {
	case ">=":
		return "<"
	case "<=":
		return ">"
	case ">":
		return "<="
	case "<":
		return ">="
	default:
		return op
	}
}

func formatFloat(v float64) string {
	return strconv.FormatFloat(v, 'f', -1, 64)
}
