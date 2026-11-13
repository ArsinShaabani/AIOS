package app

import (
	"context"
	"crypto/tls"
	"errors"
	"net/http"
	"time"
)

// Start launches background workers, seeds defaults and serves HTTP.
func (a *App) Start(ctx context.Context) error {
	ctx, cancel := context.WithCancel(ctx)
	a.cancel = cancel

	a.Bus.Start(ctx)
	a.API.HubStart(ctx)
	a.Usage.Start(ctx)
	a.Auditor.Start(ctx)
	if err := a.Seed(ctx); err != nil {
		a.Log.Warn("bootstrap seeding failed", "error", err)
	}
	a.registerJobs()
	a.Queue.Start(ctx)
	a.Scheduler.Start(ctx)
	if err := a.Memory.Warm(ctx); err != nil {
		a.Log.Warn("semantic memory warm-up failed", "error", err)
	}
	if a.Plugins != nil && a.Config.Plugins.Enabled {
		if n, err := a.Plugins.LoadDir(ctx); err != nil {
			a.Log.Warn("cannot load plugin manifests", "error", err)
		} else if n > 0 {
			a.Log.Info("plugins loaded from disk", "count", n)
		}
		if err := a.Plugins.Reload(ctx); err != nil {
			a.Log.Warn("cannot load plugins", "error", err)
		}
	}
	a.startMQTTBridge(ctx)
	a.startMetricsLoop(ctx)

	a.HTTP = &http.Server{
		Addr:           a.Config.Server.Addr,
		Handler:        a.Handler(),
		ReadTimeout:    a.Config.Server.ReadTimeout.Std(),
		WriteTimeout:   a.Config.Server.WriteTimeout.Std(),
		IdleTimeout:    a.Config.Server.IdleTimeout.Std(),
		MaxHeaderBytes: 1 << 20,
	}
	listenErr := make(chan error, 1)
	go func() {
		a.Log.Info("aios listening", "addr", a.Config.Server.Addr, "version", a.Version, "env", a.Config.App.Env)
		var err error
		if a.Config.Server.TLS.Enabled {
			a.HTTP.TLSConfig = &tls.Config{MinVersion: tls.VersionTLS12}
			err = a.HTTP.ListenAndServeTLS(a.Config.Server.TLS.CertFile, a.Config.Server.TLS.KeyFile)
		} else {
			err = a.HTTP.ListenAndServe()
		}
		if err != nil && !errors.Is(err, http.ErrServerClosed) {
			listenErr <- err
		}
	}()
	select {
	case err := <-listenErr:
		return err
	case <-time.After(150 * time.Millisecond):
		return nil
	}
}

// Close shuts everything down in reverse order. It is idempotent.
func (a *App) Close(ctx context.Context) {
	a.closeOnce.Do(func() {
		a.close(ctx)
	})
}

func (a *App) close(ctx context.Context) {
	if a.cancel != nil {
		a.cancel()
	}
	if a.HTTP != nil {
		timeout := a.Config.Server.ShutdownTimeout.Std()
		if timeout <= 0 {
			timeout = 15 * time.Second
		}
		shutdownCtx, cancel := context.WithTimeout(ctx, timeout)
		_ = a.HTTP.Shutdown(shutdownCtx)
		cancel()
	}
	if a.Scheduler != nil {
		a.Scheduler.Stop(ctx)
	}
	if a.Queue != nil {
		a.Queue.Stop(ctx)
	}
	if a.MQTT != nil {
		a.MQTT.Close()
	}
	if a.Bus != nil {
		a.Bus.Stop()
	}
	if a.Usage != nil {
		a.Usage.Stop()
	}
	if a.Auditor != nil {
		a.Auditor.Stop(ctx)
	}
	if a.Cache != nil {
		_ = a.Cache.Close()
	}
	a.wg.Wait()
	if a.Repos != nil {
		_ = a.Repos.Close()
	}
}

func (a *App) startMetricsLoop(ctx context.Context) {
	interval := a.Config.Observability.SystemStatsEvery.Std()
	if interval <= 0 {
		interval = 30 * time.Second
	}
	a.wg.Add(1)
	go func() {
		defer a.wg.Done()
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				if stats, err := a.Queue.Stats(ctx); err == nil {
					a.Metrics.SetQueue(stats.Depth, stats.Running)
				}
				if statuses, err := a.Repos.Devices.CountByStatus(ctx); err == nil {
					a.Metrics.SetDevices(statuses)
				}
				memories, _ := a.Repos.Memories.Count(ctx)
				vectors, _ := a.Vector.Count(ctx)
				a.Metrics.SetMemory(memories, vectors)
			}
		}
	}()
}
