// Package rates supplies official hryvnia exchange rates for price filters.
package rates

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/eggs-gd/place-space/internal/domain"
	"github.com/eggs-gd/place-space/internal/storage/sqlite"
)

const (
	nbuURL  = "https://bank.gov.ua/NBUStatService/v1/statdirectory/exchange?json"
	maxAge  = 12 * time.Hour
	maxBody = 1 << 20
)

// Fetcher loads current rates from an external service.
type Fetcher interface {
	Fetch(context.Context) (domain.Rates, error)
}

// NBU reads the National Bank of Ukraine's daily rates.
type NBU struct {
	URL  string
	HTTP *http.Client
}

// Fetch loads every currency the bank publishes.
func (n *NBU) Fetch(ctx context.Context) (domain.Rates, error) {
	endpoint := n.URL
	if endpoint == "" {
		endpoint = nbuURL
	}
	client := n.HTTP
	if client == nil {
		client = &http.Client{Timeout: 15 * time.Second}
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, fmt.Errorf("nbu: request: %w", err)
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("nbu: get: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("nbu: status %d", resp.StatusCode)
	}
	var rows []struct {
		Code string  `json:"cc"`
		Rate float64 `json:"rate"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, maxBody)).Decode(&rows); err != nil {
		return nil, fmt.Errorf("nbu: decode: %w", err)
	}
	rates := make(domain.Rates, len(rows))
	for _, row := range rows {
		if row.Code != "" && row.Rate > 0 {
			rates[strings.ToUpper(row.Code)] = row.Rate
		}
	}
	if len(rates) == 0 {
		return nil, errors.New("nbu: no rates in response")
	}
	return rates, nil
}

// Cache keeps rates in the database so a bank outage does not hide listings.
type Cache struct {
	Store   *sqlite.Store
	Fetcher Fetcher
	Now     func() time.Time
	Log     *slog.Logger
}

// Current returns saved rates while they are recent, otherwise fresh ones.
// When the fetch fails it falls back to whatever is saved, which may be nothing.
func (c *Cache) Current(ctx context.Context) domain.Rates {
	log := c.Log
	if log == nil {
		log = slog.New(slog.DiscardHandler)
	}
	now := time.Now().UTC()
	if c.Now != nil {
		now = c.Now()
	}
	saved, fetchedAt, err := c.Store.ExchangeRates(ctx)
	if err != nil {
		log.Warn("load exchange rates", "err", err)
	}
	if len(saved) > 0 && now.Sub(fetchedAt) < maxAge {
		return saved
	}
	fresh, err := c.Fetcher.Fetch(ctx)
	if err != nil {
		log.Warn("fetch exchange rates", "err", err, "saved", len(saved))
		return saved
	}
	if err := c.Store.SaveExchangeRates(ctx, fresh, now); err != nil {
		log.Warn("save exchange rates", "err", err)
	}
	return fresh
}
