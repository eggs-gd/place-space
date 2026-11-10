package core

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/eggs-gd/place-space/internal/domain"
	"github.com/eggs-gd/place-space/internal/storage/sqlite"
)

func TestAppDirName(t *testing.T) {
	for goos, want := range map[string]string{"windows": "Place Space", "darwin": "Place Space", "linux": "place-space"} {
		if got := appDirName(goos); got != want {
			t.Fatalf("%s: got %q, want %q", goos, got, want)
		}
	}
}

func TestDefaultDataDirHonoursEnv(t *testing.T) {
	t.Setenv(DataDirEnv, "/somewhere")
	if got, err := DefaultDataDir(); err != nil || got != "/somewhere" {
		t.Fatalf("got %q %v", got, err)
	}
}

func TestAdoptLegacyCopiesUncheckpointedData(t *testing.T) {
	ctx := context.Background()
	legacy := filepath.Join(t.TempDir(), "data", DBName)
	old, err := sqlite.Open(legacy)
	if err != nil {
		t.Fatal(err)
	}
	if err := old.Migrate(ctx); err != nil {
		t.Fatal(err)
	}
	if _, err := old.EnsureSource(ctx, domain.Source{ID: "lun", Type: "lun", Enabled: true, Status: domain.SourceUnknown}); err != nil {
		t.Fatal(err)
	}
	if _, err := old.CreateWatch(ctx, domain.Watch{Name: "Київ", Enabled: true, SourceIDs: []string{"lun"}, Query: domain.Query{City: "Київ"}, PollInterval: time.Minute}); err != nil {
		t.Fatal(err)
	}
	// The old program is still open, so its last writes sit in the WAL.
	defer old.Close()

	target := filepath.Join(t.TempDir(), "Place Space", DBName)
	copied, err := AdoptLegacy(ctx, legacy, target)
	if err != nil || !copied {
		t.Fatalf("copied=%v err=%v", copied, err)
	}
	adopted, err := sqlite.Open(target)
	if err != nil {
		t.Fatal(err)
	}
	defer adopted.Close()
	watches, err := adopted.ListWatches(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(watches) != 1 || watches[0].Name != "Київ" {
		t.Fatalf("watches %+v", watches)
	}
	if _, err := os.Stat(legacy); err != nil {
		t.Fatalf("legacy database is gone: %v", err)
	}
	if _, err := os.Stat(target + ".partial"); !os.IsNotExist(err) {
		t.Fatalf("partial file left: %v", err)
	}
}

func TestAdoptLegacyKeepsExistingTarget(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	legacy := filepath.Join(dir, "old.db")
	target := filepath.Join(dir, "new.db")
	for _, path := range []string{legacy, target} {
		store, err := sqlite.Open(path)
		if err != nil {
			t.Fatal(err)
		}
		if err := store.Migrate(ctx); err != nil {
			t.Fatal(err)
		}
		store.Close()
	}
	before, _ := os.Stat(target)
	copied, err := AdoptLegacy(ctx, legacy, target)
	if err != nil || copied {
		t.Fatalf("copied=%v err=%v", copied, err)
	}
	after, _ := os.Stat(target)
	if !before.ModTime().Equal(after.ModTime()) {
		t.Fatal("existing database was replaced")
	}
}

func TestAdoptLegacyWithoutOldDatabase(t *testing.T) {
	dir := t.TempDir()
	copied, err := AdoptLegacy(context.Background(), filepath.Join(dir, "missing.db"), filepath.Join(dir, "new.db"))
	if err != nil || copied {
		t.Fatalf("copied=%v err=%v", copied, err)
	}
	if _, err := os.Stat(filepath.Join(dir, "new.db")); !os.IsNotExist(err) {
		t.Fatal("target created without anything to copy")
	}
}
