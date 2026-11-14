package providers

// Preset is a one-click provider template shown in the GUI.
type Preset struct {
	ID          string   `json:"id"`
	Label       string   `json:"label"`
	Kind        string   `json:"kind"`
	BaseURL     string   `json:"base_url"`
	Model       string   `json:"model"`
	Models      []string `json:"models"`
	KeyRequired bool     `json:"key_required"`
	FreeTier    bool     `json:"free_tier"`
	Local       bool     `json:"local"`
	Note        string   `json:"note"`
	Homepage    string   `json:"homepage"`
}

// Catalog lists keyless options first, then free tiers that need a signup key.
func Catalog() []Preset {
	return []Preset{
		{ID: "ollama", Label: "Ollama (local)", Kind: "ollama", BaseURL: "http://127.0.0.1:11434",
			Model: "qwen2.5:0.5b", Models: []string{"qwen2.5:0.5b", "qwen2.5:1.5b", "llama3.2:1b", "gemma3:270m"},
			Local: true, Note: "No key, runs on your machine. Fastest on CPU.", Homepage: "https://ollama.com"},
		{ID: "lmstudio", Label: "LM Studio (local)", Kind: "openai", BaseURL: "http://127.0.0.1:1234/v1",
			Model: "local-model", Local: true, Note: "OpenAI-compatible local server.", Homepage: "https://lmstudio.ai"},
		{ID: "llamacpp", Label: "llama.cpp server", Kind: "openai", BaseURL: "http://127.0.0.1:8080/v1",
			Model: "local", Local: true, Note: "llama-server exposes an OpenAI API.", Homepage: "https://github.com/ggml-org/llama.cpp"},
		{ID: "vllm", Label: "vLLM (GPU host)", Kind: "openai", BaseURL: "http://127.0.0.1:8000/v1",
			Model: "local", Local: true, Note: "High-throughput OpenAI-compatible server.", Homepage: "https://docs.vllm.ai"},
		{ID: "pollinations", Label: "Pollinations (no key)", Kind: "openai", BaseURL: "https://text.pollinations.ai/openai",
			Model: "openai", Models: []string{"openai", "mistral", "llama", "gemma"}, FreeTier: true,
			Note: "Public and rate-limited, no signup. Do not send private data.", Homepage: "https://pollinations.ai"},
		{ID: "groq", Label: "Groq", Kind: "openai", BaseURL: "https://api.groq.com/openai/v1",
			Model: "llama-3.3-70b-versatile", Models: []string{"llama-3.3-70b-versatile", "llama-3.1-8b-instant"},
			KeyRequired: true, FreeTier: true, Note: "Free tier, very fast.", Homepage: "https://console.groq.com"},
		{ID: "gemini", Label: "Google Gemini", Kind: "gemini", BaseURL: "https://generativelanguage.googleapis.com/v1beta",
			Model: "gemini-2.0-flash", Models: []string{"gemini-2.0-flash", "gemini-1.5-flash"},
			KeyRequired: true, FreeTier: true, Note: "Generous free tier.", Homepage: "https://aistudio.google.com/apikey"},
		{ID: "openrouter", Label: "OpenRouter", Kind: "openai", BaseURL: "https://openrouter.ai/api/v1",
			Model:       "meta-llama/llama-3.3-70b-instruct:free",
			Models:      []string{"meta-llama/llama-3.3-70b-instruct:free", "mistralai/mistral-7b-instruct:free"},
			KeyRequired: true, FreeTier: true, Note: "`:free` models cost nothing (rate-limited).", Homepage: "https://openrouter.ai/keys"},
		{ID: "mistral", Label: "Mistral", Kind: "openai", BaseURL: "https://api.mistral.ai/v1",
			Model: "mistral-small-latest", KeyRequired: true, FreeTier: true,
			Note: "Free experimental tier.", Homepage: "https://console.mistral.ai"},
		{ID: "cerebras", Label: "Cerebras", Kind: "openai", BaseURL: "https://api.cerebras.ai/v1",
			Model: "llama3.1-8b", KeyRequired: true, FreeTier: true,
			Note: "Fast free tier.", Homepage: "https://cloud.cerebras.ai"},
		{ID: "huggingface", Label: "Hugging Face Router", Kind: "openai", BaseURL: "https://router.huggingface.co/v1",
			Model: "meta-llama/Llama-3.2-3B-Instruct", KeyRequired: true, FreeTier: true,
			Note: "Monthly free credits.", Homepage: "https://huggingface.co/settings/tokens"},
		{ID: "deepseek", Label: "DeepSeek", Kind: "openai", BaseURL: "https://api.deepseek.com/v1",
			Model: "deepseek-chat", KeyRequired: true, Note: "Very cheap paid API.", Homepage: "https://platform.deepseek.com"},
		{ID: "openai", Label: "OpenAI", Kind: "openai", BaseURL: "https://api.openai.com/v1",
			Model: "gpt-4o-mini", KeyRequired: true, Note: "Paid.", Homepage: "https://platform.openai.com/api-keys"},
		{ID: "anthropic", Label: "Anthropic", Kind: "anthropic", BaseURL: "https://api.anthropic.com",
			Model: "claude-3-5-haiku-latest", KeyRequired: true, Note: "Paid.", Homepage: "https://console.anthropic.com"},
	}
}
