// Package e2e exercises the assembled application over real HTTP.
package e2e

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/ArsinShaabani/AIOS/internal/app"
	"github.com/ArsinShaabani/AIOS/internal/config"
)

const adminPassword = "AdminPassword123!"

type env struct {
	t      *testing.T
	server *httptest.Server
	app    *app.App
	client *http.Client
	token  string
}

func newEnv(t *testing.T) *env {
	t.Helper()
	cfg := config.Default()
	cfg.App.DataDir = t.TempDir()
	cfg.Database.DSN = "e2e.db"
	cfg.Security.Bootstrap = config.BootstrapUser{Email: "admin@test.local", Name: "Admin", Password: adminPassword}
	cfg.Security.BcryptCost = 8
	cfg.Security.AccessTTL = config.Duration(10 * time.Minute)
	cfg.Voice.Enabled = true
	cfg.Voice.STT.Provider = "mock"
	cfg.Voice.TTS.Provider = "mock"
	cfg.Tools.SearchProvider = "mock"
	cfg.Plugins.Dir = t.TempDir()
	cfg.Server.RateLimit.Enabled = false
	cfg.Observability.MetricsEnabled = true
	cfg.Scheduler.Enabled = true
	cfg.Memory.LongTermEnabled = true
	cfg.Memory.SemanticEnabled = true
	if err := cfg.Validate(); err != nil {
		t.Fatalf("test config: %v", err)
	}
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	application, err := app.New(context.Background(), cfg, logger, "test")
	if err != nil {
		t.Fatalf("app.New: %v", err)
	}
	if err := application.Seed(context.Background()); err != nil {
		t.Fatalf("seed: %v", err)
	}
	srv := httptest.NewServer(application.Handler())
	e := &env{t: t, server: srv, app: application, client: srv.Client()}
	t.Cleanup(func() {
		srv.Close()
		application.Close(context.Background())
	})
	e.token = e.login("admin@test.local", adminPassword)
	return e
}

func (e *env) do(method, path, token string, body any) (int, map[string]any) {
	e.t.Helper()
	var reader io.Reader
	if body != nil {
		raw, _ := json.Marshal(body)
		reader = bytes.NewReader(raw)
	}
	req, err := http.NewRequest(method, e.server.URL+path, reader)
	if err != nil {
		e.t.Fatal(err)
	}
	req.Header.Set("Content-Type", "application/json")
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	resp, err := e.client.Do(req)
	if err != nil {
		e.t.Fatal(err)
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(io.LimitReader(resp.Body, 8<<20))
	out := map[string]any{}
	_ = json.Unmarshal(raw, &out)
	return resp.StatusCode, out
}

func (e *env) login(email, password string) string {
	e.t.Helper()
	status, body := e.do("POST", "/api/v1/auth/login", "", map[string]string{"email": email, "password": password})
	if status != 200 {
		e.t.Fatalf("login failed: %d %v", status, body)
	}
	return body["tokens"].(map[string]any)["access_token"].(string)
}

func TestHealthAndAuth(t *testing.T) {
	e := newEnv(t)
	status, body := e.do("GET", "/healthz", "", nil)
	if status != 200 || body["status"] != "ok" {
		t.Fatalf("health: %d %v", status, body)
	}
	status, body = e.do("GET", "/readyz", "", nil)
	if status != 200 {
		t.Fatalf("ready: %d %v", status, body)
	}
	if status, _ := e.do("GET", "/api/v1/auth/me", "", nil); status != 401 {
		t.Fatalf("unauthenticated request must be 401, got %d", status)
	}
	status, body = e.do("GET", "/api/v1/auth/me", e.token, nil)
	if status != 200 || body["user"] == nil {
		t.Fatalf("me: %d %v", status, body)
	}
	if status, _ := e.do("POST", "/api/v1/auth/login", "", map[string]string{"email": "admin@test.local", "password": "wrong"}); status != 401 {
		t.Fatalf("wrong password must be 401, got %d", status)
	}
}

func TestChatMemoryAndSSE(t *testing.T) {
	e := newEnv(t)
	status, out := e.do("POST", "/api/v1/chat", e.token, map[string]any{
		"message": "My name is Ada and I love green tea", "channel": "web",
	})
	if status != 200 {
		t.Fatalf("chat: %d %v", status, out)
	}
	sessionID, _ := out["session_id"].(string)
	if sessionID == "" || out["content"] == "" {
		t.Fatalf("chat output: %v", out)
	}
	status, msgs := e.do("GET", "/api/v1/sessions/"+sessionID+"/messages", e.token, nil)
	if status != 200 {
		t.Fatalf("messages: %d", status)
	}
	if items, _ := msgs["items"].([]any); len(items) < 2 {
		t.Fatalf("expected persisted turns, got %v", msgs)
	}
	status, mem := e.do("POST", "/api/v1/memories", e.token, map[string]any{"content": "Prefers oat milk", "kind": "preference"})
	if status != 201 || mem["id"] == nil {
		t.Fatalf("memory create: %d %v", status, mem)
	}
	status, list := e.do("GET", "/api/v1/memories", e.token, nil)
	if status != 200 || list["total"].(float64) < 1 {
		t.Fatalf("memory list: %d %v", status, list)
	}

	req, _ := http.NewRequest("POST", e.server.URL+"/api/v1/chat",
		strings.NewReader(`{"message":"hello there","stream":true}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+e.token)
	req.Header.Set("Accept", "text/event-stream")
	resp, err := e.client.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	text := string(raw)
	if !strings.Contains(text, `"type":"delta"`) || !strings.Contains(text, `"type":"done"`) {
		t.Fatalf("stream missing events: %s", text)
	}
}
