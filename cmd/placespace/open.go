package main

import (
	"fmt"
	"io"
	"log/slog"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
)

func defaultDBPath() string {
	return dbPathForExe(executablePath())
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

func dbPathForExe(exe string) string {
	rel := filepath.Join("data", "place-space.db")
	if exe == "" {
		return rel
	}
	dir := filepath.Dir(exe)
	if strings.Contains(filepath.ToSlash(dir), "/go-build") {
		return rel
	}
	// Place Space.app/Contents/MacOS/place-space → data поруч із .app, не всередині бандла.
	if filepath.Base(dir) == "MacOS" && filepath.Base(filepath.Dir(dir)) == "Contents" {
		bundle := filepath.Dir(filepath.Dir(dir))
		return filepath.Join(filepath.Dir(bundle), "data", "place-space.db")
	}
	return filepath.Join(dir, rel)
}

func setupAppLog(dbPath string) error {
	dir := filepath.Dir(dbPath)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	file, err := os.OpenFile(filepath.Join(dir, "place-space.log"), os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
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
