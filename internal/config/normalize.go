package config

import (
	"strings"
	"time"
)

// Normalize fills derived defaults and trims user input. It never fails; hard
// errors belong to Validate.
func (c *Config) Normalize() error {
	c.App.Name = strings.TrimSpace(c.App.Name)
	if c.App.Name == "" {
		c.App.Name = "AIOS"
	}
	if c.App.DataDir == "" {
		c.App.DataDir = "./data"
	}
	if c.App.LogFormat == "" {
		c.App.LogFormat = "json"
	}
	if c.App.LogLevel == "" {
		c.App.LogLevel = "info"
	}
	if c.App.Env == "" {
		c.App.Env = "development"
	}
	if c.Server.Addr == "" {
		c.Server.Addr = "127.0.0.1:8080"
	}
	if len(c.Server.CORSOrigins) == 0 && c.App.Env == "development" {
		c.Server.CORSOrigins = []string{"*"}
	}
	if c.Security.Issuer == "" {
		c.Security.Issuer = "aios"
	}
	for i := range c.LLM.Providers {
		p := &c.LLM.Providers[i]
		p.Name = strings.TrimSpace(strings.ToLower(p.Name))
		p.Kind = strings.TrimSpace(strings.ToLower(p.Kind))
		if p.Kind == "" {
			p.Kind = "openai"
		}
		if p.BaseURL == "" {
			switch p.Kind {
			case "openai":
				p.BaseURL = "https://api.openai.com/v1"
			case "anthropic":
				p.BaseURL = "https://api.anthropic.com"
			case "gemini":
				p.BaseURL = "https://generativelanguage.googleapis.com/v1beta"
			case "ollama":
				p.BaseURL = "http://127.0.0.1:11434"
			}
		}
		p.BaseURL = strings.TrimRight(p.BaseURL, "/")
		if p.DefaultModel == "" && len(p.Models) > 0 {
			p.DefaultModel = p.Models[0].Name
		}
		if p.DefaultModel != "" && len(p.Models) == 0 {
			p.Models = []ModelConfig{{Name: p.DefaultModel, ContextWindow: 8192, MaxOutputTokens: 2048}}
		}
		if p.EmbeddingDim <= 0 && p.Kind == "local" {
			p.EmbeddingDim = 384
		}
	}
	if c.LLM.DefaultProvider == "" {
		c.LLM.DefaultProvider = "local"
	}
	if c.LLM.EmbeddingProvider == "" {
		c.LLM.EmbeddingProvider = c.LLM.DefaultProvider
	}
	if c.Vector.Dim <= 0 {
		c.Vector.Dim = 384
	}
	if c.Vector.Collection == "" {
		c.Vector.Collection = "aios_memories"
	}
	if c.Memory.RetrievalTopK <= 0 {
		c.Memory.RetrievalTopK = 5
	}
	if c.Queue.Workers <= 0 {
		c.Queue.Workers = 2
	}
	if c.Voice.SampleRate <= 0 {
		c.Voice.SampleRate = 16000
	}
	if c.Voice.VAD.FrameMS <= 0 {
		c.Voice.VAD.FrameMS = 20
	}
	if c.Tools.UserAgent == "" {
		c.Tools.UserAgent = "AIOS/1.0"
	}
	if c.Tools.SearchProvider == "" {
		c.Tools.SearchProvider = "duckduckgo"
	}
	if c.Scheduler.Timezone == "" {
		c.Scheduler.Timezone = "UTC"
	}
	if c.Scheduler.StaleAfter <= 0 {
		c.Scheduler.StaleAfter = Duration(15 * time.Minute)
	}
	if c.Observability.SystemStatsEvery <= 0 {
		c.Observability.SystemStatsEvery = Duration(30 * time.Second)
	}
	if c.Voice.WakeWord.Threshold <= 0 {
		c.Voice.WakeWord.Threshold = 0.5
	}
	if c.Voice.WakeWord.Timeout <= 0 {
		c.Voice.WakeWord.Timeout = Duration(2 * time.Second)
	}
	if c.Voice.WakeWord.Provider == "" {
		c.Voice.WakeWord.Provider = "text"
	}
	if c.MQTT.TopicPrefix == "" {
		c.MQTT.TopicPrefix = "aios"
	}
	if c.MQTT.Bridge.RequestTopic == "" {
		c.MQTT.Bridge.RequestTopic = strings.Trim(c.MQTT.TopicPrefix, "/") + "/bridge/chat"
	}
	if c.MQTT.Bridge.ReplyPrefix == "" {
		c.MQTT.Bridge.ReplyPrefix = strings.Trim(c.MQTT.TopicPrefix, "/") + "/bridge/reply"
	}
	if c.MQTT.Bridge.MaxMessageBytes <= 0 {
		c.MQTT.Bridge.MaxMessageBytes = 64 << 10
	}
	if c.MQTT.Bridge.MaxConcurrency <= 0 {
		c.MQTT.Bridge.MaxConcurrency = 4
	}
	if c.MQTT.Bridge.ConsumerGroup != "" {
		c.MQTT.Bridge.ConsumerGroup = strings.Join(strings.Fields(c.MQTT.Bridge.ConsumerGroup), "-")
	}
	if c.MQTT.Bridge.RequestsPerMinute <= 0 {
		c.MQTT.Bridge.RequestsPerMinute = 30
	}
	if c.RAG.ChunkSize <= 0 {
		c.RAG.ChunkSize = 900
	}
	if c.RAG.ChunkOverlap < 0 {
		c.RAG.ChunkOverlap = 0
	}
	if c.RAG.ChunkOverlap >= c.RAG.ChunkSize {
		c.RAG.ChunkOverlap = c.RAG.ChunkSize / 4
	}
	if c.RAG.MaxChunks <= 0 {
		c.RAG.MaxChunks = 500
	}
	if c.RAG.TopK <= 0 {
		c.RAG.TopK = 5
	}
	if c.RAG.EmbedBatch <= 0 {
		c.RAG.EmbedBatch = 16
	}
	if c.RAG.MaxDocumentBytes <= 0 {
		c.RAG.MaxDocumentBytes = 2 << 20
	}
	if c.RAG.MinScore <= 0 {
		c.RAG.MinScore = 0.2
	}
	if c.Events.Driver == "" {
		c.Events.Driver = "memory"
	}
	if c.Events.URL == "" {
		c.Events.URL = "nats://127.0.0.1:4222"
	}
	if c.Events.SubjectPrefix == "" {
		c.Events.SubjectPrefix = "aios.events"
	}
	if c.Events.Timeout <= 0 {
		c.Events.Timeout = Duration(5 * time.Second)
	}
	return nil
}
