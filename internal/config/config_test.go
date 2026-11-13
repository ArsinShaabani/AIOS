package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDefaultsAreValid(t *testing.T) {
	cfg, err := Load("")
	if err != nil {
		t.Fatalf("default configuration must be valid: %v", err)
	}
	if cfg.LLM.DefaultProvider != "local" {
		t.Fatalf("expected local default provider, got %s", cfg.LLM.DefaultProvider)
	}
	if cfg.Vector.Dim != 384 {
		t.Fatalf("expected 384-dim vectors, got %d", cfg.Vector.Dim)
	}
	if cfg.Server.RateLimit.RequestsPerMinute == 0 {
		t.Fatal("rate limiting should be enabled by default")
	}
}

func TestEnvOverridesAndPlaceholders(t *testing.T) {
	t.Setenv("AIOS_SERVER_ADDR", "0.0.0.0:9999")
	t.Setenv("AIOS_SECURITY_BCRYPT_COST", "12")
	t.Setenv("AIOS_LLM_TEMPERATURE", "0.2")
	t.Setenv("AIOS_SERVER_CORS_ORIGINS", "https://a.example,https://b.example")
	t.Setenv("TEST_BOOTSTRAP_PASSWORD", "S3cretPassword!")
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	yaml := "security:\n  bootstrap:\n    email: admin@example.com\n    password: ${TEST_BOOTSTRAP_PASSWORD}\n"
	if err := os.WriteFile(path, []byte(yaml), 0o600); err != nil {
		t.Fatal(err)
	}
	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if cfg.Server.Addr != "0.0.0.0:9999" || cfg.Security.BcryptCost != 12 || cfg.LLM.Temperature != 0.2 {
		t.Fatalf("env overrides not applied: %+v", cfg.Server)
	}
	if len(cfg.Server.CORSOrigins) != 2 {
		t.Fatalf("cors list not parsed: %v", cfg.Server.CORSOrigins)
	}
	if cfg.Security.Bootstrap.Password != "S3cretPassword!" {
		t.Fatalf("placeholder not expanded: %q", cfg.Security.Bootstrap.Password)
	}
}

func TestValidationRejectsBadConfig(t *testing.T) {
	cfg := Default()
	cfg.Database.Driver = "mysql"
	if err := cfg.Normalize(); err != nil {
		t.Fatal(err)
	}
	if err := cfg.Validate(); err == nil {
		t.Fatal("expected validation failure")
	}
	cfg = Default()
	cfg.LLM.DefaultProvider = "missing"
	if err := cfg.Validate(); err == nil {
		t.Fatal("unknown default provider must fail")
	}
	cfg = Default()
	cfg.App.Env = "production"
	cfg.Server.CORSOrigins = []string{"*"}
	if err := cfg.Validate(); err == nil {
		t.Fatal("wildcard CORS must fail in production")
	}
}

func TestUnknownYAMLKeysRejected(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "bad.yaml")
	if err := os.WriteFile(path, []byte("serverr:\n  addr: :1\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := Load(path); err == nil {
		t.Fatal("typo in config should be rejected")
	}
}
