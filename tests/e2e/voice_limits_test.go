package e2e

import (
	"net/http"
	"strings"
	"testing"

	"github.com/ArsinShaabani/AIOS/internal/config"
)

func TestVoiceDisabled(t *testing.T) {
	e := newEnvWith(t, func(c *config.Config) { c.Voice.Enabled = false })
	if status, cfg := e.do("GET", "/api/v1/voice/config", e.token, nil); status != 200 || cfg["enabled"] != false {
		t.Fatalf("voice config: %d %v", status, cfg)
	}
	if status, _ := e.do("POST", "/api/v1/voice/synthesize", e.token, map[string]any{"text": "hello"}); status != 503 {
		t.Fatalf("disabled synthesize: %d", status)
	}
	req, _ := http.NewRequest("POST", e.server.URL+"/api/v1/voice/transcribe", strings.NewReader("x"))
	req.Header.Set("Content-Type", "multipart/form-data; boundary=x")
	req.Header.Set("Authorization", "Bearer "+e.token)
	resp, err := e.client.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != 503 {
		t.Fatalf("disabled transcribe: %d", resp.StatusCode)
	}
}

func TestRateLimitAndBodyLimit(t *testing.T) {
	e := newEnvWith(t, func(c *config.Config) {
		c.Server.RateLimit.Enabled = true
		c.Server.RateLimit.RequestsPerMinute = 1
		c.Server.RateLimit.Burst = 1
	})
	if status, _ := e.do("GET", "/api/v1/meta", e.token, nil); status != http.StatusTooManyRequests {
		t.Fatalf("rate limit not enforced: %d", status)
	}
	limited := newEnvWith(t, func(c *config.Config) { c.Server.MaxBodyBytes = 1024 })
	status, _ := limited.do("POST", "/api/v1/chat", limited.token, map[string]any{"message": strings.Repeat("x", 4000)})
	if status != 400 && status != 413 {
		t.Fatalf("body limit not enforced: %d", status)
	}
}
