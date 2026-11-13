// Package mqttbridge exposes an authenticated chat endpoint over MQTT so
// constrained devices, robots and scripts can talk to the brain without HTTP.
// Requests carry an AIOS API key; replies are JSON on a constrained topic prefix.
package mqttbridge

import (
	"context"
	"encoding/json"
	"log/slog"
	"strings"
	"time"

	"github.com/ArsinShaabani/AIOS/internal/brain"
	"github.com/ArsinShaabani/AIOS/internal/config"
	"github.com/ArsinShaabani/AIOS/internal/domain"
	"github.com/ArsinShaabani/AIOS/internal/platform"
	"github.com/ArsinShaabani/AIOS/internal/ports"
	"github.com/ArsinShaabani/AIOS/internal/security"
)

// Client is the narrow MQTT surface the bridge needs (implemented by mqtt.Client).
type Client interface {
	Publish(ctx context.Context, topic string, payload []byte, retain bool) error
	Subscribe(ctx context.Context, topic string, handler func(topic string, payload []byte)) error
	Connected() bool
}

// Chat is the brain entry point.
type Chat interface {
	Chat(ctx context.Context, in brain.ChatInput, emit brain.EmitFunc) (*brain.ChatOutput, error)
}

// Request is the JSON payload published to the request topic.
type Request struct {
	ID            string `json:"id"`
	APIKey        string `json:"api_key"`
	Message       string `json:"message"`
	SessionID     string `json:"session_id,omitempty"`
	AgentID       string `json:"agent_id,omitempty"`
	PersonalityID string `json:"personality_id,omitempty"`
	ReplyTo       string `json:"reply_to,omitempty"`
}

// Response is the JSON reply. Error/Code are set on failure and Content on success.
type Response struct {
	ID        string      `json:"id"`
	SessionID string      `json:"session_id,omitempty"`
	MessageID string      `json:"message_id,omitempty"`
	Content   string      `json:"content,omitempty"`
	AgentID   string      `json:"agent_id,omitempty"`
	Provider  string      `json:"provider,omitempty"`
	Model     string      `json:"model,omitempty"`
	Usage     ports.Usage `json:"usage,omitempty"`
	Error     string      `json:"error,omitempty"`
	Code      string      `json:"code,omitempty"`
	LatencyMS int64       `json:"latency_ms"`
	At        time.Time   `json:"at"`
}

// Bridge authenticates, rate-limits and dispatches MQTT chat requests.
type Bridge struct {
	cfg     config.MQTTConfig
	client  Client
	tokens  ports.TokenRepository
	users   ports.UserRepository
	chat    Chat
	audit   *security.Auditor
	limiter *security.RateLimiter
	log     *slog.Logger
	sem     chan struct{}
}

// New builds the bridge.
func New(cfg config.MQTTConfig, client Client, tokens ports.TokenRepository, users ports.UserRepository,
	chat Chat, audit *security.Auditor, log *slog.Logger) *Bridge {
	if log == nil {
		log = slog.Default()
	}
	var limiter *security.RateLimiter
	if cfg.Bridge.RequestsPerMinute > 0 {
		limiter = security.NewRateLimiter(cfg.Bridge.RequestsPerMinute, max(cfg.Bridge.RequestsPerMinute/2, 5))
	}
	concurrency := cfg.Bridge.MaxConcurrency
	if concurrency <= 0 {
		concurrency = 4
	}
	return &Bridge{
		cfg: cfg, client: client, tokens: tokens, users: users, chat: chat,
		audit: audit, limiter: limiter, log: log, sem: make(chan struct{}, concurrency),
	}
}

// Start subscribes to the request topic and dispatches messages concurrently.
func (b *Bridge) Start(ctx context.Context) error {
	if !b.cfg.Enabled || !b.cfg.Bridge.Enabled {
		return nil
	}
	topic := b.cfg.Bridge.RequestTopic
	if topic == "" {
		return platform.Invalid("mqttbridge.topic", "bridge request topic is required")
	}
	if err := b.client.Subscribe(ctx, topic, func(_ string, payload []byte) {
		select {
		case b.sem <- struct{}{}:
			go func() {
				defer func() { <-b.sem }()
				b.Handle(ctx, payload)
			}()
		default:
			b.rejectBusy(ctx, payload)
		}
	}); err != nil {
		return platform.Wrap(platform.KindUnavailable, "mqttbridge.subscribe", "cannot subscribe to bridge topic", err)
	}
	b.log.Info("MQTT chat bridge active", "topic", topic, "reply_prefix", b.cfg.Bridge.ReplyPrefix)
	return nil
}

func (b *Bridge) reply(ctx context.Context, topic string, resp Response) {
	if topic == "" || b.client == nil {
		return
	}
	raw, err := json.Marshal(resp)
	if err != nil {
		return
	}
	pubCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	if err := b.client.Publish(pubCtx, topic, raw, false); err != nil {
		b.log.Warn("cannot publish MQTT bridge reply", "topic", topic, "error", err)
	}
}

func (b *Bridge) rejectBusy(ctx context.Context, payload []byte) {
	var req Request
	if json.Unmarshal(payload, &req) != nil {
		return
	}
	if req.ID == "" {
		req.ID = platform.NewID("mqtt")
	}
	b.reply(ctx, b.replyTopic(req), Response{ID: req.ID, Error: "bridge is busy", Code: "mqttbridge.busy", At: time.Now().UTC()})
}

func (b *Bridge) replyTopic(req Request) string {
	prefix := strings.TrimRight(b.cfg.Bridge.ReplyPrefix, "/")
	if req.ReplyTo != "" && strings.HasPrefix(req.ReplyTo, prefix+"/") &&
		!strings.ContainsAny(req.ReplyTo, "+#") && len(req.ReplyTo) <= 180 {
		return req.ReplyTo
	}
	id := sanitizeSegment(req.ID)
	if id == "" {
		id = platform.NewID("req")
	}
	return prefix + "/" + id
}

func sanitizeSegment(s string) string {
	s = strings.TrimSpace(s)
	var b strings.Builder
	for _, r := range s {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '-' || r == '_' || r == '.' {
			b.WriteRune(r)
		}
		if b.Len() >= 64 {
			break
		}
	}
	return b.String()
}

func domainChannelMQTT() domain.Channel { return domain.ChannelMQTT }
