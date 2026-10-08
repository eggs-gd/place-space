package sqlite

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/eggs-gd/place-space/internal/domain"
)

// FeedRow is a listing plus the watch's explainable decision.
type FeedRow struct {
	Listing domain.Listing
	Match   domain.Match
}

// MatchCounts counts matched and rejected listings for a watch.
func (s *Store) MatchCounts(ctx context.Context, watchID string) (matched, rejected int, err error) {
	err = s.db.QueryRowContext(ctx, `SELECT
		COALESCE(SUM(CASE WHEN status = 'matched' THEN 1 ELSE 0 END), 0),
		COALESCE(SUM(CASE WHEN status = 'rejected' THEN 1 ELSE 0 END), 0)
		FROM matches WHERE watch_id = ?`, watchID).Scan(&matched, &rejected)
	if err != nil {
		return 0, 0, fmt.Errorf("count matches: %w", err)
	}
	return matched, rejected, nil
}

// Feed returns listings for a watch. status is matched, rejected, or all.
func (s *Store) Feed(ctx context.Context, watchID, status string) ([]FeedRow, error) {
	query := `SELECT ` + prefixedListingColumns("listings") + `,
		m.watch_id, m.listing_id, m.status, m.reasons_json, m.first_matched_at, m.last_matched_at, m.updated_at
		FROM listings
		JOIN matches m ON m.listing_id = listings.id
		WHERE m.watch_id = ?`
	args := []any{watchID}
	switch status {
	case domain.MatchMatched, domain.MatchRejected:
		query += ` AND m.status = ?`
		args = append(args, status)
	case "all":
	default:
		return nil, fmt.Errorf("unknown feed status %q", status)
	}
	query += ` ORDER BY COALESCE(listings.published_at, listings.first_seen_at) DESC, listings.id`
	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("feed: %w", err)
	}
	defer rows.Close()
	var items []FeedRow
	for rows.Next() {
		row, err := scanFeedRow(rows)
		if err != nil {
			return nil, err
		}
		items = append(items, row)
	}
	if items == nil {
		items = []FeedRow{}
	}
	return items, rows.Err()
}

// LatestRun returns the newest poll run for a watch.
func (s *Store) LatestRun(ctx context.Context, watchID string) (domain.Run, bool, error) {
	row := s.db.QueryRowContext(ctx, `SELECT id, watch_id, source_id, source_type, started_at, finished_at,
		received, new_count, changed, matched, rejected, duplicates, duplicate_links, failed, errors_json
		FROM poll_runs WHERE watch_id = ? ORDER BY started_at DESC LIMIT 1`, watchID)
	run, err := scanRun(row)
	if errors.Is(err, sql.ErrNoRows) {
		return domain.Run{}, false, nil
	}
	if err != nil {
		return domain.Run{}, false, fmt.Errorf("latest run: %w", err)
	}
	return run, true, nil
}

// GetListing loads one listing by its id.
func (s *Store) GetListing(ctx context.Context, listingID string) (domain.Listing, error) {
	row := s.db.QueryRowContext(ctx, listingSelect+` WHERE id = ?`, listingID)
	listing, err := scanListing(row)
	if errors.Is(err, sql.ErrNoRows) {
		return domain.Listing{}, ErrNotFound
	}
	if err != nil {
		return domain.Listing{}, fmt.Errorf("get listing: %w", err)
	}
	return listing, nil
}

// MatchForListing returns the decision for a listing, preferring the given watch.
func (s *Store) MatchForListing(ctx context.Context, listingID, watchID string) (domain.Match, bool, error) {
	query := `SELECT watch_id, listing_id, status, reasons_json, first_matched_at, last_matched_at, updated_at
		FROM matches WHERE listing_id = ?`
	args := []any{listingID}
	if watchID != "" {
		query += ` AND watch_id = ?`
		args = append(args, watchID)
	}
	query += ` LIMIT 1`
	match, err := scanMatch(s.db.QueryRowContext(ctx, query, args...))
	if errors.Is(err, sql.ErrNoRows) {
		return domain.Match{}, false, nil
	}
	if err != nil {
		return domain.Match{}, false, err
	}
	return match, true, nil
}

// Observations returns snapshots from oldest to newest.
func (s *Store) Observations(ctx context.Context, listingID string) ([]domain.Observation, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT id, listing_id, run_id, seen_at, price_amount, currency, title
		FROM listing_observations WHERE listing_id = ? ORDER BY seen_at, id`, listingID)
	if err != nil {
		return nil, fmt.Errorf("observations: %w", err)
	}
	defer rows.Close()
	var observations []domain.Observation
	for rows.Next() {
		var (
			observation domain.Observation
			seen        string
			price       sql.NullInt64
		)
		if err := rows.Scan(&observation.ID, &observation.ListingID, &observation.RunID, &seen, &price, &observation.Currency, &observation.Title); err != nil {
			return nil, err
		}
		parsed, err := mustParseTime(seen)
		if err != nil {
			return nil, err
		}
		observation.SeenAt = parsed
		observation.Present = true
		if price.Valid {
			value := price.Int64
			observation.PriceAmount = &value
		}
		observations = append(observations, observation)
	}
	if observations == nil {
		observations = []domain.Observation{}
	}
	return observations, rows.Err()
}

// LinkedListings returns listings tied to this one by an exact duplicate link.
func (s *Store) LinkedListings(ctx context.Context, listingID string) ([]domain.Listing, error) {
	rows, err := s.db.QueryContext(ctx, listingSelect+` WHERE id IN (
		SELECT listing_b FROM duplicate_links WHERE listing_a = ?
		UNION
		SELECT listing_a FROM duplicate_links WHERE listing_b = ?
	) ORDER BY first_seen_at`, listingID, listingID)
	if err != nil {
		return nil, fmt.Errorf("linked listings: %w", err)
	}
	defer rows.Close()
	listings, err := scanListings(rows)
	if listings == nil {
		listings = []domain.Listing{}
	}
	return listings, err
}

func scanFeedRow(row rowScanner) (FeedRow, error) {
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
		match       domain.Match
		reasons     string
		first       sql.NullString
		last        sql.NullString
		updated     string
	)
	if err := row.Scan(
		&listing.ID, &listing.Source, &listing.ExternalID, &listing.URL, &listing.Title, &listing.Description,
		&price, &listing.Currency, &listing.Location.Name, &listing.Location.Address, &latitude, &longitude,
		&rooms, &area, &floor, &totalFloors, &images, &published, &firstSeen, &lastSeen, &raw,
		&match.WatchID, &match.ListingID, &match.Status, &reasons, &first, &last, &updated,
	); err != nil {
		return FeedRow{}, err
	}
	filled, err := fillListing(listing, price, latitude, longitude, rooms, area, floor, totalFloors, images, published, firstSeen, lastSeen, raw)
	if err != nil {
		return FeedRow{}, err
	}
	if err := json.Unmarshal([]byte(reasons), &match.Reasons); err != nil {
		return FeedRow{}, fmt.Errorf("decode reasons: %w", err)
	}
	if match.Reasons == nil {
		match.Reasons = []domain.Reason{}
	}
	match.FirstMatchedAt, err = parseTime(first)
	if err != nil {
		return FeedRow{}, err
	}
	match.LastMatchedAt, err = parseTime(last)
	if err != nil {
		return FeedRow{}, err
	}
	match.UpdatedAt, err = mustParseTime(updated)
	if err != nil {
		return FeedRow{}, err
	}
	filled.RawData = nil
	return FeedRow{Listing: filled, Match: match}, nil
}

func fillListing(
	listing domain.Listing,
	price sql.NullInt64,
	latitude, longitude sql.NullFloat64,
	rooms sql.NullInt64,
	area sql.NullFloat64,
	floor, totalFloors sql.NullInt64,
	images string,
	published sql.NullString,
	firstSeen, lastSeen, raw string,
) (domain.Listing, error) {
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

func prefixedListingColumns(table string) string {
	parts := strings.Split(listingColumns, ",")
	out := make([]string, 0, len(parts))
	for _, part := range parts {
		name := strings.TrimSpace(part)
		if name == "" {
			continue
		}
		out = append(out, table+"."+name)
	}
	return strings.Join(out, ", ")
}
