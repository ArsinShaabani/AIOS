package httpapi

import (
	"embed"
	"io/fs"
	"log/slog"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/graphql-go/graphql"

	"github.com/ArsinShaabani/AIOS/internal/adapters/storage/sqlstore"
	"github.com/ArsinShaabani/AIOS/internal/agents"
	"github.com/ArsinShaabani/AIOS/internal/brain"
	"github.com/ArsinShaabani/AIOS/internal/config"
	"github.com/ArsinShaabani/AIOS/internal/devices"
	"github.com/ArsinShaabani/AIOS/internal/documents"
	"github.com/ArsinShaabani/AIOS/internal/memory"
	"github.com/ArsinShaabani/AIOS/internal/observability"
	"github.com/ArsinShaabani/AIOS/internal/personality"
	"github.com/ArsinShaabani/AIOS/internal/plugins"
	"github.com/ArsinShaabani/AIOS/internal/ports"
	"github.com/ArsinShaabani/AIOS/internal/scheduler"
	"github.com/ArsinShaabani/AIOS/internal/security"
	"github.com/ArsinShaabani/AIOS/internal/voice"
)

//go:embed static/*
var staticFS embed.FS

// API is the HTTP composition root.
type API struct {
	Config        config.Config
	Repos         *sqlstore.Repositories
	Auth          *security.TokenService
	Auditor       *security.Auditor
	Secrets       *security.Secrets
	Brain         *brain.Engine
	Memory        *memory.Service
	Documents     *documents.Service
	Devices       *devices.Service
	Personality   *personality.Engine
	Agents        *agents.Registry
	Plugins       *plugins.Host
	Scheduler     *scheduler.Service
	Router        ports.ModelRouter
	Voice         *voice.Service
	VoiceProfiles *voice.ProfileStore
	Queue         ports.Queue
	Metrics       *observability.Metrics
	System        *observability.SystemMonitor
	Health        *observability.Health
	Bus           ports.EventBus
	Log           *slog.Logger
	Version       string
	Hub           *Hub
	LoginLimiter  *security.RateLimiter
	Limiter       *security.RateLimiter

	gqlOnce   sync.Once
	gqlSchema graphql.Schema
	gqlErr    error
}

// Routes builds the HTTP handler with all middleware.
func (a *API) Routes() http.Handler {
	mux := http.NewServeMux()

	sub, err := fs.Sub(staticFS, "static")
	if err == nil {
		mux.Handle("GET /", http.FileServer(http.FS(sub)))
	}
	mux.HandleFunc("GET /healthz", a.handleLive)
	mux.HandleFunc("GET /readyz", a.handleReady)
	if a.Config.Observability.MetricsEnabled {
		mux.Handle("GET /metrics", a.Metrics.Handler())
	}
	mux.HandleFunc("GET /api/v1/meta", a.handleMeta)
	mux.HandleFunc("GET /openapi.yaml", a.handleOpenAPI)

	a.registerAuthRoutes(mux)
	a.registerChatRoutes(mux)
	a.registerDataRoutes(mux)
	a.registerOpsRoutes(mux)
	a.registerAdminRoutes(mux)
	a.registerRealtimeRoutes(mux)
	if a.Config.Server.EnableGraphQL {
		mux.HandleFunc("POST /graphql", a.authed(a.handleGraphQL))
		mux.HandleFunc("GET /graphql", a.handleGraphQLPage)
	}
	return a.global(mux)
}

func (a *API) global(next http.Handler) http.Handler {
	h := a.recoverer(next)
	h = a.requestContext(h)
	h = a.securityHeaders(h)
	h = a.cors(h)
	h = a.rateLimit(h)
	h = a.bodyLimit(h)
	h = a.accessLog(h)
	return h
}

func (a *API) handleLive(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, a.Health.Live())
}

func (a *API) handleReady(w http.ResponseWriter, r *http.Request) {
	report := a.Health.Ready(r.Context())
	status := http.StatusOK
	if report.Status != "ok" {
		status = http.StatusServiceUnavailable
	}
	writeJSON(w, status, report)
}

func (a *API) handleMeta(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{
		"name": "AIOS", "version": a.Version, "profile": a.Config.App.Env,
		"graphql": a.Config.Server.EnableGraphQL, "voice": a.Config.Voice.Enabled,
		"time": time.Now().UTC(),
	})
}

func (a *API) handleOpenAPI(w http.ResponseWriter, r *http.Request) {
	raw, err := staticFS.ReadFile("static/openapi.yaml")
	if err != nil {
		a.fail(w, r, err)
		return
	}
	w.Header().Set("Content-Type", "application/yaml")
	_, _ = w.Write(raw)
}

func originsSet(origins []string) map[string]bool {
	out := make(map[string]bool, len(origins))
	for _, o := range origins {
		out[strings.TrimSpace(o)] = true
	}
	return out
}
