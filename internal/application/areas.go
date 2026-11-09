package application

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"sort"
	"sync"
	"time"

	"github.com/eggs-gd/place-space/internal/domain"
	"github.com/eggs-gd/place-space/internal/sources"
	"github.com/eggs-gd/place-space/internal/storage/sqlite"
)

const (
	areaScanMaxAge = 7 * 24 * time.Hour
	areaScanRetry  = time.Hour
	// MaxAreas caps a watch's areas: each one is its own set of catalog requests.
	MaxAreas = 5
)

// AreaOption is one choosable part of a city across all sources.
type AreaOption struct {
	Key     string   `json:"key"`
	Name    string   `json:"name"`
	Kind    string   `json:"kind"`
	Parent  string   `json:"parent"`
	Sources []string `json:"sources"`
}

// AreaError is a mistake in the areas a person asked for.
type AreaError struct{ Message string }

func (e *AreaError) Error() string { return e.Message }

// AreaDirectory reads which areas each source publishes for a city and keeps
// them in the database. Adapters that cannot list areas are skipped.
type AreaDirectory struct {
	Store    *sqlite.Store
	Adapters map[string]sources.Adapter
	Now      func() time.Time
	Log      *slog.Logger

	mu sync.Mutex
}

// List returns the areas of a city, merged by name across sources. It fails
// only when nothing is known and a source could not be read.
func (d *AreaDirectory) List(ctx context.Context, city string) ([]AreaOption, error) {
	refreshErr := d.ensure(ctx, city)
	rows, err := d.Store.AreasForCity(ctx, domain.AreaKey(city))
	if err != nil {
		return nil, err
	}
	if len(rows) == 0 && refreshErr != nil {
		return nil, refreshErr
	}
	merged := map[string]*AreaOption{}
	options := []*AreaOption{}
	for _, row := range rows {
		option, ok := merged[row.Key]
		if !ok {
			option = &AreaOption{Key: row.Key, Name: row.Name, Kind: row.Kind}
			merged[row.Key] = option
			options = append(options, option)
		}
		// Only LUN labels districts, so its answer wins over a plain area.
		if row.Kind == domain.AreaDistrict {
			option.Kind = domain.AreaDistrict
		}
		// Rows come with LUN first, and its cards name the district directly.
		if option.Parent == "" {
			option.Parent = row.Parent
		}
		option.Sources = append(option.Sources, row.Source)
	}
	out := make([]AreaOption, 0, len(options))
	for _, option := range options {
		sort.Strings(option.Sources)
		if parent, ok := merged[option.Parent]; !ok || parent.Kind != domain.AreaDistrict || option.Kind == domain.AreaDistrict {
			option.Parent = ""
		}
		out = append(out, *option)
	}
	return out, nil
}

// Resolve turns the requested names or keys into references, rejecting any
// the city does not have.
func (d *AreaDirectory) Resolve(ctx context.Context, city string, requested []string) ([]domain.AreaRef, error) {
	if len(requested) == 0 {
		return nil, nil
	}
	options, err := d.List(ctx, city)
	if err != nil {
		return nil, err
	}
	known := make(map[string]string, len(options))
	for _, option := range options {
		known[option.Key] = option.Name
	}
	seen := map[string]struct{}{}
	refs := make([]domain.AreaRef, 0, len(requested))
	for _, raw := range requested {
		key := domain.AreaKey(raw)
		if _, dup := seen[key]; dup {
			continue
		}
		seen[key] = struct{}{}
		name, ok := known[key]
		if !ok {
			return nil, &AreaError{Message: fmt.Sprintf("район «%s» не знайдено в цьому місті", raw)}
		}
		refs = append(refs, domain.AreaRef{Key: key, Name: name})
	}
	if len(refs) > MaxAreas {
		return nil, &AreaError{Message: fmt.Sprintf("оберіть не більше %d районів", MaxAreas)}
	}
	return refs, nil
}

// Paths returns one source's catalog pages for the chosen areas. An empty
// result means the source publishes none of them.
func (d *AreaDirectory) Paths(ctx context.Context, sourceType, city string, areas []domain.AreaRef) ([]string, error) {
	if err := d.ensure(ctx, city); err != nil {
		d.log().Warn("refresh areas", "city", city, "err", err)
	}
	keys := make([]string, 0, len(areas))
	for _, area := range areas {
		keys = append(keys, area.Key)
	}
	return d.Store.AreaPaths(ctx, sourceType, domain.AreaKey(city), keys)
}

// ensure rereads sources whose areas for the city are missing or old. A failed
// read keeps what is saved and is retried on the next call.
func (d *AreaDirectory) ensure(ctx context.Context, city string) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	cityKey := domain.AreaKey(city)
	now := d.now()
	var errs []error
	for sourceType, adapter := range d.Adapters {
		reader, ok := adapter.(sources.AreaReader)
		if !ok {
			continue
		}
		scanned, found, err := d.Store.AreasScannedAt(ctx, sourceType, cityKey)
		if err != nil {
			errs = append(errs, err)
			continue
		}
		if found && now.Sub(scanned) < areaScanMaxAge {
			continue
		}
		areas, err := reader.Areas(ctx, city)
		scannedAt := now
		switch {
		case errors.Is(err, sources.ErrIncompleteAreas):
			// Keep what was found, but look again soon rather than in a week.
			scannedAt = now.Add(areaScanRetry - areaScanMaxAge)
			d.log().Warn("areas incomplete", "source", sourceType, "city", city, "err", err)
		case err != nil:
			errs = append(errs, err)
			continue
		}
		if err := d.Store.MergeAreas(ctx, sourceType, cityKey, areas, scannedAt); err != nil {
			errs = append(errs, err)
			continue
		}
		d.log().Info("areas read", "source", sourceType, "city", city, "count", len(areas))
	}
	return errors.Join(errs...)
}

func (d *AreaDirectory) now() time.Time {
	if d.Now != nil {
		return d.Now()
	}
	return time.Now().UTC()
}

func (d *AreaDirectory) log() *slog.Logger {
	if d.Log != nil {
		return d.Log
	}
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}
