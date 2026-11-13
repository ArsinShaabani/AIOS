package brain

import (
	"context"
	"strings"
	"sync"
	"testing"

	"github.com/ArsinShaabani/AIOS/internal/config"
	"github.com/ArsinShaabani/AIOS/internal/domain"
	"github.com/ArsinShaabani/AIOS/internal/platform"
)

func TestChatToolLoopAndPersistence(t *testing.T) {
	router := &scriptedRouter{}
	engine, repos, user := newEngine(t, router)
	out, err := engine.Chat(context.Background(), ChatInput{UserID: user.ID, Message: "use the tool"}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if out.Content != "final answer" || out.AgentID == "" || out.MessageID == "" {
		t.Fatalf("output: %+v", out)
	}
	if len(out.ToolCalls) != 1 || out.ToolCalls[0].Name != "echo" {
		t.Fatalf("tool calls: %+v", out.ToolCalls)
	}
	if router.calls != 2 {
		t.Fatalf("router calls=%d", router.calls)
	}
	msgs, err := repos.Messages.ListBySession(context.Background(), out.SessionID, 20)
	if err != nil || len(msgs) < 4 {
		t.Fatalf("messages: %v %d", err, len(msgs))
	}
	var sawTool bool
	for _, m := range msgs {
		if m.Role == domain.MsgTool && m.Content == "echoed" {
			sawTool = true
		}
	}
	if !sawTool {
		t.Fatalf("tool result not persisted: %+v", msgs)
	}
	if out.Usage.TotalTokens == 0 {
		t.Fatal("usage not aggregated")
	}
}

func TestChatValidationAndOwnership(t *testing.T) {
	engine, _, user := newEngine(t, &scriptedRouter{})
	ctx := context.Background()
	if _, err := engine.Chat(ctx, ChatInput{UserID: user.ID, Message: ""}, nil); !platform.IsInvalid(err) {
		t.Fatalf("empty message: %v", err)
	}
	if _, err := engine.Chat(ctx, ChatInput{UserID: "missing", Message: "hi"}, nil); !platform.IsUnauthorized(err) {
		t.Fatalf("unknown user: %v", err)
	}
	user.Status = "disabled"
	_ = engine.Users.Update(ctx, user)
	if _, err := engine.Chat(ctx, ChatInput{UserID: user.ID, Message: "hi"}, nil); !platform.IsForbidden(err) {
		t.Fatalf("disabled user: %v", err)
	}
}

func TestChatStreamingEvents(t *testing.T) {
	engine, _, user := newEngine(t, &scriptedRouter{})
	var (
		mu   sync.Mutex
		seen []string
	)
	out, err := engine.Chat(context.Background(), ChatInput{UserID: user.ID, Message: "stream please", Stream: true},
		func(ev Event) error {
			mu.Lock()
			seen = append(seen, ev.Type)
			mu.Unlock()
			return nil
		})
	if err != nil {
		t.Fatal(err)
	}
	mu.Lock()
	defer mu.Unlock()
	joined := strings.Join(seen, ",")
	for _, want := range []string{"session", "tool_start", "tool_result", "delta", "done"} {
		if !strings.Contains(joined, want) {
			t.Fatalf("missing %q in %s", want, joined)
		}
	}
	if !strings.Contains(out.Content, "streamed") {
		t.Fatalf("content: %q", out.Content)
	}
}

func TestPreviewAndContextBudget(t *testing.T) {
	engine, repos, user := newEngine(t, &scriptedRouter{})
	ctx := context.Background()
	out, err := engine.Chat(ctx, ChatInput{UserID: user.ID, Message: "hello"}, nil)
	if err != nil {
		t.Fatal(err)
	}
	preview, err := engine.PreviewPrompt(ctx, user.ID, out.SessionID, "agent_general", "per_concise")
	if err != nil {
		t.Fatal(err)
	}
	if preview.System == "" || len(preview.Messages) == 0 || preview.AgentID == "" {
		t.Fatalf("preview: %+v", preview)
	}
	other := &domain.User{ID: "u2", Email: "other@test.local", PasswordHash: "x", Role: domain.RoleUser, Status: "active"}
	_ = repos.Users.Create(ctx, other)
	if _, err := engine.PreviewPrompt(ctx, other.ID, out.SessionID, "", ""); !platform.IsForbidden(err) {
		t.Fatalf("preview ownership: %v", err)
	}

	builder := NewBuilder(nil, repos.Messages, config.MemoryConfig{MaxShortTurns: 50, MaxContextTokens: 300}, "")
	session := &domain.Session{ID: out.SessionID, UserID: user.ID}
	for i := 0; i < 10; i++ {
		_ = repos.Messages.Create(ctx, &domain.Message{ID: platform.NewID("msg"), SessionID: session.ID,
			UserID: user.ID, Role: domain.MsgUser, Content: strings.Repeat("word ", 80)})
		_ = repos.Messages.Create(ctx, &domain.Message{ID: platform.NewID("msg"), SessionID: session.ID,
			UserID: user.ID, Role: domain.MsgAssistant, Content: strings.Repeat("reply ", 80)})
	}
	built, err := builder.Build(ctx, ContextInput{User: user, Session: session,
		Personality: &domain.Personality{Name: "P"}, Agent: &domain.Agent{ID: "agent_general"}, Query: "hello",
		Intent: Detect("hello"), Channel: domain.ChannelAPI})
	if err != nil {
		t.Fatal(err)
	}
	if built.EstimatedTokens > 300 && len(built.Messages) > 3 {
		t.Fatalf("context not trimmed: tokens=%d messages=%d", built.EstimatedTokens, len(built.Messages))
	}
}
