// Package otodim reads long-term rental listings from Otodim catalog pages.
//
// Otodim is OLX Group's housing site, and its listings come from OLX. The
// catalog HTML embeds the current page as Next.js data, and Fetch reads that
// payload. The site's robots.txt disallows /api/query, so that is not used.
package otodim

import (
	"net/http"
	"strings"
	"time"
)

const (
	defaultBaseURL = "https://www.otodim.com.ua"
	// A page weighs about a megabyte and the catalog is read newest first,
	// so two pages cover what appears between checks.
	defaultMaxPages = 2
	defaultPause    = time.Second
	userAgent       = "Mozilla/5.0 (compatible; PlaceSpace/0.1; +https://github.com/eggs-gd/place-space)"
)

// Options configures the adapter. Zero values get the defaults in New.
type Options struct {
	BaseURL      string
	HTTP         *http.Client
	MaxPages     int
	Pause        time.Duration
	DisablePause bool
}

// Adapter is the Otodim source.
type Adapter struct {
	baseURL  string
	http     *http.Client
	maxPages int
	pause    time.Duration
}

// New returns an adapter with defaults filled in.
func New(opts Options) *Adapter {
	adapter := &Adapter{
		baseURL:  defaultBaseURL,
		http:     &http.Client{Timeout: 30 * time.Second},
		maxPages: defaultMaxPages,
		pause:    defaultPause,
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
	return adapter
}

// Type implements sources.Adapter.
func (a *Adapter) Type() string { return "otodim" }
