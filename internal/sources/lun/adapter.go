// Package lun reads long-term rental listings from LUN search pages.
//
// LUN does not publish a search API. Each search response embeds the realty
// cards as JSON inside the page, which is what Fetch reads. Browser automation
// is not used.
package lun

import (
	"net/http"
	"strings"
	"time"
)

const (
	defaultBaseURL  = "https://lun.ua"
	defaultMaxPages = 5
	defaultPause    = 200 * time.Millisecond
	userAgent       = "Mozilla/5.0 (compatible; PlaceSpace/0.1; +https://github.com/eggs-gd/place-space)"
)

// Options configures the adapter. Zero values get the defaults in New.
type Options struct {
	BaseURL      string
	HTTP         *http.Client
	MaxPages     int
	Pause        time.Duration
	DisablePause bool
	CityPaths    map[string]string
}

// Adapter is the LUN source.
type Adapter struct {
	baseURL  string
	http     *http.Client
	maxPages int
	pause    time.Duration
	paths    map[string]string
}

// New returns an adapter with defaults filled in.
func New(opts Options) *Adapter {
	adapter := &Adapter{
		baseURL:  strings.TrimRight(defaultBaseURL, "/"),
		http:     &http.Client{Timeout: 30 * time.Second},
		maxPages: defaultMaxPages,
		pause:    defaultPause,
		paths:    copyPaths(defaultCityPaths),
	}
	if opts.BaseURL != "" {
		adapter.baseURL = strings.TrimRight(opts.BaseURL, "/")
	}
	if opts.HTTP != nil {
		adapter.http = opts.HTTP
	}
	if opts.MaxPages > 0 {
		adapter.maxPages = opts.MaxPages
	}
	if opts.DisablePause {
		adapter.pause = 0
	} else if opts.Pause > 0 {
		adapter.pause = opts.Pause
	}
	for name, path := range opts.CityPaths {
		adapter.paths[normCity(name)] = path
	}
	return adapter
}

// Type implements sources.Adapter.
func (a *Adapter) Type() string { return "lun" }

func copyPaths(in map[string]string) map[string]string {
	out := make(map[string]string, len(in))
	for key, value := range in {
		out[key] = value
	}
	return out
}
