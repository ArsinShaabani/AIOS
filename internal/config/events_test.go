package config

import "testing"

func TestEventsAndConsumerGroupValidation(t *testing.T) {
	cases := []struct {
		name   string
		mutate func(*Config)
		valid  bool
	}{
		{"memory events", func(c *Config) { c.Events.Driver = "memory" }, true},
		{"nats events", func(c *Config) { c.Events.Driver = "nats"; c.Events.URL = "nats://x:4222" }, true},
		{"bad events driver", func(c *Config) { c.Events.Driver = "kafka" }, false},
		{"bad subject prefix", func(c *Config) { c.Events.SubjectPrefix = "aios.*" }, false},
		{"consumer group ok", func(c *Config) {
			c.MQTT.Enabled = true
			c.MQTT.Bridge.Enabled = true
			c.MQTT.Bridge.ConsumerGroup = "aios-core"
		}, true},
		{"consumer group wildcard", func(c *Config) {
			c.MQTT.Enabled = true
			c.MQTT.Bridge.Enabled = true
			c.MQTT.Bridge.ConsumerGroup = "bad/group"
		}, false},
		{"bridge without mqtt", func(c *Config) { c.MQTT.Bridge.Enabled = true }, false},
		{"rag chunk overlap", func(c *Config) { c.RAG.ChunkOverlap = c.RAG.ChunkSize }, false},
		{"rag min score", func(c *Config) { c.RAG.MinScore = 2 }, false},
	}
	for _, tc := range cases {
		cfg := Default()
		_ = cfg.Normalize()
		tc.mutate(&cfg)
		err := cfg.Validate()
		if tc.valid && err != nil {
			t.Errorf("%s: unexpected error %v", tc.name, err)
		}
		if !tc.valid && err == nil {
			t.Errorf("%s: expected validation error", tc.name)
		}
	}
	// Consumer groups are normalised to a single dash-separated token.
	cfg := Default()
	cfg.MQTT.Bridge.ConsumerGroup = "  my group  "
	_ = cfg.Normalize()
	if cfg.MQTT.Bridge.ConsumerGroup != "my-group" {
		t.Fatalf("normalise group: %q", cfg.MQTT.Bridge.ConsumerGroup)
	}
}
