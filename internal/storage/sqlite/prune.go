package sqlite

import (
	"context"
	"fmt"
)

// Age is measured from each source's latest productive check, so reopening
// the app after a long break does not erase what it had collected.
const latestBySourceCTE = `WITH latest AS (
	SELECT source_id, MAX(started_at) AS at FROM poll_runs
	WHERE received > 0 GROUP BY source_id
) `

// Prune deletes listings unseen for 30 days, with their history, and check
// records of the same age. A run that still anchors an observation stays, as
// does the newest run of each watch and source.
func (s *Store) Prune(ctx context.Context) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx, latestBySourceCTE+`DELETE FROM listings WHERE id IN (
		SELECT l.id FROM listings l
		JOIN latest ON latest.source_id = l.source
		WHERE julianday(l.last_seen_at) < julianday(latest.at) - 30
	)`); err != nil {
		return fmt.Errorf("prune listings: %w", err)
	}
	if _, err := tx.ExecContext(ctx, latestBySourceCTE+`DELETE FROM poll_runs WHERE id IN (
		SELECT r.id FROM poll_runs r
		JOIN latest ON latest.source_id = r.source_id
		WHERE julianday(r.started_at) < julianday(latest.at) - 30
		AND NOT EXISTS (SELECT 1 FROM listing_observations o WHERE o.run_id = r.id)
		AND r.started_at < (
			SELECT MAX(x.started_at) FROM poll_runs x
			WHERE x.watch_id = r.watch_id AND x.source_id = r.source_id
		)
	)`); err != nil {
		return fmt.Errorf("prune runs: %w", err)
	}
	return tx.Commit()
}
