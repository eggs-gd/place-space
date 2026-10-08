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

// EnsureSource inserts the source when its id is new and otherwise returns the stored row.
func (s *Store) EnsureSource(ctx context.Context, source domain.Source) (domain.Source, error) {
	existing, err := s.GetSource(ctx, source.ID)
	if err == nil {
		return existing, nil
	}
	if !errors.Is(err, ErrNotFound) {
		return domain.Source{}, err
	}
	if source.ID == "" {
		source.ID = id.New()
	}
	if source.Type == "" {
		return domain.Source{}, fmt.Errorf("source type is empty")
	}
	if source.Status == "" {
		source.Status = domain.SourceUnknown
	}
	if len(source.Configuration) == 0 {
		source.Configuration = json.RawMessage(`{}`)
	}
	now := time.Now().UTC()
	source.CreatedAt = now
	source.UpdatedAt = now
	_, err = s.db.ExecContext(ctx, `INSERT INTO sources (
		id, type, enabled, configuration, status, last_success_at, last_error_at, last_error, created_at, updated_at
	) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		source.ID, source.Type, boolInt(source.Enabled), string(source.Configuration), source.Status,
		bindTime(source.LastSuccessAt), bindTime(source.LastErrorAt), source.LastError,
		formatTime(source.CreatedAt), formatTime(source.UpdatedAt),
	)
	if err != nil {
		return domain.Source{}, fmt.Errorf("insert source: %w", err)
	}
	return source, nil
}

// GetSource loads one source by id.
func (s *Store) GetSource(ctx context.Context, sourceID string) (domain.Source, error) {
	row := s.db.QueryRowContext(ctx, sourceSelect+` WHERE id = ?`, sourceID)
	source, err := scanSource(row)
	if errors.Is(err, sql.ErrNoRows) {
		return domain.Source{}, ErrNotFound
	}
	return source, err
}

// ListSources returns every source ordered by id.
func (s *Store) ListSources(ctx context.Context) ([]domain.Source, error) {
	rows, err := s.db.QueryContext(ctx, sourceSelect+` ORDER BY id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var sources []domain.Source
	for rows.Next() {
		source, err := scanSource(rows)
		if err != nil {
			return nil, err
		}
		sources = append(sources, source)
	}
	if sources == nil {
		sources = []domain.Source{}
	}
	return sources, rows.Err()
}

const sourceSelect = `SELECT id, type, enabled, configuration, status, last_success_at, last_error_at, last_error, created_at, updated_at FROM sources`

func scanSource(row rowScanner) (domain.Source, error) {
	var (
		source    domain.Source
		enabled   int
		config    string
		successAt sql.NullString
		errorAt   sql.NullString
		created   string
		updated   string
	)
	if err := row.Scan(
		&source.ID, &source.Type, &enabled, &config, &source.Status,
		&successAt, &errorAt, &source.LastError, &created, &updated,
	); err != nil {
		return domain.Source{}, err
	}
	source.Enabled = enabled == 1
	source.Configuration = json.RawMessage(config)
	var err error
	source.LastSuccessAt, err = parseTime(successAt)
	if err != nil {
		return domain.Source{}, err
	}
	source.LastErrorAt, err = parseTime(errorAt)
	if err != nil {
		return domain.Source{}, err
	}
	source.CreatedAt, err = mustParseTime(created)
	if err != nil {
		return domain.Source{}, err
	}
	source.UpdatedAt, err = mustParseTime(updated)
	if err != nil {
		return domain.Source{}, err
	}
	return source, nil
}
