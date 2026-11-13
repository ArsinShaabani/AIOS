// Package httpx contains small HTTP helpers shared by provider adapters:
// bounded bodies, JSON round-trips and SSE parsing.
package httpx

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// NewClient returns an HTTP client tuned for edge hardware: a bounded idle
// connection pool, sane timeouts and no automatic redirects beyond 5 hops.
func NewClient(timeout time.Duration) *http.Client {
	if timeout <= 0 {
		timeout = 30 * time.Second
	}
	return &http.Client{
		Timeout: timeout,
		Transport: &http.Transport{
			MaxIdleConns:          16,
			MaxIdleConnsPerHost:   4,
			IdleConnTimeout:       60 * time.Second,
			TLSHandshakeTimeout:   10 * time.Second,
			ResponseHeaderTimeout: timeout,
		},
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if len(via) >= 5 {
				return fmt.Errorf("too many redirects")
			}
			return nil
		},
	}
}

// StatusError describes a non-2xx provider response.
type StatusError struct {
	StatusCode int
	Body       string
	URL        string
}

func (e *StatusError) Error() string {
	return fmt.Sprintf("http %d from %s: %s", e.StatusCode, e.URL, truncate(e.Body, 300))
}

// DoJSON performs a JSON request and decodes the response into out (when non-nil).
func DoJSON(ctx context.Context, client *http.Client, method, url string, headers map[string]string, body any, out any, maxBytes int64) error {
	var reader io.Reader
	if body != nil {
		raw, err := json.Marshal(body)
		if err != nil {
			return fmt.Errorf("encode request: %w", err)
		}
		reader = bytes.NewReader(raw)
	}
	req, err := http.NewRequestWithContext(ctx, method, url, reader)
	if err != nil {
		return err
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	req.Header.Set("Accept", "application/json")
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if maxBytes <= 0 {
		maxBytes = 4 << 20
	}
	raw, err := io.ReadAll(io.LimitReader(resp.Body, maxBytes))
	if err != nil {
		return err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return &StatusError{StatusCode: resp.StatusCode, Body: string(raw), URL: url}
	}
	if out != nil && len(raw) > 0 {
		if err := json.Unmarshal(raw, out); err != nil {
			return fmt.Errorf("decode response: %w", err)
		}
	}
	return nil
}

// SSELine represents one server-sent event line.
type SSELine struct {
	Event string
	Data  string
}

// ReadSSE parses an SSE stream, invoking fn for each data payload. Returning an
// error stops consumption.
func ReadSSE(ctx context.Context, body io.Reader, fn func(line SSELine) error) error {
	scanner := bufio.NewScanner(body)
	scanner.Buffer(make([]byte, 0, 64*1024), 1<<20)
	var event string
	for scanner.Scan() {
		if err := ctx.Err(); err != nil {
			return err
		}
		line := scanner.Text()
		if strings.HasPrefix(line, "event:") {
			event = strings.TrimSpace(strings.TrimPrefix(line, "event:"))
			continue
		}
		if !strings.HasPrefix(line, "data:") {
			if line == "" {
				event = ""
			}
			continue
		}
		data := strings.TrimSpace(strings.TrimPrefix(line, "data:"))
		if data == "" || data == "[DONE]" {
			continue
		}
		if err := fn(SSELine{Event: event, Data: data}); err != nil {
			return err
		}
	}
	return scanner.Err()
}

// ReadLimit reads at most n bytes from r.
func ReadLimit(r io.Reader, n int64) ([]byte, error) {
	if n <= 0 {
		n = 4 << 20
	}
	return io.ReadAll(io.LimitReader(r, n))
}

func truncate(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n]) + "…"
}
