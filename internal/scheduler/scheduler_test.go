package scheduler

import (
	"context"
	"io"
	"log/slog"
	"sync"
	"testing"
	"time"

	"github.com/eggs-gd/place-space/internal/domain"
)

const waitLimit = 5 * time.Second

// fixture is a mutable watch list with a channel of polled watch ids.
type fixture struct {
	mu      sync.Mutex
	watches []domain.Watch
	polled  chan string
}

func newFixture(watches ...domain.Watch) *fixture {
	return &fixture{watches: watches, polled: make(chan string, 16)}
}

func (f *fixture) set(watches ...domain.Watch) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.watches = watches
}

func (f *fixture) start(t *testing.T) *Scheduler {
	t.Helper()
	s := New(func(context.Context) ([]domain.Watch, error) {
		f.mu.Lock()
		defer f.mu.Unlock()
		return append([]domain.Watch{}, f.watches...), nil
	}, func(ctx context.Context, watch domain.Watch) error {
		select {
		case f.polled <- watch.ID:
		case <-ctx.Done():
		}
		return nil
	}, slog.New(slog.NewTextHandler(io.Discard, nil)))
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		defer close(done)
		_ = s.Run(ctx)
	}()
	t.Cleanup(func() {
		cancel()
		<-done
	})
	return s
}

func (f *fixture) expect(t *testing.T, id string) {
	t.Helper()
	select {
	case got := <-f.polled:
		if got != id {
			t.Fatalf("polled %q, want %q", got, id)
		}
	case <-time.After(waitLimit):
		t.Fatalf("no poll of %q", id)
	}
}

func (f *fixture) expectNone(t *testing.T) {
	t.Helper()
	select {
	case got := <-f.polled:
		t.Fatalf("unexpected poll of %q", got)
	case <-time.After(50 * time.Millisecond):
	}
}

func watch(id string, enabled bool) domain.Watch {
	return domain.Watch{ID: id, Name: id, Enabled: enabled, PollInterval: time.Hour}
}

func TestPollsEnabledWatchesOnly(t *testing.T) {
	f := newFixture(watch("on", true), watch("off", false))
	f.start(t)
	f.expect(t, "on")
	f.expectNone(t)
}

func TestPicksUpWatchCreatedAfterStart(t *testing.T) {
	f := newFixture()
	s := f.start(t)
	f.expectNone(t)
	f.set(watch("new", true))
	s.Refresh("new")
	f.expect(t, "new")
	f.expectNone(t)
}

func TestRefreshWakesRunningWatch(t *testing.T) {
	f := newFixture(watch("a", true))
	s := f.start(t)
	f.expect(t, "a")
	s.Refresh("a")
	f.expect(t, "a")
}

func TestPollsRepeatOnInterval(t *testing.T) {
	short := watch("a", true)
	short.PollInterval = 5 * time.Millisecond
	f := newFixture(short)
	f.start(t)
	f.expect(t, "a")
	f.expect(t, "a")
}

func TestDisabledWatchStopsPolling(t *testing.T) {
	short := watch("a", true)
	short.PollInterval = 5 * time.Millisecond
	f := newFixture(short, watch("other", true))
	s := f.start(t)
	deadline := time.After(waitLimit)
	for seen := false; !seen; {
		select {
		case id := <-f.polled:
			seen = id == "other"
		case <-deadline:
			t.Fatal("other watch never polled")
		}
	}
	f.set(watch("other", true))
	s.Refresh("other")
	// Drain until the refresh lands; after that "a" has been cancelled.
	for seen := false; !seen; {
		select {
		case id := <-f.polled:
			seen = id == "other"
		case <-deadline:
			t.Fatal("refresh never polled")
		}
	}
	time.Sleep(20 * time.Millisecond)
	for len(f.polled) > 0 {
		<-f.polled
	}
	f.expectNone(t)
}
