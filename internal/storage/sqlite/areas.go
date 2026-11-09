package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/eggs-gd/place-space/internal/domain"
)

// SourceArea is an area together with the source that publishes it.
type SourceArea struct {
	Source string
	domain.Area
}

// AreasScannedAt reports when a source's areas for a city were last read.
func (s *Store) AreasScannedAt(ctx context.Context, sourceType, cityKey string) (time.Time, bool, error) {
	var at string
	err := s.db.QueryRowContext(ctx, `SELECT scanned_at FROM area_scans WHERE source_type = ? AND city_key = ?`,
		sourceType, cityKey).Scan(&at)
	if errors.Is(err, sql.ErrNoRows) {
		return time.Time{}, false, nil
	}
	if err != nil {
		return time.Time{}, false, fmt.Errorf("load area scan: %w", err)
	}
	parsed, err := mustParseTime(at)
	if err != nil {
		return time.Time{}, false, err
	}
	return parsed, true, nil
}

// MergeAreas adds a read of a source's areas to what is known. Nothing is
// removed: a scan sees only the links on the pages it read, so an area missing
// from one scan may still exist and a watch may depend on it. An area that is
// really gone shows up as a redirect error on its page. The scan time is
// recorded even for an empty list, so small cities are not rescanned each time.
func (s *Store) MergeAreas(ctx context.Context, sourceType, cityKey string, areas []domain.Area, scannedAt time.Time) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	for _, area := range areas {
		if _, err := tx.ExecContext(ctx, `INSERT INTO areas (source_type, city_key, area_key, name, kind, path, parent_key)
			VALUES (?, ?, ?, ?, ?, ?, ?)
			ON CONFLICT (source_type, city_key, area_key) DO UPDATE SET
				name = excluded.name, kind = excluded.kind, path = excluded.path,
				parent_key = CASE WHEN excluded.parent_key != '' THEN excluded.parent_key ELSE parent_key END`,
			sourceType, cityKey, area.Key, area.Name, area.Kind, area.Path, area.Parent); err != nil {
			return fmt.Errorf("save area %s: %w", area.Key, err)
		}
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO area_scans (source_type, city_key, scanned_at) VALUES (?, ?, ?)
		ON CONFLICT (source_type, city_key) DO UPDATE SET scanned_at = excluded.scanned_at`,
		sourceType, cityKey, formatTime(scannedAt)); err != nil {
		return fmt.Errorf("save area scan: %w", err)
	}
	return tx.Commit()
}

// AreasForCity returns every source's areas for a city, LUN's names first.
func (s *Store) AreasForCity(ctx context.Context, cityKey string) ([]SourceArea, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT source_type, area_key, name, kind, path, parent_key FROM areas
		WHERE city_key = ? ORDER BY area_key, source_type DESC`, cityKey)
	if err != nil {
		return nil, fmt.Errorf("list areas: %w", err)
	}
	defer rows.Close()
	out := []SourceArea{}
	for rows.Next() {
		var area SourceArea
		if err := rows.Scan(&area.Source, &area.Key, &area.Name, &area.Kind, &area.Path, &area.Parent); err != nil {
			return nil, err
		}
		out = append(out, area)
	}
	return out, rows.Err()
}

// AreaPaths returns one source's catalog paths for the given area keys. Keys
// the source does not publish are left out.
func (s *Store) AreaPaths(ctx context.Context, sourceType, cityKey string, keys []string) ([]string, error) {
	if len(keys) == 0 {
		return nil, nil
	}
	args := []any{sourceType, cityKey}
	for _, key := range keys {
		args = append(args, key)
	}
	marks := strings.TrimSuffix(strings.Repeat("?,", len(keys)), ",")
	rows, err := s.db.QueryContext(ctx, `SELECT path FROM areas
		WHERE source_type = ? AND city_key = ? AND area_key IN (`+marks+`) ORDER BY area_key`, args...)
	if err != nil {
		return nil, fmt.Errorf("load area paths: %w", err)
	}
	defer rows.Close()
	var paths []string
	for rows.Next() {
		var path string
		if err := rows.Scan(&path); err != nil {
			return nil, err
		}
		paths = append(paths, path)
	}
	return paths, rows.Err()
}
