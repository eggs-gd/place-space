package domain

import (
	"crypto/sha256"
	"encoding/hex"
	"strconv"
	"strings"
)

// Change is one important difference between two snapshots of a listing.
type Change struct {
	Field string `json:"field"`
	From  string `json:"from"`
	To    string `json:"to"`
}

// Diff compares the fields a person would notice. Raw payload and seen-at
// timestamps are not changes: a source can rewrite them on every fetch.
func Diff(prev, next Listing) []Change {
	var changes []Change
	if priceText(prev) != priceText(next) {
		changes = append(changes, Change{Field: "price", From: priceText(prev), To: priceText(next)})
	}
	changes = append(changes, textChange("title", prev.Title, next.Title)...)
	changes = append(changes, textChange("description", prev.Description, next.Description)...)
	changes = append(changes, textChange("url", prev.URL, next.URL)...)
	changes = append(changes, textChange("address", prev.Location.Address, next.Location.Address)...)
	changes = append(changes, textChange("location", prev.Location.Name, next.Location.Name)...)
	if !sameStrings(prev.Images, next.Images) {
		changes = append(changes, Change{
			Field: "photos",
			From:  strconv.Itoa(len(prev.Images)),
			To:    strconv.Itoa(len(next.Images)),
		})
	}
	changes = append(changes, optionalIntChange("rooms", prev.Rooms, next.Rooms)...)
	changes = append(changes, optionalFloatChange("area", prev.Area, next.Area)...)
	changes = append(changes, optionalIntChange("floor", prev.Floor, next.Floor)...)
	changes = append(changes, optionalIntChange("totalFloors", prev.TotalFloors, next.TotalFloors)...)
	return changes
}

// Fingerprint is a stable hash of the fields Diff compares.
func Fingerprint(listing Listing) string {
	parts := []string{
		priceText(listing),
		listing.Title,
		listing.Description,
		listing.URL,
		listing.Location.Address,
		listing.Location.Name,
		strings.Join(listing.Images, "\n"),
		optionalIntText(listing.Rooms),
		optionalFloatText(listing.Area),
		optionalIntText(listing.Floor),
		optionalIntText(listing.TotalFloors),
	}
	sum := sha256.Sum256([]byte(strings.Join(parts, "\x00")))
	return hex.EncodeToString(sum[:])
}

func textChange(field, prev, next string) []Change {
	if prev == next {
		return nil
	}
	return []Change{{Field: field, From: prev, To: next}}
}

func optionalIntChange(field string, prev, next *int) []Change {
	if optionalIntText(prev) == optionalIntText(next) {
		return nil
	}
	return []Change{{Field: field, From: optionalIntText(prev), To: optionalIntText(next)}}
}

func optionalFloatChange(field string, prev, next *float64) []Change {
	if optionalFloatText(prev) == optionalFloatText(next) {
		return nil
	}
	return []Change{{Field: field, From: optionalFloatText(prev), To: optionalFloatText(next)}}
}

func priceText(listing Listing) string {
	if listing.PriceAmount == nil {
		return "unknown"
	}
	text := strconv.FormatInt(*listing.PriceAmount, 10)
	if listing.Currency == "" {
		return text
	}
	return text + " " + listing.Currency
}

func optionalIntText(value *int) string {
	if value == nil {
		return "unknown"
	}
	return strconv.Itoa(*value)
}

func optionalFloatText(value *float64) string {
	if value == nil {
		return "unknown"
	}
	return strconv.FormatFloat(*value, 'f', -1, 64)
}

func sameStrings(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
