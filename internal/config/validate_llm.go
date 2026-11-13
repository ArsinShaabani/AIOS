package config

// validateLLM validates the model layer and routing table.
func (c *Config) validateLLM(add func(string, ...any)) {
	if c.LLM.Temperature < 0 || c.LLM.Temperature > 2 {
		add("llm.temperature must be within [0,2]")
	}
	if c.LLM.TopP <= 0 || c.LLM.TopP > 1 {
		add("llm.top_p must be within (0,1]")
	}
	if c.LLM.MaxTokens <= 0 {
		add("llm.max_tokens must be positive")
	}
	if c.LLM.MaxConcurrency < 1 || c.LLM.MaxConcurrency > 64 {
		add("llm.max_concurrency must be within [1,64]")
	}
	if c.LLM.MaxToolIterations < 1 || c.LLM.MaxToolIterations > 10 {
		add("llm.max_tool_iterations must be within [1,10]")
	}
	seen := map[string]bool{}
	for _, p := range c.LLM.Providers {
		if p.Name == "" {
			add("llm provider with empty name")
			continue
		}
		if seen[p.Name] {
			add("duplicate llm provider %q", p.Name)
		}
		seen[p.Name] = true
		switch p.Kind {
		case "openai", "anthropic", "gemini", "ollama", "local":
		default:
			add("provider %q has unsupported kind %q", p.Name, p.Kind)
		}
	}
	if !seen[c.LLM.DefaultProvider] {
		add("llm.default_provider %q is not configured", c.LLM.DefaultProvider)
	} else if !c.providerEnabled(c.LLM.DefaultProvider) {
		add("llm.default_provider %q is disabled", c.LLM.DefaultProvider)
	}
	if !seen[c.LLM.EmbeddingProvider] {
		add("llm.embedding_provider %q is not configured", c.LLM.EmbeddingProvider)
	}
	for _, rule := range c.LLM.Routing {
		if rule.Provider != "" && !seen[rule.Provider] {
			add("routing rule %q references unknown provider %q", rule.Name, rule.Provider)
		}
	}
}
