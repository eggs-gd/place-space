package application

import (
	"context"
	"fmt"
	"io"
	"log/slog"
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
	Now      func() time.Time
	Log      *slog.Logger
}

// Report is the visible result of checking one watch.
type Report struct {
	Watch domain.Watch
	Runs  []domain.Run
	Items []Item
}

// PollWatch checks every enabled source on the watch. One source failing does
// not stop the others. Listings returned before a later page error are kept.
func (p *Pipeline) PollWatch(ctx context.Context, watch domain.Watch) (Report, error) {
	report := Report{Watch: watch, Runs: []domain.Run{}, Items: []Item{}}
	if len(watch.SourceIDs) == 0 {
		return report, fmt.Errorf("watch %s has no sources", watch.Name)
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
		runReport, err := p.pollSource(ctx, watch, source, adapter)
		if err != nil {
			return report, err
		}
		report.Runs = append(report.Runs, runReport.Run)
		report.Items = append(report.Items, runReport.Items...)
	}
	return report, nil
}

func (p *Pipeline) pollSource(ctx context.Context, watch domain.Watch, source domain.Source, adapter sources.Adapter) (Classified, error) {
	started := p.now()
	raws, fetchErr := adapter.Fetch(ctx, watch.Query)
	var fetchErrors []string
	if fetchErr != nil {
		fetchErrors = append(fetchErrors, fetchErr.Error())
		p.log().Warn("source fetch", "source", source.ID, "received", len(raws), "err", fetchErr)
	} else {
		p.log().Info("source fetch", "source", source.ID, "received", len(raws))
	}

	externalIDs := make([]string, 0, len(raws))
	urls := make([]string, 0, len(raws))
	for _, raw := range raws {
		listing, err := adapter.Normalize(ctx, raw)
		if err != nil {
			continue
		}
		externalIDs = append(externalIDs, listing.ExternalID)
		if listing.URL != "" {
			urls = append(urls, listing.URL)
		}
	}

	existing, err := p.Store.ListingsByKeys(ctx, source.ID, externalIDs, urls)
	if err != nil {
		return Classified{}, fmt.Errorf("load listings: %w", err)
	}
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
		Normalize: func(raw sources.RawListing) (domain.Listing, error) {
			return adapter.Normalize(ctx, raw)
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
