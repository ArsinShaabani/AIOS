package app

import (
	"context"
	"log/slog"

	"github.com/ArsinShaabani/AIOS/internal/adapters/eventbus"
	"github.com/ArsinShaabani/AIOS/internal/config"
	"github.com/ArsinShaabani/AIOS/internal/ports"
)

// newEventBus selects the configured event transport. NATS lets several AIOS
// processes share chat/device/reminder events; memory keeps edge installs
// dependency-free.
func newEventBus(ctx context.Context, cfg config.EventsConfig, log *slog.Logger) (ports.EventBus, error) {
	if cfg.Driver == "nats" {
		return eventbus.NewNATS(cfg, log)
	}
	return NewBus(), nil
}
