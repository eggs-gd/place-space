package sqlite

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/eggs-gd/place-space/internal/domain"
	"github.com/eggs-gd/place-space/internal/id"
)

// CreateWatch inserts a watch and its source links.
func (s *Store) CreateWatch(ctx context.Context, watch domain.Watch) (domain.Watch, error) {
	if watch.Name == "" {
		return domain.Watch{}, fmt.Errorf("watch name is empty")
	}
	if watch.PollInterval <= 0 {
		return domain.Watch{}, fmt.Errorf("poll interval must be positive")
	}
	if watch.ID == "" {
		watch.ID = id.New()
	}
	now := time.Now().UTC()
	if watch.CreatedAt.IsZero() {
		watch.CreatedAt = now
	}
	watch.UpdatedAt = now
	if watch.SourceIDs == nil {
		watch.SourceIDs = []string{}
	}
	queryJSON, err := encodeJSON(watch.Query)
	if err != nil {
		return domain.Watch{}, fmt.Errorf("encode query: %w", err)
	}
	filtersJSON, err := encodeJSON(watch.Filters)
	if err != nil {
		return domain.Watch{}, fmt.Errorf("encode filters: %w", err)
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return domain.Watch{}, err
	}
	defer tx.Rollback()
	_, err = tx.ExecContext(ctx, `INSERT INTO watches (
		id, name, enabled, query_json, filters_json, poll_interval_seconds, created_at, updated_at
	) VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		watch.ID, watch.Name, boolInt(watch.Enabled), queryJSON, filtersJSON,
		intervalSeconds(watch.PollInterval), formatTime(watch.CreatedAt), formatTime(watch.UpdatedAt),
	)
	if err != nil {
		return domain.Watch{}, fmt.Errorf("insert watch: %w", err)
	}
	if err := replaceWatchSources(ctx, tx, watch.ID, watch.SourceIDs); err != nil {
		return domain.Watch{}, err
	}
	if err := tx.Commit(); err != nil {
		return domain.Watch{}, err
	}
	return watch, nil
}

// UpdateWatch replaces the mutable fields and the source links.
func (s *Store) UpdateWatch(ctx context.Context, watch domain.Watch) error {
	if watch.ID == "" {
		return fmt.Errorf("watch id is empty")
	}
	if watch.PollInterval <= 0 {
		return fmt.Errorf("poll interval must be positive")
	}
	watch.UpdatedAt = time.Now().UTC()
	queryJSON, err := encodeJSON(watch.Query)
	if err != nil {
		return fmt.Errorf("encode query: %w", err)
	}
	filtersJSON, err := encodeJSON(watch.Filters)
	if err != nil {
		return fmt.Errorf("encode filters: %w", err)
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	result, err := tx.ExecContext(ctx, `UPDATE watches SET
		name = ?, enabled = ?, query_json = ?, filters_json = ?, poll_interval_seconds = ?, updated_at = ?
		WHERE id = ?`,
		watch.Name, boolInt(watch.Enabled), queryJSON, filtersJSON,
		intervalSeconds(watch.PollInterval), formatTime(watch.UpdatedAt), watch.ID,
	)
	if err != nil {
		return fmt.Errorf("update watch: %w", err)
	}
	n, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if n == 0 {
		return ErrNotFound
	}
	if err := replaceWatchSources(ctx, tx, watch.ID, watch.SourceIDs); err != nil {
		return err
	}
	return tx.Commit()
}

// DeleteWatch removes the watch. Listings stay.
func (s *Store) DeleteWatch(ctx context.Context, watchID string) error {
	result, err := s.db.ExecContext(ctx, `DELETE FROM watches WHERE id = ?`, watchID)
	if err != nil {
		return fmt.Errorf("delete watch: %w", err)
	}
	n, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if n == 0 {
		return ErrNotFound
	}
	return nil
}

// GetWatch loads one watch by id.
func (s *Store) GetWatch(ctx context.Context, watchID string) (domain.Watch, error) {
	row := s.db.QueryRowContext(ctx, watchSelect+` WHERE w.id = ?`, watchID)
	watch, err := scanWatch(row)
	if errors.Is(err, sql.ErrNoRows) {
		return domain.Watch{}, ErrNotFound
	}
	if err != nil {
		return domain.Watch{}, err
	}
	sources, err := s.watchSources(ctx, watch.ID)
	if err != nil {
		return domain.Watch{}, err
	}
	watch.SourceIDs = sources
	return watch, nil
}

// FindWatchByName loads one watch by its unique name.
func (s *Store) FindWatchByName(ctx context.Context, name string) (domain.Watch, bool, error) {
	row := s.db.QueryRowContext(ctx, watchSelect+` WHERE w.name = ?`, name)
	watch, err := scanWatch(row)
	if errors.Is(err, sql.ErrNoRows) {
		return domain.Watch{}, false, nil
	}
	if err != nil {
		return domain.Watch{}, false, err
	}
	sources, err := s.watchSources(ctx, watch.ID)
	if err != nil {
		return domain.Watch{}, false, err
	}
	watch.SourceIDs = sources
	return watch, true, nil
}

// ListWatches returns every watch ordered by name.
func (s *Store) ListWatches(ctx context.Context) ([]domain.Watch, error) {
	rows, err := s.db.QueryContext(ctx, watchSelect+` ORDER BY w.name`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var watches []domain.Watch
	for rows.Next() {
		watch, err := scanWatch(rows)
		if err != nil {
			return nil, err
		}
		watches = append(watches, watch)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	for i := range watches {
		sources, err := s.watchSources(ctx, watches[i].ID)
		if err != nil {
			return nil, err
		}
		watches[i].SourceIDs = sources
	}
	if watches == nil {
		watches = []domain.Watch{}
	}
	return watches, nil
}

const watchSelect = `SELECT w.id, w.name, w.enabled, w.query_json, w.filters_json, w.poll_interval_seconds, w.created_at, w.updated_at FROM watches w`

type rowScanner interface {
	Scan(dest ...any) error
}

func scanWatch(row rowScanner) (domain.Watch, error) {
	var (
		watch   domain.Watch
		enabled int
		query   string
		filters string
		seconds int64
		created string
		updated string
	)
	if err := row.Scan(&watch.ID, &watch.Name, &enabled, &query, &filters, &seconds, &created, &updated); err != nil {
		return domain.Watch{}, err
	}
	watch.Enabled = enabled == 1
	if err := json.Unmarshal([]byte(query), &watch.Query); err != nil {
		return domain.Watch{}, fmt.Errorf("decode query: %w", err)
	}
	if err := json.Unmarshal([]byte(filters), &watch.Filters); err != nil {
		return domain.Watch{}, fmt.Errorf("decode filters: %w", err)
	}
	watch.PollInterval = time.Duration(seconds) * time.Second
	var err error
	watch.CreatedAt, err = mustParseTime(created)
	if err != nil {
		return domain.Watch{}, err
	}
	watch.UpdatedAt, err = mustParseTime(updated)
	if err != nil {
		return domain.Watch{}, err
	}
	return watch, nil
}

func (s *Store) watchSources(ctx context.Context, watchID string) ([]string, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT source_id FROM watch_sources WHERE watch_id = ? ORDER BY source_id`, watchID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	if ids == nil {
		ids = []string{}
	}
	return ids, rows.Err()
}

func replaceWatchSources(ctx context.Context, tx *sql.Tx, watchID string, sourceIDs []string) error {
	if _, err := tx.ExecContext(ctx, `DELETE FROM watch_sources WHERE watch_id = ?`, watchID); err != nil {
		return fmt.Errorf("clear watch sources: %w", err)
	}
	for _, sourceID := range sourceIDs {
		if _, err := tx.ExecContext(ctx, `INSERT INTO watch_sources (watch_id, source_id) VALUES (?, ?)`, watchID, sourceID); err != nil {
			return fmt.Errorf("link source %s: %w", sourceID, err)
		}
	}
	return nil
}

func intervalSeconds(d time.Duration) int64 {
	seconds := int64(d / time.Second)
	if d%time.Second != 0 {
		seconds++
	}
	if seconds < 1 {
		return 1
	}
	return seconds
}
