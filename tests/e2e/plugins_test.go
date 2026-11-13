package e2e

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/ArsinShaabani/AIOS/internal/ports"
)

func TestPluginLifecycle(t *testing.T) {
	e := newEnv(t)
	pluginSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"content":"sunny, 24C","data":{"temp":24}}`))
	}))
	defer pluginSrv.Close()

	manifest := map[string]any{
		"name": "weather-test", "version": "1.0.0", "description": "weather",
		"endpoint": pluginSrv.URL, "permissions": []string{"internet"},
		"tools": []map[string]any{{
			"name": "weather_test", "description": "current weather", "method": "POST",
			"path": "/tools/weather", "parameters": map[string]any{"type": "object"},
		}},
		"enabled": true,
	}
	status, installed := e.do("POST", "/api/v1/plugins", e.token, manifest)
	if status != 201 {
		t.Fatalf("install plugin: %d %v", status, installed)
	}
	pluginID := installed["id"].(string)
	if status, list := e.do("GET", "/api/v1/plugins", e.token, nil); status != 200 || len(list["items"].([]any)) != 1 {
		t.Fatalf("list plugins: %d %v", status, list)
	}
	tool, ok := e.app.Tools.Get("weather_test")
	if !ok {
		t.Fatal("plugin tool not registered")
	}
	result, err := tool.Executor(context.Background(), ports.ToolCallContext{UserID: "u1", Arguments: map[string]any{"city": "Tehran"}})
	if err != nil || result.Content != "sunny, 24C" || result.Data["temp"].(float64) != 24 {
		t.Fatalf("plugin execution: %v %+v", err, result)
	}
	if status, _ := e.do("PATCH", "/api/v1/plugins/"+pluginID, e.token, map[string]any{"enabled": false}); status != 200 {
		t.Fatalf("disable plugin: %d", status)
	}
	if _, ok := e.app.Tools.Get("weather_test"); ok {
		t.Fatal("disabled plugin tool still registered")
	}
	if status, _ := e.do("PATCH", "/api/v1/plugins/"+pluginID, e.token, map[string]any{"enabled": true}); status != 200 {
		t.Fatalf("enable plugin: %d", status)
	}
	if status, _ := e.do("DELETE", "/api/v1/plugins/"+pluginID, e.token, nil); status != 204 {
		t.Fatalf("delete plugin: %d", status)
	}
	if _, ok := e.app.Tools.Get("weather_test"); ok {
		t.Fatal("deleted plugin tool still registered")
	}

	bad := map[string]any{"name": "bad", "endpoint": pluginSrv.URL, "tools": []map[string]any{
		{"name": "Bad Name"},
	}}
	if status, _ := e.do("POST", "/api/v1/plugins", e.token, bad); status != 400 {
		t.Fatalf("invalid plugin must be 400, got %d", status)
	}
	if status, _ := e.do("POST", "/api/v1/plugins", e.token, map[string]any{
		"name": "no-tools", "endpoint": pluginSrv.URL, "tools": []any{},
	}); status != 400 {
		t.Fatalf("plugin without tools must be 400, got %d", status)
	}
	if status, _ := e.do("POST", "/api/v1/plugins", e.token, map[string]any{
		"name": "perm", "endpoint": pluginSrv.URL, "permissions": []string{"admin"},
		"tools": []map[string]any{{"name": "perm_tool"}},
	}); status != 400 {
		t.Fatalf("admin permission must be rejected, got %d", status)
	}
}
