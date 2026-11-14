package e2e

import (
	"strings"
	"testing"
)

func TestPersonalitiesAndAgents(t *testing.T) {
	e := newEnv(t)
	status, personalities := e.do("GET", "/api/v1/personalities", e.token, nil)
	if status != 200 || len(personalities["items"].([]any)) < 5 {
		t.Fatalf("presets missing: %d %v", status, personalities)
	}
	status, created := e.do("POST", "/api/v1/personalities", e.token, map[string]any{
		"name": "Mine", "tone": "warm", "humor": 9, "emotional_level": 8,
		"response_length": "short", "custom_prompt": "Be brief.",
	})
	if status != 201 {
		t.Fatalf("create personality: %d %v", status, created)
	}
	pid := created["id"].(string)
	status, updated := e.do("PUT", "/api/v1/personalities/"+pid, e.token, map[string]any{"name": "Mine2", "humor": 2})
	if status != 200 || updated["name"] != "Mine2" {
		t.Fatalf("update personality: %d %v", status, updated)
	}
	if status, _ := e.do("DELETE", "/api/v1/personalities/"+pid, e.token, nil); status != 204 {
		t.Fatalf("delete personality: %d", status)
	}

	status, _ = e.do("GET", "/api/v1/agents?all=true", e.token, nil)
	if status != 200 {
		t.Fatalf("list agents: %d", status)
	}
	status, agent := e.do("POST", "/api/v1/agents", e.token, map[string]any{
		"name": "Helper", "description": "test", "system_prompt": "Help.", "tools": []string{"memory_search"},
	})
	if status != 200 {
		t.Fatalf("create agent: %d %v", status, agent)
	}
	agentID := agent["id"].(string)
	agent["enabled"] = false
	if status, _ := e.do("PUT", "/api/v1/agents/"+agentID, e.token, agent); status != 200 {
		t.Fatalf("update agent: %d", status)
	}
	if status, _ := e.do("DELETE", "/api/v1/agents/"+agentID, e.token, nil); status != 204 {
		t.Fatalf("delete agent: %d", status)
	}
	if status, _ := e.do("DELETE", "/api/v1/agents/agent_general", e.token, nil); status != 403 {
		t.Fatalf("builtin delete must be forbidden, got %d", status)
	}
	if status, _ := e.do("POST", "/api/v1/agents", e.token, map[string]any{"name": "x"}); status != 400 {
		t.Fatalf("agent without prompt must be 400, got %d", status)
	}
}

func TestSessionsAndPromptPreview(t *testing.T) {
	e := newEnv(t)
	_, chat := e.do("POST", "/api/v1/chat", e.token, map[string]any{"message": "hello there"})
	sid := chat["session_id"].(string)
	status, patched := e.do("PATCH", "/api/v1/sessions/"+sid, e.token, map[string]any{
		"title": "Renamed", "personality_id": "per_concise",
	})
	if status != 200 || patched["title"] != "Renamed" {
		t.Fatalf("patch session: %d %v", status, patched)
	}
	status, sessions := e.do("GET", "/api/v1/sessions?limit=10", e.token, nil)
	if status != 200 || sessions["total"].(float64) < 1 {
		t.Fatalf("list sessions: %d %v", status, sessions)
	}
	status, preview := e.do("GET", "/api/v1/admin/prompt/"+sid, e.token, nil)
	if status != 200 || preview["system"] == "" || len(preview["messages"].([]any)) == 0 {
		t.Fatalf("prompt preview: %d %v", status, preview)
	}
	if status, _ := e.do("GET", "/api/v1/admin/prompt/ses_missing", e.token, nil); status != 404 {
		t.Fatalf("unknown prompt must be 404, got %d", status)
	}
	if status, _ := e.do("DELETE", "/api/v1/sessions/"+sid, e.token, nil); status != 204 {
		t.Fatalf("delete session: %d", status)
	}
	if status, _ := e.do("GET", "/api/v1/sessions/"+sid, e.token, nil); status != 404 {
		t.Fatalf("deleted session must be 404, got %d", status)
	}
	if status, _ := e.do("POST", "/api/v1/chat", e.token, map[string]any{"message": ""}); status != 400 {
		t.Fatalf("empty message must be 400, got %d", status)
	}
	if status, _ := e.do("POST", "/api/v1/chat", e.token, map[string]any{"message": strings.Repeat("x", 200001)}); status != 400 {
		t.Fatalf("oversized message must be 400, got %d", status)
	}
}

func TestMemoryAndSettings(t *testing.T) {
	e := newEnv(t)
	status, mem := e.do("POST", "/api/v1/memories", e.token, map[string]any{
		"content": "Likes hiking", "kind": "preference", "importance": 0.8, "pinned": true,
	})
	if status != 201 {
		t.Fatalf("create memory: %d %v", status, mem)
	}
	id := mem["id"].(string)
	if status, list := e.do("GET", "/api/v1/memories?kind=preference&q=hiking", e.token, nil); status != 200 || list["total"].(float64) < 1 {
		t.Fatalf("filter memories: %d %v", status, list)
	}
	if status, _ := e.do("DELETE", "/api/v1/memories/"+id, e.token, nil); status != 204 {
		t.Fatalf("delete memory: %d", status)
	}
	_, mem2 := e.do("POST", "/api/v1/memories", e.token, map[string]any{"content": "Second", "kind": "fact"})
	adminMem := "/api/v1/admin/memories?user_id="
	_, me := e.do("GET", "/api/v1/auth/me", e.token, nil)
	uid := me["user"].(map[string]any)["id"].(string)
	if status, list := e.do("GET", adminMem+uid, e.token, nil); status != 200 || list["total"].(float64) < 1 {
		t.Fatalf("admin memories: %d %v", status, list)
	}
	if status, _ := e.do("GET", "/api/v1/admin/memories", e.token, nil); status != 400 {
		t.Fatalf("admin memories without user must be 400, got %d", status)
	}
	if status, wipe := e.do("DELETE", "/api/v1/memories", e.token, nil); status != 200 || wipe["deleted"].(float64) < 1 {
		t.Fatalf("wipe memories: %d %v", status, wipe)
	}
	_ = mem2

	// Settings.
	if status, _ := e.do("PUT", "/api/v1/settings/integration", e.token, map[string]any{"enabled": true}); status != 200 {
		t.Fatalf("set setting: %d", status)
	}
	if status, list := e.do("GET", "/api/v1/settings", e.token, nil); status != 200 || len(list["items"].([]any)) < 1 {
		t.Fatalf("list settings: %d %v", status, list)
	}
	if status, _ := e.do("PUT", "/api/v1/settings/secret.foo", e.token, map[string]any{"x": 1}); status != 400 {
		t.Fatalf("secret-prefixed setting must be rejected, got %d", status)
	}
	if status, _ := e.do("DELETE", "/api/v1/settings/integration", e.token, nil); status != 204 {
		t.Fatalf("delete setting: %d", status)
	}
}
