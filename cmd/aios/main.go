// Command aios is the single binary for the AIOS core: server, migrations,
// seeding, diagnostics and utilities.
package main

import (
	"context"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"runtime/debug"
	"syscall"
	"time"

	"github.com/ArsinShaabani/AIOS/internal/app"
	"github.com/ArsinShaabani/AIOS/internal/config"
	"github.com/ArsinShaabani/AIOS/internal/platform"
	"github.com/ArsinShaabani/AIOS/internal/version"
)

func main() {
	if len(os.Args) < 2 {
		usage()
		os.Exit(2)
	}
	command := os.Args[1]
	args := os.Args[2:]
	switch command {
	case "server":
		runServer(args)
	case "migrate":
		runMigrate(args)
	case "seed":
		runSeed(args)
	case "doctor":
		runDoctor(args)
	case "llm":
		runLLMTest(args)
	case "config":
		runConfig(args)
	case "hash-password":
		runHashPassword(args)
	case "plugin-keygen":
		runPluginKeygen(args)
	case "plugin-sign":
		runPluginSign(args)
	case "version", "--version", "-v":
		fmt.Printf("AIOS %s (%s, %s)\n", version.Version, version.Commit, version.Date)
	case "help", "--help", "-h":
		usage()
	default:
		fmt.Fprintf(os.Stderr, "unknown command %q\n\n", command)
		usage()
		os.Exit(2)
	}
}

func usage() {
	fmt.Print(`AIOS — local-first AI operating system

Usage:
  aios <command> [flags]

Commands:
  server          Run the API, dashboard, voice and device services
  migrate         Apply database migrations and exit
  seed            Create presets, agents and the bootstrap administrator
  doctor          Check configuration and dependencies
  llm test        Diagnose model providers (ping + real completion)
  config          Print the effective configuration as YAML
  hash-password   Hash a password for manual seeding
  plugin-keygen   Generate an Ed25519 plugin signing key pair
  plugin-sign     Sign a plugin manifest (and optional WASM module)
  version         Print build information

Common flags:
  -config <path>  YAML configuration file (default $AIOS_CONFIG or ./config.yaml)
`)
}

func configPath(fs *flag.FlagSet, args []string) (string, error) {
	path := fs.String("config", os.Getenv("AIOS_CONFIG"), "path to config.yaml")
	if err := fs.Parse(args); err != nil {
		return "", err
	}
	if *path == "" {
		if _, err := os.Stat("config.yaml"); err == nil {
			*path = "config.yaml"
		}
	}
	return *path, nil
}

func loadConfig(path string, log *slog.Logger) (config.Config, error) {
	cfg, err := config.Load(path)
	if err != nil {
		return cfg, err
	}
	if path != "" {
		log.Info("configuration loaded", "path", path)
	}
	return cfg, nil
}

func newLogger(cfg config.Config) *slog.Logger {
	return platform.NewLogger(cfg.App.LogLevel, cfg.App.LogFormat, os.Stdout)
}

func tuneRuntime() {
	if os.Getenv("GOMEMLIMIT") == "" {
		debug.SetMemoryLimit(512 << 20)
	}
	if os.Getenv("GOGC") == "" {
		debug.SetGCPercent(60)
	}
}

func runServer(args []string) {
	fs := flag.NewFlagSet("server", flag.ExitOnError)
	path, err := configPath(fs, args)
	if err != nil {
		os.Exit(2)
	}
	bootLog := platform.NewLogger("info", "text", os.Stdout)
	cfg, err := loadConfig(path, bootLog)
	if err != nil {
		fmt.Fprintln(os.Stderr, "configuration error:", err)
		os.Exit(1)
	}
	log := newLogger(cfg)
	tuneRuntime()

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	start := time.Now()
	application, err := app.New(ctx, cfg, log, version.Version)
	if err != nil {
		log.Error("cannot start AIOS", "error", err)
		os.Exit(1)
	}
	if err := application.Start(ctx); err != nil {
		log.Error("server failed", "error", err)
		application.Close(context.Background())
		os.Exit(1)
	}
	log.Info("AIOS ready", "boot_ms", time.Since(start).Milliseconds())
	<-ctx.Done()
	log.Info("shutting down")
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	application.Close(shutdownCtx)
	log.Info("stopped")
}
