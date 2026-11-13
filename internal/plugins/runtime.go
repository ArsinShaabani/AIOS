package plugins

import (
	"context"
	"os"
	"path/filepath"
	"time"

	"github.com/ArsinShaabani/AIOS/internal/domain"
	"github.com/ArsinShaabani/AIOS/internal/platform"
	"github.com/ArsinShaabani/AIOS/internal/ports"
)

// Reload re-registers tools for enabled plugins (HTTP and WASM).
func (h *Host) Reload(ctx context.Context) error {
	h.mu.Lock()
	defer h.mu.Unlock()
	for _, names := range h.names {
		for _, name := range names {
			h.registry.Unregister(name)
		}
	}
	h.names = map[string][]string{}
	h.wasm = map[string][]byte{}
	rows, err := h.repo.List(ctx)
	if err != nil {
		return err
	}
	for i := range rows {
		p := rows[i]
		if !p.Enabled {
			continue
		}
		var (
			names []string
			err   error
		)
		switch p.Kind {
		case "http":
			names, err = h.register(&p)
		case "wasm":
			var code []byte
			code, err = h.loadModule(&p)
			if err == nil {
				names, err = h.registerWASM(&p, code)
				if err == nil {
					h.wasm[p.ID] = code
				}
			}
		default:
			continue
		}
		if err != nil {
			h.log.Warn("cannot load plugin", "plugin", p.Name, "kind", p.Kind, "error", err)
			continue
		}
		h.names[p.ID] = names
	}
	return nil
}

// loadModule reads and verifies a persisted WASM module. Size, digest and
// signature are re-checked on every boot, so tampering with the file on disk is
// detected before execution.
func (h *Host) loadModule(p *domain.Plugin) ([]byte, error) {
	path := p.Endpoint
	if path == "" {
		return nil, platform.Internal("plugin.wasm_path", "WASM plugin has no module path")
	}
	info, err := os.Stat(path)
	if err != nil {
		return nil, platform.Wrap(platform.KindNotFound, "plugin.wasm_missing", "WASM module is missing", err)
	}
	if info.Size() > h.maxWASMBytes() {
		return nil, platform.Invalid("plugin.wasm_size", "WASM module exceeds plugins.max_wasm_bytes")
	}
	code, err := os.ReadFile(path)
	if err != nil {
		return nil, platform.Wrap(platform.KindInternal, "plugin.wasm_read", "cannot read WASM module", err)
	}
	if p.WasmSHA256 == "" || Digest(code) != p.WasmSHA256 {
		return nil, platform.Forbidden("plugin.wasm_digest", "WASM module digest does not match the signed manifest")
	}
	p.WasmSize = int64(len(code))
	if err := h.checkSignature(p, code); err != nil {
		return nil, err
	}
	return code, nil
}

// registerWASM exposes a module's tools. Every invocation runs in a fresh sandbox.
func (h *Host) registerWASM(p *domain.Plugin, code []byte) ([]string, error) {
	names := make([]string, 0, len(p.Tools))
	timeout := int(h.cfg.WASMTimeout.Std().Seconds())
	if timeout <= 0 {
		timeout = 10
	}
	for _, t := range p.Tools {
		tool := ports.Tool{
			Name:        t.Name,
			Description: t.Description,
			Parameters:  t.Parameters,
			Permissions: p.Permissions,
			Timeout:     timeout,
			Executor:    h.wasmExecutor(p, t, code),
		}
		if err := h.registry.Register(tool); err != nil {
			return names, err
		}
		names = append(names, t.Name)
	}
	return names, nil
}

// Uninstall removes a plugin, its tools and its WASM module.
func (h *Host) Uninstall(ctx context.Context, id string) error {
	p, err := h.repo.Get(ctx, id)
	if err != nil {
		return err
	}
	h.mu.Lock()
	for _, name := range h.names[id] {
		h.registry.Unregister(name)
	}
	delete(h.names, id)
	code := h.wasm[id]
	delete(h.wasm, id)
	h.mu.Unlock()
	if p.Kind == "wasm" && p.Endpoint != "" {
		if filepath.Clean(p.Endpoint) != "" {
			_ = os.Remove(p.Endpoint)
		}
	}
	_ = code
	return h.repo.Delete(ctx, p.ID)
}

// SetEnabled toggles a plugin.
func (h *Host) SetEnabled(ctx context.Context, id string, enabled bool) error {
	p, err := h.repo.Get(ctx, id)
	if err != nil {
		return err
	}
	p.Enabled = enabled
	if err := h.repo.Upsert(ctx, p); err != nil {
		return err
	}
	return h.Reload(ctx)
}

// List returns installed plugins.
func (h *Host) List(ctx context.Context) ([]domain.Plugin, error) {
	return h.repo.List(ctx)
}

// LoadDir imports manifests from the configured directory (declarative
// installs baked into an image or mounted from a volume). WASM modules are
// installed through the API/CLI because they carry binary payloads.
func (h *Host) LoadDir(ctx context.Context) (int, error) {
	if h.cfg.Dir == "" || !h.cfg.Enabled {
		return 0, nil
	}
	return LoadManifestDir(ctx, h.cfg.Dir, func(p *domain.Plugin) error {
		p.Enabled = true
		existing, err := h.findByName(ctx, p.Name)
		if err == nil {
			p.ID = existing.ID
		}
		_, err = h.Install(ctx, p)
		return err
	})
}

var _ = time.Second

func (h *Host) findByName(ctx context.Context, name string) (*domain.Plugin, error) {
	rows, err := h.repo.List(ctx)
	if err != nil {
		return nil, err
	}
	for i := range rows {
		if rows[i].Name == name {
			return &rows[i], nil
		}
	}
	return nil, platform.NotFound("plugin.not_found", "plugin not found")
}
