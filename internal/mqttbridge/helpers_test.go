package mqttbridge

import (
	"context"
	"log/slog"
	"sync"
	"testing"
	"time"

	"github.com/ArsinShaabani/AIOS/internal/adapters/storage/sqlstore"
	"github.com/ArsinShaabani/AIOS/internal/brain"
	"github.com/ArsinShaabani/AIOS/internal/config"
	"github.com/ArsinShaabani/AIOS/internal/domain"
	"github.com/ArsinShaabani/AIOS/internal/ports"
	"github.com/ArsinShaabani/AIOS/internal/security"
)

type publication struct {
	topic   string
	payload []byte
}

type fakeClient struct {
	mu        sync.Mutex
	pubs      []publication
	handler   func(topic string, payload []byte)
	topic     string
	subErr    error
	pubErr    error
	connected bool
}

func (f *fakeClient) Publish(_ context.Context, topic string, payload []byte, _ bool) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.pubErr != nil {
		return f.pubErr
	}
	f.pubs = append(f.pubs, publication{topic: topic, payload: append([]byte(nil), payload...)})
	return nil
}
func (f *fakeClient) Subscribe(_ context.Context, topic string, handler func(string, []byte)) error {
	if f.subErr != nil {
		return f.subErr
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	f.topic = topic
	f.handler = handler
	return nil
}
func (f *fakeClient) Connected() bool { return f.connected }

func (f *fakeClient) last() (publication, bool) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if len(f.pubs) == 0 {
		return publication{}, false
	}
	return f.pubs[len(f.pubs)-1], true
}

func (f *fakeClient) count() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.pubs)
}

type fakeChat struct {
	mu   sync.Mutex
	out  *brain.ChatOutput
	err  error
	last brain.ChatInput
}

func (f *fakeChat) Chat(_ context.Context, in brain.ChatInput, _ brain.EmitFunc) (*brain.ChatOutput, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.last = in
	if f.err != nil {
		return nil, f.err
	}
	out := f.out
	if out == nil {
		out = &brain.ChatOutput{SessionID: "ses_mqtt", MessageID: "msg_mqtt", Content: "hello from mqtt",
			AgentID: "agent_general", Provider: "local", Model: "m", Usage: ports.Usage{PromptTokens: 3, CompletionTokens: 4, TotalTokens: 7}}
	}
	return out, nil
}

type testEnv struct {
	bridge *Bridge
	client *fakeClient
	chat   *fakeChat
	repos  *sqlstore.Repositories
	user   *domain.User
	apiKey string
	cfg    config.MQTTConfig
}

func newBridgeEnv(t *testing.T, mutate func(*config.MQTTConfig)) *testEnv {
	t.Helper()
	db, err := sqlstore.Open(context.Background(), config.DatabaseConfig{
		Driver: "sqlite", DSN: "bridge.db", AutoMigrate: true, MaxOpenConns: 1,
	}, t.TempDir(), slog.Default())
	if err != nil {
		t.Fatal(err)
	}
	repos := sqlstore.NewRepositories(db)
	t.Cleanup(func() { _ = db.Close() })
	user := &domain.User{ID: "u1", Email: "mqtt@test.local", PasswordHash: "x", Role: domain.RoleUser, Status: "active"}
	if err := repos.Users.Create(context.Background(), user); err != nil {
		t.Fatal(err)
	}
	raw, err := security.NewSecret("aios")
	if err != nil {
		t.Fatal(err)
	}
	key := &domain.APIKey{ID: "k1", UserID: user.ID, Name: "mqtt", KeyHash: security.HashToken(raw),
		Prefix: security.TokenPrefix(raw), Scopes: []string{"chat"}, CreatedAt: time.Now().UTC()}
	if err := repos.Tokens.CreateAPIKey(context.Background(), key); err != nil {
		t.Fatal(err)
	}
	cfg := config.MQTTConfig{
		Enabled: true, TopicPrefix: "aios",
		Bridge: config.MQTTBridgeConfig{Enabled: true, RequestTopic: "aios/bridge/chat",
			ReplyPrefix: "aios/bridge/reply", MaxMessageBytes: 4096, MaxConcurrency: 2, RequestsPerMinute: 0},
	}
	if mutate != nil {
		mutate(&cfg)
	}
	client := &fakeClient{connected: true}
	chat := &fakeChat{}
	bridge := New(cfg, client, repos.Tokens, repos.Users, chat, nil, slog.Default())
	return &testEnv{bridge: bridge, client: client, chat: chat, repos: repos, user: user, apiKey: raw, cfg: cfg}
}
