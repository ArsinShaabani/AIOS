package tools

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/ArsinShaabani/AIOS/internal/ports"
)

func TestRegistryLifecycle(t *testing.T) {
	r := NewRegistry()
	if err := r.Register(ports.Tool{Name: "bad"}); err == nil {
		t.Fatal("tool without executor accepted")
	}
	if err := r.Register(ports.Tool{Name: "x", Executor: func(context.Context, ports.ToolCallContext) (ports.ToolResult, error) {
		return ports.ToolResult{}, nil
	}, Parameters: json.RawMessage(`{invalid`)}); err == nil {
		t.Fatal("invalid schema accepted")
	}
	err := r.Register(ports.Tool{
		Name: "echo", Description: "echo", Parameters: Schema(map[string]any{"text": map[string]any{"type": "string"}}, "text"),
		Executor: func(ctx context.Context, call ports.ToolCallContext) (ports.ToolResult, error) {
			return ports.ToolResult{Content: stringArg(call.Arguments, "text")}, nil
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := r.Get("echo"); !ok {
		t.Fatal("tool missing")
	}
	specs := r.Specs([]string{"echo", "missing"})
	if len(specs) != 1 || specs[0].Name != "echo" {
		t.Fatalf("specs: %+v", specs)
	}
	res, err := r.Execute(context.Background(), "echo", ports.ToolCallContext{Arguments: map[string]any{"text": "hi"}})
	if err != nil || res.Content != "hi" {
		t.Fatalf("execute: %v %+v", err, res)
	}
	if _, err := r.Execute(context.Background(), "nope", ports.ToolCallContext{}); err == nil {
		t.Fatal("unknown tool should error")
	}
	if names := r.Names(); len(names) != 1 || names[0] != "echo" {
		t.Fatalf("names: %v", names)
	}
	r.Unregister("echo")
	if _, ok := r.Get("echo"); ok {
		t.Fatal("unregister failed")
	}
}

func TestExecuteHonoursTimeoutAndPanic(t *testing.T) {
	r := NewRegistry()
	_ = r.Register(ports.Tool{Name: "slow", Timeout: 1, Executor: func(ctx context.Context, _ ports.ToolCallContext) (ports.ToolResult, error) {
		select {
		case <-ctx.Done():
			return ports.ToolResult{IsError: true, Content: "timeout"}, ctx.Err()
		case <-time.After(2 * time.Second):
			return ports.ToolResult{}, nil
		}
	}})
	start := time.Now()
	_, err := r.Execute(context.Background(), "slow", ports.ToolCallContext{})
	if err == nil {
		t.Fatal("expected timeout error")
	}
	if time.Since(start) > 1500*time.Millisecond {
		t.Fatal("timeout not enforced")
	}
}

func TestExtractText(t *testing.T) {
	html := `<html><head><title>Hello</title><style>x{}</style></head><body>
	<h1>Title</h1><script>evil()</script><p>First para.</p><p>Second <b>para</b>.</p></body></html>`
	title, text := ExtractText([]byte(html))
	if title != "Hello" {
		t.Fatalf("title=%q", title)
	}
	if strings.Contains(text, "evil") || strings.Contains(text, "x{}") {
		t.Fatalf("scripts/styles leaked: %q", text)
	}
	if !strings.Contains(text, "First para.") || !strings.Contains(text, "Second para.") {
		t.Fatalf("content missing: %q", text)
	}
}

func TestParseWhen(t *testing.T) {
	if _, err := ParseWhen("10m"); err != nil {
		t.Fatal(err)
	}
	if _, err := ParseWhen(time.Now().Add(time.Hour).Format(time.RFC3339)); err != nil {
		t.Fatal(err)
	}
	if _, err := ParseWhen("not-a-time"); err == nil {
		t.Fatal("garbage accepted")
	}
	if _, err := ParseWhen("-5m"); err == nil {
		t.Fatal("negative duration accepted")
	}
	if _, err := ParseWhen(time.Now().Add(-time.Hour).Format(time.RFC3339)); err == nil {
		t.Fatal("past time accepted")
	}
}
