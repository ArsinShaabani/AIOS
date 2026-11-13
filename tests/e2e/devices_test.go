package e2e

import (
	"bytes"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestDevicesAndRBAC(t *testing.T) {
	e := newEnv(t)
	cmdServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"ok":true,"detail":"device acknowledged"}`))
	}))
	defer cmdServer.Close()

	status, body := e.do("POST", "/api/v1/devices", e.token, map[string]any{
		"name": "Kitchen light", "kind": "light", "protocol": "http", "room": "Kitchen",
		"metadata": map[string]any{"command_url": cmdServer.URL},
		"capabilities": []map[string]any{
			{"name": "power", "type": "switch"},
			{"name": "brightness", "type": "number", "min": 0, "max": 100},
		},
	})
	if status != 201 {
		t.Fatalf("device create: %d %v", status, body)
	}
	device := body["device"].(map[string]any)
	deviceID := device["id"].(string)
	token := body["token"].(string)

	req, _ := http.NewRequest("POST", e.server.URL+"/api/v1/device/telemetry",
		strings.NewReader(`{"metric":"temperature","value":21.5,"unit":"C"}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)
	resp, err := e.client.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != 202 {
		t.Fatalf("telemetry ingest: %d", resp.StatusCode)
	}

	status, cmd := e.do("POST", "/api/v1/devices/"+deviceID+"/command", e.token, map[string]any{"capability": "brightness", "value": 1000})
	if status != 400 {
		t.Fatalf("out-of-range command must be rejected, got %d %v", status, cmd)
	}
	status, cmd = e.do("POST", "/api/v1/devices/"+deviceID+"/command", e.token, map[string]any{"capability": "brightness", "value": 60})
	if status != 200 {
		t.Fatalf("valid command failed: %d %v", status, cmd)
	}

	status, user := e.do("POST", "/api/v1/admin/users", e.token, map[string]any{
		"email": "viewer@test.local", "password": "ViewerPassword123!", "role": "viewer",
	})
	if status != 201 {
		t.Fatalf("create viewer: %d %v", status, user)
	}
	viewerToken := e.login("viewer@test.local", "ViewerPassword123!")
	if status, _ := e.do("POST", "/api/v1/devices", viewerToken, map[string]any{"name": "Nope"}); status != 403 {
		t.Fatalf("viewer device create must be 403, got %d", status)
	}
	if status, _ := e.do("GET", "/api/v1/admin/users", viewerToken, nil); status != 403 {
		t.Fatalf("viewer user list must be 403, got %d", status)
	}
	if status, _ := e.do("GET", "/api/v1/devices", viewerToken, nil); status != 200 {
		t.Fatalf("viewer may read devices, got %d", status)
	}
}

func TestGraphQLAndMetrics(t *testing.T) {
	e := newEnv(t)
	query := `{"query":"query { me { email role } agents { name } }"}`
	req, _ := http.NewRequest("POST", e.server.URL+"/graphql", strings.NewReader(query))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+e.token)
	resp, err := e.client.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if resp.StatusCode != 200 || !strings.Contains(string(raw), "admin@test.local") {
		t.Fatalf("graphql: %d %s", resp.StatusCode, raw)
	}
	metrics, err := e.client.Get(e.server.URL + "/metrics")
	if err != nil {
		t.Fatal(err)
	}
	defer metrics.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(metrics.Body, 4<<20))
	if !bytes.Contains(body, []byte("aios_http_requests_total")) {
		t.Fatal("prometheus metrics missing")
	}
	ready, err := e.client.Get(e.server.URL + "/readyz")
	if err != nil || ready.StatusCode != 200 {
		t.Fatalf("readyz: %v", err)
	}
	ready.Body.Close()
}
