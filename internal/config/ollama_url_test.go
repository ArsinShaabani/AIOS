package config

import "testing"

func TestNormalizeOllamaBaseURL(t *testing.T) {
	cases := []struct{ in, want string }{
		{"http://127.0.0.1:11434", "http://127.0.0.1:11434/v1"},
		{"http://127.0.0.1:11434/", "http://127.0.0.1:11434/v1"},
		{"http://ollama.lan:11434/v1", "http://ollama.lan:11434/v1"},
		{"https://ollama.example.com/v1/", "https://ollama.example.com/v1"},
	}
	for _, tc := range cases {
		cfg := Default()
		cfg.LLM.Providers = []ProviderConfig{{Name: "ollama", Kind: "ollama", Enabled: boolPtr(true), BaseURL: tc.in, DefaultModel: "qwen2.5:1.5b"}}
		cfg.LLM.DefaultProvider = "ollama"
		cfg.Normalize()
		if got := cfg.LLM.Providers[0].BaseURL; got != tc.want {
			t.Fatalf("base_url %q normalized to %q, want %q", tc.in, got, tc.want)
		}
	}

	// Other kinds must not be rewritten.
	cfg := Default()
	cfg.LLM.Providers = []ProviderConfig{{Name: "openai", Kind: "openai", Enabled: boolPtr(true), BaseURL: "https://api.openai.com/v1"}}
	cfg.LLM.DefaultProvider = "openai"
	cfg.Normalize()
	if got := cfg.LLM.Providers[0].BaseURL; got != "https://api.openai.com/v1" {
		t.Fatalf("openai base_url changed: %q", got)
	}
}
