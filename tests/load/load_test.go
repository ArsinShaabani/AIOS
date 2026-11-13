//go:build load

// Package load contains an HTTP load generator for AIOS. It is excluded from the
// normal test run and targets a running instance, so it measures the real stack
// (SQLite/Postgres, router, mock or real model) rather than an in-process fake.
//
// Usage:
//
//	AIOS_LOAD_URL=http://127.0.0.1:8080 \
//	AIOS_LOAD_EMAIL=admin@aios.local AIOS_LOAD_PASSWORD=... \
//	AIOS_LOAD_CONCURRENCY=8 AIOS_LOAD_DURATION=30s \
//	go test -tags load -run TestLoad -v ./tests/load/...
package load

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"sort"
	"strconv"
	"sync"
	"testing"
	"time"
)

func TestLoad(t *testing.T) {
	base := os.Getenv("AIOS_LOAD_URL")
	if base == "" {
		t.Skip("AIOS_LOAD_URL not set; skipping load test")
	}
	email := envOr("AIOS_LOAD_EMAIL", "admin@aios.local")
	password := envOr("AIOS_LOAD_PASSWORD", "AdminPassword123!")
	concurrency := envInt("AIOS_LOAD_CONCURRENCY", 4)
	duration := envDuration("AIOS_LOAD_DURATION", 15*time.Second)
	if concurrency < 1 {
		concurrency = 1
	}

	client := &http.Client{Timeout: 120 * time.Second}
	token := login(t, client, base, email, password)

	var (
		mu        sync.Mutex
		latencies []time.Duration
		errors    int
		requests  int
	)
	deadline := time.Now().Add(duration)
	var wg sync.WaitGroup
	for i := 0; i < concurrency; i++ {
		wg.Add(1)
		go func(worker int) {
			defer wg.Done()
			for n := 0; time.Now().Before(deadline); n++ {
				payload, _ := json.Marshal(map[string]any{
					"message": fmt.Sprintf("load test worker %d message %d: summarise what AIOS is", worker, n),
					"channel": "api",
				})
				req, _ := http.NewRequest(http.MethodPost, base+"/api/v1/chat", bytes.NewReader(payload))
				req.Header.Set("Content-Type", "application/json")
				req.Header.Set("Authorization", "Bearer "+token)
				start := time.Now()
				resp, err := client.Do(req)
				elapsed := time.Since(start)
				ok := err == nil && resp != nil && resp.StatusCode == 200
				if resp != nil {
					_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 1<<20))
					resp.Body.Close()
				}
				mu.Lock()
				requests++
				if ok {
					latencies = append(latencies, elapsed)
				} else {
					errors++
				}
				mu.Unlock()
				time.Sleep(20 * time.Millisecond)
			}
		}(i)
	}
	wg.Wait()

	if requests == 0 {
		t.Fatal("no requests were sent")
	}
	sort.Slice(latencies, func(i, j int) bool { return latencies[i] < latencies[j] })
	p := func(q float64) time.Duration {
		if len(latencies) == 0 {
			return 0
		}
		idx := int(q * float64(len(latencies)-1))
		return latencies[idx]
	}
	errRate := float64(errors) / float64(requests) * 100
	t.Logf("requests=%d ok=%d errors=%d err_rate=%.2f%% p50=%s p95=%s p99=%s max=%s",
		requests, len(latencies), errors, errRate, p50(p(0.50)), p50(p(0.95)), p50(p(0.99)), p50(p(1.0)))
	if errRate > 5 {
		t.Fatalf("error rate %.2f%% exceeds 5%%", errRate)
	}
}

func login(t *testing.T, client *http.Client, base, email, password string) string {
	t.Helper()
	body, _ := json.Marshal(map[string]string{"email": email, "password": password})
	resp, err := client.Post(base+"/api/v1/auth/login", "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatalf("login: %v", err)
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if resp.StatusCode != 200 {
		t.Fatalf("login status %d: %s", resp.StatusCode, raw)
	}
	var out struct {
		Tokens struct {
			AccessToken string `json:"access_token"`
		} `json:"tokens"`
	}
	if err := json.Unmarshal(raw, &out); err != nil || out.Tokens.AccessToken == "" {
		t.Fatalf("login response: %v", err)
	}
	return out.Tokens.AccessToken
}

func p50(d time.Duration) string { return d.Round(time.Millisecond).String() }

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func envInt(key string, fallback int) int {
	if v, err := strconv.Atoi(os.Getenv(key)); err == nil && v > 0 {
		return v
	}
	return fallback
}

func envDuration(key string, fallback time.Duration) time.Duration {
	if v, err := time.ParseDuration(os.Getenv(key)); err == nil && v > 0 {
		return v
	}
	return fallback
}
