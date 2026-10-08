package application

import (
	"context"

	"github.com/eggs-gd/place-space/internal/domain"
	"github.com/eggs-gd/place-space/internal/storage/sqlite"
)

// EnsureDefaultSources stores LUN and DOM.RIA and attaches any that a watch
// does not already have. Sources a watch already lists stay in their order.
func EnsureDefaultSources(ctx context.Context, store *sqlite.Store) ([]string, error) {
	ids := make([]string, 0, len(defaultSourceRows))
	for _, source := range defaultSourceRows {
		saved, err := store.EnsureSource(ctx, source)
		if err != nil {
			return nil, err
		}
		ids = append(ids, saved.ID)
	}
	watches, err := store.ListWatches(ctx)
	if err != nil {
		return nil, err
	}
	for _, watch := range watches {
		next, changed := mergeSourceIDs(watch.SourceIDs, ids)
		if !changed {
			continue
		}
		watch.SourceIDs = next
		if err := store.UpdateWatch(ctx, watch); err != nil {
			return nil, err
		}
	}
	return ids, nil
}

var defaultSourceRows = []domain.Source{
	{ID: "lun", Type: "lun", Enabled: true, Status: domain.SourceUnknown, Configuration: []byte(`{}`)},
	{ID: "domria", Type: "domria", Enabled: true, Status: domain.SourceUnknown, Configuration: []byte(`{}`)},
}

func mergeSourceIDs(have, extra []string) ([]string, bool) {
	seen := map[string]struct{}{}
	next := make([]string, 0, len(have)+len(extra))
	for _, id := range have {
		if id == "" {
			continue
		}
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		next = append(next, id)
	}
	changed := false
	for _, id := range extra {
		if id == "" {
			continue
		}
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		next = append(next, id)
		changed = true
	}
	return next, changed
}
