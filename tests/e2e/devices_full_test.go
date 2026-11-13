package e2e

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestDeviceFullLifecycle(t *testing.T) {
	e := newEnv(t)
	cmdServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"ok":true,"detail":"ack"}`))
	}))
	defer cmdServer.Close()

	status, body := e.do("POST", "/api/v1/devices", e.token, map[string]any{
		"name": "Desk lamp", "kind": "light", "protocol": "http", "room": "Office",
		"metadata": map[string]any{"command_url": cmdServer.URL},
		"capabilities": []map[string]any{
			{"name": "power", "type": "switch"},
			{"name": "brightness", "type": "number", "min": 0, "max": 100},
			{"name": "scene", "type": "enum", "enum": []string{"reading", "movie"}},
		},
	})
	if status != 201 {
		t.Fatalf("create device: %d %v", status, body)
	}
	device := body["device"].(map[string]any)
	id := device["id"].(string)
	token := body["token"].(string)

	if status, got := e.do("GET", "/api/v1/devices/"+id, e.token, nil); status != 200 || got["name"] != "Desk lamp" {
		t.Fatalf("get device: %d %v", status, got)
	}
	if status, _ := e.do("PATCH", "/api/v1/devices/"+id, e.token, map[string]any{"name": "Desk lamp 2", "room": "Study"}); status != 200 {
		t.Fatalf("patch device: %d", status)
	}
	if status, all := e.do("GET", "/api/v1/devices?kind=light&q=Desk", e.token, nil); status != 200 || all["total"].(float64) < 1 {
		t.Fatalf("filter devices: %d %v", status, all)
	}
	if status, online := e.do("GET", "/api/v1/devices?status=online", e.token, nil); status != 200 || online["total"].(float64) != 0 {
		t.Fatalf("online filter: %d %v", status, online)
	}
	if status, _ := e.do("GET", "/api/v1/devices/"+id+"/telemetry", e.token, nil); status != 200 {
		t.Fatalf("telemetry read: %d", status)
	}
	if status, _ := e.do("POST", "/api/v1/devices/"+id+"/command", e.token, map[string]any{"capability": "unknown", "value": 1}); status != 400 {
		t.Fatalf("unknown capability must be 400, got %d", status)
	}
	if status, _ := e.do("POST", "/api/v1/devices/"+id+"/command", e.token, map[string]any{"capability": "scene", "value": "nope"}); status != 400 {
		t.Fatalf("bad enum must be 400, got %d", status)
	}
	if status, res := e.do("POST", "/api/v1/devices/"+id+"/command", e.token, map[string]any{"capability": "scene", "value": "reading"}); status != 200 || res["result"].(map[string]any)["ok"] != true {
		t.Fatalf("valid command: %d %v", status, res)
	}

	status, rotated := e.do("POST", "/api/v1/devices/"+id+"/rotate-token", e.token, nil)
	if status != 200 {
		t.Fatalf("rotate token: %d", status)
	}
	newToken := rotated["token"].(string)
	if status := e.devicePost("/api/v1/device/heartbeat", token, `{}`); status != 401 {
		t.Fatalf("old token must be rejected, got %d", status)
	}
	if status := e.devicePost("/api/v1/device/heartbeat", newToken, `{}`); status != 200 {
		t.Fatalf("new token heartbeat: %d", status)
	}
	if status := e.devicePost("/api/v1/device/telemetry", newToken, `{"metric":"power","value":true}`); status != 202 {
		t.Fatalf("telemetry ingest: %d", status)
	}
	req, _ := http.NewRequest("GET", e.server.URL+"/api/v1/device/me", nil)
	req.Header.Set("Authorization", "Bearer "+newToken)
	resp, _ := e.client.Do(req)
	resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Fatalf("device me: %d", resp.StatusCode)
	}
	if status, _ := e.do("DELETE", "/api/v1/devices/"+id, e.token, nil); status != 204 {
		t.Fatalf("delete device: %d", status)
	}
}

func (e *env) devicePost(path, token, body string) int {
	e.t.Helper()
	req, _ := http.NewRequest("POST", e.server.URL+path, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)
	resp, err := e.client.Do(req)
	if err != nil {
		e.t.Fatal(err)
	}
	defer resp.Body.Close()
	return resp.StatusCode
}
