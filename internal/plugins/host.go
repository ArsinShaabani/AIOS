// Package plugins implements AIOS' extension system. Plugins are manifests that
// describe HTTP tools (VSCode/Obsidian style); they run out of process, are
// permission-labelled and are limited by host allow-lists, timeouts and response
// size caps.
package plugins

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/ArsinShaabani/AIOS/internal/adapters/httpx"
	"github.com/ArsinShaabani/AIOS/internal/config"
	"github.com/ArsinShaabani/AIOS/internal/domain"
	"github.com/ArsinShaabani/AIOS/internal/platform"
	"github.com/ArsinShaabani/AIOS/internal/ports"
	"github.com/ArsinShaabani/AIOS/internal/tools"
)

// Host installs, validates and executes plugins.
type Host struct {
	repo     ports.PluginRepository
	registry *tools.Registry
	cfg      config.PluginConfig
	client   *http.Client
	log      *slog.Logger
	mu       sync.Mutex
	names    map[string][]string // plugin ID -> registered tool names
}

// New builds the host.
func New(repo ports.PluginRepository, registry *tools.Registry, cfg config.PluginConfig, log *slog.Logger) *Host {
	if log == nil {
		log = slog.Default()
	}
	timeout := cfg.Timeout.Std()
	if timeout <= 0 {
		timeout = 20 * time.Second
	}
	return &Host{
		repo: repo, registry: registry, cfg: cfg, log: log,
		client: httpx.NewClient(timeout),
		names:  map[string][]string{},
	}
}

var allowedPermissions = map[string]bool{
	domain.PermChat: true, domain.PermMemoryRead: true, domain.PermMemoryWrite: true,
	domain.PermDevicesRead: true, domain.PermDevicesWrite: true, domain.PermInternet: true,
}

// Install validates and stores a plugin, then loads it when enabled.
func (h *Host) Install(ctx context.Context, p *domain.Plugin) (*domain.Plugin, error) {
	if !h.cfg.Enabled {
		return nil, platform.Unavailable("plugin.disabled", "plugins are disabled in configuration")
	}
	if err := h.validate(p); err != nil {
		return nil, err
	}
	if p.ID == "" {
		p.ID = platform.NewID("plg")
	}
	if p.Kind == "" {
		p.Kind = "http"
	}
	if p.Version == "" {
		p.Version = "0.1.0"
	}
	if len(p.Manifest) == 0 {
		if raw, err := json.Marshal(p); err == nil {
			p.Manifest = raw
		}
	}
	if err := h.repo.Upsert(ctx, p); err != nil {
		return nil, err
	}
	if p.Enabled {
		if err := h.Reload(ctx); err != nil {
			return nil, err
		}
	}
	return p, nil
}

func (h *Host) validate(p *domain.Plugin) error {
	if strings.TrimSpace(p.Name) == "" {
		return platform.Invalid("plugin.name", "plugin name is required")
	}
	if len(p.Name) > 64 {
		return platform.Invalid("plugin.name", "plugin name is too long")
	}
	if p.Kind != "" && p.Kind != "http" {
		return platform.Invalid("plugin.kind", "only http plugins can be installed at runtime")
	}
	if !h.cfg.AllowHTTP {
		return platform.Forbidden("plugin.http_disabled", "HTTP plugins are disabled")
	}
	u, err := url.Parse(p.Endpoint)
	if err != nil || (u.Scheme != "https" && u.Scheme != "http") || u.Host == "" {
		return platform.Invalid("plugin.endpoint", "a valid http(s) endpoint is required")
	}
	if !h.hostAllowed(u.Hostname()) {
		return platform.Forbidden("plugin.host", "plugin host is not in plugins.allowed_hosts")
	}
	if len(p.Tools) == 0 {
		return platform.Invalid("plugin.tools", "a plugin must expose at least one tool")
	}
	if len(p.Tools) > 32 {
		return platform.Invalid("plugin.tools", "a plugin may expose at most 32 tools")
	}
	seen := map[string]bool{}
	for i := range p.Tools {
		t := &p.Tools[i]
		if !validName(t.Name) {
			return platform.Invalid("plugin.tool_name", "tool names must be lowercase letters, digits, dots, dashes or underscores: "+t.Name)
		}
		if seen[t.Name] {
			return platform.Invalid("plugin.tool_duplicate", "duplicate tool: "+t.Name)
		}
		seen[t.Name] = true
		if t.Method == "" {
			t.Method = http.MethodPost
		}
		if t.Method != http.MethodPost && t.Method != http.MethodGet {
			return platform.Invalid("plugin.method", "tool methods must be GET or POST")
		}
		if len(t.Parameters) > 0 && !json.Valid(t.Parameters) {
			return platform.Invalid("plugin.schema", "tool parameters must be valid JSON: "+t.Name)
		}
	}
	for _, perm := range p.Permissions {
		if !allowedPermissions[perm] {
			return platform.Invalid("plugin.permission", "unknown or forbidden plugin permission: "+perm)
		}
	}
	return nil
}

func validName(name string) bool {
	if name == "" || len(name) > 64 {
		return false
	}
	for _, r := range name {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '_' || r == '-' || r == '.' {
			continue
		}
		return false
	}
	return true
}

func (h *Host) hostAllowed(host string) bool {
	if len(h.cfg.AllowedHosts) == 0 {
		return true // set plugins.allowed_hosts to restrict egress
	}
	for _, allowed := range h.cfg.AllowedHosts {
		if strings.EqualFold(allowed, host) {
			return true
		}
	}
	return false
}
