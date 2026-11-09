// Package scheduler polls watches until the context is cancelled.
package scheduler

import (
	"context"
	"log/slog"
	"sync"
	"sync/atomic"
	"time"

	"github.com/eggs-gd/place-space/internal/domain"
)

const (
	defaultRescan   = time.Minute
	defaultInterval = 10 * time.Minute
)

// Scheduler keeps one worker per enabled watch. The set of watches is reread
// periodically and on Refresh, so watches created or switched on after start
// are picked up. Checks of one watch never overlap.
type Scheduler struct {
	list   func(context.Context) ([]domain.Watch, error)
	poll   func(context.Context, domain.Watch) error
	log    *slog.Logger
	rescan time.Duration

	mu      sync.Mutex
	pending map[string]struct{}
	notify  chan struct{}
}

// New builds a scheduler. list returns all watches; poll checks one of them.
func New(list func(context.Context) ([]domain.Watch, error), poll func(context.Context, domain.Watch) error, log *slog.Logger) *Scheduler {
	if log == nil {
		log = slog.Default()
	}
	return &Scheduler{
		list:    list,
		poll:    poll,
		log:     log,
		rescan:  defaultRescan,
		pending: map[string]struct{}{},
		notify:  make(chan struct{}, 1),
	}
}

// Refresh asks for a check of the watch as soon as possible. It never blocks
// and may be called before Run.
func (s *Scheduler) Refresh(watchID string) {
	s.mu.Lock()
	s.pending[watchID] = struct{}{}
	s.mu.Unlock()
	select {
	case s.notify <- struct{}{}:
	default:
	}
}

// Run checks each enabled watch on its own interval. A new worker checks
// immediately. A failing check is logged and the loop continues.
func (s *Scheduler) Run(ctx context.Context) error {
	workers := map[string]*worker{}
	var group sync.WaitGroup
	ticker := time.NewTicker(s.rescan)
	defer ticker.Stop()
	for {
		s.reconcile(ctx, workers, &group)
		select {
		case <-ctx.Done():
			group.Wait()
			return nil
		case <-ticker.C:
		case <-s.notify:
		}
	}
}

func (s *Scheduler) reconcile(ctx context.Context, workers map[string]*worker, group *sync.WaitGroup) {
	watches, err := s.list(ctx)
	if err != nil {
		if ctx.Err() == nil {
			s.log.Error("list watches", "err", err)
		}
		return
	}
	s.mu.Lock()
	pending := s.pending
	s.pending = map[string]struct{}{}
	s.mu.Unlock()

	enabled := make(map[string]struct{}, len(watches))
	for _, watch := range watches {
		if !watch.Enabled {
			continue
		}
		enabled[watch.ID] = struct{}{}
		current, running := workers[watch.ID]
		if !running {
			current = s.start(ctx, watch, group)
			workers[watch.ID] = current
			continue
		}
		current.interval.Store(int64(intervalOf(watch)))
		if _, wanted := pending[watch.ID]; wanted {
			select {
			case current.wake <- struct{}{}:
			default:
			}
		}
	}
	for id, current := range workers {
		if _, keep := enabled[id]; !keep {
			current.cancel()
			delete(workers, id)
		}
	}
}

type worker struct {
	cancel   context.CancelFunc
	wake     chan struct{}
	interval atomic.Int64
}

func (s *Scheduler) start(ctx context.Context, watch domain.Watch, group *sync.WaitGroup) *worker {
	workerCtx, cancel := context.WithCancel(ctx)
	current := &worker{cancel: cancel, wake: make(chan struct{}, 1)}
	current.interval.Store(int64(intervalOf(watch)))
	group.Add(1)
	go func() {
		defer group.Done()
		defer cancel()
		s.work(workerCtx, watch, current)
	}()
	return current
}

func (s *Scheduler) work(ctx context.Context, watch domain.Watch, current *worker) {
	for {
		if err := s.poll(ctx, watch); err != nil && ctx.Err() == nil {
			s.log.Error("poll failed", "watch", watch.Name, "err", err)
		}
		if ctx.Err() != nil {
			return
		}
		timer := time.NewTimer(time.Duration(current.interval.Load()))
		select {
		case <-ctx.Done():
			timer.Stop()
			return
		case <-current.wake:
			timer.Stop()
		case <-timer.C:
		}
	}
}

func intervalOf(watch domain.Watch) time.Duration {
	if watch.PollInterval > 0 {
		return watch.PollInterval
	}
	return defaultInterval
}
