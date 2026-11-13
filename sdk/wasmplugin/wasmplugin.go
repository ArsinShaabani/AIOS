// Package wasmplugin is the guest SDK for AIOS WASM tools.
//
// A plugin is a WASI command: AIOS starts it with one JSON request on stdin and
// reads one JSON response from stdout. The sandbox has no filesystem pre-opens,
// no environment variables and no network, so capabilities come only from the
// response the guest returns.
package wasmplugin

import (
	"encoding/json"
	"io"
	"os"
)

// Request is the invocation sent by the AIOS host.
type Request struct {
	Tool      string         `json:"tool"`
	Arguments map[string]any `json:"arguments"`
	UserID    string         `json:"user_id"`
	SessionID string         `json:"session_id"`
	AgentID   string         `json:"agent_id"`
	Plugin    string         `json:"plugin"`
}

// Response is what the host renders as the tool result.
type Response struct {
	Content string         `json:"content"`
	Data    map[string]any `json:"data,omitempty"`
	IsError bool           `json:"is_error,omitempty"`
}

// Serve reads one request from stdin, runs handler and writes one response.
func Serve(handler func(Request) Response) {
	raw, err := io.ReadAll(io.LimitReader(os.Stdin, 1<<20))
	if err != nil {
		write(Response{Content: "cannot read request: " + err.Error(), IsError: true})
		return
	}
	var req Request
	if err := json.Unmarshal(raw, &req); err != nil {
		write(Response{Content: "invalid JSON request: " + err.Error(), IsError: true})
		return
	}
	if req.Arguments == nil {
		req.Arguments = map[string]any{}
	}
	if handler == nil {
		write(Response{Content: "plugin has no handler", IsError: true})
		return
	}
	write(handler(req))
}

func write(resp Response) {
	enc := json.NewEncoder(os.Stdout)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(resp); err != nil {
		os.Exit(1)
	}
}

// String extracts a string argument with a fallback.
func String(args map[string]any, key, fallback string) string {
	if v, ok := args[key].(string); ok {
		return v
	}
	return fallback
}

// Number extracts a numeric argument with a fallback.
func Number(args map[string]any, key string, fallback float64) float64 {
	switch v := args[key].(type) {
	case float64:
		return v
	case int:
		return float64(v)
	}
	return fallback
}
