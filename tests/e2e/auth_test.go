package e2e

import (
	"net/http"
	"testing"
)

func TestAuthLifecycle(t *testing.T) {
	e := newEnv(t)
	status, login := e.do("POST", "/api/v1/auth/login", "", map[string]string{"email": "admin@test.local", "password": adminPassword})
	if status != 200 {
		t.Fatalf("login: %d %v", status, login)
	}
	tokens := login["tokens"].(map[string]any)
	refresh := tokens["refresh_token"].(string)

	status, refreshed := e.do("POST", "/api/v1/auth/refresh", "", map[string]string{"refresh_token": refresh})
	if status != 200 {
		t.Fatalf("refresh: %d %v", status, refreshed)
	}
	newPair := refreshed["tokens"].(map[string]any)
	newRefresh := newPair["refresh_token"].(string)
	if newRefresh == refresh {
		t.Fatal("refresh token was not rotated")
	}
	if status, _ := e.do("POST", "/api/v1/auth/refresh", "", map[string]string{"refresh_token": refresh}); status != 401 {
		t.Fatalf("reused refresh must be 401, got %d", status)
	}
	if status, _ := e.do("POST", "/api/v1/auth/refresh", "", map[string]string{"refresh_token": newRefresh}); status != 401 {
		t.Fatalf("family must be revoked after reuse, got %d", status)
	}

	status, created := e.do("POST", "/api/v1/auth/api-keys", e.token, map[string]any{
		"name": "cli", "scopes": []string{"chat"}, "expires_in_days": 1,
	})
	if status != 201 {
		t.Fatalf("api key create: %d %v", status, created)
	}
	apiKey := created["token"].(string)
	req, _ := http.NewRequest("GET", e.server.URL+"/api/v1/auth/me", nil)
	req.Header.Set("X-API-Key", apiKey)
	resp, err := e.client.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Fatalf("api key auth: %d", resp.StatusCode)
	}
	req, _ = http.NewRequest("GET", e.server.URL+"/api/v1/admin/users", nil)
	req.Header.Set("X-API-Key", apiKey)
	resp, _ = e.client.Do(req)
	resp.Body.Close()
	if resp.StatusCode != 403 {
		t.Fatalf("scope must forbid user listing, got %d", resp.StatusCode)
	}
	status, keys := e.do("GET", "/api/v1/auth/api-keys", e.token, nil)
	if status != 200 || len(keys["items"].([]any)) != 1 {
		t.Fatalf("api key list: %d %v", status, keys)
	}
	keyID := keys["items"].([]any)[0].(map[string]any)["id"].(string)
	if status, _ := e.do("DELETE", "/api/v1/auth/api-keys/"+keyID, e.token, nil); status != 204 {
		t.Fatalf("api key revoke: %d", status)
	}
	if status, _ := e.do("POST", "/api/v1/auth/logout", e.token, map[string]string{"refresh_token": newRefresh}); status != 204 {
		t.Fatalf("logout: %d", status)
	}
}

func TestUserAdministration(t *testing.T) {
	e := newEnv(t)
	status, created := e.do("POST", "/api/v1/admin/users", e.token, map[string]any{
		"email": "op@test.local", "password": "OperatorPass123!", "role": "operator", "display_name": "Op",
	})
	if status != 201 {
		t.Fatalf("create operator: %d %v", status, created)
	}
	opID := created["id"].(string)
	if status, _ := e.do("POST", "/api/v1/admin/users", e.token, map[string]any{
		"email": "op@test.local", "password": "AnotherPass123!",
	}); status != 409 {
		t.Fatalf("duplicate email must conflict, got %d", status)
	}
	status, list := e.do("GET", "/api/v1/admin/users?q=op", e.token, nil)
	if status != 200 || list["total"].(float64) < 1 {
		t.Fatalf("user list: %d %v", status, list)
	}
	status, updated := e.do("PATCH", "/api/v1/admin/users/"+opID, e.token, map[string]any{"display_name": "Operator", "role": "operator"})
	if status != 200 || updated["display_name"] != "Operator" {
		t.Fatalf("user update: %d %v", status, updated)
	}
	if status, _ := e.do("PATCH", "/api/v1/admin/users/"+opID, e.token, map[string]any{"role": "bogus"}); status != 400 {
		t.Fatalf("invalid role must be 400, got %d", status)
	}
	if status, _ := e.do("POST", "/api/v1/admin/users/"+opID+"/reset-password", e.token, map[string]string{"password": "NewOperatorPass123!"}); status != 204 {
		t.Fatalf("reset password: %d", status)
	}
	opToken := e.login("op@test.local", "NewOperatorPass123!")
	if status, _ := e.do("GET", "/api/v1/admin/users", opToken, nil); status != 200 {
		t.Fatalf("operator may read users, got %d", status)
	}
	if status, _ := e.do("POST", "/api/v1/admin/users", opToken, map[string]any{"email": "x@y.z", "password": "Password123!"}); status != 403 {
		t.Fatalf("operator must not create users, got %d", status)
	}
	if status, _ := e.do("GET", "/api/v1/admin/overview", opToken, nil); status != 200 {
		t.Fatalf("operator may read overview, got %d", status)
	}
	if status, _ := e.do("GET", "/api/v1/devices", opToken, nil); status != 200 {
		t.Fatalf("operator may read devices, got %d", status)
	}
	if status, _ := e.do("POST", "/api/v1/admin/users/"+opID+"/revoke-tokens", e.token, nil); status != 200 {
		t.Fatalf("revoke tokens: %d", status)
	}
	if status, _ := e.do("DELETE", "/api/v1/admin/users/"+opID, e.token, nil); status != 204 {
		t.Fatalf("delete user: %d", status)
	}
}

func TestSelfProtection(t *testing.T) {
	e := newEnv(t)
	_, me := e.do("GET", "/api/v1/auth/me", e.token, nil)
	adminID := me["user"].(map[string]any)["id"].(string)
	if status, _ := e.do("DELETE", "/api/v1/admin/users/"+adminID, e.token, nil); status != 403 {
		t.Fatalf("self delete must be forbidden, got %d", status)
	}
	if status, _ := e.do("PATCH", "/api/v1/admin/users/"+adminID, e.token, map[string]any{"role": "user"}); status != 403 {
		t.Fatalf("self demote must be forbidden, got %d", status)
	}
	if status, _ := e.do("PATCH", "/api/v1/admin/users/"+adminID, e.token, map[string]any{"status": "disabled"}); status != 403 {
		t.Fatalf("self disable must be forbidden, got %d", status)
	}
}
