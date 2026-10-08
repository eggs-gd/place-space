package main

import (
	"path/filepath"
	"testing"
)

func TestDBPathForExe(t *testing.T) {
	rel := filepath.Join("data", "place-space.db")
	if got := dbPathForExe(""); got != rel {
		t.Fatalf("empty exe: got %s", got)
	}
	temp := filepath.Join("/tmp", "go-build999", "place-space")
	if got := dbPathForExe(temp); got != rel {
		t.Fatalf("go run: got %s", got)
	}
	bundle := filepath.Join("/Applications", "Place Space.app", "Contents", "MacOS", "place-space")
	wantBundle := filepath.Join("/Applications", "data", "place-space.db")
	if got := dbPathForExe(bundle); got != wantBundle {
		t.Fatalf("app bundle: got %s, want %s", got, wantBundle)
	}
	exe := filepath.Join("/opt", "place-space", "place-space")
	wantExe := filepath.Join("/opt", "place-space", "data", "place-space.db")
	if got := dbPathForExe(exe); got != wantExe {
		t.Fatalf("exe: got %s, want %s", got, wantExe)
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
