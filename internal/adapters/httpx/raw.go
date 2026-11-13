package httpx

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
)

// DoRaw performs a JSON request and returns the raw response body bounded by
// maxBytes. It is used by the plugin host, where responses are plugin-defined.
func DoRaw(ctx context.Context, client *http.Client, method, url string, body any, maxBytes int64) ([]byte, int, error) {
	var reader io.Reader
	if body != nil {
		raw, err := json.Marshal(body)
		if err != nil {
			return nil, 0, err
		}
		reader = bytes.NewReader(raw)
	}
	req, err := http.NewRequestWithContext(ctx, method, url, reader)
	if err != nil {
		return nil, 0, err
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	req.Header.Set("Accept", "application/json")
	resp, err := client.Do(req)
	if err != nil {
		return nil, 0, err
	}
	defer resp.Body.Close()
	if maxBytes <= 0 {
		maxBytes = 512 << 10
	}
	raw, err := io.ReadAll(io.LimitReader(resp.Body, maxBytes))
	if err != nil {
		return nil, resp.StatusCode, err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return raw, resp.StatusCode, &StatusError{StatusCode: resp.StatusCode, Body: string(raw), URL: url}
	}
	return raw, resp.StatusCode, nil
}
