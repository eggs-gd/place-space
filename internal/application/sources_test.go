package application

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/eggs-gd/place-space/internal/domain"
	"github.com/eggs-gd/place-space/internal/storage/sqlite"
)

func TestEnsureDefaultSourcesKeepsExistingOrder(t *testing.T) {
	ctx := context.Background()
	store, err := sqlite.Open(filepath.Join(t.TempDir(), "place.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	if err := store.Migrate(ctx); err != nil {
		t.Fatal(err)
	}
	if _, err := store.EnsureSource(ctx, domain.Source{
		ID: "lun", Type: "lun", Enabled: true, Status: domain.SourceUnknown, Configuration: []byte(`{}`),
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := store.CreateWatch(ctx, domain.Watch{
		Name: "Кам'янець", Enabled: true, SourceIDs: []string{"lun"},
		Query:        domain.Query{City: "Кам'янець-Подільський", Deal: domain.DealRent},
		PollInterval: 10 * time.Minute,
	}); err != nil {
		t.Fatal(err)
	}
	ids, err := EnsureDefaultSources(ctx, store)
	if err != nil {
		t.Fatal(err)
	}
	if len(ids) != 2 || ids[0] != "lun" || ids[1] != "domria" {
		t.Fatalf("ids %v", ids)
	}
	watches, err := store.ListWatches(ctx)
	if err != nil {
		t.Fatal(err)
	}
	got := map[string]bool{}
	for _, id := range watches[0].SourceIDs {
		got[id] = true
	}
	if len(watches) != 1 || !got["lun"] || !got["domria"] {
		t.Fatalf("sources %v", watches[0].SourceIDs)
	}
	if watches[0].Query.City != "Кам'янець-Подільський" {
		t.Fatalf("city changed: %s", watches[0].Query.City)
	}
}
