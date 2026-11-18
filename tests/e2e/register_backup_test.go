package e2e

import (
	"testing"

	"github.com/ArsinShaabani/AIOS/internal/config"
)

func TestRegistrationAndUserAdmin(t *testing.T) {
	e := newEnv(t)
	status, meta := e.do("GET", "/api/v1/meta", "", nil)
	if status != 200 || meta["allow_registration"] != true {
		t.Fatalf("meta registration: %d %v", status, meta)
	}
	status, created := e.do("POST", "/api/v1/auth/register", "", map[string]any{
		"email": "newbie@test.local", "password": "NewbiePass123!", "display_name": "Newbie",
	})
	if status != 201 {
		t.Fatalf("register: %d %v", status, created)
	}
	user := created["user"].(map[string]any)
	if user["role"] != "user" || user["email"] != "newbie@test.local" {
		t.Fatalf("registered user: %v", user)
	}
	if status, _ := e.do("POST", "/api/v1/auth/register", "", map[string]any{
		"email": "newbie@test.local", "password": "AnotherPass123!",
	}); status != 409 {
		t.Fatalf("duplicate registration must be 409, got %d", status)
	}
	if status, _ := e.do("POST", "/api/v1/auth/register", "", map[string]any{
		"email": "bad", "password": "short",
	}); status != 400 {
		t.Fatalf("invalid registration must be 400, got %d", status)
	}
	newbie := e.login("newbie@test.local", "NewbiePass123!")
	if status, _ := e.do("GET", "/api/v1/admin/users", newbie, nil); status != 403 {
		t.Fatalf("normal user must not list users, got %d", status)
	}
	status, users := e.do("GET", "/api/v1/admin/users?q=newbie", e.token, nil)
	if status != 200 || users["total"].(float64) < 1 {
		t.Fatalf("admin user list: %d %v", status, users)
	}
}

func TestBackupAndRestore(t *testing.T) {
	e := newEnv(t)
	status, doc := e.do("GET", "/api/v1/admin/backup", e.token, nil)
	if status != 200 || doc["format"] != "aios-backup" {
		t.Fatalf("backup: %d %v", status, doc)
	}
	tables := doc["tables"].(map[string]any)
	if len(tables["users"].([]any)) < 1 {
		t.Fatal("backup has no users")
	}
	status, restored := e.do("POST", "/api/v1/admin/restore", e.token, doc)
	if status != 200 || restored["ok"] != true {
		t.Fatalf("restore: %d %v", status, restored)
	}
	if token := e.login("admin@test.local", adminPassword); token == "" {
		t.Fatal("login after restore failed")
	}
	if status, _ := e.do("POST", "/api/v1/admin/restore", e.token, map[string]any{"format": "nope"}); status != 400 {
		t.Fatalf("invalid backup must be 400, got %d", status)
	}
}

func TestRegistrationDisabled(t *testing.T) {
	e := newEnvWith(t, func(c *config.Config) { c.Security.AllowRegistration = false })
	status, _ := e.do("POST", "/api/v1/auth/register", "", map[string]any{
		"email": "blocked@test.local", "password": "BlockedPass123!",
	})
	if status != 403 {
		t.Fatalf("disabled registration must be 403, got %d", status)
	}
}

func TestBackupWithDataAndDeviceProfile(t *testing.T) {
	e := newEnv(t)
	status, dev := e.do("POST", "/api/v1/devices", e.token, map[string]any{
		"name": "robot-1", "kind": "robot", "protocol": "mqtt", "room": "living",
		"capabilities": []map[string]any{{"name": "move", "type": "number"}},
		"metadata": map[string]any{"connection": "wifi", "tone": "دوستانه", "language": "fa",
			"requirements": []string{"برق"}, "provider": "my-openai", "model": "test"},
	})
	if status != 201 {
		t.Fatalf("device create: %d %v", status, dev)
	}
	id := dev["device"].(map[string]any)["id"].(string)
	if status, _ := e.do("PATCH", "/api/v1/devices/"+id, e.token, map[string]any{
		"metadata": map[string]any{"tone": "رسمی", "connection": "mqtt", "language": "fa"},
	}); status != 200 {
		t.Fatalf("device patch: %d", status)
	}
	status, got := e.do("GET", "/api/v1/devices/"+id, e.token, nil)
	if status != 200 || got["metadata"].(map[string]any)["tone"] != "رسمی" {
		t.Fatalf("device metadata: %d %v", status, got)
	}

	status, doc := e.do("GET", "/api/v1/admin/backup?data=1", e.token, nil)
	if status != 200 {
		t.Fatalf("data backup: %d", status)
	}
	tables := doc["tables"].(map[string]any)
	if _, ok := tables["sessions"]; !ok {
		t.Fatal("data backup must include sessions")
	}
	if status, _ := e.do("POST", "/api/v1/admin/restore", e.token, doc); status != 200 {
		t.Fatalf("data restore: %d", status)
	}
}
