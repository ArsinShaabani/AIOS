package config

import (
	"crypto/ed25519"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"net/url"
	"strings"
	"time"
)

// Validate rejects configurations that cannot boot safely.
func (c *Config) Validate() error {
	var problems []string
	add := func(format string, args ...any) { problems = append(problems, fmt.Sprintf(format, args...)) }

	if c.Server.ReadTimeout.Std() <= 0 || c.Server.WriteTimeout.Std() <= 0 {
		add("server timeouts must be positive")
	}
	if c.Server.MaxBodyBytes < 1024 {
		add("server.max_body_bytes must be >= 1024")
	}
	switch c.Database.Driver {
	case "sqlite":
		if strings.TrimSpace(c.Database.DSN) == "" {
			c.Database.DSN = "aios.db"
		}
	case "postgres":
		if !strings.HasPrefix(c.Database.DSN, "postgres://") && !strings.HasPrefix(c.Database.DSN, "postgresql://") && !strings.Contains(c.Database.DSN, "host=") {
			add("database.dsn must be a postgres URL or key/value DSN")
		}
	default:
		add("database.driver must be sqlite or postgres, got %q", c.Database.Driver)
	}
	if c.Database.MaxOpenConns < 1 {
		add("database.max_open_conns must be >= 1")
	}
	switch c.Cache.Driver {
	case "memory", "redis":
	default:
		add("cache.driver must be memory or redis")
	}
	if c.Cache.Driver == "redis" && c.Cache.Redis.Addr == "" {
		add("cache.redis.addr is required for the redis cache")
	}
	switch c.Queue.Driver {
	case "memory", "redis":
	default:
		add("queue.driver must be memory or redis")
	}
	if c.Queue.Workers > 16 {
		add("queue.workers above 16 will starve a 1-CPU host")
	}
	switch c.Vector.Driver {
	case "local":
	case "qdrant":
		if c.Vector.QdrantURL == "" {
			add("vector.qdrant_url is required for the qdrant driver")
		} else if _, err := url.Parse(c.Vector.QdrantURL); err != nil {
			add("vector.qdrant_url is invalid: %v", err)
		}
	default:
		add("vector.driver must be local or qdrant")
	}
	if c.Vector.Dim < 8 || c.Vector.Dim > 8192 {
		add("vector.dim must be between 8 and 8192")
	}
	if c.Security.BcryptCost < 8 || c.Security.BcryptCost > 14 {
		add("security.bcrypt_cost must be between 8 and 14")
	}
	if c.Security.AccessTTL.Std() <= 0 || c.Security.RefreshTTL.Std() <= 0 {
		add("security token TTLs must be positive")
	}
	if c.Security.RefreshTTL.Std() <= c.Security.AccessTTL.Std() {
		add("security.refresh_ttl must exceed access_ttl")
	}
	if c.Security.EncryptionKey != "" {
		if _, err := DecodeKey(c.Security.EncryptionKey); err != nil {
			add("security.encryption_key: %v", err)
		}
	}
	if c.App.Env == "production" {
		if len(c.Security.JWTSecret) < 32 && !c.Security.AutoGenerateKeys {
			add("production requires security.jwt_secret (>=32 chars) or auto_generate_keys")
		}
		if c.Security.Bootstrap.Email != "" && len(c.Security.Bootstrap.Password) < 12 {
			add("production requires security.bootstrap.password with >=12 chars (or leave email empty)")
		}
		if hasWildcard(c.Server.CORSOrigins) {
			add("production must not use wildcard CORS origins")
		}
	}
	if c.Memory.MaxShortTurns < 1 || c.Memory.MaxShortTurns > 200 {
		add("memory.max_short_turns must be within [1,200]")
	}
	if c.Memory.RetrievalTopK < 1 || c.Memory.RetrievalTopK > 50 {
		add("memory.retrieval_top_k must be within [1,50]")
	}
	if c.Memory.MinSimilarity < 0 || c.Memory.MinSimilarity > 1 {
		add("memory.min_similarity must be within [0,1]")
	}
	if c.Memory.MaxContextTokens < 256 || c.Memory.MaxContextTokens > 64000 {
		add("memory.max_context_tokens must be within [256,64000]")
	}
	if c.Voice.Enabled {
		if c.Voice.SampleRate < 8000 || c.Voice.SampleRate > 48000 {
			add("voice.sample_rate must be within [8000,48000]")
		}
		if c.Voice.VAD.FrameMS < 10 || c.Voice.VAD.FrameMS > 60 {
			add("voice.vad.frame_ms must be within [10,60]")
		}
		if c.Voice.VAD.SilenceMS < 200 {
			add("voice.vad.silence_ms must be >= 200")
		}
		switch c.Voice.STT.Provider {
		case "whisper_http", "openai", "mock":
		default:
			add("voice.stt.provider must be whisper_http, openai or mock")
		}
		switch c.Voice.TTS.Provider {
		case "piper_http", "edge_http", "openai", "mock":
		default:
			add("voice.tts.provider must be piper_http, edge_http, openai or mock")
		}
		switch c.Voice.TTS.Format {
		case "", "wav", "mp3", "opus":
		default:
			add("voice.tts.format must be wav, mp3 or opus")
		}
		switch c.Voice.WakeWord.Provider {
		case "", "text":
		case "openwakeword_http":
			if c.Voice.WakeWord.Endpoint == "" {
				add("voice.wake_word.endpoint is required for openwakeword_http")
			}
		default:
			add("voice.wake_word.provider must be text or openwakeword_http")
		}
		if c.Voice.WakeWord.Threshold < 0 || c.Voice.WakeWord.Threshold > 1 {
			add("voice.wake_word.threshold must be within [0,1]")
		}
	}
	switch c.Tools.SearchProvider {
	case "searxng", "duckduckgo", "wikipedia", "mock":
	default:
		add("tools.search_provider must be searxng, duckduckgo, wikipedia or mock")
	}
	if c.Tools.SearchProvider == "searxng" && c.Tools.SearxURL == "" {
		add("tools.searx_url is required when search_provider=searxng")
	}
	if c.Plugins.Enabled && c.Plugins.Dir == "" {
		add("plugins.dir is required when plugins are enabled")
	}
	if c.MQTT.Bridge.Enabled {
		if !c.MQTT.Enabled {
			add("mqtt.bridge requires mqtt.enabled")
		}
		for _, topic := range []string{c.MQTT.Bridge.RequestTopic, c.MQTT.Bridge.ReplyPrefix} {
			if topic == "" || strings.ContainsAny(topic, "+#") {
				add("mqtt.bridge topics must be concrete (no wildcards)")
			}
		}
		if c.MQTT.Bridge.MaxMessageBytes < 1024 || c.MQTT.Bridge.MaxMessageBytes > 4<<20 {
			add("mqtt.bridge.max_message_bytes must be within [1024, 4194304]")
		}
		if c.MQTT.Bridge.MaxConcurrency < 1 || c.MQTT.Bridge.MaxConcurrency > 64 {
			add("mqtt.bridge.max_concurrency must be within [1,64]")
		}
		if len(c.MQTT.Bridge.ConsumerGroup) > 64 || strings.ContainsAny(c.MQTT.Bridge.ConsumerGroup, "$+/# ") {
			add("mqtt.bridge.consumer_group must be <= 64 chars without $ + / # or spaces")
		}
	}
	if c.RAG.Enabled {
		if c.RAG.MaxDocumentBytes < 1024 || c.RAG.MaxDocumentBytes > 32<<20 {
			add("rag.max_document_bytes must be within [1024, 33554432]")
		}
		if c.RAG.ChunkSize < 200 || c.RAG.ChunkSize > 8000 {
			add("rag.chunk_size must be within [200,8000]")
		}
		if c.RAG.ChunkOverlap < 0 || c.RAG.ChunkOverlap >= c.RAG.ChunkSize {
			add("rag.chunk_overlap must be >= 0 and smaller than chunk_size")
		}
		if c.RAG.MaxChunks < 1 || c.RAG.MaxChunks > 5000 {
			add("rag.max_chunks must be within [1,5000]")
		}
		if c.RAG.TopK < 1 || c.RAG.TopK > 50 {
			add("rag.top_k must be within [1,50]")
		}
		if c.RAG.EmbedBatch < 1 || c.RAG.EmbedBatch > 128 {
			add("rag.embed_batch must be within [1,128]")
		}
		if c.RAG.MinScore < 0 || c.RAG.MinScore > 1 {
			add("rag.min_score must be within [0,1]")
		}
	}
	switch c.Events.Driver {
	case "memory":
	case "nats":
		if c.Events.URL == "" {
			add("events.url is required for the nats event bus")
		}
	default:
		add("events.driver must be memory or nats")
	}
	if strings.ContainsAny(c.Events.SubjectPrefix, " >*") || c.Events.SubjectPrefix == "" {
		add("events.subject_prefix must be a concrete NATS subject prefix")
	}
	if c.Plugins.MaxWASMBytes <= 0 {
		c.Plugins.MaxWASMBytes = 16 << 20
	}
	if c.Plugins.WASMTimeout <= 0 {
		c.Plugins.WASMTimeout = Duration(10 * time.Second)
	}
	if c.Plugins.WASMMemoryPages == 0 {
		c.Plugins.WASMMemoryPages = 256
	}
	if c.Plugins.RequireSignature && len(c.Plugins.TrustedKeys) == 0 {
		add("plugins.trusted_keys is required when require_signature is true")
	}
	for _, key := range c.Plugins.TrustedKeys {
		if _, err := configDecodePublicKey(key); err != nil {
			add("plugins.trusted_keys: %v", err)
		}
	}
	if _, err := time.LoadLocation(c.Scheduler.Timezone); err != nil {
		add("scheduler.timezone %q is invalid", c.Scheduler.Timezone)
	}
	c.validateLLM(add)

	if len(problems) > 0 {
		return fmt.Errorf("invalid configuration:\n  - %s", strings.Join(problems, "\n  - "))
	}
	return nil
}

func (c *Config) providerEnabled(name string) bool {
	for _, p := range c.LLM.Providers {
		if p.Name == name {
			return p.IsEnabled()
		}
	}
	return false
}

func hasWildcard(origins []string) bool {
	for _, o := range origins {
		if o == "*" || o == "" {
			return true
		}
	}
	return false
}

// DecodeKey accepts 32-byte keys encoded as hex or base64.
func DecodeKey(s string) ([]byte, error) {
	if b, err := hex.DecodeString(strings.TrimSpace(s)); err == nil && len(b) == 32 {
		return b, nil
	}
	if b, err := base64.StdEncoding.DecodeString(strings.TrimSpace(s)); err == nil && len(b) == 32 {
		return b, nil
	}
	if b, err := base64.RawStdEncoding.DecodeString(strings.TrimSpace(s)); err == nil && len(b) == 32 {
		return b, nil
	}
	return nil, fmt.Errorf("must be 32 bytes encoded as hex or base64")
}

// configDecodePublicKey validates an Ed25519 plugin signing key.
func configDecodePublicKey(s string) (ed25519.PublicKey, error) {
	raw, err := base64.StdEncoding.DecodeString(strings.TrimSpace(s))
	if err != nil {
		raw, err = base64.RawStdEncoding.DecodeString(strings.TrimSpace(s))
	}
	if err != nil {
		return nil, fmt.Errorf("must be base64-encoded Ed25519 public key")
	}
	if len(raw) != ed25519.PublicKeySize {
		return nil, fmt.Errorf("must be a %d-byte Ed25519 public key", ed25519.PublicKeySize)
	}
	return ed25519.PublicKey(raw), nil
}
