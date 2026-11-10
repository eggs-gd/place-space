package core

import (
	"context"
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
	partial := target + ".partial"
	if err := os.Remove(partial); err != nil && !errors.Is(err, fs.ErrNotExist) {
		return false, err
	}
	old, err := sqlite.Open(legacy)
	if err != nil {
		return false, err
	}
	err = old.CopyTo(ctx, partial)
	if closeErr := old.Close(); err == nil {
		err = closeErr
	}
	if err != nil {
		_ = os.Remove(partial)
		return false, fmt.Errorf("copy %s: %w", legacy, err)
	}
	if err := os.Rename(partial, target); err != nil {
		_ = os.Remove(partial)
		return false, err
	}
	return true, nil
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
