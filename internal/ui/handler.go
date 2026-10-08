// Package ui serves the packed interface next to the API.
package ui

import (
	"io"
	"io/fs"
	"net/http"
	"path"
	"strings"
)

// Mount serves API calls itself and every other path from the interface.
func Mount(api http.Handler, assets fs.FS) http.Handler {
	pages := Handler(assets)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/api/") {
			api.ServeHTTP(w, r)
			return
		}
		pages.ServeHTTP(w, r)
	})
}

// Handler serves packed files. A path without a file extension falls back to
// the application shell so client-side routes work.
func Handler(assets fs.FS) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet && r.Method != http.MethodHead {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		name := strings.TrimPrefix(path.Clean(r.URL.Path), "/")
		if name == "" || name == "." {
			name = "index.html"
		}
		info, err := fs.Stat(assets, name)
		if err != nil || info.IsDir() {
			if path.Ext(name) != "" {
				http.NotFound(w, r)
				return
			}
			name = "index.html"
		}
		writeAsset(w, r, assets, name)
	})
}

func writeAsset(w http.ResponseWriter, r *http.Request, assets fs.FS, name string) {
	file, err := assets.Open(name)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	defer file.Close()
	seeker, ok := file.(io.ReadSeeker)
	if !ok {
		http.Error(w, "file", http.StatusInternalServerError)
		return
	}
	info, err := file.Stat()
	if err != nil {
		http.Error(w, "file", http.StatusInternalServerError)
		return
	}
	http.ServeContent(w, r, info.Name(), info.ModTime(), seeker)
}
