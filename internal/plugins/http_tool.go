package plugins

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/ArsinShaabani/AIOS/internal/adapters/httpx"
	"github.com/ArsinShaabani/AIOS/internal/domain"
	"github.com/ArsinShaabani/AIOS/internal/platform"
	"github.com/ArsinShaabani/AIOS/internal/ports"
)

// register installs tools for one plugin, skipping names already taken.
func (h *Host) register(p *domain.Plugin) ([]string, error) {
	names := make([]string, 0, len(p.Tools))
	for _, t := range p.Tools {
		if existing, ok := h.registry.Get(t.Name); ok {
			if !strings.HasPrefix(existing.Name, t.Name) {
				h.log.Warn("plugin tool name already registered", "tool", t.Name, "plugin", p.Name)
				continue
			}
		}
		tool := ports.Tool{
			Name:        t.Name,
			Description: t.Description,
			Parameters:  t.Parameters,
			Permissions: p.Permissions,
			Timeout:     int(h.cfg.Timeout.Std().Seconds()),
			Executor:    h.executor(p, t),
		}
		if tool.Timeout <= 0 {
			tool.Timeout = 20
		}
		if err := h.registry.Register(tool); err != nil {
			return names, err
		}
		names = append(names, t.Name)
	}
	return names, nil
}

func (h *Host) executor(p *domain.Plugin, t domain.PluginTool) ports.ToolExecutor {
	endpoint := strings.TrimRight(p.Endpoint, "/")
	path := t.Path
	if path == "" {
		path = "/tools/" + t.Name
	}
	if !strings.HasPrefix(path, "/") {
		path = "/" + path
	}
	return func(ctx context.Context, call ports.ToolCallContext) (ports.ToolResult, error) {
		body := map[string]any{
			"tool":        t.Name,
			"arguments":   call.Arguments,
			"user_id":     call.UserID,
			"session_id":  call.SessionID,
			"agent_id":    call.AgentID,
			"plugin":      p.Name,
			"permissions": p.Permissions,
		}
		var (
			raw []byte
			err error
		)
		if t.Method == http.MethodGet {
			query := buildQuery(call.Arguments)
			full := endpoint + path
			if query != "" {
				full += "?" + query
			}
			raw, _, err = httpx.DoRaw(ctx, h.client, http.MethodGet, full, nil, h.cfg.MaxResponseBytes)
		} else {
			raw, _, err = httpx.DoRaw(ctx, h.client, http.MethodPost, endpoint+path, body, h.cfg.MaxResponseBytes)
		}
		if err != nil {
			return ports.ToolResult{IsError: true, Content: "plugin call failed: " + err.Error()}, nil
		}
		return parsePluginResponse(raw), nil
	}
}

func parsePluginResponse(raw []byte) ports.ToolResult {
	var payload struct {
		Content string         `json:"content"`
		Data    map[string]any `json:"data"`
		IsError bool           `json:"is_error"`
	}
	if err := json.Unmarshal(raw, &payload); err == nil && (payload.Content != "" || payload.Data != nil) {
		return ports.ToolResult{Content: platform.Truncate(payload.Content, 16000), Data: payload.Data, IsError: payload.IsError}
	}
	return ports.ToolResult{Content: platform.Truncate(string(raw), 16000)}
}

func buildQuery(args map[string]any) string {
	if len(args) == 0 {
		return ""
	}
	parts := make([]string, 0, len(args))
	for k, v := range args {
		parts = append(parts, fmt.Sprintf("%s=%v", k, v))
	}
	return strings.Join(parts, "&")
}

// LoadManifestDir reads *.json plugin manifests from dir and hands them to
// install. Declarative installs keep air-gapped deployments reproducible.
func LoadManifestDir(ctx context.Context, dir string, install func(*domain.Plugin) error) (int, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return 0, nil
		}
		return 0, err
	}
	count := 0
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".json") {
			continue
		}
		if err := ctx.Err(); err != nil {
			return count, err
		}
		raw, err := os.ReadFile(filepath.Join(dir, entry.Name()))
		if err != nil {
			continue
		}
		var manifest struct {
			Name        string              `json:"name"`
			Version     string              `json:"version"`
			Description string              `json:"description"`
			Author      string              `json:"author"`
			Homepage    string              `json:"homepage"`
			Endpoint    string              `json:"endpoint"`
			Permissions []string            `json:"permissions"`
			Tools       []domain.PluginTool `json:"tools"`
		}
		if err := json.Unmarshal(raw, &manifest); err != nil {
			continue
		}
		plugin := &domain.Plugin{
			Name: manifest.Name, Version: manifest.Version, Description: manifest.Description,
			Author: manifest.Author, Homepage: manifest.Homepage, Kind: "http",
			Endpoint: manifest.Endpoint, Permissions: manifest.Permissions, Tools: manifest.Tools,
			Manifest: raw, Enabled: true, Verified: true,
		}
		if err := install(plugin); err != nil {
			continue
		}
		count++
	}
	return count, nil
}
