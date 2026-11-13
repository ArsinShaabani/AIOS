package e2e

import (
	"bytes"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"
)

func TestStaticFilesHeadersAndMeta(t *testing.T) {
	e := newEnv(t)
	req, _ := http.NewRequest("GET", e.server.URL+"/", nil)
	req.Header.Set("X-Request-ID", "req-test-123")
	resp, err := e.client.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	body, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if resp.StatusCode != 200 || !bytes.Contains(body, []byte("AIOS")) {
		t.Fatalf("index: %d", resp.StatusCode)
	}
	if resp.Header.Get("X-Content-Type-Options") != "nosniff" || resp.Header.Get("X-Frame-Options") != "DENY" {
		t.Fatal("security headers missing")
	}
	if resp.Header.Get("Content-Security-Policy") == "" || resp.Header.Get("X-Request-ID") != "req-test-123" {
		t.Fatal("CSP or request id missing")
	}
	for _, path := range []string{"/app.js", "/styles.css", "/views.js", "/voice.js"} {
		r, err := e.client.Get(e.server.URL + path)
		if err != nil {
			t.Fatal(err)
		}
		r.Body.Close()
		if r.StatusCode != 200 {
			t.Fatalf("%s: %d", path, r.StatusCode)
		}
	}
	openapi, err := e.client.Get(e.server.URL + "/openapi.yaml")
	if err != nil {
		t.Fatal(err)
	}
	spec, _ := io.ReadAll(openapi.Body)
	openapi.Body.Close()
	if openapi.StatusCode != 200 || !strings.Contains(string(spec), "openapi: 3.0.3") {
		t.Fatalf("openapi: %d", openapi.StatusCode)
	}
	if status, meta := e.do("GET", "/api/v1/meta", "", nil); status != 200 || meta["name"] != "AIOS" {
		t.Fatalf("meta: %d %v", status, meta)
	}
	if _, err := e.client.Get(e.server.URL + "/graphql"); err != nil {
		t.Fatal(err)
	}
	page, err := e.client.Get(e.server.URL + "/graphql")
	if err != nil {
		t.Fatal(err)
	}
	page.Body.Close()
	if page.StatusCode != 200 || !strings.HasPrefix(page.Header.Get("Content-Type"), "text/html") {
		t.Fatalf("graphql page: %d %s", page.StatusCode, page.Header.Get("Content-Type"))
	}
	if status, _ := e.do("GET", "/api/v1/does-not-exist", e.token, nil); status != 404 {
		t.Fatalf("unknown route: %d", status)
	}
	if status, _ := e.do("GET", "/api/v1/chat", e.token, nil); status != 405 && status != 404 {
		t.Fatalf("wrong method: %d", status)
	}
	if status, _ := e.do("POST", "/api/v1/chat", e.token, map[string]any{"message": 123}); status != 400 {
		t.Fatalf("invalid json type: %d", status)
	}
	if status, _ := e.do("POST", "/api/v1/devices", e.token, map[string]any{"name": "", "capabilities": []any{}}); status != 400 {
		t.Fatalf("invalid device: %d", status)
	}

	// CORS preflight from a wildcard origin.
	req, _ = http.NewRequest(http.MethodOptions, e.server.URL+"/api/v1/chat", nil)
	req.Header.Set("Origin", "https://frontend.test")
	req.Header.Set("Access-Control-Request-Method", "POST")
	resp, err = e.client.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusNoContent || resp.Header.Get("Access-Control-Allow-Origin") == "" {
		t.Fatalf("preflight: %d %q", resp.StatusCode, resp.Header.Get("Access-Control-Allow-Origin"))
	}
}

func TestAdminExtras(t *testing.T) {
	e := newEnv(t)
	_, chat := e.do("POST", "/api/v1/chat", e.token, map[string]any{"message": "hello"})
	sid := chat["session_id"].(string)

	if status, usage := e.do("GET", "/api/v1/usage?all=true", e.token, nil); status != 200 || usage["totals"] == nil {
		t.Fatalf("usage all: %d", status)
	}
	if status, _ := e.do("GET", "/api/v1/usage/daily?all=true", e.token, nil); status != 200 {
		t.Fatal("daily usage")
	}
	if status, _ := e.do("GET", "/api/v1/usage/providers", e.token, nil); status != 200 {
		t.Fatal("provider usage")
	}
	// Flush buffered audit events (the auditor is started by App.Start, not by newEnv).
	e.app.Auditor.Stop(t.Context())
	if status, audit := e.do("GET", "/api/v1/admin/audit?action=auth.login&status=success", e.token, nil); status != 200 || audit["total"].(float64) < 1 {
		t.Fatalf("audit filter: %d %v", status, audit)
	}
	if status, _ := e.do("GET", "/api/v1/admin/queue", e.token, nil); status != 200 {
		t.Fatal("queue")
	}
	if status, _ := e.do("GET", "/api/v1/admin/system", e.token, nil); status != 200 {
		t.Fatal("system")
	}
	if status, _ := e.do("GET", "/api/v1/models", e.token, nil); status != 200 {
		t.Fatal("models")
	}
	if status, devs := e.do("GET", "/api/v1/devices?all=true", e.token, nil); status != 200 || devs["items"] == nil {
		t.Fatalf("devices all: %d", status)
	}
	if status, _ := e.do("GET", "/api/v1/admin/prompt/"+sid+"?agent_id=agent_general&personality_id=per_concise", e.token, nil); status != 200 {
		t.Fatal("prompt overrides")
	}
	if status, _ := e.do("PUT", "/api/v1/settings/theme?scope=user", e.token, map[string]any{"mode": "dark"}); status != 200 {
		t.Fatal("user setting")
	}
	if status, list := e.do("GET", "/api/v1/settings?scope=user", e.token, nil); status != 200 || len(list["items"].([]any)) != 1 {
		t.Fatalf("user settings list: %d %v", status, list)
	}
	if status, _ := e.do("DELETE", "/api/v1/settings/theme?scope=user", e.token, nil); status != 204 {
		t.Fatal("delete user setting")
	}
	if status, pruned := e.do("POST", "/api/v1/admin/maintenance/prune", e.token, map[string]any{}); status != 200 || pruned["tokens_deleted"] == nil {
		t.Fatalf("prune: %d %v", status, pruned)
	}
	// Durable task cancellation by an operator/admin.
	taskID, err := e.app.Scheduler.CreateReminder(t.Context(), "usr_any", "test", time.Now().Add(time.Hour), "")
	if err != nil {
		t.Fatal(err)
	}
	if status, tasks := e.do("GET", "/api/v1/tasks?status=pending", e.token, nil); status != 200 || tasks["total"].(float64) < 1 {
		t.Fatalf("tasks: %d %v", status, tasks)
	}
	if status, _ := e.do("DELETE", "/api/v1/tasks/"+taskID, e.token, nil); status != 204 {
		t.Fatalf("cancel task: %d", status)
	}
}
