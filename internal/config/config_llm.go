package config

type SecurityConfig struct {
	JWTSecret         string        `yaml:"jwt_secret"`
	Issuer            string        `yaml:"issuer"`
	AccessTTL         Duration      `yaml:"access_ttl"`
	RefreshTTL        Duration      `yaml:"refresh_ttl"`
	DeviceTokenTTL    Duration      `yaml:"device_token_ttl"`
	AllowedClockSkew  Duration      `yaml:"allowed_clock_skew"`
	EncryptionKey     string        `yaml:"encryption_key"`
	BcryptCost        int           `yaml:"bcrypt_cost"`
	MaxLoginAttempts  int           `yaml:"max_login_attempts"`
	LockoutDuration   Duration      `yaml:"lockout_duration"`
	Bootstrap         BootstrapUser `yaml:"bootstrap"`
	AutoGenerateKeys  bool          `yaml:"auto_generate_keys"`
	AllowRegistration bool          `yaml:"allow_registration"`
}

type BootstrapUser struct {
	Email    string `yaml:"email"`
	Password string `yaml:"password"`
	Name     string `yaml:"name"`
}

type LLMConfig struct {
	DefaultProvider   string           `yaml:"default_provider"`
	DefaultModel      string           `yaml:"default_model"`
	EmbeddingProvider string           `yaml:"embedding_provider"`
	EmbeddingModel    string           `yaml:"embedding_model"`
	Timeout           Duration         `yaml:"timeout"`
	MaxRetries        int              `yaml:"max_retries"`
	MaxToolIterations int              `yaml:"max_tool_iterations"`
	MaxConcurrency    int              `yaml:"max_concurrency"`
	Temperature       float64          `yaml:"temperature"`
	TopP              float64          `yaml:"top_p"`
	MaxTokens         int              `yaml:"max_tokens"`
	DailyBudgetUSD    float64          `yaml:"daily_budget_usd"`
	SystemPrompt      string           `yaml:"system_prompt"`
	Providers         []ProviderConfig `yaml:"providers"`
	Routing           []RoutingRule    `yaml:"routing"`
}

type ProviderConfig struct {
	Name            string            `yaml:"name"`
	Kind            string            `yaml:"kind"` // openai | anthropic | gemini | ollama
	BaseURL         string            `yaml:"base_url"`
	APIKey          string            `yaml:"api_key"`
	DefaultModel    string            `yaml:"default_model"`
	Enabled         *bool             `yaml:"enabled"`
	Timeout         Duration          `yaml:"timeout"`
	MaxConcurrency  int               `yaml:"max_concurrency"`
	Headers         map[string]string `yaml:"headers"`
	Models          []ModelConfig     `yaml:"models"`
	Priority        int               `yaml:"priority"`
	EmbeddingDim    int               `yaml:"embedding_dim"`
	Temperature     float64           `yaml:"temperature,omitempty"`
	TopP            float64           `yaml:"top_p,omitempty"`
	MaxTokens       int               `yaml:"max_tokens,omitempty"`
	ContextWindow   int               `yaml:"context_window,omitempty"`
	ReasoningEffort string            `yaml:"reasoning_effort,omitempty"`
}

// IsEnabled defaults to true when the flag is omitted.
func (p ProviderConfig) IsEnabled() bool { return p.Enabled == nil || *p.Enabled }

type ModelConfig struct {
	Name            string  `yaml:"name"`
	ContextWindow   int     `yaml:"context_window"`
	MaxOutputTokens int     `yaml:"max_output_tokens"`
	PromptPer1K     float64 `yaml:"prompt_per_1k"`
	CompletionPer1K float64 `yaml:"completion_per_1k"`
	SupportsTools   *bool   `yaml:"supports_tools"`
	SupportsVision  bool    `yaml:"supports_vision"`
	SupportsStream  *bool   `yaml:"supports_stream"`
	ReasoningEffort string  `yaml:"reasoning_effort,omitempty"`
}

func (m ModelConfig) Tools() bool  { return m.SupportsTools == nil || *m.SupportsTools }
func (m ModelConfig) Stream() bool { return m.SupportsStream == nil || *m.SupportsStream }

type RoutingRule struct {
	Name        string   `yaml:"name"`
	Intents     []string `yaml:"intents"`
	Keywords    []string `yaml:"keywords"`
	Agent       string   `yaml:"agent"`
	Provider    string   `yaml:"provider"`
	Model       string   `yaml:"model"`
	Fallback    []string `yaml:"fallback"` // provider/model pairs
	MaxTokens   int      `yaml:"max_tokens"`
	Temperature *float64 `yaml:"temperature"`
}

type VoiceConfig struct {
	Enabled       bool           `yaml:"enabled"`
	MaxSession    Duration       `yaml:"max_session"`
	SampleRate    int            `yaml:"sample_rate"`
	STT           STTConfig      `yaml:"stt"`
	TTS           TTSConfig      `yaml:"tts"`
	VAD           VADConfig      `yaml:"vad"`
	WakeWord      WakeWordConfig `yaml:"wake_word"`
	Interruptions bool           `yaml:"interruptions"`
}

type STTConfig struct {
	Provider string   `yaml:"provider"` // whisper_http | openai | mock
	BaseURL  string   `yaml:"base_url"`
	APIKey   string   `yaml:"api_key"`
	Model    string   `yaml:"model"`
	Language string   `yaml:"language"`
	Timeout  Duration `yaml:"timeout"`
}

type TTSConfig struct {
	Provider string   `yaml:"provider"` // piper_http | coqui_http | edge_http | azure | openai | mock
	BaseURL  string   `yaml:"base_url"`
	APIKey   string   `yaml:"api_key"`
	Region   string   `yaml:"region"` // Azure Speech region (westeurope, ...)
	Voice    string   `yaml:"voice"`
	Model    string   `yaml:"model"`
	Language string   `yaml:"language"`
	Format   string   `yaml:"format"` // wav | mp3 | opus
	Speed    float64  `yaml:"speed"`
	Timeout  Duration `yaml:"timeout"`
}

type VADConfig struct {
	// Provider selects the speech detector: energy (built-in) or silero_http
	// (Silero VAD behind an HTTP bridge, keeps the core cgo-free).
	Provider        string   `yaml:"provider"`
	Endpoint        string   `yaml:"endpoint"`
	Threshold       float64  `yaml:"threshold"` // silero speech probability
	EnergyThreshold float64  `yaml:"energy_threshold"`
	FrameMS         int      `yaml:"frame_ms"`
	SilenceMS       int      `yaml:"silence_ms"`
	MinSpeechMS     int      `yaml:"min_speech_ms"`
	MaxUtteranceMS  int      `yaml:"max_utterance_ms"`
	Adaptive        bool     `yaml:"adaptive"`
	Timeout         Duration `yaml:"timeout"`
}

type WakeWordConfig struct {
	Enabled   bool     `yaml:"enabled"`
	Words     []string `yaml:"words"`
	Provider  string   `yaml:"provider"` // text | openwakeword_http
	Endpoint  string   `yaml:"endpoint"`
	Threshold float64  `yaml:"threshold"` // openwakeword activation score
	Timeout   Duration `yaml:"timeout"`
}
