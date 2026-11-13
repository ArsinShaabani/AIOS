package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/ArsinShaabani/AIOS/internal/app"
	"github.com/ArsinShaabani/AIOS/internal/config"
	"github.com/ArsinShaabani/AIOS/internal/security"
	"github.com/ArsinShaabani/AIOS/internal/version"
)

func runMigrate(args []string) {
	fs := flag.NewFlagSet("migrate", flag.ExitOnError)
	path, _ := configPath(fs, args)
	cfg, err := config.Load(path)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	application, err := app.New(context.Background(), cfg, newLogger(cfg), version.Version)
	if err != nil {
		fmt.Fprintln(os.Stderr, "migrate:", err)
		os.Exit(1)
	}
	defer application.Close(context.Background())
	fmt.Println("migrations applied")
}

func runSeed(args []string) {
	fs := flag.NewFlagSet("seed", flag.ExitOnError)
	path, _ := configPath(fs, args)
	cfg, err := config.Load(path)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	application, err := app.New(context.Background(), cfg, newLogger(cfg), version.Version)
	if err != nil {
		fmt.Fprintln(os.Stderr, "seed:", err)
		os.Exit(1)
	}
	defer application.Close(context.Background())
	if err := application.Seed(context.Background()); err != nil {
		fmt.Fprintln(os.Stderr, "seed:", err)
		os.Exit(1)
	}
	fmt.Println("seed complete")
}

func runDoctor(args []string) {
	fs := flag.NewFlagSet("doctor", flag.ExitOnError)
	path, _ := configPath(fs, args)
	cfg, err := config.Load(path)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	log := newLogger(cfg)
	application, err := app.New(context.Background(), cfg, log, version.Version)
	if err != nil {
		fmt.Fprintln(os.Stderr, "doctor:", err)
		os.Exit(1)
	}
	defer application.Close(context.Background())

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	report := map[string]any{
		"version":   version.Version,
		"profile":   cfg.App.Env,
		"providers": application.Router.Providers(),
		"voice": map[string]any{
			"enabled": cfg.Voice.Enabled, "stt": cfg.Voice.STT.Provider, "tts": cfg.Voice.TTS.Provider,
		},
	}
	ready := application.Health.Ready(ctx)
	report["health"] = ready
	if ready.Status != "ok" {
		report["status"] = "degraded"
	} else {
		report["status"] = "ok"
	}
	if application.MQTT != nil {
		report["mqtt"] = application.MQTT.Connected()
	}
	raw, _ := json.MarshalIndent(report, "", "  ")
	fmt.Println(string(raw))
	if ready.Status != "ok" {
		os.Exit(1)
	}
}

func runConfig(args []string) {
	fs := flag.NewFlagSet("config", flag.ExitOnError)
	path, _ := configPath(fs, args)
	cfg, err := config.Load(path)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	fmt.Print(cfg.String())
}

func runHashPassword(args []string) {
	fs := flag.NewFlagSet("hash-password", flag.ExitOnError)
	password := fs.String("password", "", "password to hash (omit to read from stdin)")
	cost := fs.Int("cost", 10, "bcrypt cost")
	_ = fs.Parse(args)
	value := *password
	if value == "" {
		fmt.Fprint(os.Stderr, "password: ")
		var input string
		_, _ = fmt.Scanln(&input)
		value = strings.TrimSpace(input)
	}
	hash, err := security.HashPassword(value, *cost)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	fmt.Println(hash)
}
