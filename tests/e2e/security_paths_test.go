package e2e

import (
	"net/http"
	"strings"
	"testing"

	"github.com/gorilla/websocket"
)

func TestOwnershipAndRBACPaths(t *testing.T) {
	e := newEnv(t)
	_, chat := e.do("POST", "/api/v1/chat", e.token, map[string]any{"message": "admin session"})
	sid := chat["session_id"].(string)
	_, created := e.do("POST", "/api/v1/admin/users", e.token, map[string]any{
		"email": "viewer2@test.local", "password": "ViewerPassword123!", "role": "viewer",
	})
	viewerID := created["id"].(string)
	viewer := e.login("viewer2@test.local", "ViewerPassword123!")

	if status, _ := e.do("GET", "/api/v1/sessions/"+sid, viewer, nil); status != 403 {
		t.Fatalf("session read ownership: %d", status)
	}
	if status, _ := e.do("PATCH", "/api/v1/sessions/"+sid, viewer, map[string]any{"title": "x"}); status != 403 {
		t.Fatalf("session patch ownership: %d", status)
	}
	if status, _ := e.do("GET", "/api/v1/sessions/"+sid+"/messages", viewer, nil); status != 403 {
		t.Fatalf("messages ownership: %d", status)
	}
	if status, _ := e.do("DELETE", "/api/v1/sessions/"+sid, viewer, nil); status != 403 {
		t.Fatalf("session delete ownership: %d", status)
	}
	if status, _ := e.do("POST", "/api/v1/memories", viewer, map[string]any{"content": "nope"}); status != 403 {
		t.Fatalf("viewer memory write: %d", status)
	}
	if status, _ := e.do("GET", "/api/v1/plugins", viewer, nil); status != 200 {
		t.Fatalf("viewer plugin read: %d", status)
	}
	if status, _ := e.do("POST", "/api/v1/plugins", viewer, map[string]any{"name": "x"}); status != 403 {
		t.Fatalf("viewer plugin write: %d", status)
	}
	if status, _ := e.do("GET", "/api/v1/admin/audit", viewer, nil); status != 403 {
		t.Fatalf("viewer audit: %d", status)
	}
	if status, _ := e.do("GET", "/api/v1/admin/prompt/"+sid, viewer, nil); status != 403 {
		t.Fatalf("viewer prompt: %d", status)
	}
	if status, _ := e.do("GET", "/api/v1/models", viewer, nil); status != 200 {
		t.Fatalf("viewer models: %d", status)
	}
	if status, _ := e.do("GET", "/api/v1/usage", viewer, nil); status != 200 {
		t.Fatalf("viewer usage: %d", status)
	}
	if status, _ := e.do("PATCH", "/api/v1/admin/users/"+viewerID, e.token, map[string]any{"status": "disabled"}); status != 200 {
		t.Fatalf("disable user: %d", status)
	}
	if status, _ := e.do("POST", "/api/v1/auth/login", "", map[string]string{"email": "viewer2@test.local", "password": "ViewerPassword123!"}); status != 401 {
		t.Fatalf("disabled login: %d", status)
	}
	if status, _ := e.do("PATCH", "/api/v1/admin/users/"+viewerID, e.token, map[string]any{"status": "active"}); status != 200 {
		t.Fatalf("enable user: %d", status)
	}
	if status, _ := e.do("POST", "/api/v1/auth/login", "", map[string]string{"email": "viewer2@test.local", "password": "ViewerPassword123!"}); status != 200 {
		t.Fatalf("re-enabled login: %d", status)
	}
	if status, _ := e.do("DELETE", "/api/v1/personalities/per_concise", e.token, nil); status != 204 {
		t.Fatalf("admin preset delete: %d", status)
	}
	_, custom := e.do("POST", "/api/v1/personalities", e.token, map[string]any{"name": "Private"})
	pid := custom["id"].(string)
	if status, _ := e.do("DELETE", "/api/v1/personalities/"+pid, viewer, nil); status != 403 {
		t.Fatalf("foreign personality delete: %d", status)
	}
	if status, _ := e.do("PUT", "/api/v1/personalities/"+pid, viewer, map[string]any{"name": "Hack"}); status != 403 {
		t.Fatalf("foreign personality update: %d", status)
	}
}

func TestDeviceAuthFailuresAndWSAPIKey(t *testing.T) {
	e := newEnv(t)
	if status := e.devicePost("/api/v1/device/telemetry", "", `{"metric":"x","value":1}`); status != 401 {
		t.Fatalf("missing device token: %d", status)
	}
	if status := e.devicePost("/api/v1/device/telemetry", "dev_invalid", `{"metric":"x","value":1}`); status != 401 {
		t.Fatalf("invalid device token: %d", status)
	}
	req, _ := http.NewRequest("GET", e.server.URL+"/api/v1/device/me", nil)
	resp, _ := e.client.Do(req)
	resp.Body.Close()
	if resp.StatusCode != 401 {
		t.Fatalf("device me without token: %d", resp.StatusCode)
	}

	_, key := e.do("POST", "/api/v1/auth/api-keys", e.token, map[string]any{"name": "ws", "scopes": []string{"chat"}})
	apiKey := key["token"].(string)
	url := strings.Replace(e.server.URL, "http://", "ws://", 1) + "/api/v1/ws/chat?token=" + apiKey
	conn, _, err := websocket.DefaultDialer.Dial(url, nil)
	if err != nil {
		t.Fatalf("ws api key dial: %v", err)
	}
	defer conn.Close()
	readUntil(t, conn, "ready", 3)
	if err := conn.WriteJSON(map[string]any{"type": "unknown"}); err != nil {
		t.Fatal(err)
	}
	readUntil(t, conn, "error", 3)
	if _, _, err := websocket.DefaultDialer.Dial(strings.Replace(e.server.URL, "http://", "ws://", 1)+"/api/v1/ws/chat?token=bad", nil); err == nil {
		t.Fatal("invalid ws token accepted")
	}
}
