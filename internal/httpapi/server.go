// Package httpapi serves the listing feed and the state behind it.
package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/url"
	"slices"
	"time"

	"github.com/eggs-gd/place-space/internal/application"
	"github.com/eggs-gd/place-space/internal/domain"
	"github.com/eggs-gd/place-space/internal/storage/sqlite"
)

const newWindow = 24 * time.Hour

// Server reads the store for the UI.
type Server struct {
	Store *sqlite.Store
	Now   func() time.Time
	// Refresh starts a source check after criteria change. It may be nil.
	Refresh func(watchID string)
	// Areas lists and validates city areas. Without it, areas cannot be chosen.
	Areas AreaDirectory
	// Places resolves place names and keeps the Google key. Without it, a
	// watch cannot search near a place.
	Places PlaceResolver
}

// AreaDirectory is what the API needs from the area directory.
type AreaDirectory interface {
	List(ctx context.Context, city string) ([]application.AreaOption, error)
	Resolve(ctx context.Context, city string, requested []string) ([]domain.AreaRef, error)
}

// Handler is the HTTP API.
func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/overview", s.overview)
	mux.HandleFunc("GET /api/listings", s.listings)
	mux.HandleFunc("GET /api/listings/{id}", s.listing)
	mux.HandleFunc("GET /api/areas", s.areas)
	mux.HandleFunc("POST /api/watches", s.createWatch)
	mux.HandleFunc("PATCH /api/watches/{id}", s.updateWatch)
	mux.HandleFunc("GET /api/settings", s.settings)
	mux.HandleFunc("PUT /api/settings", s.updateSettings)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		w.Header().Add("Vary", "Origin")
		if origin := r.Header.Get("Origin"); origin != "" {
			if !originAllowed(origin, r.Host) {
				writeJSON(w, http.StatusForbidden, errorBody{Error: "запит з іншого сайту заборонено"})
				return
			}
			w.Header().Set("Access-Control-Allow-Origin", origin)
			w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PATCH, PUT, OPTIONS")
			w.Header().Set("Access-Control-Allow-Headers", "Content-Type")
		}
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		mux.ServeHTTP(w, r)
	})
}

// originAllowed accepts the page the API serves and anything on this machine,
// which covers the SvelteKit dev server on whatever port it picks. Every other
// site is refused outright: without this, a page open in the browser could
// post criteria with a simple request that needs no preflight.
func originAllowed(origin, host string) bool {
	parsed, err := url.Parse(origin)
	if err != nil {
		return false
	}
	if parsed.Host == host {
		return true
	}
	switch parsed.Hostname() {
	case "localhost", "127.0.0.1", "::1":
		return true
	}
	return false
}

func (s *Server) overview(w http.ResponseWriter, r *http.Request) {
	watch, ok, err := s.currentWatch(r)
	if err != nil {
		writeError(w, err)
		return
	}
	if !ok {
		writeJSON(w, http.StatusOK, overviewBody{Summary: summaryBody{}})
		return
	}
	body, err := s.buildOverview(r.Context(), watch)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, body)
}

func (s *Server) listings(w http.ResponseWriter, r *http.Request) {
	watch, ok, err := s.currentWatch(r)
	if err != nil {
		writeError(w, err)
		return
	}
	status := r.URL.Query().Get("status")
	if status == "" {
		status = domain.MatchMatched
	}
	if status != domain.MatchMatched && status != domain.MatchRejected && status != "all" {
		writeJSON(w, http.StatusBadRequest, errorBody{Error: "status має бути matched, rejected або all"})
		return
	}
	if !ok {
		writeJSON(w, http.StatusOK, feedBody{Status: status, Items: []cardBody{}})
		return
	}
	rows, err := s.Store.Feed(r.Context(), watch.ID, status)
	if err != nil {
		writeError(w, err)
		return
	}
	now := s.now()
	items := make([]cardBody, 0, len(rows))
	for _, row := range rows {
		items = append(items, cardFrom(row, now))
	}
	writeJSON(w, http.StatusOK, feedBody{Status: status, Items: items})
}

func (s *Server) listing(w http.ResponseWriter, r *http.Request) {
	listing, err := s.Store.GetListing(r.Context(), r.PathValue("id"))
	if errors.Is(err, sqlite.ErrNotFound) {
		writeJSON(w, http.StatusNotFound, errorBody{Error: "оголошення не знайдено"})
		return
	}
	if err != nil {
		writeError(w, err)
		return
	}
	match, _, err := s.Store.MatchForListing(r.Context(), listing.ID, r.URL.Query().Get("watch"))
	if err != nil {
		writeError(w, err)
		return
	}
	observations, err := s.Store.Observations(r.Context(), listing.ID)
	if err != nil {
		writeError(w, err)
		return
	}
	linked, err := s.Store.LinkedListings(r.Context(), listing.ID)
	if err != nil {
		writeError(w, err)
		return
	}
	images := listing.Images
	if images == nil {
		images = []string{}
	}
	dups := make([]duplicateBody, 0, len(linked))
	for _, other := range linked {
		dups = append(dups, duplicateBody{
			ID:          other.ID,
			Source:      other.Source,
			URL:         other.URL,
			Title:       other.Title,
			PriceAmount: other.PriceAmount,
			Currency:    other.Currency,
		})
	}
	writeJSON(w, http.StatusOK, detailBody{
		cardBody:    cardFrom(sqlite.FeedRow{Listing: listing, Match: match}, s.now()),
		Description: listing.Description,
		Images:      images,
		History:     priceHistory(observations),
		Duplicates:  dups,
	})
}

func (s *Server) buildOverview(ctx context.Context, watch domain.Watch) (overviewBody, error) {
	matched, rejected, err := s.Store.MatchCounts(ctx, watch.ID)
	if err != nil {
		return overviewBody{}, err
	}
	rows, err := s.Store.Feed(ctx, watch.ID, domain.MatchMatched)
	if err != nil {
		return overviewBody{}, err
	}
	now := s.now()
	fresh := 0
	for _, row := range rows {
		if isNew(row.Listing.FirstSeenAt, now) {
			fresh++
		}
	}
	sources := make([]sourceJSON, 0, len(watch.SourceIDs))
	for _, sourceID := range watch.SourceIDs {
		source, err := s.Store.GetSource(ctx, sourceID)
		if errors.Is(err, sqlite.ErrNotFound) {
			continue
		}
		if err != nil {
			return overviewBody{}, err
		}
		sources = append(sources, sourceJSON{
			ID:            source.ID,
			Type:          source.Type,
			Enabled:       source.Enabled,
			Status:        source.Status,
			LastSuccessAt: source.LastSuccessAt,
			LastErrorAt:   source.LastErrorAt,
			LastError:     source.LastError,
		})
	}
	slices.SortStableFunc(sources, func(a, b sourceJSON) int {
		return sourceOrder(a.Type) - sourceOrder(b.Type)
	})
	var sourceBody *sourceJSON
	if len(sources) > 0 {
		first := sources[0]
		sourceBody = &first
	}
	var runBody *runJSON
	run, ok, err := s.Store.LatestRun(ctx, watch.ID)
	if err != nil {
		return overviewBody{}, err
	}
	if ok {
		errors := run.Errors
		if errors == nil {
			errors = []string{}
		}
		runBody = &runJSON{
			SourceType: run.SourceType,
			StartedAt:  run.StartedAt,
			FinishedAt: run.FinishedAt,
			DurationMs: run.Duration().Milliseconds(),
			Received:   run.Received,
			New:        run.New,
			Changed:    run.Changed,
			Matched:    run.Matched,
			Rejected:   run.Rejected,
			Duplicates: run.Duplicates,
			Failed:     run.Failed,
			Errors:     errors,
		}
	}
	body := watchBody(watch)
	return overviewBody{
		Watch:   &body,
		Summary: summaryBody{Places: matched, New: fresh, Rejected: rejected},
		Source:  sourceBody,
		Sources: sources,
		Run:     runBody,
	}, nil
}

func (s *Server) currentWatch(r *http.Request) (domain.Watch, bool, error) {
	if id := r.URL.Query().Get("watch"); id != "" {
		watch, err := s.Store.GetWatch(r.Context(), id)
		if errors.Is(err, sqlite.ErrNotFound) {
			return domain.Watch{}, false, nil
		}
		if err != nil {
			return domain.Watch{}, false, err
		}
		return watch, true, nil
	}
	watches, err := s.Store.ListWatches(r.Context())
	if err != nil {
		return domain.Watch{}, false, err
	}
	for _, watch := range watches {
		if watch.Enabled {
			return watch, true, nil
		}
	}
	if len(watches) > 0 {
		return watches[0], true, nil
	}
	return domain.Watch{}, false, nil
}

func (s *Server) now() time.Time {
	if s.Now != nil {
		return s.Now()
	}
	return time.Now().UTC()
}

func cardFrom(row sqlite.FeedRow, now time.Time) cardBody {
	listing := row.Listing
	image := ""
	if len(listing.Images) > 0 {
		image = listing.Images[0]
	}
	reasons := row.Match.Reasons
	if reasons == nil {
		reasons = []domain.Reason{}
	}
	return cardBody{
		ID:          listing.ID,
		Source:      listing.Source,
		URL:         listing.URL,
		Title:       listing.Title,
		PriceAmount: listing.PriceAmount,
		Currency:    listing.Currency,
		Location:    listing.Location.Name,
		Address:     listing.Location.Address,
		Rooms:       listing.Rooms,
		Area:        listing.Area,
		Floor:       listing.Floor,
		TotalFloors: listing.TotalFloors,
		Image:       image,
		PublishedAt: listing.PublishedAt,
		FirstSeenAt: listing.FirstSeenAt,
		LastSeenAt:  listing.LastSeenAt,
		IsNew:       isNew(listing.FirstSeenAt, now),
		Status:      row.Match.Status,
		Reasons:     reasons,
	}
}

func sourceOrder(sourceType string) int {
	switch sourceType {
	case "lun":
		return 0
	case "domria":
		return 1
	case "otodim":
		return 2
	default:
		return 3
	}
}

func isNew(firstSeen, now time.Time) bool {
	if firstSeen.IsZero() {
		return false
	}
	return !firstSeen.Before(now.Add(-newWindow))
}

func priceHistory(observations []domain.Observation) []historyBody {
	history := make([]historyBody, 0, len(observations))
	var previous *int64
	previousCurrency := ""
	for _, observation := range observations {
		if observation.PriceAmount == nil {
			continue
		}
		if previous != nil && *previous == *observation.PriceAmount && previousCurrency == observation.Currency {
			continue
		}
		amount := *observation.PriceAmount
		history = append(history, historyBody{
			SeenAt:      observation.SeenAt,
			PriceAmount: amount,
			Currency:    observation.Currency,
		})
		previous = &amount
		previousCurrency = observation.Currency
	}
	return history
}

func writeJSON(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	enc := json.NewEncoder(w)
	enc.SetEscapeHTML(false)
	_ = enc.Encode(body)
}

func writeError(w http.ResponseWriter, err error) {
	writeJSON(w, http.StatusInternalServerError, errorBody{Error: err.Error()})
}
