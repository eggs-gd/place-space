package lun

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"strconv"
	"strings"
	"time"

	"github.com/eggs-gd/place-space/internal/domain"
	"github.com/eggs-gd/place-space/internal/sources"
)

type card struct {
	ID         int64     `json:"id"`
	URLRaw     string    `json:"urlRaw"`
	Price      *float64  `json:"price"`
	Currency   string    `json:"currency"`
	RoomCount  *float64  `json:"roomCount"`
	AreaTotal  *float64  `json:"areaTotal"`
	Floor      *float64  `json:"floor"`
	FloorCount *float64  `json:"floorCount"`
	Location   []float64 `json:"location"`
	Text       string    `json:"text"`
	Header     string    `json:"header"`
	InsertTime string    `json:"insertTime"`
	Images     []struct {
		ImageID int64 `json:"imageId"`
	} `json:"images"`
	GeoEntities []struct {
		Type string `json:"type"`
		Name string `json:"name"`
	} `json:"geoEntities"`
}

// Normalize maps one LUN card onto the canonical listing.
// Missing numbers stay nil. Zero is treated as missing because LUN uses it
// that way for unknown price, rooms, area, and floor.
func (a *Adapter) Normalize(_ context.Context, raw sources.RawListing) (domain.Listing, error) {
	card, err := decodeCard(raw.Payload)
	if err != nil {
		return domain.Listing{}, err
	}
	if card.ID <= 0 {
		return domain.Listing{}, fmt.Errorf("lun: listing id is missing")
	}
	rawURL := strings.TrimSpace(card.URLRaw)
	if rawURL == "" {
		rawURL = "https://lun.ua/uk/realty/" + strconv.FormatInt(card.ID, 10)
	}
	canonical, err := domain.CanonicalURL(rawURL)
	if err != nil {
		return domain.Listing{}, fmt.Errorf("lun: listing %d: %w", card.ID, err)
	}
	city, address := addressOf(card)
	title := strings.TrimSpace(card.Header)
	if title == "" {
		title = address
	}
	published, err := parseTime(card.InsertTime)
	if err != nil {
		return domain.Listing{}, fmt.Errorf("lun: listing %d: %w", card.ID, err)
	}
	payload := raw.Payload
	if len(payload) == 0 {
		payload = []byte("{}")
	}
	listing := domain.Listing{
		Source:      a.Type(),
		ExternalID:  strconv.FormatInt(card.ID, 10),
		URL:         canonical,
		Title:       title,
		Description: strings.TrimSpace(card.Text),
		PriceAmount: wholeNumber(card.Price),
		Currency:    currencyCode(card.Currency),
		Location: domain.Location{
			Name:    city,
			Address: address,
		},
		Rooms:       positiveInt(card.RoomCount),
		Area:        positiveFloat(card.AreaTotal),
		Floor:       positiveInt(card.Floor),
		TotalFloors: positiveInt(card.FloorCount),
		Images:      imageURLs(card),
		PublishedAt: published,
		RawData:     append(json.RawMessage(nil), payload...),
	}
	if lat, lng, ok := coordinates(card.Location); ok {
		listing.Location.Latitude = &lat
		listing.Location.Longitude = &lng
	}
	if listing.Images == nil {
		listing.Images = []string{}
	}
	return listing, nil
}

func decodeCard(raw json.RawMessage) (card, error) {
	var decoded card
	if len(raw) == 0 {
		return decoded, fmt.Errorf("lun: empty listing payload")
	}
	if err := json.Unmarshal(raw, &decoded); err != nil {
		return decoded, fmt.Errorf("lun: listing payload: %w", err)
	}
	return decoded, nil
}

func addressOf(card card) (city, address string) {
	var street, house string
	for _, geo := range card.GeoEntities {
		switch geo.Type {
		case "city":
			if city == "" {
				city = strings.TrimSpace(geo.Name)
			}
		case "street":
			if street == "" {
				street = strings.TrimSpace(geo.Name)
			}
		case "house":
			if house == "" {
				house = strings.TrimSpace(geo.Name)
			}
		}
	}
	switch {
	case street != "" && house != "":
		address = street + ", " + house
	case street != "":
		address = street
	default:
		address = house
	}
	return city, address
}

func imageURLs(card card) []string {
	urls := make([]string, 0, len(card.Images))
	for _, image := range card.Images {
		if image.ImageID <= 0 {
			continue
		}
		urls = append(urls, fmt.Sprintf("https://market-images.lunstatic.net/lun-ua/1200/1200/images/%d.jpg", image.ImageID))
	}
	return urls
}

// coordinates reads LUN's [longitude, latitude] pair. Out-of-range values are
// dropped rather than swapped.
func coordinates(pair []float64) (lat, lng float64, ok bool) {
	if len(pair) != 2 {
		return 0, 0, false
	}
	lng, lat = pair[0], pair[1]
	if math.IsNaN(lat) || math.IsNaN(lng) || lat < -90 || lat > 90 || lng < -180 || lng > 180 {
		return 0, 0, false
	}
	return lat, lng, true
}

func wholeNumber(value *float64) *int64 {
	number, ok := finitePositive(value)
	if !ok || number != math.Trunc(number) {
		return nil
	}
	whole := int64(number)
	return &whole
}

func positiveInt(value *float64) *int {
	whole := wholeNumber(value)
	if whole == nil {
		return nil
	}
	converted := int(*whole)
	return &converted
}

func positiveFloat(value *float64) *float64 {
	number, ok := finitePositive(value)
	if !ok {
		return nil
	}
	return &number
}

func finitePositive(value *float64) (float64, bool) {
	if value == nil || math.IsNaN(*value) || math.IsInf(*value, 0) || *value <= 0 {
		return 0, false
	}
	return *value, true
}

func currencyCode(raw string) string {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "":
		return ""
	case "uah", "грн":
		return "UAH"
	case "usd":
		return "USD"
	case "eur":
		return "EUR"
	default:
		return strings.ToUpper(strings.TrimSpace(raw))
	}
}

func parseTime(raw string) (*time.Time, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil, nil
	}
	for _, layout := range []string{time.RFC3339Nano, time.RFC3339, "2006-01-02T15:04:05"} {
		parsed, err := time.Parse(layout, raw)
		if err == nil {
			utc := parsed.UTC()
			return &utc, nil
		}
	}
	return nil, fmt.Errorf("published time %q", raw)
}
