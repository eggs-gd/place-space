package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/eggs-gd/place-space/internal/id"
)

// Profile is one person on a shared install.
type Profile struct {
	ID        string
	Name      string
	CreatedAt time.Time
}

// ErrProfileExists is returned when a profile of that name already exists.
var ErrProfileExists = errors.New("profile exists")

// CreateProfile adds a profile. The first profile takes over the watches
// that belong to no one, so an install that turns profiles on keeps its
// search.
func (s *Store) CreateProfile(ctx context.Context, name string) (Profile, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return Profile{}, fmt.Errorf("profile name is empty")
	}
	profile := Profile{ID: id.New(), Name: name, CreatedAt: time.Now().UTC()}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return Profile{}, err
	}
	defer tx.Rollback()
	var others int
	if err := tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM profiles`).Scan(&others); err != nil {
		return Profile{}, err
	}
	_, err = tx.ExecContext(ctx, `INSERT INTO profiles (id, name, created_at) VALUES (?, ?, ?)`,
		profile.ID, profile.Name, formatTime(profile.CreatedAt))
	if err != nil {
		if strings.Contains(err.Error(), "UNIQUE") {
			return Profile{}, ErrProfileExists
		}
		return Profile{}, fmt.Errorf("insert profile: %w", err)
	}
	if others == 0 {
		if _, err := tx.ExecContext(ctx, `UPDATE watches SET profile_id = ? WHERE profile_id IS NULL`, profile.ID); err != nil {
			return Profile{}, fmt.Errorf("adopt watches: %w", err)
		}
	}
	if err := tx.Commit(); err != nil {
		return Profile{}, err
	}
	return profile, nil
}

// GetProfile loads one profile by id.
func (s *Store) GetProfile(ctx context.Context, profileID string) (Profile, error) {
	var (
		profile Profile
		created string
	)
	err := s.db.QueryRowContext(ctx, `SELECT id, name, created_at FROM profiles WHERE id = ?`, profileID).
		Scan(&profile.ID, &profile.Name, &created)
	if errors.Is(err, sql.ErrNoRows) {
		return Profile{}, ErrNotFound
	}
	if err != nil {
		return Profile{}, err
	}
	profile.CreatedAt, err = mustParseTime(created)
	return profile, err
}

// ListProfiles returns every profile ordered by name.
func (s *Store) ListProfiles(ctx context.Context) ([]Profile, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT id, name, created_at FROM profiles ORDER BY name`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	profiles := []Profile{}
	for rows.Next() {
		var (
			profile Profile
			created string
		)
		if err := rows.Scan(&profile.ID, &profile.Name, &created); err != nil {
			return nil, err
		}
		if profile.CreatedAt, err = mustParseTime(created); err != nil {
			return nil, err
		}
		profiles = append(profiles, profile)
	}
	return profiles, rows.Err()
}

func nullString(value string) any {
	if value == "" {
		return nil
	}
	return value
}
