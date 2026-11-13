package observability

import (
	"context"
	"errors"
	"io"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestMetricsObservers(t *testing.T) {
	m := NewMetrics()
	m.SetBuild("1.2.3", "go1.27")
	m.ObserveHTTP("GET", "/x", 200, 5*time.Millisecond)
	m.ObserveHTTP("POST", "/x", 500, 10*time.Millisecond)
	m.ObserveLLM("openai", "gpt", time.Second, nil)
	m.ObserveLLM("openai", "gpt", time.Second, errors.New("boom"))
	m.ObserveTokens("openai", "gpt", 10, 5, 0.01)
	m.ObserveTokens("local", "m", 1, 1, 0)
	m.ObserveFallback("ollama", "qwen")
	m.ObserveChat("agent_general", "web")
	m.ObserveTool("web_search", false)
	m.ObserveTool("web_search", true)
	m.SetQueue(3, 1)
	m.SetDevices(map[string]int{"online": 2, "offline": 1})
	m.SetMemory(10, 8)
	if m.Uptime() <= 0 {
		t.Fatal("uptime not tracked")
	}
	rec := httptest.NewRecorder()
	m.Handler().ServeHTTP(rec, httptest.NewRequest("GET", "/metrics", nil))
	body, _ := io.ReadAll(rec.Result().Body)
	for _, want := range []string{"aios_http_requests_total", "aios_llm_tokens_total", "aios_queue_depth", "aios_build_info"} {
		if !strings.Contains(string(body), want) {
			t.Fatalf("metric %s missing", want)
		}
	}
}

func TestHealthReports(t *testing.T) {
	h := NewHealth("1.0.0")
	h.Add("ok", func(context.Context) error { return nil })
	h.Add("bad", func(context.Context) error { return errors.New("down") })
	live := h.Live()
	if live.Status != "ok" || live.Version != "1.0.0" {
		t.Fatalf("live: %+v", live)
	}
	ready := h.Ready(context.Background())
	if ready.Status != "degraded" || len(ready.Checks) != 2 {
		t.Fatalf("ready: %+v", ready)
	}
	found := false
	for _, c := range ready.Checks {
		if c.Name == "bad" && c.Status == "error" && c.Detail == "down" {
			found = true
		}
	}
	if !found {
		t.Fatalf("failed check not reported: %+v", ready.Checks)
	}
}

func TestSystemSnapshot(t *testing.T) {
	m := NewSystemMonitor()
	first := m.Snapshot()
	if first.NumCPU < 1 || first.GoVersion == "" || first.UptimeSec < 0 {
		t.Fatalf("snapshot: %+v", first)
	}
	time.Sleep(15 * time.Millisecond)
	second := m.Snapshot()
	if second.CPUPercent < 0 || second.CPUPercent > 100 {
		t.Fatalf("cpu percent: %f", second.CPUPercent)
	}
	if second.Goroutines < 1 || second.MemTotalMB < 1 {
		t.Fatalf("snapshot fields: %+v", second)
	}
}
