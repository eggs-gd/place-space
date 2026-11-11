package application

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"sync"
	"time"

	"github.com/eggs-gd/place-space/internal/domain"
	"github.com/eggs-gd/place-space/internal/id"
	"github.com/eggs-gd/place-space/internal/sources"
	"github.com/eggs-gd/place-space/internal/storage/sqlite"
)

// Pipeline runs fetch, normalize, exact dedup, filter, and change detection.
type Pipeline struct {
	Store    *sqlite.Store
	Adapters map[string]sources.Adapter
	Rates    RateSource
	Areas    *AreaDirectory
	// Places maps points to areas. Without it, sources cannot be checked by
	// area or distance on their own.
	Places *PlaceIndex
	Now    func() time.Time
	Log    *slog.Logger

	mu      sync.Mutex
	blocked map[string]time.Time
	points  map[string][2]float64
}

// blockCooldown is how long a source rests after refusing a request. Polling
// on regardless is how a temporary limit becomes a ban.
const blockCooldown = 30 * time.Minute

// RateSource supplies the exchange rates used by price filters.
type RateSource interface {
	Current(context.Context) domain.Rates
}

// Report is the visible result of checking one watch.
type Report struct {
	Watch domain.Watch
	Runs  []domain.Run
	Items []Item
}

// PollWatch checks every enabled source on the watch. One source failing does
// not stop the others. Listings returned before a later page error are kept.
// A check overtaken by a criteria change is dropped without an error; the
// change starts its own check.
func (p *Pipeline) PollWatch(ctx context.Context, watch domain.Watch) (Report, error) {
	report := Report{Watch: watch, Runs: []domain.Run{}, Items: []Item{}}
	if len(watch.SourceIDs) == 0 {
		return report, fmt.Errorf("watch %s has no sources", watch.Name)
	}
	var rates domain.Rates
	if p.Rates != nil {
		rates = p.Rates.Current(ctx)
	}
	for _, sourceID := range watch.SourceIDs {
		if err := ctx.Err(); err != nil {
			return report, err
		}
		source, err := p.Store.GetSource(ctx, sourceID)
		if err != nil {
			return report, fmt.Errorf("load source %s: %w", sourceID, err)
		}
		if !source.Enabled {
			p.log().Info("skip disabled source", "source", source.ID)
			continue
		}
		adapter, ok := p.Adapters[source.Type]
		if !ok {
			return report, fmt.Errorf("no adapter for source type %q", source.Type)
		}
		if p.resting(source.ID) {
			p.log().Info("skip resting source", "source", source.ID)
			continue
		}
		scoped, check, ok := p.scope(ctx, watch, source, adapter)
		if !ok {
			continue
		}
		runReport, err := p.pollSource(ctx, scoped, check, source, adapter, rates)
		if errors.Is(err, sqlite.ErrWatchChanged) {
			p.log().Info("drop poll for changed watch", "watch", watch.Name, "source", source.ID)
			return report, nil
		}
		if err != nil {
			return report, err
		}
		report.Runs = append(report.Runs, runReport.Run)
		report.Items = append(report.Items, runReport.Items...)
	}
	return report, nil
}

// scope decides what this source reads and what is checked afterwards. A
// source reads the narrowest catalog it has: the circle around the place, the
// chosen areas' pages, the districts around the place, or the whole city.
// Whatever the catalog cannot guarantee is checked per listing by area or
// distance. It reports false when the source cannot honour the watch:
// falling back to the whole city unchecked would fill the feed with places
// the person excluded.
func (p *Pipeline) scope(ctx context.Context, watch domain.Watch, source domain.Source, adapter sources.Adapter) (domain.Watch, placeCheck, bool) {
	query := watch.Query
	check := placeCheck{city: query.City}
	_, searchesNear := adapter.(sources.NearSearcher)

	if searchesNear {
		if query.Near == nil && len(query.Areas) == 0 {
			return watch, check, true
		}
		if len(query.Areas) > 0 {
			if p.Places == nil {
				p.log().Info("source cannot check areas", "source", source.ID, "watch", watch.Name)
				return watch, check, false
			}
			check.areas = areaSet(query.Areas)
			check.names = p.areaNames(ctx, query.City)
		}
		watch.Query.Areas = nil
		return watch, check, true
	}

	if query.Near != nil {
		near := *query.Near
		if !near.HasPoint() {
			if lat, lng, ok := p.PlacePoint(ctx, near.PlaceID); ok {
				near.Latitude, near.Longitude = &lat, &lng
			}
		}
		if !near.HasPoint() || p.Places == nil {
			p.log().Info("source cannot check distance", "source", source.ID, "watch", watch.Name)
			return watch, check, false
		}
		check.near = &near
		watch.Query.Near = nil
	}
	areas := query.Areas
	if len(areas) == 0 && check.near != nil {
		keys, err := p.Places.DistrictsAround(ctx, query.City, *check.near.Latitude, *check.near.Longitude, float64(check.near.RadiusMeters)/1000)
		if err != nil {
			p.log().Warn("districts around place", "watch", watch.Name, "err", err)
		}
		for _, key := range keys {
			areas = append(areas, domain.AreaRef{Key: key, Name: key})
		}
		if len(areas) == 0 {
			// Nothing is known around the place: read the city, newest first,
			// and let the distance check keep what is close.
			return watch, check, true
		}
	}
	if len(areas) == 0 {
		return watch, check, true
	}
	if p.Areas == nil {
		p.log().Warn("watch has areas but no directory", "watch", watch.Name)
		return watch, check, false
	}
	paths, err := p.Areas.Paths(ctx, source.Type, query.City, areas)
	if err != nil {
		p.log().Warn("load area paths", "source", source.ID, "err", err)
		return watch, check, false
	}
	if len(paths) == 0 {
		p.log().Info("source has none of the areas", "source", source.ID, "watch", watch.Name)
		return watch, check, false
	}
	watch.Query.Areas = areas
	watch.Query.AreaPaths = paths
	return watch, check, true
}

// PlacePoint returns the centre of a Google place, asking the first adapter
// that knows. Results are kept for the life of the pipeline.
func (p *Pipeline) PlacePoint(ctx context.Context, placeID string) (float64, float64, bool) {
	if placeID == "" {
		return 0, 0, false
	}
	p.mu.Lock()
	point, ok := p.points[placeID]
	p.mu.Unlock()
	if ok {
		return point[0], point[1], true
	}
	for _, adapter := range p.Adapters {
		pointer, ok := adapter.(sources.PlacePointer)
		if !ok {
			continue
		}
		lat, lng, err := pointer.PlaceCenter(ctx, placeID)
		if err != nil {
			p.log().Warn("place centre", "source", adapter.Type(), "err", err)
			continue
		}
		p.mu.Lock()
		if p.points == nil {
			p.points = map[string][2]float64{}
		}
		p.points[placeID] = [2]float64{lat, lng}
		p.mu.Unlock()
		return lat, lng, true
	}
	return 0, 0, false
}

func (p *Pipeline) resting(sourceID string) bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	until, ok := p.blocked[sourceID]
	if !ok {
		return false
	}
	if !p.now().Before(until) {
		delete(p.blocked, sourceID)
		return false
	}
	return true
}

func (p *Pipeline) rest(sourceID string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.blocked == nil {
		p.blocked = map[string]time.Time{}
	}
	p.blocked[sourceID] = p.now().Add(blockCooldown)
}

func (p *Pipeline) pollSource(ctx context.Context, watch domain.Watch, check placeCheck, source domain.Source, adapter sources.Adapter, rates domain.Rates) (Classified, error) {
	started := p.now()
	raws, fetchErr := adapter.Fetch(ctx, watch.Query)
	var fetchErrors []string
	if fetchErr != nil {
		fetchErrors = append(fetchErrors, fetchErr.Error())
		p.log().Warn("source fetch", "source", source.ID, "received", len(raws), "err", fetchErr)
		if errors.Is(fetchErr, sources.ErrBlocked) {
			p.rest(source.ID)
		}
	} else {
		p.log().Info("source fetch", "source", source.ID, "received", len(raws))
	}

	externalIDs := make([]string, 0, len(raws))
	urls := make([]string, 0, len(raws))
	normalized := make([]domain.Listing, 0, len(raws))
	for _, raw := range raws {
		listing, err := adapter.Normalize(ctx, raw)
		if err != nil {
			continue
		}
		normalized = append(normalized, listing)
		externalIDs = append(externalIDs, listing.ExternalID)
		if listing.URL != "" {
			urls = append(urls, listing.URL)
		}
	}

	existing, err := p.Store.ListingsByKeys(ctx, source.ID, externalIDs, urls)
	if err != nil {
		return Classified{}, fmt.Errorf("load listings: %w", err)
	}
	points := p.locate(ctx, check, source, adapter, watch.Filters, rates, normalized, existing)
	matches, err := p.Store.MatchesForWatch(ctx, watch.ID)
	if err != nil {
		return Classified{}, fmt.Errorf("load matches: %w", err)
	}
	classified, err := Classify(ClassifyInput{
		Now:         p.now(),
		RunID:       id.New(),
		Watch:       watch,
		Source:      source,
		Raw:         raws,
		Existing:    existing,
		Matches:     matches,
		FetchErrors: fetchErrors,
		Rates:       rates,
		Normalize: func(raw sources.RawListing) (domain.Listing, error) {
			listing, err := adapter.Normalize(ctx, raw)
			if err == nil && listing.Location.Latitude == nil {
				if point, ok := points[listing.ExternalID]; ok {
					listing.Location.Latitude, listing.Location.Longitude = &point[0], &point[1]
				}
			}
			return listing, err
		},
		Place: func(listing domain.Listing) []domain.Reason {
			return p.placeReasons(ctx, check, listing)
		},
	})
	if err != nil {
		return Classified{}, err
	}
	finished := p.now()
	classified.Run.StartedAt = started
	classified.Run.FinishedAt = finished
	classified.Write.Run = classified.Run

	source.UpdatedAt = finished
	if fetchErr != nil && len(raws) == 0 {
		source.Status = domain.SourceError
		source.LastError = fetchErr.Error()
		failedAt := finished
		source.LastErrorAt = &failedAt
	} else {
		source.Status = domain.SourceHealthy
		succeededAt := finished
		source.LastSuccessAt = &succeededAt
		if fetchErr != nil {
			source.LastError = fetchErr.Error()
			failedAt := finished
			source.LastErrorAt = &failedAt
		} else {
			source.LastError = ""
			source.LastErrorAt = nil
		}
	}
	classified.Write.Source = source
	classified.Write.WatchUpdatedAt = watch.UpdatedAt
	if err := p.Store.SavePoll(ctx, classified.Write); err != nil {
		return Classified{}, fmt.Errorf("save poll: %w", err)
	}
	p.log().Info("poll saved",
		"source", source.ID,
		"received", classified.Run.Received,
		"new", classified.Run.New,
		"changed", classified.Run.Changed,
		"matched", classified.Run.Matched,
		"rejected", classified.Run.Rejected,
		"duplicates", classified.Run.Duplicates,
	)
	return classified, nil
}

func (p *Pipeline) now() time.Time {
	if p.Now != nil {
		return p.Now()
	}
	return time.Now().UTC()
}

func (p *Pipeline) log() *slog.Logger {
	if p.Log != nil {
		return p.Log
	}
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}
