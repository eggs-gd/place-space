package sqlite

import (
	"context"
	"fmt"
)

const (
	// missedForMonthSQL holds when the first complete check that missed a
	// listing is at least 30 days older than the latest complete check, within
	// runScope. It is NULL when no complete check has missed the listing yet.
	missedForMonthSQL = `julianday((
		SELECT MIN(r.started_at) FROM poll_runs r
		WHERE %[1]s AND ` + completeRunSQL + ` AND r.started_at > l.last_seen_at
	)) <= julianday((
		SELECT MAX(r.started_at) FROM poll_runs r
		WHERE %[1]s AND ` + completeRunSQL + `
	)) - 30`

	sourceScopeSQL = `r.source_id = l.source`
	watchScopeSQL  = `r.source_id = l.source AND r.watch_id = m.watch_id`
)

// Prune deletes listings the source has failed to show across 30 days of
// complete checks, with their history, and check records of the same age.
//
// Sources are shared between watches, so a listing a watch still holds is
// judged by that watch's own checks: a search elsewhere, or a disabled watch
// whose clock has stopped, cannot erase it. A listing no watch holds is judged
// by the source as a whole. The span starts at the first check that missed the
// listing, so reopening the app after a long break erases nothing.
//
// A run that still anchors an observation stays, as does the newest complete
// run of each watch and source.
func (s *Store) Prune(ctx context.Context) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx, `DELETE FROM listings WHERE id IN (
		SELECT l.id FROM listings l
		WHERE COALESCE(`+fmt.Sprintf(missedForMonthSQL, sourceScopeSQL)+`, 0)
		AND NOT EXISTS (
			SELECT 1 FROM matches m
			WHERE m.listing_id = l.id
			AND NOT COALESCE(`+fmt.Sprintf(missedForMonthSQL, watchScopeSQL)+`, 0)
		)
	)`); err != nil {
		return fmt.Errorf("prune listings: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM poll_runs WHERE id IN (
		SELECT o.id FROM poll_runs o
		WHERE julianday(o.started_at) < julianday((
			SELECT MAX(r.started_at) FROM poll_runs r
			WHERE r.watch_id = o.watch_id AND r.source_id = o.source_id AND `+completeRunSQL+`
		)) - 30
		AND NOT EXISTS (SELECT 1 FROM listing_observations x WHERE x.run_id = o.id)
	)`); err != nil {
		return fmt.Errorf("prune runs: %w", err)
	}
	return tx.Commit()
}
