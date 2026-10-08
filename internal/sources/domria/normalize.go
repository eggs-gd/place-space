package domria

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"strconv"
	"strings"
	"time"
	_ "time/tzdata"

	"github.com/eggs-gd/place-space/internal/domain"
	"github.com/eggs-gd/place-space/internal/sources"
)

type card struct {
	ID           int64    `json:"realty_id"`
	URL          string   `json:"beautiful_url"`
	Title        string   `json:"realtyTitle"`
	Description  string   `json:"description_uk"`
	PriceTotal   *float64 `json:"price_total"`
	Price        *float64 `json:"price"`
	Currency     string   `json:"currency_type"`
	Rooms        *float64 `json:"rooms_count"`
	Area         *float64 `json:"total_square_meters"`
	Floor        *float64 `json:"floor"`
	Floors       *float64 `json:"floors_count"`
	City         string   `json:"city_name_uk"`
	Street       string   `json:"street_name_uk"`
	Building     string   `json:"building_number_str"`
	ShowStreet   *int     `json:"is_show_street"`
	ShowBuilding *int     `json:"is_show_building_no"`
	Latitude     *float64 `json:"latitude"`
	Longitude    *float64 `json:"longitude"`
	Published    string   `json:"publishing_date"`
	MainPhoto    string   `json:"main_photo"`
	Photos       []struct {
		File string `json:"file"`
	} `json:"photos"`
}

// Normalize maps one DOM.RIA card onto the canonical listing.
// Missing numbers stay nil. Zero means the source did not know the value.
func (a *Adapter) Normalize(_ context.Context, raw sources.RawListing) (domain.Listing, error) {
	card, err := decodeCard(raw.Payload)
	if err != nil {
		return domain.Listing{}, err
	}
	if card.ID <= 0 {
		return domain.Listing{}, fmt.Errorf("domria: listing id is missing")
	}
	canonical, err := domain.CanonicalURL(listingURL(card.URL))
	if err != nil {
		return domain.Listing{}, fmt.Errorf("domria: listing %d: %w", card.ID, err)
	}
	published, err := parsePublished(card.Published)
	if err != nil {
		return domain.Listing{}, fmt.Errorf("domria: listing %d: %w", card.ID, err)
	}
	city, address := addressOf(card)
	title := strings.TrimSpace(card.Title)
	if title == "" {
		title = address
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
		Description: strings.TrimSpace(card.Description),
		PriceAmount: wholeNumber(firstPositive(card.PriceTotal, card.Price)),
		Currency:    currencyCode(card.Currency),
		Location: domain.Location{
			Name:    city,
			Address: address,
		},
		Rooms:       positiveInt(card.Rooms),
		Area:        positiveFloat(card.Area),
		Floor:       positiveInt(card.Floor),
		TotalFloors: positiveInt(card.Floors),
		Images:      imageURLs(card),
		PublishedAt: published,
		RawData:     append(json.RawMessage(nil), payload...),
	}
	if lat, lng, ok := coordinates(card.Latitude, card.Longitude); ok {
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
		return decoded, fmt.Errorf("domria: empty listing payload")
	}
	if err := json.Unmarshal(raw, &decoded); err != nil {
		return decoded, fmt.Errorf("domria: listing payload: %w", err)
	}
	return decoded, nil
}

func listingURL(raw string) string {
	raw = strings.TrimSpace(raw)
	switch {
	case raw == "":
		return ""
	case strings.HasPrefix(raw, "http://") || strings.HasPrefix(raw, "https://"):
		return raw
	default:
		return "https://dom.ria.com/uk/" + strings.TrimPrefix(raw, "/")
	}
}

func addressOf(card card) (city, address string) {
	city = strings.TrimSpace(card.City)
	if card.ShowStreet != nil && *card.ShowStreet == 0 {
		return city, ""
	}
	street := strings.TrimSpace(card.Street)
	number := ""
	if card.ShowBuilding == nil || *card.ShowBuilding != 0 {
		number = strings.TrimSpace(card.Building)
	}
	switch {
	case street != "" && number != "":
		address = street + ", " + number
	case street != "":
		address = street
	default:
		address = number
	}
	return city, address
}

func imageURLs(card card) []string {
	files := make([]string, 0, len(card.Photos)+1)
	for _, photo := range card.Photos {
		files = append(files, photo.File)
	}
	if len(files) == 0 {
		files = append(files, card.MainPhoto)
	}
	urls := make([]string, 0, len(files))
	seen := map[string]struct{}{}
	for _, file := range files {
		file = strings.TrimSpace(file)
		if file == "" {
			continue
		}
		var raw string
		if strings.HasPrefix(file, "http://") || strings.HasPrefix(file, "https://") {
			raw = file
		} else {
			raw = photoHost + strings.TrimPrefix(file, "/")
		}
		if _, ok := seen[raw]; ok {
			continue
		}
		seen[raw] = struct{}{}
		urls = append(urls, raw)
	}
	return urls
}

func coordinates(lat, lng *float64) (float64, float64, bool) {
	if lat == nil || lng == nil {
		return 0, 0, false
	}
	if math.IsNaN(*lat) || math.IsNaN(*lng) || math.IsInf(*lat, 0) || math.IsInf(*lng, 0) {
		return 0, 0, false
	}
	if *lat == 0 && *lng == 0 {
		return 0, 0, false
	}
	if *lat < -90 || *lat > 90 || *lng < -180 || *lng > 180 {
		return 0, 0, false
	}
	return *lat, *lng, true
}

func firstPositive(values ...*float64) *float64 {
	for _, value := range values {
		if _, ok := finitePositive(value); ok {
			return value
		}
	}
	return nil
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
	case "usd", "$":
		return "USD"
	case "eur", "€":
		return "EUR"
	default:
		return strings.ToUpper(strings.TrimSpace(raw))
	}
}

func parsePublished(raw string) (*time.Time, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil, nil
	}
	loc, err := time.LoadLocation("Europe/Kyiv")
	if err != nil {
		return nil, err
	}
	parsed, err := time.ParseInLocation("2006-01-02 15:04:05", raw, loc)
	if err != nil {
		return nil, fmt.Errorf("published time %q", raw)
	}
	utc := parsed.UTC()
	return &utc, nil
}
