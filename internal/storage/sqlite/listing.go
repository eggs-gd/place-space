package sqlite

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/eggs-gd/place-space/internal/domain"
)

// ListingsByKeys loads listings for one source that match an external id or a canonical URL.
func (s *Store) ListingsByKeys(ctx context.Context, source string, externalIDs, urls []string) ([]domain.Listing, error) {
	if len(externalIDs) == 0 && len(urls) == 0 {
		return nil, nil
	}
	args := []any{source}
	var clauses []string
	if len(externalIDs) > 0 {
		clauses = append(clauses, "external_id IN ("+placeholders(len(externalIDs))+")")
		for _, externalID := range externalIDs {
			args = append(args, externalID)
		}
	}
	if len(urls) > 0 {
		clauses = append(clauses, "url IN ("+placeholders(len(urls))+")")
		for _, rawURL := range urls {
			args = append(args, rawURL)
		}
	}
	query := listingSelect + ` WHERE source = ? AND (` + strings.Join(clauses, " OR ") + `)`
	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("list listings: %w", err)
	}
	defer rows.Close()
	return scanListings(rows)
}

// MatchesForWatch loads the current explainable decisions for a watch.
func (s *Store) MatchesForWatch(ctx context.Context, watchID string) ([]domain.Match, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT watch_id, listing_id, status, reasons_json, first_matched_at, last_matched_at, updated_at
		FROM matches WHERE watch_id = ?`, watchID)
	if err != nil {
		return nil, fmt.Errorf("list matches: %w", err)
	}
	defer rows.Close()
	var matches []domain.Match
	for rows.Next() {
		match, err := scanMatch(rows)
		if err != nil {
			return nil, err
		}
		matches = append(matches, match)
	}
	return matches, rows.Err()
}

const listingColumns = `id, source, external_id, url, title, description, price_amount, currency,
	location_name, address, latitude, longitude, rooms, area, floor, total_floors, images_json,
	published_at, first_seen_at, last_seen_at, raw_data`

const listingSelect = `SELECT ` + listingColumns + ` FROM listings`

func scanListings(rows *sql.Rows) ([]domain.Listing, error) {
	var listings []domain.Listing
	for rows.Next() {
		listing, err := scanListing(rows)
		if err != nil {
			return nil, err
		}
		listings = append(listings, listing)
	}
	return listings, rows.Err()
}

func scanListing(row rowScanner) (domain.Listing, error) {
	var (
		listing     domain.Listing
		price       sql.NullInt64
		latitude    sql.NullFloat64
		longitude   sql.NullFloat64
		rooms       sql.NullInt64
		area        sql.NullFloat64
		floor       sql.NullInt64
		totalFloors sql.NullInt64
		images      string
		published   sql.NullString
		firstSeen   string
		lastSeen    string
		raw         string
	)
	if err := row.Scan(
		&listing.ID, &listing.Source, &listing.ExternalID, &listing.URL, &listing.Title, &listing.Description,
		&price, &listing.Currency, &listing.Location.Name, &listing.Location.Address, &latitude, &longitude,
		&rooms, &area, &floor, &totalFloors, &images, &published, &firstSeen, &lastSeen, &raw,
	); err != nil {
		return domain.Listing{}, err
	}
	if price.Valid {
		value := price.Int64
		listing.PriceAmount = &value
	}
	if latitude.Valid {
		value := latitude.Float64
		listing.Location.Latitude = &value
	}
	if longitude.Valid {
		value := longitude.Float64
		listing.Location.Longitude = &value
	}
	if rooms.Valid {
		value := int(rooms.Int64)
		listing.Rooms = &value
	}
	if area.Valid {
		value := area.Float64
		listing.Area = &value
	}
	if floor.Valid {
		value := int(floor.Int64)
		listing.Floor = &value
	}
	if totalFloors.Valid {
		value := int(totalFloors.Int64)
		listing.TotalFloors = &value
	}
	if err := json.Unmarshal([]byte(images), &listing.Images); err != nil {
		return domain.Listing{}, fmt.Errorf("decode images: %w", err)
	}
	if listing.Images == nil {
		listing.Images = []string{}
	}
	var err error
	listing.PublishedAt, err = parseTime(published)
	if err != nil {
		return domain.Listing{}, err
	}
	listing.FirstSeenAt, err = mustParseTime(firstSeen)
	if err != nil {
		return domain.Listing{}, err
	}
	listing.LastSeenAt, err = mustParseTime(lastSeen)
	if err != nil {
		return domain.Listing{}, err
	}
	listing.RawData = json.RawMessage(raw)
	return listing, nil
}

func scanMatch(row rowScanner) (domain.Match, error) {
	var (
		match   domain.Match
		reasons string
		first   sql.NullString
		last    sql.NullString
		updated string
	)
	if err := row.Scan(&match.WatchID, &match.ListingID, &match.Status, &reasons, &first, &last, &updated); err != nil {
		return domain.Match{}, err
	}
	if err := json.Unmarshal([]byte(reasons), &match.Reasons); err != nil {
		return domain.Match{}, fmt.Errorf("decode reasons: %w", err)
	}
	if match.Reasons == nil {
		match.Reasons = []domain.Reason{}
	}
	var err error
	match.FirstMatchedAt, err = parseTime(first)
	if err != nil {
		return domain.Match{}, err
	}
	match.LastMatchedAt, err = parseTime(last)
	if err != nil {
		return domain.Match{}, err
	}
	match.UpdatedAt, err = mustParseTime(updated)
	if err != nil {
		return domain.Match{}, err
	}
	return match, nil
}

func placeholders(n int) string {
	return strings.TrimRight(strings.Repeat("?,", n), ",")
}
