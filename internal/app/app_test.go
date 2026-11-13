package app

import (
	"context"
	"log/slog"
	"os"
	"path/filepath"
	"testing"

	"github.com/ArsinShaabani/AIOS/internal/config"
	"github.com/ArsinShaabani/AIOS/internal/domain"
	"github.com/ArsinShaabani/AIOS/internal/ports"
)

func testConfig(t *testing.T) config.Config {
	t.Helper()
	cfg := config.Default()
	cfg.App.DataDir = t.TempDir()
	cfg.Database.DSN = "app.db"
	cfg.Security.BcryptCost = 8
	cfg.Security.Bootstrap = config.BootstrapUser{Email: "admin@test.local", Password: "AdminPassword123!"}
	cfg.Voice.STT.Provider = "mock"
	cfg.Voice.TTS.Provider = "mock"
	cfg.Tools.SearchProvider = "mock"
	cfg.Plugins.Dir = t.TempDir()
	if err := cfg.Validate(); err != nil {
		t.Fatal(err)
	}
	return cfg
}

func TestSeedIsIdempotentAndGeneratesPassword(t *testing.T) {
	cfg := testConfig(t)
	cfg.Security.Bootstrap.Password = "" // must generate and log once
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))
	a, err := New(context.Background(), cfg, logger, "test")
	if err != nil {
		t.Fatal(err)
	}
	defer a.Close(context.Background())
	if err := a.Seed(context.Background()); err != nil {
		t.Fatal(err)
	}
	if n, _ := a.Repos.Users.Count(context.Background()); n != 1 {
		t.Fatalf("users after seed: %d", n)
	}
	if err := a.Seed(context.Background()); err != nil {
		t.Fatal(err)
	}
	if n, _ := a.Repos.Users.Count(context.Background()); n != 1 {
		t.Fatalf("seed not idempotent: %d", n)
	}
}

func TestNewRejectsBadProviderAndDataDir(t *testing.T) {
	cfg := testConfig(t)
	cfg.LLM.Providers = []config.ProviderConfig{{Name: "local", Kind: "quantum", Enabled: boolTrue()}}
	if _, err := New(context.Background(), cfg, slog.Default(), "test"); err == nil {
		t.Fatal("invalid provider accepted")
	}

	dir := t.TempDir()
	file := filepath.Join(dir, "file")
	if err := os.WriteFile(file, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	cfg2 := testConfig(t)
	cfg2.App.DataDir = file
	if _, err := New(context.Background(), cfg2, slog.Default(), "test"); err == nil {
		t.Fatal("file as data dir accepted")
	}
}

func boolTrue() *bool { b := true; return &b }

func TestBuildEmailAndTools(t *testing.T) {
	cfg := testConfig(t)
	a, err := New(context.Background(), cfg, slog.Default(), "test")
	if err != nil {
		t.Fatal(err)
	}
	defer a.Close(context.Background())
	ctx := context.Background()
	if a.buildEmail(ctx) != nil {
		t.Fatal("email configured unexpectedly")
	}
	if err := a.Repos.Settings.Set(ctx, &domain.Setting{
		Scope: "global", Key: "smtp", Value: []byte(`{"host":"smtp.test","port":2525,"from":"aios@test.local"}`),
	}); err != nil {
		t.Fatal(err)
	}
	if err := a.Secrets.Set(ctx, "smtp.password", "secret"); err != nil {
		t.Fatal(err)
	}
	if a.buildEmail(ctx) == nil {
		t.Fatal("email sender not built from settings")
	}
	if a.mqttPublisher() != nil {
		t.Fatal("mqtt publisher should be nil without a broker")
	}
	// Tools wired by the composition root are available for the general agent.
	if _, ok := a.Tools.Get("web_search"); !ok {
		t.Fatal("web_search missing")
	}
	if _, ok := a.Tools.Get("device_command"); !ok {
		t.Fatal("device_command missing")
	}
	// Register a custom tool and ensure it is visible through the registry.
	if err := a.Tools.Register(ports.Tool{Name: "custom", Executor: func(context.Context, ports.ToolCallContext) (ports.ToolResult, error) {
		return ports.ToolResult{Content: "ok"}, nil
	}}); err != nil {
		t.Fatal(err)
	}
}
