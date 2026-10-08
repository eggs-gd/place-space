package scheduler

import (
	"context"
	"io"
	"log/slog"
	"testing"
	"time"

	"github.com/eggs-gd/place-space/internal/domain"
)

func TestRunPollsOnceThenStops(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	var calls int
	err := Run(ctx, []domain.Watch{
		{Name: "Кам'янець", Enabled: true, PollInterval: time.Hour},
		{Name: "вимкнений", Enabled: false, PollInterval: time.Hour},
	}, func(context.Context, domain.Watch) error {
		calls++
		cancel()
		return nil
	}, slog.New(slog.NewTextHandler(io.Discard, nil)))
	if err != nil {
		t.Fatal(err)
	}
	if calls != 1 {
		t.Fatalf("calls %d", calls)
	}
}
