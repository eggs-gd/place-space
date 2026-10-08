package main

import (
	"context"
	"errors"
	"log/slog"
	"net"
	"net/http"
	"time"

	"github.com/crgimenes/glaze"
)

const externalLinkScript = `(function () {
  if (window.__placeSpaceExternal) return;
  window.__placeSpaceExternal = true;
  document.addEventListener('click', function (event) {
    var node = event.target;
    while (node && node.tagName !== 'A') node = node.parentElement;
    if (!node || !node.href) return;
    var dest;
    try { dest = new URL(node.href); } catch (e) { return; }
    if (dest.origin === location.origin) return;
    if (dest.protocol !== 'http:' && dest.protocol !== 'https:') return;
    event.preventDefault();
    openExternal(dest.href);
  }, true);
})();`

func openWindow(ctx context.Context, handler http.Handler, start func()) error {
	// AppKit finishes launching inside NewWithOptions. Other goroutines that
	// are already in syscalls make that temporary run loop abort on macOS.
	window, err := glaze.NewWithOptions(glaze.Options{
		AcceptsFirstMouse: true,
		HideUntilLoaded:   true,
		FrameAutosaveName: "PlaceSpace",
		OnNewWindow: func(rawURL string) {
			if err := openExternal(rawURL); err != nil {
				slog.Error("open link", "url", rawURL, "err", err)
			}
		},
	})
	if err != nil {
		return err
	}
	defer window.Destroy()

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return err
	}
	server := &http.Server{
		Handler:           handler,
		ReadHeaderTimeout: 5 * time.Second,
	}
	go func() {
		err := server.Serve(listener)
		if err != nil && !errors.Is(err, http.ErrServerClosed) {
			slog.Error("http", "err", err)
		}
	}()
	defer func() {
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = server.Shutdown(shutdownCtx)
	}()
	if start != nil {
		start()
	}

	window.SetTitle("Place Space")
	window.SetSize(1200, 800, glaze.HintNone)
	if err := window.Bind("openExternal", func(rawURL string) error {
		return openExternal(rawURL)
	}); err != nil {
		return err
	}
	window.Init(externalLinkScript)

	pageURL := "http://" + listener.Addr().String() + "/"
	window.Navigate(pageURL)
	slog.Info("window", "addr", pageURL)

	go func() {
		<-ctx.Done()
		window.Terminate()
	}()
	window.Run()
	return nil
}
