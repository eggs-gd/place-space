package sqlite

import (
	"context"
	"fmt"
	"time"

	"github.com/eggs-gd/place-space/internal/domain"
)

// ExchangeRates returns the saved rates and when they were fetched.
func (s *Store) ExchangeRates(ctx context.Context) (domain.Rates, time.Time, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT currency, uah_rate, fetched_at FROM exchange_rates`)
	if err != nil {
		return nil, time.Time{}, fmt.Errorf("load rates: %w", err)
	}
	defer rows.Close()
	rates := domain.Rates{}
	var fetchedAt time.Time
	for rows.Next() {
		var (
			currency string
			rate     float64
			at       string
		)
		if err := rows.Scan(&currency, &rate, &at); err != nil {
			return nil, time.Time{}, err
		}
		parsed, err := mustParseTime(at)
		if err != nil {
			return nil, time.Time{}, err
		}
		rates[currency] = rate
		if fetchedAt.IsZero() || parsed.Before(fetchedAt) {
			fetchedAt = parsed
		}
	}
	return rates, fetchedAt, rows.Err()
}

// SaveExchangeRates replaces the saved value of each given currency.
func (s *Store) SaveExchangeRates(ctx context.Context, rates domain.Rates, fetchedAt time.Time) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	for currency, rate := range rates {
		if _, err := tx.ExecContext(ctx, `INSERT INTO exchange_rates (currency, uah_rate, fetched_at)
			VALUES (?, ?, ?)
			ON CONFLICT (currency) DO UPDATE SET uah_rate = excluded.uah_rate, fetched_at = excluded.fetched_at`,
			currency, rate, formatTime(fetchedAt),
		); err != nil {
			return fmt.Errorf("save rate %s: %w", currency, err)
		}
	}
	return tx.Commit()
}
