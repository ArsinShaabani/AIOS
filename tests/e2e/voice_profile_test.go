package e2e

import (
	"net/http"
	"strings"
	"testing"
)

func TestVoiceProfileAPI(t *testing.T) {
	e := newEnv(t)
	status, profile := e.do("GET", "/api/v1/voice/profile", e.token, nil)
	if status != 200 || profile["customised"] != false {
		t.Fatalf("default profile: %d %v", status, profile)
	}
	status, saved := e.do("PUT", "/api/v1/voice/profile", e.token, map[string]any{
		"voice": "fa_IR-amir", "language": "fa", "format": "wav", "speed": 1.2,
	})
	if status != 200 {
		t.Fatalf("set profile: %d %v", status, saved)
	}
	status, fetched := e.do("GET", "/api/v1/voice/profile", e.token, nil)
	if status != 200 || fetched["customised"] != true {
		t.Fatalf("custom profile: %d %v", status, fetched)
	}
	stored := fetched["profile"].(map[string]any)
	if stored["voice"] != "fa_IR-amir" || stored["language"] != "fa" || stored["speed"].(float64) != 1.2 {
		t.Fatalf("stored profile: %v", stored)
	}
	if status, _ := e.do("PUT", "/api/v1/voice/profile", e.token, map[string]any{"speed": 5}); status != 400 {
		t.Fatalf("invalid speed accepted: %d", status)
	}
	if status, _ := e.do("PUT", "/api/v1/voice/profile", e.token, map[string]any{"format": "flac"}); status != 400 {
		t.Fatalf("invalid format accepted: %d", status)
	}
	// Synthesize still works with the profile applied.
	req, _ := http.NewRequest("POST", e.server.URL+"/api/v1/voice/synthesize", strings.NewReader(`{"text":"hello"}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+e.token)
	resp, err := e.client.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Fatalf("synthesize with profile: %d", resp.StatusCode)
	}
	if status, _ := e.do("DELETE", "/api/v1/voice/profile", e.token, nil); status != 204 {
		t.Fatalf("delete profile: %d", status)
	}
	if _, fetched := e.do("GET", "/api/v1/voice/profile", e.token, nil); fetched["customised"] != false {
		t.Fatalf("profile not reset: %v", fetched)
	}
}
