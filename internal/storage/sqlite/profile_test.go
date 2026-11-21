package sqlite

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/eggs-gd/place-space/internal/domain"
)

func TestProfilesOwnWatches(t *testing.T) {
	ctx := context.Background()
	store := openTestStore(t)
	if err := store.Migrate(ctx); err != nil {
		t.Fatal(err)
	}
	orphan, err := store.CreateWatch(ctx, domain.Watch{Name: "Київ", Enabled: true, PollInterval: time.Minute})
	if err != nil {
		t.Fatal(err)
	}

	first, err := store.CreateProfile(ctx, " Оля ")
	if err != nil {
		t.Fatal(err)
	}
	if first.Name != "Оля" {
		t.Fatalf("name not trimmed: %q", first.Name)
	}
	adopted, err := store.GetWatch(ctx, orphan.ID)
	if err != nil {
		t.Fatal(err)
	}
	if adopted.ProfileID != first.ID {
		t.Fatalf("first profile did not take the existing watch: %q", adopted.ProfileID)
	}

	second, err := store.CreateProfile(ctx, "Саша")
	if err != nil {
		t.Fatal(err)
	}
	own, err := store.CreateWatch(ctx, domain.Watch{Name: "Київ", ProfileID: second.ID, Enabled: true, PollInterval: time.Minute})
	if err != nil {
		t.Fatalf("two profiles cannot both watch one city: %v", err)
	}
	if _, err := store.CreateWatch(ctx, domain.Watch{Name: "Київ", ProfileID: second.ID, Enabled: true, PollInterval: time.Minute}); err == nil {
		t.Fatal("one profile got two watches with one name")
	}
	loaded, err := store.GetWatch(ctx, own.ID)
	if err != nil {
		t.Fatal(err)
	}
	if loaded.ProfileID != second.ID {
		t.Fatalf("profile not stored: %q", loaded.ProfileID)
	}

	cli, err := store.CreateWatch(ctx, domain.Watch{Name: "Київ", Enabled: true, PollInterval: time.Minute})
	if err != nil {
		t.Fatal(err)
	}
	found, ok, err := store.FindWatchByName(ctx, "Київ")
	if err != nil || !ok || found.ID != cli.ID {
		t.Fatalf("find by name = %v %v %v, want the watch without a profile", found.ID, ok, err)
	}

	third, err := store.CreateProfile(ctx, "Інший")
	if err != nil {
		t.Fatal(err)
	}
	still, err := store.GetWatch(ctx, cli.ID)
	if err != nil {
		t.Fatal(err)
	}
	if still.ProfileID != "" {
		t.Fatalf("a later profile %s took a watch", third.ID)
	}

	if _, err := store.CreateProfile(ctx, "Оля"); !errors.Is(err, ErrProfileExists) {
		t.Fatalf("duplicate name: %v", err)
	}
	profiles, err := store.ListProfiles(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(profiles) != 3 {
		t.Fatalf("profiles = %d", len(profiles))
	}
	if _, err := store.GetProfile(ctx, "missing"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("missing profile: %v", err)
	}
}
