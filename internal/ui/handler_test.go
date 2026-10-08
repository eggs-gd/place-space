package ui

import (
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"testing/fstest"
)

func TestMountServesAPIAndApp(t *testing.T) {
	assets := fstest.MapFS{
		"index.html": {Data: []byte("app-shell")},
		"_app/a.js":  {Data: []byte("asset")},
	}
	handler := Mount(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("api:" + r.URL.Path))
	}), assets)

	if got := get(t, handler, "/"); got != "app-shell" {
		t.Fatalf("shell %q", got)
	}
	if got := get(t, handler, "/listings/abc"); got != "app-shell" {
		t.Fatalf("route %q", got)
	}
	if got := get(t, handler, "/_app/a.js"); got != "asset" {
		t.Fatalf("asset %q", got)
	}
	if got := get(t, handler, "/api/overview"); got != "api:/api/overview" {
		t.Fatalf("api %q", got)
	}

	missing := httptest.NewRecorder()
	handler.ServeHTTP(missing, httptest.NewRequest(http.MethodGet, "/missing.js", nil))
	if missing.Code != http.StatusNotFound {
		t.Fatalf("missing status %d", missing.Code)
	}
}

func get(t *testing.T, handler http.Handler, target string) string {
	t.Helper()
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, target, nil))
	if response.Code != http.StatusOK {
		t.Fatalf("%s status %d", target, response.Code)
	}
	body, err := io.ReadAll(response.Body)
	if err != nil {
		t.Fatal(err)
	}
	return string(body)
}
