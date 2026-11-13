package config

import (
	"strings"
	"testing"
)

func TestValidationMatrix(t *testing.T) {
	cases := []struct {
		name   string
		mutate func(*Config)
		want   bool
	}{
		{"valid", func(c *Config) {}, false},
		{"bad db driver", func(c *Config) { c.Database.Driver = "mysql" }, true},
		{"postgres dsn", func(c *Config) { c.Database.Driver = "postgres"; c.Database.DSN = "nope" }, true},
		{"bad cache", func(c *Config) { c.Cache.Driver = "memcached" }, true},
		{"redis addr", func(c *Config) { c.Cache.Driver = "redis"; c.Cache.Redis.Addr = "" }, true},
		{"bad queue", func(c *Config) { c.Queue.Driver = "kafka" }, true},
		{"workers", func(c *Config) { c.Queue.Workers = 32 }, true},
		{"bad vector", func(c *Config) { c.Vector.Driver = "weaviate" }, true},
		{"vector dim", func(c *Config) { c.Vector.Dim = 2 }, true},
		{"qdrant url", func(c *Config) { c.Vector.Driver = "qdrant" }, true},
		{"bcrypt", func(c *Config) { c.Security.BcryptCost = 3 }, true},
		{"token ttl", func(c *Config) { c.Security.AccessTTL = 0 }, true},
		{"refresh ttl", func(c *Config) { c.Security.RefreshTTL = c.Security.AccessTTL }, true},
		{"enc key", func(c *Config) { c.Security.EncryptionKey = "short" }, true},
		{"no provider", func(c *Config) { c.LLM.DefaultProvider = "missing" }, true},
		{"disabled provider", func(c *Config) {
			for i := range c.LLM.Providers {
				if c.LLM.Providers[i].Name == "local" {
					off := false
					c.LLM.Providers[i].Enabled = &off
				}
			}
		}, true},
		{"duplicate provider", func(c *Config) { c.LLM.Providers = append(c.LLM.Providers, c.LLM.Providers[0]) }, true},
		{"bad provider kind", func(c *Config) { c.LLM.Providers[0].Kind = "quantum" }, true},
		{"routing unknown", func(c *Config) { c.LLM.Routing[0].Provider = "nope" }, true},
		{"temperature", func(c *Config) { c.LLM.Temperature = 5 }, true},
		{"top_p", func(c *Config) { c.LLM.TopP = 0 }, true},
		{"max tokens", func(c *Config) { c.LLM.MaxTokens = 0 }, true},
		{"concurrency", func(c *Config) { c.LLM.MaxConcurrency = 100 }, true},
		{"tool iterations", func(c *Config) { c.LLM.MaxToolIterations = 0 }, true},
		{"short turns", func(c *Config) { c.Memory.MaxShortTurns = 0 }, true},
		{"topk", func(c *Config) { c.Memory.RetrievalTopK = 0 }, true},
		{"similarity", func(c *Config) { c.Memory.MinSimilarity = 2 }, true},
		{"context tokens", func(c *Config) { c.Memory.MaxContextTokens = 10 }, true},
		{"voice rate", func(c *Config) { c.Voice.SampleRate = 200 }, true},
		{"vad frame", func(c *Config) { c.Voice.VAD.FrameMS = 1 }, true},
		{"vad silence", func(c *Config) { c.Voice.VAD.SilenceMS = 10 }, true},
		{"stt provider", func(c *Config) { c.Voice.STT.Provider = "sphinx" }, true},
		{"tts provider", func(c *Config) { c.Voice.TTS.Provider = "festival" }, true},
		{"tts format", func(c *Config) { c.Voice.TTS.Format = "flac" }, true},
		{"search provider", func(c *Config) { c.Tools.SearchProvider = "bing" }, true},
		{"searx url", func(c *Config) { c.Tools.SearchProvider = "searxng"; c.Tools.SearxURL = "" }, true},
		{"plugins dir", func(c *Config) { c.Plugins.Dir = "" }, true},
		{"timezone", func(c *Config) { c.Scheduler.Timezone = "Mars/Olympus" }, true},
		{"server timeouts", func(c *Config) { c.Server.ReadTimeout = 0 }, true},
		{"body limit", func(c *Config) { c.Server.MaxBodyBytes = 10 }, true},
		{"db pool", func(c *Config) { c.Database.MaxOpenConns = 0 }, true},
	}
	for _, tc := range cases {
		cfg := Default()
		_ = cfg.Normalize()
		tc.mutate(&cfg)
		err := cfg.Validate()
		if tc.want && err == nil {
			t.Errorf("%s: expected validation error", tc.name)
		}
		if !tc.want && err != nil {
			t.Errorf("%s: unexpected error %v", tc.name, err)
		}
	}
}

func TestProductionRules(t *testing.T) {
	cfg := Default()
	_ = cfg.Normalize()
	cfg.App.Env = "production"
	cfg.Server.CORSOrigins = []string{"https://app.test"}
	cfg.Security.AutoGenerateKeys = false
	cfg.Security.Bootstrap.Email = ""
	if err := cfg.Validate(); err == nil {
		t.Fatal("production without secrets must fail")
	}
	cfg.Security.JWTSecret = strings.Repeat("x", 40)
	cfg.Security.EncryptionKey = strings.Repeat("ab", 32)
	cfg.Security.Bootstrap.Email = "admin@test.local"
	cfg.Security.Bootstrap.Password = "short"
	if err := cfg.Validate(); err == nil {
		t.Fatal("short production bootstrap password must fail")
	}
	cfg.Security.Bootstrap.Password = "long-enough-password"
	if err := cfg.Validate(); err != nil {
		t.Fatalf("valid production config rejected: %v", err)
	}
}
