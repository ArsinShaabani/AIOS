package app

import (
	"context"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"sync"
	"time"

	"github.com/ArsinShaabani/AIOS/internal/adapters/mqtt"
	"github.com/ArsinShaabani/AIOS/internal/adapters/storage/sqlstore"
	"github.com/ArsinShaabani/AIOS/internal/agents"
	"github.com/ArsinShaabani/AIOS/internal/brain"
	"github.com/ArsinShaabani/AIOS/internal/config"
	"github.com/ArsinShaabani/AIOS/internal/devices"
	"github.com/ArsinShaabani/AIOS/internal/documents"
	"github.com/ArsinShaabani/AIOS/internal/memory"
	"github.com/ArsinShaabani/AIOS/internal/mqttbridge"
	"github.com/ArsinShaabani/AIOS/internal/observability"
	"github.com/ArsinShaabani/AIOS/internal/personality"
	"github.com/ArsinShaabani/AIOS/internal/plugins"
	"github.com/ArsinShaabani/AIOS/internal/ports"
	"github.com/ArsinShaabani/AIOS/internal/providers"
	"github.com/ArsinShaabani/AIOS/internal/scheduler"
	"github.com/ArsinShaabani/AIOS/internal/security"
	"github.com/ArsinShaabani/AIOS/internal/tools"
	"github.com/ArsinShaabani/AIOS/internal/transport/httpapi"
	"github.com/ArsinShaabani/AIOS/internal/userai"
	"github.com/ArsinShaabani/AIOS/internal/voice"
)

// App owns every subsystem and their lifecycle.
type App struct {
	Config  config.Config
	Log     *slog.Logger
	Version string

	DB        *sqlstore.DB
	Repos     *sqlstore.Repositories
	Cache     ports.Cache
	Vector    ports.VectorStore
	Queue     ports.Queue
	Router    ports.ModelRouter
	Usage     *UsageRecorder
	Keyring   *security.Keyring
	Tokens    *security.TokenService
	Auditor   *security.Auditor
	Secrets   *security.Secrets
	Providers *providers.Service
	UserAI    *userai.Service

	Memory        *memory.Service
	Documents     *documents.Service
	Personality   *personality.Engine
	Agents        *agents.Registry
	Devices       *devices.Service
	Scheduler     *scheduler.Service
	Tools         *tools.Registry
	Brain         *brain.Engine
	Voice         *voice.Service
	Plugins       *plugins.Host
	MQTT          *mqtt.Client
	MQTTTransport *mqtt.Transport
	MQTTBridge    *mqttbridge.Bridge

	Bus     ports.EventBus
	Metrics *observability.Metrics
	System  *observability.SystemMonitor
	Health  *observability.Health
	API     *httpapi.API
	HTTP    *http.Server

	cancel    context.CancelFunc
	wg        sync.WaitGroup
	started   time.Time
	closeOnce sync.Once
}

// New builds the application graph. It does not start listeners; call Start.
func New(ctx context.Context, cfg config.Config, log *slog.Logger, version string) (*App, error) {
	if log == nil {
		log = slog.Default()
	}
	a := &App{
		Config: cfg, Log: log, Version: version,
		Metrics: observability.NewMetrics(), System: observability.NewSystemMonitor(),
		Health: observability.NewHealth(version), started: time.Now(),
	}
	bus, err := newEventBus(ctx, cfg.Events, log)
	if err != nil {
		return nil, err
	}
	a.Bus = bus
	a.Metrics.SetBuild(version, runtime.Version())
	for _, dir := range []string{cfg.App.DataDir, filepath.Join(cfg.App.DataDir, "keys")} {
		if err := os.MkdirAll(dir, 0o750); err != nil {
			return nil, err
		}
	}
	if err := a.initCore(ctx); err != nil {
		a.Close(context.Background())
		return nil, err
	}
	if err := a.initAI(ctx); err != nil {
		a.Close(context.Background())
		return nil, err
	}
	if err := a.initDomain(ctx); err != nil {
		a.Close(context.Background())
		return nil, err
	}
	if err := a.initAPI(); err != nil {
		a.Close(context.Background())
		return nil, err
	}
	return a, nil
}

// Handler exposes the HTTP handler (tests and custom servers).
func (a *App) Handler() http.Handler { return a.API.Routes() }
