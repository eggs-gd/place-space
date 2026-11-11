package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
)

// Setting returns a stored setting, or "" when it was never set.
func (s *Store) Setting(ctx context.Context, key string) (string, error) {
	var value string
	err := s.db.QueryRowContext(ctx, `SELECT value FROM settings WHERE key = ?`, key).Scan(&value)
	if errors.Is(err, sql.ErrNoRows) {
		return "", nil
	}
	if err != nil {
		return "", fmt.Errorf("read setting %s: %w", key, err)
	}
	return value, nil
}

// SetSetting stores a setting. An empty value removes it.
func (s *Store) SetSetting(ctx context.Context, key, value string) error {
	var err error
	if value == "" {
		_, err = s.db.ExecContext(ctx, `DELETE FROM settings WHERE key = ?`, key)
	} else {
		_, err = s.db.ExecContext(ctx, `INSERT INTO settings (key, value) VALUES (?, ?)
			ON CONFLICT(key) DO UPDATE SET value = excluded.value`, key, value)
	}
	if err != nil {
		return fmt.Errorf("write setting %s: %w", key, err)
	}
	return nil
}
