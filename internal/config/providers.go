package config

func boolPtr(b bool) *bool        { return &b }
func floatPtr(f float64) *float64 { return &f }

// defaultProviders ships a fully offline default ("local") plus disabled
// presets. Operators enable a preset and drop in a key/base URL; the core never
// needs to change. All presets use the openai-compatible adapter except
// Anthropic and Gemini, which have distinct wire formats.
func defaultProviders() []ProviderConfig {
	return []ProviderConfig{
		{
			Name:         "local",
			Kind:         "local",
			DefaultModel: "local-template",
			Enabled:      boolPtr(true),
			Priority:     100,
			EmbeddingDim: 384,
			Models: []ModelConfig{
				{Name: "local-template", ContextWindow: 8192, MaxOutputTokens: 1024, PromptPer1K: 0, CompletionPer1K: 0},
			},
		},
		{
			Name:         "ollama",
			Kind:         "ollama",
			BaseURL:      "http://127.0.0.1:11434",
			DefaultModel: "qwen2.5:1.5b",
			Enabled:      boolPtr(false),
			Priority:     10,
			Models: []ModelConfig{
				{Name: "qwen2.5:1.5b", ContextWindow: 32768, MaxOutputTokens: 2048},
				{Name: "llama3.2:1b", ContextWindow: 131072, MaxOutputTokens: 2048},
				{Name: "phi3.5", ContextWindow: 131072, MaxOutputTokens: 2048},
			},
		},
		{
			Name:         "openai",
			Kind:         "openai",
			BaseURL:      "https://api.openai.com/v1",
			DefaultModel: "gpt-4o-mini",
			Enabled:      boolPtr(false),
			Priority:     20,
			Models: []ModelConfig{
				{Name: "gpt-4o-mini", ContextWindow: 128000, MaxOutputTokens: 4096, PromptPer1K: 0.00015, CompletionPer1K: 0.0006},
				{Name: "gpt-4.1-mini", ContextWindow: 128000, MaxOutputTokens: 4096, PromptPer1K: 0.0004, CompletionPer1K: 0.0016},
				{Name: "text-embedding-3-small", ContextWindow: 8191, PromptPer1K: 0.00002},
			},
		},
		{
			Name:         "anthropic",
			Kind:         "anthropic",
			BaseURL:      "https://api.anthropic.com",
			DefaultModel: "claude-3-5-haiku-latest",
			Enabled:      boolPtr(false),
			Priority:     30,
			Models: []ModelConfig{
				{Name: "claude-3-5-haiku-latest", ContextWindow: 200000, MaxOutputTokens: 8192, PromptPer1K: 0.0008, CompletionPer1K: 0.004},
				{Name: "claude-sonnet-4-5", ContextWindow: 200000, MaxOutputTokens: 8192, PromptPer1K: 0.003, CompletionPer1K: 0.015},
			},
		},
		{
			Name:         "gemini",
			Kind:         "gemini",
			BaseURL:      "https://generativelanguage.googleapis.com/v1beta",
			DefaultModel: "gemini-2.0-flash",
			Enabled:      boolPtr(false),
			Priority:     40,
			Models: []ModelConfig{
				{Name: "gemini-2.0-flash", ContextWindow: 1000000, MaxOutputTokens: 8192, PromptPer1K: 0.0001, CompletionPer1K: 0.0004},
				{Name: "text-embedding-004", ContextWindow: 2048, PromptPer1K: 0},
			},
		},
		{
			Name:         "deepseek",
			Kind:         "openai",
			BaseURL:      "https://api.deepseek.com/v1",
			DefaultModel: "deepseek-chat",
			Enabled:      boolPtr(false),
			Priority:     50,
			Models: []ModelConfig{
				{Name: "deepseek-chat", ContextWindow: 64000, MaxOutputTokens: 8192, PromptPer1K: 0.00027, CompletionPer1K: 0.0011},
			},
		},
		{
			Name:         "mistral",
			Kind:         "openai",
			BaseURL:      "https://api.mistral.ai/v1",
			DefaultModel: "mistral-small-latest",
			Enabled:      boolPtr(false),
			Priority:     60,
			Models: []ModelConfig{
				{Name: "mistral-small-latest", ContextWindow: 128000, MaxOutputTokens: 4096, PromptPer1K: 0.0002, CompletionPer1K: 0.0006},
			},
		},
	}
}

// defaultRouting demonstrates intent-aware model routing without requiring any
// paid provider: rules only change token budgets/temperature unless the
// operator pins a provider/model.
func defaultRouting() []RoutingRule {
	return []RoutingRule{
		{
			Name:        "voice-fast",
			Keywords:    []string{"alarm", "timer", "remind", "turn on", "turn off", "light", "switch"},
			MaxTokens:   512,
			Temperature: floatPtr(0.4),
		},
		{
			Name:        "deep-research",
			Intents:     []string{"research", "analysis", "compare", "investigate", "plan"},
			MaxTokens:   2048,
			Temperature: floatPtr(0.3),
		},
		{
			Name:        "code",
			Intents:     []string{"code", "debug", "programming"},
			MaxTokens:   2048,
			Temperature: floatPtr(0.2),
		},
	}
}
