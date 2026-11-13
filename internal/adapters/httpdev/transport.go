// Package httpdev implements the HTTP device transport: firmware that cannot run
// MQTT or WebSockets can poll/accept a signed JSON command envelope.
package httpdev

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/ArsinShaabani/AIOS/internal/domain"
	"github.com/ArsinShaabani/AIOS/internal/platform"
	"github.com/ArsinShaabani/AIOS/internal/ports"
)

// Transport sends commands to devices exposing a command URL in metadata
// (metadata.command_url).
type Transport struct {
	client *http.Client
}

var _ ports.DeviceTransport = (*Transport)(nil)

// New builds the transport.
func New() *Transport {
	return &Transport{client: &http.Client{Timeout: 15 * time.Second}}
}

func (t *Transport) Name() string { return "http" }

func (t *Transport) Supports(protocol domain.DeviceProtocol) bool {
	return protocol == domain.ProtoHTTP
}

// Send posts the command envelope and parses the acknowledgement.
func (t *Transport) Send(ctx context.Context, device domain.Device, cmd ports.Command) (ports.CommandResult, error) {
	start := time.Now()
	url := ""
	if device.Metadata != nil {
		url, _ = device.Metadata["command_url"].(string)
	}
	if url == "" {
		return ports.CommandResult{}, platform.Invalid("httpdev.url", "device metadata.command_url is required for HTTP devices")
	}
	payload, err := json.Marshal(map[string]any{
		"device_id": device.ID, "request_id": cmd.RequestID,
		"capability": cmd.Capability, "value": cmd.Value, "expires_at": cmd.ExpiresAt,
	})
	if err != nil {
		return ports.CommandResult{}, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(payload))
	if err != nil {
		return ports.CommandResult{}, platform.Wrap(platform.KindInvalid, "httpdev.request", "invalid command URL", err)
	}
	req.Header.Set("Content-Type", "application/json")
	if device.TokenPrefix != "" {
		req.Header.Set("X-Device-ID", device.ID)
	}
	resp, err := t.client.Do(req)
	if err != nil {
		return ports.CommandResult{LatencyMS: time.Since(start).Milliseconds()}, platform.Wrap(platform.KindUnavailable, "httpdev.send", "device unreachable", err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 64<<10))
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return ports.CommandResult{LatencyMS: time.Since(start).Milliseconds(), Detail: strings.TrimSpace(string(body))},
			platform.Unavailable("httpdev.status", "device returned an error")
	}
	result := ports.CommandResult{OK: true, Detail: strings.TrimSpace(string(body)), LatencyMS: time.Since(start).Milliseconds()}
	var parsed struct {
		OK     *bool          `json:"ok"`
		Detail string         `json:"detail"`
		Data   map[string]any `json:"data"`
	}
	if json.Unmarshal(body, &parsed) == nil {
		if parsed.OK != nil {
			result.OK = *parsed.OK
		}
		if parsed.Detail != "" {
			result.Detail = parsed.Detail
		}
		result.Data = parsed.Data
	}
	return result, nil
}

// Subscribe is a no-op: HTTP devices push telemetry to /api/v1/device/telemetry.
func (t *Transport) Subscribe(context.Context, func(ports.Telemetry)) error { return nil }

// Ping always succeeds.
func (t *Transport) Ping(context.Context) error { return nil }

// Close is a no-op.
func (t *Transport) Close() error { return nil }
