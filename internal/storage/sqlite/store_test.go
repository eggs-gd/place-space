package sqlite

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/eggs-gd/place-space/internal/domain"
)

func TestWatchCRUDAndMigration(t *testing.T) {
	ctx := context.Background()
	store := openTestStore(t)
	if err := store.Migrate(ctx); err != nil {
		t.Fatal(err)
	}

	source, err := store.EnsureSource(ctx, domain.Source{
		ID: "lun", Type: "lun", Enabled: true, Status: domain.SourceUnknown,
		Configuration: json.RawMessage(`{"cityPaths":{}}`),
	})
	if err != nil {
		t.Fatal(err)
	}
	again, err := store.EnsureSource(ctx, domain.Source{ID: "lun", Type: "other", Enabled: false})
	if err != nil {
		t.Fatal(err)
	}
	if again.Type != "lun" || !again.Enabled {
		t.Fatalf("ensure overwrote the source: %+v", again)
	}

	_, err = store.CreateWatch(ctx, domain.Watch{
		Name: "без джерела", Enabled: true, PollInterval: time.Minute,
		SourceIDs: []string{"missing"},
	})
	if err == nil {
		t.Fatal("expected a foreign key error")
	}

	created, err := store.CreateWatch(ctx, domain.Watch{
		Name: "Кам'янець", Enabled: true, SourceIDs: []string{source.ID},
		Query:        domain.Query{City: "Кам'янець-Подільський", Deal: domain.DealRent},
		Filters:      domain.Filters{},
		PollInterval: 10 * time.Minute,
	})
	if err != nil {
		t.Fatal(err)
	}
	loaded, ok, err := store.FindWatchByName(ctx, "Кам'янець")
	if err != nil || !ok {
		t.Fatalf("find %v %v", ok, err)
	}
	if loaded.ID != created.ID || loaded.Query.City != "Кам'янець-Подільський" || len(loaded.SourceIDs) != 1 {
		t.Fatalf("loaded %+v", loaded)
	}

	loaded.Name = "Кам'янець 2"
	loaded.Enabled = false
	if err := store.UpdateWatch(ctx, loaded); err != nil {
		t.Fatal(err)
	}
	updated, err := store.GetWatch(ctx, loaded.ID)
	if err != nil {
		t.Fatal(err)
	}
	if updated.Name != "Кам'янець 2" || updated.Enabled {
		t.Fatalf("updated %+v", updated)
	}
	if err := store.DeleteWatch(ctx, updated.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := store.GetWatch(ctx, updated.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("after delete: %v", err)
	}
}

func openTestStore(t *testing.T) *Store {
	t.Helper()
	store, err := Open(t.TempDir() + "/place-space.db")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	if err := store.Migrate(context.Background()); err != nil {
		t.Fatal(err)
	}
	return store
}
