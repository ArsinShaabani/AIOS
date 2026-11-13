package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestExampleConfigLoads(t *testing.T) {
	path := filepath.Join("..", "..", "config.example.yaml")
	if _, err := os.Stat(path); err != nil {
		t.Skip("example config not present")
	}
	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("example config invalid: %v", err)
	}
	if cfg.LLM.DefaultProvider != "local" || cfg.Voice.SampleRate != 16000 {
		t.Fatalf("unexpected example provider %q", cfg.LLM.DefaultProvider)
	}
	out := cfg.String()
	if !strings.Contains(out, "providers:") || !strings.Contains(out, "memory:") {
		t.Fatal("String() did not render YAML")
	}
}

func TestEnvTypes(t *testing.T) {
	t.Setenv("AIOS_SERVER_ENABLE_GRAPHQL", "false")
	t.Setenv("AIOS_MQTT_QOS", "2")
	t.Setenv("AIOS_CACHE_DEFAULT_TTL", "90s")
	t.Setenv("AIOS_OBSERVABILITY_METRICS_ENABLED", "false")
	t.Setenv("AIOS_VECTOR_DIM", "768")
	t.Setenv("AIOS_QUEUE_WORKERS", "3")
	t.Setenv("AIOS_SERVER_CORS_ORIGINS", "https://a.test, https://b.test")
	cfg, err := Load("")
	if err != nil {
		t.Fatalf("env config: %v", err)
	}
	if cfg.Server.EnableGraphQL || cfg.MQTT.QoS != 2 || cfg.Vector.Dim != 768 || cfg.Queue.Workers != 3 {
		t.Fatalf("env not applied: graphql=%v qos=%d dim=%d workers=%d",
			cfg.Server.EnableGraphQL, cfg.MQTT.QoS, cfg.Vector.Dim, cfg.Queue.Workers)
	}
	if cfg.Cache.DefaultTTL.Std() != 90*time.Second || cfg.Observability.MetricsEnabled {
		t.Fatal("duration/bool env not applied")
	}
	if len(cfg.Server.CORSOrigins) != 2 {
		t.Fatalf("cors: %v", cfg.Server.CORSOrigins)
	}
}

func TestNormalizeDefaults(t *testing.T) {
	cfg := Config{}
	if err := cfg.Normalize(); err != nil {
		t.Fatal(err)
	}
	if cfg.App.Name != "AIOS" || cfg.Server.Addr == "" || cfg.Security.Issuer != "aios" {
		t.Fatalf("normalize: %+v", cfg.App)
	}
	if cfg.LLM.DefaultProvider != "local" || cfg.Voice.SampleRate != 16000 || cfg.Vector.Dim != 384 {
		t.Fatal("derived defaults missing")
	}
	if cfg.Tools.UserAgent == "" || cfg.Scheduler.Timezone != "UTC" {
		t.Fatal("tool/scheduler defaults missing")
	}
}

func TestDurationAndPlaceholders(t *testing.T) {
	var d Duration
	if err := d.UnmarshalText([]byte("45s")); err != nil || d.Std() != 45*time.Second {
		t.Fatalf("duration: %v %v", d, err)
	}
	if _, err := d.MarshalYAML(); err != nil {
		t.Fatal(err)
	}
	if b, err := d.MarshalText(); err != nil || string(b) != "45s" {
		t.Fatalf("marshal text: %s %v", b, err)
	}
	if err := d.UnmarshalText([]byte("bad")); err == nil {
		t.Fatal("invalid duration accepted")
	}
	if expandPlaceholders("x ${MISSING_VAR} y") != "x  y" {
		t.Fatal("placeholder expansion failed")
	}
}
