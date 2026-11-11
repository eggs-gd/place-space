package otodim

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/eggs-gd/place-space/internal/domain"
	"github.com/eggs-gd/place-space/internal/sources"
)

type card struct {
	ID         int64  `json:"id"`
	Title      string `json:"title"`
	Slug       string `json:"slug"`
	HidePrice  bool   `json:"hidePrice"`
	TotalPrice *struct {
		Value    float64 `json:"value"`
		Currency string  `json:"currency"`
	} `json:"totalPrice"`
	Area      *float64 `json:"areaInSquareMeters"`
	Rooms     string   `json:"roomsNumber"`
	Floor     string   `json:"floorNumber"`
	Created   string   `json:"dateCreated"`
	FirstSeen string   `json:"createdAtFirst"`
	Images    []struct {
		Large string `json:"large"`
	} `json:"images"`
	Location struct {
		ReverseGeocoding struct {
			Locations []struct {
				Level string `json:"locationLevel"`
				Name  string `json:"name"`
			} `json:"locations"`
		} `json:"reverseGeocoding"`
	} `json:"location"`
}

// ordinals maps the site's words to numbers. Anything else, such as
// ABOVE_TENTH or MORE, stays unknown rather than becoming a wrong number.
var (
	roomCounts = map[string]int{
		"ONE": 1, "TWO": 2, "THREE": 3, "FOUR": 4, "FIVE": 5,
		"SIX": 6, "SEVEN": 7, "EIGHT": 8, "NINE": 9, "TEN": 10,
	}
	floorNumbers = map[string]int{
		"FIRST": 1, "SECOND": 2, "THIRD": 3, "FOURTH": 4, "FIFTH": 5,
		"SIXTH": 6, "SEVENTH": 7, "EIGHTH": 8, "NINTH": 9, "TENTH": 10,
	}
)

func decodeCard(raw json.RawMessage) (card, error) {
	var out card
	if err := json.Unmarshal(raw, &out); err != nil {
		return card{}, fmt.Errorf("otodim: decode listing: %w", err)
	}
	return out, nil
}

// Normalize maps one Otodim card onto the canonical listing. The catalog
// names the city but not the street, so the address stays empty.
func (a *Adapter) Normalize(_ context.Context, raw sources.RawListing) (domain.Listing, error) {
	card, err := decodeCard(raw.Payload)
	if err != nil {
		return domain.Listing{}, err
	}
	if card.ID <= 0 {
		return domain.Listing{}, fmt.Errorf("otodim: listing id is missing")
	}
	slug := strings.TrimSpace(card.Slug)
	if slug == "" {
		return domain.Listing{}, fmt.Errorf("otodim: listing %d: slug is missing", card.ID)
	}
	canonical, err := domain.CanonicalURL(defaultBaseURL + "/uk/oholoshennia/" + slug)
	if err != nil {
		return domain.Listing{}, fmt.Errorf("otodim: listing %d: %w", card.ID, err)
	}
	listing := domain.Listing{
		Source:      a.Type(),
		ExternalID:  strconv.FormatInt(card.ID, 10),
		URL:         canonical,
		Title:       strings.TrimSpace(card.Title),
		Location:    domain.Location{Name: cityName(card)},
		Rooms:       lookup(roomCounts, card.Rooms),
		Floor:       lookup(floorNumbers, card.Floor),
		PublishedAt: published(card),
		RawData:     append(json.RawMessage(nil), raw.Payload...),
	}
	if card.TotalPrice != nil && !card.HidePrice && card.TotalPrice.Value > 0 {
		price := int64(card.TotalPrice.Value)
		listing.PriceAmount = &price
		listing.Currency = strings.ToUpper(strings.TrimSpace(card.TotalPrice.Currency))
	}
	if card.Area != nil && *card.Area > 0 {
		area := *card.Area
		listing.Area = &area
	}
	for _, image := range card.Images {
		if image.Large != "" {
			listing.Images = append(listing.Images, image.Large)
		}
	}
	return listing, nil
}

func cityName(c card) string {
	for _, location := range c.Location.ReverseGeocoding.Locations {
		if location.Level == "city_village" {
			return location.Name
		}
	}
	return ""
}

func lookup(table map[string]int, key string) *int {
	value, ok := table[key]
	if !ok {
		return nil
	}
	return &value
}

// published is when the listing first appeared. dateCreated moves when the
// owner refreshes it, so it is only the fallback.
func published(c card) *time.Time {
	for _, raw := range []string{c.FirstSeen, c.Created} {
		parsed, err := time.Parse(time.RFC3339Nano, raw)
		if err == nil {
			utc := parsed.UTC()
			return &utc
		}
	}
	return nil
}
