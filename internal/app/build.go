package app

import (
	"context"
	"fmt"

	"github.com/ArsinShaabani/AIOS/internal/adapters/cache"
	llmadapter "github.com/ArsinShaabani/AIOS/internal/adapters/llm"
	"github.com/ArsinShaabani/AIOS/internal/adapters/queue"
	"github.com/ArsinShaabani/AIOS/internal/adapters/storage/sqlstore"
	"github.com/ArsinShaabani/AIOS/internal/adapters/vector"
	"github.com/ArsinShaabani/AIOS/internal/agents"
	"github.com/ArsinShaabani/AIOS/internal/brain"
	"github.com/ArsinShaabani/AIOS/internal/documents"
	"github.com/ArsinShaabani/AIOS/internal/llm"
	"github.com/ArsinShaabani/AIOS/internal/memory"
	"github.com/ArsinShaabani/AIOS/internal/personality"
	"github.com/ArsinShaabani/AIOS/internal/platform"
	"github.com/ArsinShaabani/AIOS/internal/plugins"
	"github.com/ArsinShaabani/AIOS/internal/security"
	"github.com/ArsinShaabani/AIOS/internal/tools"
	"github.com/ArsinShaabani/AIOS/internal/transport/httpapi"
	"github.com/ArsinShaabani/AIOS/internal/voice"
)

func (a *App) initCore(ctx context.Context) error {
	db, err := sqlstore.Open(ctx, a.Config.Database, a.Config.App.DataDir, a.Log)
	if err != nil {
		return err
	}
	a.DB = db
	a.Repos = sqlstore.NewRepositories(db)
	keyring, err := security.LoadOrCreate(a.Config.Security, a.Config.App.DataDir)
	if err != nil {
		return err
	}
	a.Keyring = keyring
	cacheImpl, err := cache.New(ctx, a.Config.Cache)
	if err != nil {
		return err
	}
	a.Cache = cacheImpl
	a.Vector = vector.New(a.Config.Vector)
	queueImpl, err := queue.New(a.Config.Queue, a.Config.Cache, a.Log)
	if err != nil {
		return err
	}
	a.Queue = queueImpl
	a.Health.Add("database", func(ctx context.Context) error { return a.DB.PingContext(ctx) })
	a.Health.Add("cache", func(ctx context.Context) error { return a.Cache.Ping(ctx) })
	a.Health.Add("vector", func(ctx context.Context) error { return a.Vector.Ping(ctx) })
	return nil
}

func (a *App) initAI(ctx context.Context) error {
	providers, errs := llmadapter.BuildAll(a.Config.LLM.Providers)
	for _, err := range errs {
		a.Log.Warn("model provider disabled", "error", err)
	}
	if _, ok := providers[a.Config.LLM.DefaultProvider]; !ok {
		return fmt.Errorf("default provider %q could not be constructed", a.Config.LLM.DefaultProvider)
	}
	a.Usage = NewUsageRecorder(a.Repos.Usage, a.Log)
	a.Router = llm.NewRouter(a.Config.LLM, providers, a.Cache, a.Usage, a.Metrics, a.Log)
	a.Tokens = security.NewTokenService(a.Config.Security, a.Keyring, a.Repos.Tokens, a.Repos.Users)
	a.Auditor = security.NewAuditor(a.Repos.Audit, a.Log)
	a.Secrets = security.NewSecrets(a.Repos.Settings, a.Keyring)
	a.Memory = memory.New(a.Repos.Memories, a.Repos.Sessions, a.Repos.Messages, a.Repos.Users,
		a.Vector, a.Cache, a.Router, a.Config.Memory, a.Config.Vector.Dim, a.Log)
	a.Documents = documents.New(a.Repos.Documents, a.Vector, a.Router, a.Config.RAG, a.Config.Vector.Dim, a.Log)
	a.Personality = personality.New(a.Repos.Personalities)
	a.Agents = agents.New(a.Repos.Agents, a.Log)
	return nil
}

func (a *App) initAPI() error {
	a.API = &httpapi.API{
		Config: a.Config, Repos: a.Repos, Auth: a.Tokens, Auditor: a.Auditor, Secrets: a.Secrets,
		Brain: a.Brain, Memory: a.Memory, Documents: a.Documents, Devices: a.Devices, Personality: a.Personality,
		Agents: a.Agents, Plugins: a.Plugins, Scheduler: a.Scheduler, Router: a.Router,
		Voice: a.Voice, Queue: a.Queue, Metrics: a.Metrics, System: a.System, Health: a.Health,
		Bus: a.Bus, Log: a.Log, Version: a.Version,
		VoiceProfiles: voice.NewProfileStore(a.Repos.Settings),
		Limiter:       security.NewRateLimiter(a.Config.Server.RateLimit.RequestsPerMinute, a.Config.Server.RateLimit.Burst),
		LoginLimiter:  security.NewRateLimiter(a.Config.Server.RateLimit.AuthPerMinute, max(a.Config.Server.RateLimit.AuthPerMinute, 5)),
	}
	if !a.Config.Server.RateLimit.Enabled {
		a.API.Limiter = nil
		a.API.LoginLimiter = nil
	}
	a.Health.Add("queue", func(ctx context.Context) error {
		_, err := a.Queue.Stats(ctx)
		return err
	})
	a.Health.Add("events", func(ctx context.Context) error {
		if conn, ok := a.Bus.(interface{ Connected() bool }); ok && !conn.Connected() {
			return platform.Unavailable("events.disconnected", "event bus is disconnected")
		}
		return nil
	})
	return nil
}

var _ = brain.Engine{}
var _ = memory.Service{}
var _ = personality.Engine{}
var _ = plugins.Host{}
var _ = tools.Registry{}
var _ = voice.Service{}
var _ = agents.Registry{}
