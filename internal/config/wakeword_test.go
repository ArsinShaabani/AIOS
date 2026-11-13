package config

import "testing"

func TestWakeWordValidation(t *testing.T) {
	cases := []struct {
		name   string
		mutate func(*Config)
		valid  bool
	}{
		{"text", func(c *Config) { c.Voice.WakeWord.Provider = "text" }, true},
		{"openwakeword", func(c *Config) {
			c.Voice.WakeWord.Provider = "openwakeword_http"
			c.Voice.WakeWord.Endpoint = "http://127.0.0.1:9000/wake"
		}, true},
		{"missing endpoint", func(c *Config) { c.Voice.WakeWord.Provider = "openwakeword_http" }, false},
		{"bad provider", func(c *Config) { c.Voice.WakeWord.Provider = "porcupine" }, false},
		{"bad threshold", func(c *Config) { c.Voice.WakeWord.Threshold = 2 }, false},
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
}
