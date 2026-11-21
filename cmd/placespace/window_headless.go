//go:build headless

package main

import (
	"context"
	"errors"
	"net/http"
)

// Server images are built with the headless tag: the window library needs
// the system's dynamic loader, which a static image does not have.
func openWindow(context.Context, http.Handler, func()) error {
	return errors.New("ця збірка без вікна: запустіть serve без --open")
}
