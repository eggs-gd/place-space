// Package core is the part of Place Space every host shares: the database,
// the sources, the scheduler and the local HTTP interface. A host (the desktop
// window, a phone shell, the CLI) only chooses where the data lives, starts or
// stops the core, and decides when checks may run.
package core

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"sync"
	"time"

	"github.com/eggs-gd/place-space/internal/application"
	"github.com/eggs-gd/place-space/internal/domain"
	"github.com/eggs-gd/place-space/internal/httpapi"
	"github.com/eggs-gd/place-space/internal/rates"
	"github.com/eggs-gd/place-space/internal/scheduler"
	"github.com/eggs-gd/place-space/internal/sources"
	"github.com/eggs-gd/place-space/internal/sources/domria"
	"github.com/eggs-gd/place-space/internal/sources/lun"
	"github.com/eggs-gd/place-space/internal/storage/sqlite"
	"github.com/eggs-gd/place-space/internal/ui"
)

// DefaultPages is how many search pages each catalog reads per property type.
const DefaultPages = 5

// Options configure a core.
type Options struct {
	// DBPath is the SQLite file. Its directory is created when missing.
	DBPath string
	// Pages limits search pages per catalog; zero means DefaultPages.
	Pages int
	// Adapters replace the built-in LUN and DOM.RIA adapters, for tests.
	Adapters map[string]sources.Adapter
	// Rates replaces the NBU exchange rates, for tests.
	Rates application.RateSource
	// Now replaces the clock, for tests.
	Now func() time.Time
	Log *slog.Logger
}

// Core owns one open database and everything that works on it.
type Core struct {
	Store     *sqlite.Store
	Pipeline  *application.Pipeline
	Scheduler *scheduler.Scheduler
	// SourceIDs are the sources new watches check.
	SourceIDs []string

	log *slog.Logger
	now func() time.Time

	mu      sync.Mutex
	polling map[string]struct{}
}

// Summary is what one round of checks found.
type Summary struct {
	// Checked counts the watches that were checked.
	Checked int `json:"checked"`
	// Fresh counts listings seen for the first time that match their watch.
	Fresh int `json:"fresh"`
	// Failed counts the watches whose check ended with errors.
	Failed int `json:"failed"`
}

// Open opens and migrates the database and registers the default sources.
func Open(ctx context.Context, opts Options) (*Core, error) {
	if opts.DBPath == "" {
		return nil, errors.New("core: no database path")
	}
	log := opts.Log
	if log == nil {
		log = slog.Default()
	}
	now := opts.Now
	if now == nil {
		now = time.Now
	}
	store, err := sqlite.Open(opts.DBPath)
	if err != nil {
		return nil, err
	}
	if err := store.Migrate(ctx); err != nil {
		store.Close()
		return nil, err
	}
	sourceIDs, err := application.EnsureDefaultSources(ctx, store)
	if err != nil {
		store.Close()
		return nil, err
	}
	pages := opts.Pages
	if pages <= 0 {
		pages = DefaultPages
	}
	adapters := opts.Adapters
	if adapters == nil {
		adapters = map[string]sources.Adapter{
			"lun":    lun.New(lun.Options{MaxPages: pages}),
			"domria": domria.New(domria.Options{MaxPages: pages}),
		}
	}
	rateSource := opts.Rates
	if rateSource == nil {
		rateSource = &rates.Cache{Store: store, Fetcher: &rates.NBU{}, Log: log}
	}
	c := &Core{
		Store: store,
		Pipeline: &application.Pipeline{
			Store:    store,
			Adapters: adapters,
			Rates:    rateSource,
			Areas:    &application.AreaDirectory{Store: store, Adapters: adapters, Log: log},
			Now:      opts.Now,
			Log:      log,
		},
		SourceIDs: sourceIDs,
		log:       log,
		now:       now,
		polling:   map[string]struct{}{},
	}
	c.Scheduler = scheduler.New(store.ListWatches, func(ctx context.Context, watch domain.Watch) error {
		_, err := c.poll(ctx, watch.ID)
		if errors.Is(err, errBusy) {
			return nil
		}
		return err
	}, log)
	return c, nil
}

// Close closes the database. Stop Run and wait for CheckDue first.
func (c *Core) Close() error {
	return c.Store.Close()
}

// Handler serves the API and, in builds with the interface, the pages.
func (c *Core) Handler() http.Handler {
	api := &httpapi.Server{Store: c.Store, Refresh: c.Scheduler.Refresh, Areas: c.Pipeline.Areas}
	return ui.Mount(api.Handler(), ui.Assets())
}

// Run checks every enabled watch on its interval until ctx ends. Hosts that
// stay in the foreground use this.
func (c *Core) Run(ctx context.Context) error {
	c.log.Info("scheduler started")
	return c.Scheduler.Run(ctx)
}

// CheckDue checks, once, every enabled watch whose interval has passed since
// its latest check. Hosts woken briefly in the background use this. A watch
// that Run is already checking is skipped.
func (c *Core) CheckDue(ctx context.Context) (Summary, error) {
	var summary Summary
	watches, err := c.Store.ListWatches(ctx)
	if err != nil {
		return summary, err
	}
	for _, watch := range watches {
		if err := ctx.Err(); err != nil {
			return summary, err
		}
		if !watch.Enabled {
			continue
		}
		due, err := c.due(ctx, watch)
		if err != nil {
			return summary, err
		}
		if !due {
			continue
		}
		report, err := c.poll(ctx, watch.ID)
		if errors.Is(err, errBusy) {
			continue
		}
		summary.Checked++
		summary.Fresh += fresh(report)
		if err != nil {
			if ctx.Err() != nil {
				return summary, ctx.Err()
			}
			summary.Failed++
			c.log.Error("poll failed", "watch", watch.Name, "err", err)
		}
	}
	return summary, nil
}

var errBusy = errors.New("watch is being checked")

func (c *Core) due(ctx context.Context, watch domain.Watch) (bool, error) {
	run, found, err := c.Store.LatestRun(ctx, watch.ID)
	if err != nil || !found {
		return err == nil, err
	}
	return !c.now().Before(run.FinishedAt.Add(interval(watch))), nil
}

// poll checks one watch with its current criteria. Checks of one watch never
// overlap, whichever of Run and CheckDue started them.
func (c *Core) poll(ctx context.Context, watchID string) (application.Report, error) {
	c.mu.Lock()
	if _, busy := c.polling[watchID]; busy {
		c.mu.Unlock()
		return application.Report{}, errBusy
	}
	c.polling[watchID] = struct{}{}
	c.mu.Unlock()
	defer func() {
		c.mu.Lock()
		delete(c.polling, watchID)
		c.mu.Unlock()
	}()

	watch, err := c.Store.GetWatch(ctx, watchID)
	if err != nil {
		return application.Report{}, err
	}
	if !watch.Enabled {
		return application.Report{}, nil
	}
	report, err := c.Pipeline.PollWatch(ctx, watch)
	if err != nil {
		return report, err
	}
	if err := c.Store.Prune(ctx); err != nil {
		c.log.Warn("prune", "err", err)
	}
	if application.HasErrors(report) {
		return report, errors.New("перевірка джерела завершилась з помилками")
	}
	return report, nil
}

// Poll checks one watch now, as the CLI poll command does. The report is
// returned even when the check ended with errors.
func (c *Core) Poll(ctx context.Context, watchID string) (application.Report, error) {
	return c.poll(ctx, watchID)
}

func fresh(report application.Report) int {
	count := 0
	for _, item := range report.Items {
		if item.Outcome == application.OutcomeNew && item.DuplicateOf == "" && item.Decision.Status == domain.MatchMatched {
			count++
		}
	}
	return count
}

const defaultInterval = 10 * time.Minute

func interval(watch domain.Watch) time.Duration {
	if watch.PollInterval > 0 {
		return watch.PollInterval
	}
	return defaultInterval
}
