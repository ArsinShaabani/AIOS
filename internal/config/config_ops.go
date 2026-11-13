package config

type MQTTConfig struct {
	Enabled       bool             `yaml:"enabled"`
	Broker        string           `yaml:"broker"`
	ClientID      string           `yaml:"client_id"`
	Username      string           `yaml:"username"`
	Password      string           `yaml:"password"`
	TopicPrefix   string           `yaml:"topic_prefix"`
	QoS           byte             `yaml:"qos"`
	KeepAlive     int              `yaml:"keep_alive_seconds"`
	AutoDiscovery bool             `yaml:"auto_discovery"`
	Bridge        MQTTBridgeConfig `yaml:"bridge"`
}

// MQTTBridgeConfig exposes an authenticated chat endpoint over MQTT. Requests
// carry an AIOS API key; replies are published to a constrained topic prefix.
type MQTTBridgeConfig struct {
	Enabled           bool   `yaml:"enabled"`
	RequestTopic      string `yaml:"request_topic"`
	ReplyPrefix       string `yaml:"reply_prefix"`
	MaxMessageBytes   int    `yaml:"max_message_bytes"`
	MaxConcurrency    int    `yaml:"max_concurrency"`
	RequestsPerMinute int    `yaml:"requests_per_minute"`
}

type HomeAssistantConfig struct {
	Enabled bool     `yaml:"enabled"`
	BaseURL string   `yaml:"base_url"`
	Token   string   `yaml:"token"`
	Timeout Duration `yaml:"timeout"`
}

type PluginConfig struct {
	Enabled          bool     `yaml:"enabled"`
	Dir              string   `yaml:"dir"`
	AllowHTTP        bool     `yaml:"allow_http"`
	AllowWASM        bool     `yaml:"allow_wasm"`
	AllowedHosts     []string `yaml:"allowed_hosts"`
	Timeout          Duration `yaml:"timeout"`
	MaxResponseBytes int64    `yaml:"max_response_bytes"`
	// Signing: Ed25519 public keys (base64) trusted for WASM/plugin manifests.
	RequireSignature bool     `yaml:"require_signature"`
	TrustedKeys      []string `yaml:"trusted_keys"`
	MaxWASMBytes     int64    `yaml:"max_wasm_bytes"`
	WASMTimeout      Duration `yaml:"wasm_timeout"`
	WASMMemoryPages  uint32   `yaml:"wasm_memory_pages"` // 64 KiB pages; 256 = 16 MiB
}

type MemoryConfig struct {
	MaxShortTurns    int     `yaml:"max_short_turns"`
	LongTermEnabled  bool    `yaml:"long_term_enabled"`
	SemanticEnabled  bool    `yaml:"semantic_enabled"`
	ConsolidateEvery int     `yaml:"consolidate_every"`
	SummarizeAfter   int     `yaml:"summarize_after_messages"`
	RetentionDays    int     `yaml:"retention_days"`
	MaxPerUser       int     `yaml:"max_per_user"`
	RetrievalTopK    int     `yaml:"retrieval_top_k"`
	MinSimilarity    float64 `yaml:"min_similarity"`
	MaxContextTokens int     `yaml:"max_context_tokens"`
}

type ObservabilityConfig struct {
	MetricsEnabled   bool     `yaml:"metrics_enabled"`
	PprofEnabled     bool     `yaml:"pprof_enabled"`
	SystemStatsEvery Duration `yaml:"system_stats_interval"`
}

type SchedulerConfig struct {
	Enabled       bool     `yaml:"enabled"`
	PollInterval  Duration `yaml:"poll_interval"`
	MaxConcurrent int      `yaml:"max_concurrent"`
	Timezone      string   `yaml:"timezone"`
	// StaleAfter reclaims tasks left "running" by a crashed worker.
	StaleAfter Duration `yaml:"stale_after"`
}

type ToolsConfig struct {
	InternetEnabled  bool     `yaml:"internet_enabled"`
	SearchProvider   string   `yaml:"search_provider"` // searxng | duckduckgo | wikipedia | mock
	SearxURL         string   `yaml:"searx_url"`
	HTTPTimeout      Duration `yaml:"http_timeout"`
	MaxFetchBytes    int64    `yaml:"max_fetch_bytes"`
	MaxSearchResults int      `yaml:"max_search_results"`
	UserAgent        string   `yaml:"user_agent"`
	BlockPrivateIPs  bool     `yaml:"block_private_ips"`
}

// EventsConfig selects the domain-event transport. NATS enables horizontal
// scaling: multiple gateway/brain instances share chat/device events.
type EventsConfig struct {
	Driver        string   `yaml:"driver"` // memory | nats
	URL           string   `yaml:"url"`
	SubjectPrefix string   `yaml:"subject_prefix"`
	Timeout       Duration `yaml:"timeout"`
}

// RAGConfig controls document ingestion and retrieval.
type RAGConfig struct {
	Enabled          bool    `yaml:"enabled"`
	MaxDocumentBytes int64   `yaml:"max_document_bytes"`
	ChunkSize        int     `yaml:"chunk_size"`
	ChunkOverlap     int     `yaml:"chunk_overlap"`
	MaxChunks        int     `yaml:"max_chunks"`
	TopK             int     `yaml:"top_k"`
	EmbedBatch       int     `yaml:"embed_batch"`
	MinScore         float64 `yaml:"min_score"`
}
