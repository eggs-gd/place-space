package main

import (
	"context"
	"errors"
	"flag"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"github.com/eggs-gd/place-space/internal/httpapi"
	"github.com/eggs-gd/place-space/internal/ui"
)

func cmdServe(args []string) error {
	fs := flag.NewFlagSet("serve", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	dbPath := fs.String("db", defaultDBPath(), "шлях до SQLite")
	addr := fs.String("addr", "127.0.0.1:8080", "адреса API")
	pages := fs.Int("pages", 5, "максимум сторінок пошуку на кожен тип житла")
	open := fs.Bool("open", false, "відкрити інтерфейс у власному вікні")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *open {
		if err := setupAppLog(*dbPath); err != nil {
			slog.Error("log file", "err", err)
		}
	}
	// On macOS, signal handlers installed before the native window exists
	// make AppKit abort while it is still launching. The window path arms
	// them after the window is created.
	var ctx context.Context
	var stop context.CancelFunc
	if *open {
		ctx, stop = context.WithCancel(context.Background())
	} else {
		ctx, stop = signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	}
	defer stop()
	store, err := openStore(ctx, *dbPath)
	if err != nil {
		return err
	}
	defer store.Close()

	var polls sync.WaitGroup
	startPolls := func() {
		polls.Add(1)
		go func() {
			defer polls.Done()
			if err := pollLoop(ctx, store, *pages); err != nil && ctx.Err() == nil {
				slog.Error("scheduler stopped", "err", err)
			}
		}()
	}

	api := &httpapi.Server{
		Store: store,
		Refresh: func(watchID string) {
			go refreshWatch(store, *pages, watchID)
		},
	}
	handler := ui.Mount(api.Handler(), ui.Assets())
	if *open {
		err = openWindow(ctx, handler, func() {
			armSignals(stop)
			startPolls()
		})
	} else {
		startPolls()
		err = serveHTTP(ctx, handler, *addr)
	}
	stop()
	polls.Wait()
	if errors.Is(err, http.ErrServerClosed) {
		return nil
	}
	return err
}

func armSignals(stop context.CancelFunc) {
	signals := make(chan os.Signal, 1)
	signal.Notify(signals, os.Interrupt, syscall.SIGTERM)
	go func() {
		<-signals
		stop()
	}()
}

func serveHTTP(ctx context.Context, handler http.Handler, addr string) error {
	server := &http.Server{
		Addr:              addr,
		Handler:           handler,
		ReadHeaderTimeout: 5 * time.Second,
	}
	listener, err := net.Listen("tcp", addr)
	if err != nil {
		return err
	}
	go func() {
		<-ctx.Done()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = server.Shutdown(shutdownCtx)
	}()
	slog.Info("api listening", "addr", "http://"+addr+"/")
	return server.Serve(listener)
}
