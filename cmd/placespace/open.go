package main

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/eggs-gd/place-space/internal/core"
)

const dbFlagHelp = "шлях до SQLite; типово каталог даних користувача"

// dbLocation is the database a command uses and, for a built program, the
// place older versions kept it.
type dbLocation struct {
	path   string
	legacy string
}

// locateDB picks the database. An explicit path wins. A program started by
// go run keeps data/ in the working directory, as during development. A built
// program uses the user data directory, or PLACE_SPACE_DATA when set.
func locateDB(explicit string) (dbLocation, error) {
	if explicit != "" {
		return dbLocation{path: explicit}, nil
	}
	exe := executablePath()
	_, custom := os.LookupEnv(core.DataDirEnv)
	if !custom && devBuild(exe) {
		return dbLocation{path: filepath.Join("data", core.DBName)}, nil
	}
	dir, err := core.DefaultDataDir()
	if err != nil {
		return dbLocation{}, err
	}
	return dbLocation{path: filepath.Join(dir, core.DBName), legacy: legacyDBPath(exe)}, nil
}

// adopt carries over a database from the place older versions kept it.
func (l dbLocation) adopt(ctx context.Context) error {
	copied, err := core.AdoptLegacy(ctx, l.legacy, l.path)
	if err != nil {
		return err
	}
	if copied {
		slog.Info("database copied from the old location", "from", l.legacy, "to", l.path)
	}
	return nil
}

func resolveDBPath(ctx context.Context, explicit string) (string, error) {
	location, err := locateDB(explicit)
	if err != nil {
		return "", err
	}
	return location.path, location.adopt(ctx)
}

func executablePath() string {
	exe, err := os.Executable()
	if err != nil {
		return ""
	}
	if resolved, err := filepath.EvalSymlinks(exe); err == nil {
		exe = resolved
	}
	return exe
}

func devBuild(exe string) bool {
	return exe == "" || strings.Contains(filepath.ToSlash(filepath.Dir(exe)), "/go-build")
}

// legacyDBPath is where versions up to 0.0.7 kept the database: in data/ next
// to the program, or next to the .app bundle on macOS.
func legacyDBPath(exe string) string {
	if devBuild(exe) {
		return ""
	}
	dir := filepath.Dir(exe)
	if filepath.Base(dir) == "MacOS" && filepath.Base(filepath.Dir(dir)) == "Contents" {
		bundle := filepath.Dir(filepath.Dir(dir))
		return filepath.Join(filepath.Dir(bundle), "data", core.DBName)
	}
	return filepath.Join(dir, "data", core.DBName)
}

func setupAppLog(dbPath string) error {
	dir := filepath.Dir(dbPath)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	file, err := os.OpenFile(filepath.Join(dir, core.LogName), os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	slog.SetDefault(slog.New(slog.NewTextHandler(io.MultiWriter(os.Stderr, file), nil)))
	return nil
}

func openExternal(raw string) error {
	parsed, err := url.Parse(raw)
	if err != nil {
		return err
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return fmt.Errorf("непідтримуване посилання")
	}
	return openBrowser(raw)
}

func openBrowser(rawURL string) error {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "windows":
		cmd = exec.Command("cmd", "/c", "start", "", rawURL)
	case "darwin":
		cmd = exec.Command("open", rawURL)
	default:
		cmd = exec.Command("xdg-open", rawURL)
	}
	return cmd.Start()
}
