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
	"os"
	"path/filepath"
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
	wasm     map[string][]byte   // plugin ID -> module bytes
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
		wasm:   map[string][]byte{},
	}
}

var allowedPermissions = map[string]bool{
	domain.PermChat: true, domain.PermMemoryRead: true, domain.PermMemoryWrite: true,
	domain.PermDevicesRead: true, domain.PermDevicesWrite: true, domain.PermInternet: true,
}

// Install validates and stores an HTTP plugin, then loads it when enabled.
func (h *Host) Install(ctx context.Context, p *domain.Plugin) (*domain.Plugin, error) {
	return h.InstallModule(ctx, p, nil)
}

// InstallModule installs a plugin together with an optional WASM module. WASM
// modules are verified against trusted Ed25519 keys before they ever run.
func (h *Host) InstallModule(ctx context.Context, p *domain.Plugin, wasm []byte) (*domain.Plugin, error) {
	if !h.cfg.Enabled {
		return nil, platform.Unavailable("plugin.disabled", "plugins are disabled in configuration")
	}
	if p.Kind == "" {
		p.Kind = "http"
	}
	if p.ID == "" {
		p.ID = platform.NewID("plg")
	}
	if p.Version == "" {
		p.Version = "0.1.0"
	}
	if err := h.validate(p, wasm); err != nil {
		return nil, err
	}
	if p.Kind == "wasm" {
		path, err := h.storeModule(p, wasm)
		if err != nil {
			return nil, err
		}
		p.Endpoint = path
		p.WasmSHA256 = Digest(wasm)
		p.WasmSize = int64(len(wasm))
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

// storeModule writes the module under plugins.dir/wasm with restrictive
// permissions and returns its path.
func (h *Host) storeModule(p *domain.Plugin, wasm []byte) (string, error) {
	dir := filepath.Join(h.cfg.Dir, "wasm")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return "", platform.Wrap(platform.KindInternal, "plugin.wasm_dir", "cannot create plugin directory", err)
	}
	path := filepath.Join(dir, p.ID+".wasm")
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, wasm, 0o600); err != nil {
		return "", platform.Wrap(platform.KindInternal, "plugin.wasm_write", "cannot store module", err)
	}
	if err := os.Rename(tmp, path); err != nil {
		return "", platform.Wrap(platform.KindInternal, "plugin.wasm_write", "cannot persist module", err)
	}
	return path, nil
}

func (h *Host) validate(p *domain.Plugin, wasm []byte) error {
	if strings.TrimSpace(p.Name) == "" {
		return platform.Invalid("plugin.name", "plugin name is required")
	}
	if len(p.Name) > 64 {
		return platform.Invalid("plugin.name", "plugin name is too long")
	}
	switch p.Kind {
	case "", "http":
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
	case "wasm":
		if !h.cfg.AllowWASM {
			return platform.Forbidden("plugin.wasm_disabled", "WASM plugins are disabled")
		}
		if len(wasm) == 0 {
			return platform.Invalid("plugin.wasm_module", "wasm_module is required for WASM plugins")
		}
		if int64(len(wasm)) > h.maxWASMBytes() {
			return platform.Invalid("plugin.wasm_size", "WASM module exceeds plugins.max_wasm_bytes")
		}
	default:
		return platform.Invalid("plugin.kind", "kind must be http or wasm")
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
	return h.checkSignature(p, wasm)
}

// checkSignature enforces plugins.require_signature and verifies any signature
// that is present against the configured Ed25519 keys.
func (h *Host) checkSignature(p *domain.Plugin, wasm []byte) error {
	keys := TrustedKeys(h.cfg.TrustedKeys)
	if p.Signature != "" {
		if len(keys) == 0 {
			if h.cfg.RequireSignature {
				return platform.Forbidden("plugin.signature", "no trusted plugin keys are configured")
			}
			return platform.Invalid("plugin.signature", "signature provided but plugins.trusted_keys is empty")
		}
		if !VerifyPlugin(p, wasm, keys) {
			return platform.Forbidden("plugin.signature", "plugin signature verification failed")
		}
		p.Verified = true
		return nil
	}
	if h.cfg.RequireSignature {
		return platform.Forbidden("plugin.unsigned", "this deployment requires signed plugins")
	}
	return nil
}

func (h *Host) maxWASMBytes() int64 {
	if h.cfg.MaxWASMBytes > 0 {
		return h.cfg.MaxWASMBytes
	}
	return 16 << 20
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
