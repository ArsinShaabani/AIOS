package sqlstore

import (
	"context"
	"time"

	"github.com/ArsinShaabani/AIOS/internal/domain"
)

// PluginRepo implements ports.PluginRepository.
type PluginRepo struct{ db *DB }

func NewPluginRepo(db *DB) *PluginRepo { return &PluginRepo{db: db} }

const pluginCols = `id, name, version, description, author, homepage, kind, endpoint, permissions,
	tools, manifest, signature, wasm_sha256, wasm_size, enabled, verified, installed_at, updated_at`

func scanPlugin(row rowScanner) (*domain.Plugin, error) {
	var (
		p         domain.Plugin
		perms     string
		tools     string
		manifest  string
		enabled   int
		verified  int
		installed string
		updated   string
	)
	if err := row.Scan(&p.ID, &p.Name, &p.Version, &p.Description, &p.Author, &p.Homepage, &p.Kind, &p.Endpoint,
		&perms, &tools, &manifest, &p.Signature, &p.WasmSHA256, &p.WasmSize, &enabled, &verified, &installed, &updated); err != nil {
		return nil, err
	}
	p.Permissions = decodeStrings(perms)
	decodeJSON(tools, &p.Tools)
	p.Manifest = []byte(manifest)
	p.Enabled = enabled == 1
	p.Verified = verified == 1
	p.InstalledAt = parseTime(installed)
	p.UpdatedAt = parseTime(updated)
	return &p, nil
}

// Upsert inserts or updates a plugin by name.
func (r *PluginRepo) Upsert(ctx context.Context, p *domain.Plugin) error {
	now := time.Now().UTC()
	if p.InstalledAt.IsZero() {
		p.InstalledAt = now
	}
	p.UpdatedAt = now
	_, err := r.db.ExecContext(ctx, r.db.Rebind(`INSERT INTO plugins (`+pluginCols+`)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(id) DO UPDATE SET
			name = excluded.name, version = excluded.version, description = excluded.description,
			author = excluded.author, homepage = excluded.homepage, kind = excluded.kind,
			endpoint = excluded.endpoint, permissions = excluded.permissions, tools = excluded.tools,
			manifest = excluded.manifest, signature = excluded.signature,
			wasm_sha256 = excluded.wasm_sha256, wasm_size = excluded.wasm_size,
			enabled = excluded.enabled, verified = excluded.verified,
			updated_at = excluded.updated_at`),
		p.ID, p.Name, p.Version, p.Description, p.Author, p.Homepage, p.Kind, p.Endpoint,
		jsonSliceText(p.Permissions), jsonText(p.Tools), string(p.Manifest), p.Signature,
		p.WasmSHA256, p.WasmSize, b2i(p.Enabled), b2i(p.Verified), ft(p.InstalledAt), ft(p.UpdatedAt))
	return mapErr("plugin.upsert", err)
}

// Get loads one plugin.
func (r *PluginRepo) Get(ctx context.Context, id string) (*domain.Plugin, error) {
	row := r.db.QueryRowContext(ctx, r.db.Rebind(`SELECT `+pluginCols+` FROM plugins WHERE id = ?`), id)
	p, err := scanPlugin(row)
	if err != nil {
		return nil, mapErr("plugin.get", err)
	}
	return p, nil
}

// List returns all installed plugins.
func (r *PluginRepo) List(ctx context.Context) ([]domain.Plugin, error) {
	rows, err := r.db.QueryContext(ctx, `SELECT `+pluginCols+` FROM plugins ORDER BY name ASC`)
	if err != nil {
		return nil, mapErr("plugin.list", err)
	}
	defer rows.Close()
	out := make([]domain.Plugin, 0, 8)
	for rows.Next() {
		p, err := scanPlugin(rows)
		if err != nil {
			return nil, mapErr("plugin.list_scan", err)
		}
		out = append(out, *p)
	}
	return out, mapErr("plugin.list_iter", rows.Err())
}

// Delete removes a plugin.
func (r *PluginRepo) Delete(ctx context.Context, id string) error {
	res, err := r.db.ExecContext(ctx, r.db.Rebind(`DELETE FROM plugins WHERE id = ?`), id)
	if err != nil {
		return mapErr("plugin.delete", err)
	}
	return ensureAffected(res, "plugin.delete")
}

// Count counts plugins.
func (r *PluginRepo) Count(ctx context.Context) (int, error) {
	var n int
	err := r.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM plugins`).Scan(&n)
	return n, mapErr("plugin.count", err)
}
