// Package scheduler polls watches until the context is cancelled.
package scheduler

import (
	"context"
	"log/slog"
	"sync"
	"time"

	"github.com/eggs-gd/place-space/internal/domain"
)

// Run checks each enabled watch on its own interval. The first check happens
// immediately. A failing check is logged and the loop continues.
func Run(ctx context.Context, watches []domain.Watch, poll func(context.Context, domain.Watch) error, log *slog.Logger) error {
	if log == nil {
		log = slog.Default()
	}
	enabled := make([]domain.Watch, 0, len(watches))
	for _, watch := range watches {
		if watch.Enabled {
			enabled = append(enabled, watch)
		}
	}
	if len(enabled) == 0 {
		log.Info("no enabled watches")
		<-ctx.Done()
		return nil
	}
	var group sync.WaitGroup
	for _, watch := range enabled {
		group.Add(1)
		go func(watch domain.Watch) {
			defer group.Done()
			runWatch(ctx, watch, poll, log)
		}(watch)
	}
	group.Wait()
	return nil
}

func runWatch(ctx context.Context, watch domain.Watch, poll func(context.Context, domain.Watch) error, log *slog.Logger) {
	interval := watch.PollInterval
	if interval <= 0 {
		interval = 10 * time.Minute
	}
	for {
		if err := poll(ctx, watch); err != nil && ctx.Err() == nil {
			log.Error("poll failed", "watch", watch.Name, "err", err)
		}
		if ctx.Err() != nil {
			return
		}
		timer := time.NewTimer(interval)
		select {
		case <-ctx.Done():
			timer.Stop()
			return
		case <-timer.C:
		}
	}
}
