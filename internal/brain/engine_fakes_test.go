package brain

import (
	"context"
	"log/slog"
	"sync"
	"testing"

	"github.com/ArsinShaabani/AIOS/internal/adapters/storage/sqlstore"
	"github.com/ArsinShaabani/AIOS/internal/agents"
	"github.com/ArsinShaabani/AIOS/internal/config"
	"github.com/ArsinShaabani/AIOS/internal/domain"
	"github.com/ArsinShaabani/AIOS/internal/personality"
	"github.com/ArsinShaabani/AIOS/internal/ports"
	"github.com/ArsinShaabani/AIOS/internal/tools"
)

type scriptedRouter struct {
	mu    sync.Mutex
	calls int
}

func (r *scriptedRouter) Resolve(string, string, string) ports.RouteDecision {
	return ports.RouteDecision{Provider: "fake", Model: "fake-model", MaxTokens: 128, Temperature: 0.2}
}
func (r *scriptedRouter) Chat(context.Context, ports.RouteDecision, ports.ChatRequest) (*ports.ChatResponse, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.calls++
	if r.calls == 1 {
		return &ports.ChatResponse{Provider: "fake", Model: "fake-model", FinishReason: "tool_calls",
			ToolCalls: []domain.ToolCall{{ID: "call_1", Name: "echo", Arguments: map[string]any{"text": "tool input"}}},
			Usage:     ports.Usage{PromptTokens: 10, CompletionTokens: 2}}, nil
	}
	return &ports.ChatResponse{Provider: "fake", Model: "fake-model", Content: "final answer",
		FinishReason: "stop", Usage: ports.Usage{PromptTokens: 12, CompletionTokens: 5}}, nil
}
func (r *scriptedRouter) Stream(context.Context, ports.RouteDecision, ports.ChatRequest) (<-chan ports.Chunk, error) {
	r.mu.Lock()
	r.calls++
	call := r.calls
	r.mu.Unlock()
	ch := make(chan ports.Chunk, 4)
	if call%2 == 1 {
		ch <- ports.Chunk{ToolCalls: []domain.ToolCall{{ID: "call_s", Name: "echo", Arguments: map[string]any{"text": "stream"}}}, Done: true,
			Usage: &ports.Usage{PromptTokens: 3, CompletionTokens: 1}}
	} else {
		ch <- ports.Chunk{Delta: "streamed "}
		ch <- ports.Chunk{Delta: "answer", Done: true, Usage: &ports.Usage{PromptTokens: 4, CompletionTokens: 2}}
	}
	close(ch)
	return ch, nil
}
func (r *scriptedRouter) Embed(context.Context, []string) ([][]float32, error) {
	return [][]float32{{0.1, 0.2, 0.3}}, nil
}
func (r *scriptedRouter) Providers() []ports.ProviderSummary { return nil }

func newEngine(t *testing.T, router ports.ModelRouter) (*Engine, *sqlstore.Repositories, *domain.User) {
	t.Helper()
	db, err := sqlstore.Open(context.Background(), config.DatabaseConfig{
		Driver: "sqlite", DSN: "brain.db", AutoMigrate: true, MaxOpenConns: 1,
	}, t.TempDir(), slog.Default())
	if err != nil {
		t.Fatal(err)
	}
	repos := sqlstore.NewRepositories(db)
	t.Cleanup(func() { _ = db.Close() })

	user := &domain.User{ID: "u1", Email: "brain@test.local", PasswordHash: "x", Role: domain.RoleUser, Status: "active"}
	if err := repos.Users.Create(context.Background(), user); err != nil {
		t.Fatal(err)
	}
	pers := personality.New(repos.Personalities)
	if err := pers.Seed(context.Background()); err != nil {
		t.Fatal(err)
	}
	agentReg := agents.New(repos.Agents, slog.Default())
	if err := agentReg.Seed(context.Background()); err != nil {
		t.Fatal(err)
	}
	registry := tools.NewRegistry()
	if err := registry.Register(ports.Tool{Name: "echo", Description: "echo",
		Parameters: tools.Schema(map[string]any{"text": map[string]any{"type": "string"}}),
		Executor: func(ctx context.Context, call ports.ToolCallContext) (ports.ToolResult, error) {
			return ports.ToolResult{Content: "echoed"}, nil
		}}); err != nil {
		t.Fatal(err)
	}
	cfg := config.Default()
	cfg.Memory.MaxShortTurns = 10
	engine := &Engine{
		Users: repos.Users, Sessions: repos.Sessions, Messages: repos.Messages,
		Personalities: pers, Agents: agentReg, Tools: registry, Router: router,
		Config:  cfg,
		Builder: NewBuilder(nil, repos.Messages, cfg.Memory, cfg.LLM.SystemPrompt),
		Log:     slog.Default(),
	}
	return engine, repos, user
}
