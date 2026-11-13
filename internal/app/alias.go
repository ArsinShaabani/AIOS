package app

import (
	"context"
	"net/http"
	"time"

	"github.com/ArsinShaabani/AIOS/internal/platform"
	"github.com/ArsinShaabani/AIOS/internal/ports"
)

// deviceFilter aliases ports.DeviceFilter for local readability.
type deviceFilter = ports.DeviceFilter

// commandResult aliases ports.CommandResult.
type commandResult = ports.CommandResult

// pingContext returns a short-lived context for health probes.
func pingContext(parent context.Context, d time.Duration) (context.Context, context.CancelFunc) {
	return context.WithTimeout(parent, d)
}

// noopHandler is used when an optional subsystem is disabled.
var noopHandler = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusNotFound)
})

var _ = platform.NewID
