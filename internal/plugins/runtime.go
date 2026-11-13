package plugins

import (
	"context"

	"github.com/ArsinShaabani/AIOS/internal/domain"
	"github.com/ArsinShaabani/AIOS/internal/platform"
)

// Reload re-registers tools for enabled plugins.
func (h *Host) Reload(ctx context.Context) error {
	h.mu.Lock()
	defer h.mu.Unlock()
	for _, names := range h.names {
		for _, name := range names {
			h.registry.Unregister(name)
		}
	}
	h.names = map[string][]string{}
	rows, err := h.repo.List(ctx)
	if err != nil {
		return err
	}
	for i := range rows {
		p := rows[i]
		if !p.Enabled || p.Kind != "http" {
			continue
		}
		names, err := h.register(&p)
		if err != nil {
			h.log.Warn("cannot load plugin", "plugin", p.Name, "error", err)
			continue
		}
		h.names[p.ID] = names
	}
	return nil
}

// Uninstall removes a plugin and its tools.
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
	h.mu.Unlock()
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
// installs baked into an image or mounted from a volume).
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
