package main

import (
	"path/filepath"
	"testing"

	"github.com/eggs-gd/place-space/internal/core"
)

func TestLegacyDBPath(t *testing.T) {
	if got := legacyDBPath(""); got != "" {
		t.Fatalf("empty exe: got %s", got)
	}
	temp := filepath.Join("/tmp", "go-build999", "place-space")
	if got := legacyDBPath(temp); got != "" {
		t.Fatalf("go run: got %s", got)
	}
	bundle := filepath.Join("/Applications", "Place Space.app", "Contents", "MacOS", "place-space")
	wantBundle := filepath.Join("/Applications", "data", "place-space.db")
	if got := legacyDBPath(bundle); got != wantBundle {
		t.Fatalf("app bundle: got %s, want %s", got, wantBundle)
	}
	exe := filepath.Join("/opt", "place-space", "place-space")
	wantExe := filepath.Join("/opt", "place-space", "data", "place-space.db")
	if got := legacyDBPath(exe); got != wantExe {
		t.Fatalf("exe: got %s, want %s", got, wantExe)
	}
}

func TestLocateDB(t *testing.T) {
	location, err := locateDB("custom.db")
	if err != nil || location.path != "custom.db" || location.legacy != "" {
		t.Fatalf("explicit: %+v %v", location, err)
	}
	dir := t.TempDir()
	t.Setenv(core.DataDirEnv, dir)
	location, err = locateDB("")
	if err != nil {
		t.Fatal(err)
	}
	if want := filepath.Join(dir, core.DBName); location.path != want {
		t.Fatalf("env: got %s, want %s", location.path, want)
	}
}

func TestOpenExternalRejectsOtherSchemes(t *testing.T) {
	if err := openExternal("file:///etc/passwd"); err == nil {
		t.Fatal("file url was accepted")
	}
	if err := openExternal("javascript:alert(1)"); err == nil {
		t.Fatal("javascript url was accepted")
	}
}
