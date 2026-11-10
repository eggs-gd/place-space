package core

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"

	"github.com/eggs-gd/place-space/internal/storage/sqlite"
)

// DBName is the database file inside a data directory.
const DBName = "place-space.db"

// LogName is the log file inside a data directory.
const LogName = "place-space.log"

// DataDirEnv overrides the data directory.
const DataDirEnv = "PLACE_SPACE_DATA"

// DefaultDataDir is where a desktop install keeps its data: %AppData% on
// Windows, Application Support on macOS, $XDG_CONFIG_HOME elsewhere.
func DefaultDataDir() (string, error) {
	if dir := os.Getenv(DataDirEnv); dir != "" {
		return dir, nil
	}
	base, err := os.UserConfigDir()
	if err != nil {
		return "", fmt.Errorf("find the user data directory: %w", err)
	}
	return filepath.Join(base, appDirName(runtime.GOOS)), nil
}

func appDirName(goos string) string {
	if goos == "windows" || goos == "darwin" {
		return "Place Space"
	}
	return "place-space"
}

// AdoptLegacy copies the database at legacy to target when target does not
// exist yet, so data from versions that kept it next to the program carries
// over. The copy is consistent even with an unmerged WAL, and legacy is left
// untouched. It reports whether a copy was made.
func AdoptLegacy(ctx context.Context, legacy, target string) (bool, error) {
	if legacy == "" || sameFile(legacy, target) {
		return false, nil
	}
	if exists, err := fileExists(target); err != nil || exists {
		return false, err
	}
	if exists, err := fileExists(legacy); err != nil || !exists {
		return false, err
	}
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		return false, fmt.Errorf("create data directory: %w", err)
	}
	// Each process copies into a file of its own, so two programs starting
	// at once never touch each other's copy.
	partial, err := partialPath(target)
	if err != nil {
		return false, err
	}
	defer os.Remove(partial)
	old, err := sqlite.Open(legacy)
	if err != nil {
		return false, err
	}
	err = old.CopyTo(ctx, partial)
	if closeErr := old.Close(); err == nil {
		err = closeErr
	}
	if err != nil {
		return false, fmt.Errorf("copy %s: %w", legacy, err)
	}
	return publish(partial, target)
}

// publish puts the finished copy in place unless a database appeared there in
// the meantime: another instance that got there first may already be writing
// to it. A hard link fails instead of replacing; rename would replace it.
func publish(partial, target string) (bool, error) {
	err := os.Link(partial, target)
	if err == nil {
		return true, nil
	}
	if errors.Is(err, fs.ErrExist) {
		return false, nil
	}
	// The file system has no hard links. Rename is the fallback, with only
	// the moment between the check and the rename left open.
	if exists, statErr := fileExists(target); statErr != nil || exists {
		return false, statErr
	}
	if err := os.Rename(partial, target); err != nil {
		return false, err
	}
	return true, nil
}

func partialPath(target string) (string, error) {
	var suffix [8]byte
	if _, err := rand.Read(suffix[:]); err != nil {
		return "", err
	}
	return fmt.Sprintf("%s.partial-%d-%s", target, os.Getpid(), hex.EncodeToString(suffix[:])), nil
}

func fileExists(path string) (bool, error) {
	_, err := os.Stat(path)
	if err == nil {
		return true, nil
	}
	if errors.Is(err, fs.ErrNotExist) {
		return false, nil
	}
	return false, err
}

func sameFile(a, b string) bool {
	absA, errA := filepath.Abs(a)
	absB, errB := filepath.Abs(b)
	return errA == nil && errB == nil && absA == absB
}
