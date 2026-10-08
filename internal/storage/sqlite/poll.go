package sqlite

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"

	"github.com/eggs-gd/place-space/internal/domain"
)

// ListingUpsert is one listing to insert or update inside a poll.
type ListingUpsert struct {
	Listing domain.Listing
	Insert  bool
}

// PollWrite is the durable result of one source check.
type PollWrite struct {
	Run          domain.Run
	Source       domain.Source
	Listings     []ListingUpsert
	Observations []domain.Observation
	Matches      []domain.Match
	Links        []domain.DuplicateLink
}

// SavePoll writes the run, listing snapshots, matches, and source status together.
func (s *Store) SavePoll(ctx context.Context, write PollWrite) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if err := updateSourceStatus(ctx, tx, write.Source); err != nil {
		return err
	}
	if err := insertRun(ctx, tx, write.Run); err != nil {
		return err
	}
	for _, listing := range write.Listings {
		if err := upsertListing(ctx, tx, listing); err != nil {
			return err
		}
	}
	for _, observation := range write.Observations {
		if err := insertObservation(ctx, tx, observation); err != nil {
			return err
		}
	}
	for _, match := range write.Matches {
		if err := upsertMatch(ctx, tx, match); err != nil {
			return err
		}
	}
	for _, link := range write.Links {
		if err := insertLink(ctx, tx, link); err != nil {
			return err
		}
	}
	return tx.Commit()
}

// CountObservations reports how many snapshots are stored.
// An empty listingID counts every observation.
func (s *Store) CountObservations(ctx context.Context, listingID string) (int, error) {
	query := `SELECT COUNT(*) FROM listing_observations`
	var args []any
	if listingID != "" {
		query += ` WHERE listing_id = ?`
		args = append(args, listingID)
	}
	var count int
	if err := s.db.QueryRowContext(ctx, query, args...).Scan(&count); err != nil {
		return 0, err
	}
	return count, nil
}

// RecentRuns returns the newest poll runs.
func (s *Store) RecentRuns(ctx context.Context, limit int) ([]domain.Run, error) {
	if limit <= 0 {
		limit = 20
	}
	rows, err := s.db.QueryContext(ctx, `SELECT id, watch_id, source_id, source_type, started_at, finished_at,
		received, new_count, changed, matched, rejected, duplicates, duplicate_links, failed, errors_json
		FROM poll_runs ORDER BY started_at DESC LIMIT ?`, limit)
	if err != nil {
		return nil, fmt.Errorf("list runs: %w", err)
	}
	defer rows.Close()
	var runs []domain.Run
	for rows.Next() {
		run, err := scanRun(rows)
		if err != nil {
			return nil, err
		}
		runs = append(runs, run)
	}
	if runs == nil {
		runs = []domain.Run{}
	}
	return runs, rows.Err()
}

func updateSourceStatus(ctx context.Context, tx *sql.Tx, source domain.Source) error {
	result, err := tx.ExecContext(ctx, `UPDATE sources SET
		status = ?, last_success_at = ?, last_error_at = ?, last_error = ?, updated_at = ?
		WHERE id = ?`,
		source.Status, bindTime(source.LastSuccessAt), bindTime(source.LastErrorAt), source.LastError,
		formatTime(source.UpdatedAt), source.ID,
	)
	if err != nil {
		return fmt.Errorf("update source status: %w", err)
	}
	n, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if n == 0 {
		return fmt.Errorf("update source status: %w", ErrNotFound)
	}
	return nil
}

func insertRun(ctx context.Context, tx *sql.Tx, run domain.Run) error {
	if run.Errors == nil {
		run.Errors = []string{}
	}
	errorsJSON, err := encodeJSON(run.Errors)
	if err != nil {
		return fmt.Errorf("encode run errors: %w", err)
	}
	_, err = tx.ExecContext(ctx, `INSERT INTO poll_runs (
		id, watch_id, source_id, source_type, started_at, finished_at, duration_ms,
		received, new_count, changed, matched, rejected, duplicates, duplicate_links, failed, errors_json
	) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		run.ID, run.WatchID, run.SourceID, run.SourceType,
		formatTime(run.StartedAt), formatTime(run.FinishedAt), run.Duration().Milliseconds(),
		run.Received, run.New, run.Changed, run.Matched, run.Rejected,
		run.Duplicates, run.DuplicateLinks, run.Failed, errorsJSON,
	)
	if err != nil {
		return fmt.Errorf("insert run: %w", err)
	}
	return nil
}

func upsertListing(ctx context.Context, tx *sql.Tx, write ListingUpsert) error {
	listing := write.Listing
	images, err := encodeJSON(nonNilStrings(listing.Images))
	if err != nil {
		return fmt.Errorf("encode images: %w", err)
	}
	raw := string(listing.RawData)
	if raw == "" {
		raw = "{}"
	}
	if write.Insert {
		_, err = tx.ExecContext(ctx, `INSERT INTO listings (
			id, source, external_id, url, title, description, price_amount, currency,
			location_name, address, latitude, longitude, rooms, area, floor, total_floors,
			images_json, published_at, first_seen_at, last_seen_at, raw_data
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			listing.ID, listing.Source, listing.ExternalID, listing.URL, listing.Title, listing.Description,
			bindInt64(listing.PriceAmount), listing.Currency, listing.Location.Name, listing.Location.Address,
			bindFloat(listing.Location.Latitude), bindFloat(listing.Location.Longitude),
			bindInt(listing.Rooms), bindFloat(listing.Area), bindInt(listing.Floor), bindInt(listing.TotalFloors),
			images, bindTime(listing.PublishedAt), formatTime(listing.FirstSeenAt), formatTime(listing.LastSeenAt), raw,
		)
	} else {
		_, err = tx.ExecContext(ctx, `UPDATE listings SET
			url = ?, title = ?, description = ?, price_amount = ?, currency = ?,
			location_name = ?, address = ?, latitude = ?, longitude = ?, rooms = ?, area = ?,
			floor = ?, total_floors = ?, images_json = ?, published_at = ?, last_seen_at = ?, raw_data = ?
			WHERE id = ?`,
			listing.URL, listing.Title, listing.Description, bindInt64(listing.PriceAmount), listing.Currency,
			listing.Location.Name, listing.Location.Address, bindFloat(listing.Location.Latitude), bindFloat(listing.Location.Longitude),
			bindInt(listing.Rooms), bindFloat(listing.Area), bindInt(listing.Floor), bindInt(listing.TotalFloors),
			images, bindTime(listing.PublishedAt), formatTime(listing.LastSeenAt), raw, listing.ID,
		)
	}
	if err != nil {
		return fmt.Errorf("save listing %s: %w", listing.ExternalID, err)
	}
	return nil
}

func insertObservation(ctx context.Context, tx *sql.Tx, observation domain.Observation) error {
	images, err := encodeJSON(nonNilStrings(observation.Images))
	if err != nil {
		return err
	}
	_, err = tx.ExecContext(ctx, `INSERT INTO listing_observations (
		id, listing_id, run_id, seen_at, price_amount, currency, title, description, images_json, present, content_hash
	) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		observation.ID, observation.ListingID, observation.RunID, formatTime(observation.SeenAt),
		bindInt64(observation.PriceAmount), observation.Currency, observation.Title, observation.Description,
		images, boolInt(observation.Present), observation.ContentHash,
	)
	if err != nil {
		return fmt.Errorf("insert observation: %w", err)
	}
	return nil
}

func upsertMatch(ctx context.Context, tx *sql.Tx, match domain.Match) error {
	if match.Reasons == nil {
		match.Reasons = []domain.Reason{}
	}
	reasons, err := encodeJSON(match.Reasons)
	if err != nil {
		return err
	}
	_, err = tx.ExecContext(ctx, `INSERT INTO matches (
		watch_id, listing_id, status, reasons_json, first_matched_at, last_matched_at, updated_at
	) VALUES (?, ?, ?, ?, ?, ?, ?)
	ON CONFLICT (watch_id, listing_id) DO UPDATE SET
		status = excluded.status,
		reasons_json = excluded.reasons_json,
		first_matched_at = excluded.first_matched_at,
		last_matched_at = excluded.last_matched_at,
		updated_at = excluded.updated_at`,
		match.WatchID, match.ListingID, match.Status, reasons,
		bindTime(match.FirstMatchedAt), bindTime(match.LastMatchedAt), formatTime(match.UpdatedAt),
	)
	if err != nil {
		return fmt.Errorf("save match: %w", err)
	}
	return nil
}

func insertLink(ctx context.Context, tx *sql.Tx, link domain.DuplicateLink) error {
	_, err := tx.ExecContext(ctx, `INSERT INTO duplicate_links (listing_a, listing_b, reason, created_at)
		VALUES (?, ?, ?, ?)
		ON CONFLICT DO NOTHING`,
		link.ListingA, link.ListingB, link.Reason, formatTime(link.CreatedAt),
	)
	if err != nil {
		return fmt.Errorf("save duplicate link: %w", err)
	}
	return nil
}

func scanRun(row rowScanner) (domain.Run, error) {
	var (
		run      domain.Run
		started  string
		finished string
		errors   string
	)
	if err := row.Scan(
		&run.ID, &run.WatchID, &run.SourceID, &run.SourceType, &started, &finished,
		&run.Received, &run.New, &run.Changed, &run.Matched, &run.Rejected,
		&run.Duplicates, &run.DuplicateLinks, &run.Failed, &errors,
	); err != nil {
		return domain.Run{}, err
	}
	var err error
	run.StartedAt, err = mustParseTime(started)
	if err != nil {
		return domain.Run{}, err
	}
	run.FinishedAt, err = mustParseTime(finished)
	if err != nil {
		return domain.Run{}, err
	}
	if err := decodeStrings(errors, &run.Errors); err != nil {
		return domain.Run{}, err
	}
	return run, nil
}

func decodeStrings(raw string, dest *[]string) error {
	if raw == "" {
		*dest = []string{}
		return nil
	}
	if err := json.Unmarshal([]byte(raw), dest); err != nil {
		return err
	}
	if *dest == nil {
		*dest = []string{}
	}
	return nil
}

func nonNilStrings(values []string) []string {
	if values == nil {
		return []string{}
	}
	return values
}
