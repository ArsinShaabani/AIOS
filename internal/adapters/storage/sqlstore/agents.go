package sqlstore

import (
	"context"
	"time"

	"github.com/ArsinShaabani/AIOS/internal/domain"
)

// AgentRepo implements ports.AgentRepository.
type AgentRepo struct{ db *DB }

func NewAgentRepo(db *DB) *AgentRepo { return &AgentRepo{db: db} }

const agentCols = `id, owner_id, name, description, system_prompt, tools, keywords, model_hint,
	priority, enabled, builtin, created_at, updated_at`

func scanAgent(row rowScanner) (*domain.Agent, error) {
	var (
		a       domain.Agent
		tools   string
		keys    string
		enabled int
		builtin int
		created string
		updated string
	)
	if err := row.Scan(&a.ID, &a.OwnerID, &a.Name, &a.Description, &a.SystemPrompt, &tools, &keys,
		&a.ModelHint, &a.Priority, &enabled, &builtin, &created, &updated); err != nil {
		return nil, err
	}
	a.Tools = decodeStrings(tools)
	a.Keywords = decodeStrings(keys)
	a.Enabled = enabled == 1
	a.Builtin = builtin == 1
	a.CreatedAt = parseTime(created)
	a.UpdatedAt = parseTime(updated)
	return &a, nil
}

// Upsert inserts or updates an agent definition.
func (r *AgentRepo) Upsert(ctx context.Context, a *domain.Agent) error {
	now := time.Now().UTC()
	if a.CreatedAt.IsZero() {
		a.CreatedAt = now
	}
	a.UpdatedAt = now
	_, err := r.db.ExecContext(ctx, r.db.Rebind(`INSERT INTO agents (`+agentCols+`)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(id) DO UPDATE SET
			name = excluded.name, description = excluded.description, system_prompt = excluded.system_prompt,
			tools = excluded.tools, keywords = excluded.keywords, model_hint = excluded.model_hint,
			priority = excluded.priority, enabled = excluded.enabled, builtin = excluded.builtin,
			updated_at = excluded.updated_at`),
		a.ID, a.OwnerID, a.Name, a.Description, a.SystemPrompt, jsonSliceText(a.Tools), jsonSliceText(a.Keywords),
		a.ModelHint, a.Priority, b2i(a.Enabled), b2i(a.Builtin), ft(a.CreatedAt), ft(a.UpdatedAt))
	return mapErr("agent.upsert", err)
}

// Get loads one agent.
func (r *AgentRepo) Get(ctx context.Context, id string) (*domain.Agent, error) {
	row := r.db.QueryRowContext(ctx, r.db.Rebind(`SELECT `+agentCols+` FROM agents WHERE id = ?`), id)
	a, err := scanAgent(row)
	if err != nil {
		return nil, mapErr("agent.get", err)
	}
	return a, nil
}

// List returns agents, optionally enabled only.
func (r *AgentRepo) List(ctx context.Context, enabledOnly bool) ([]domain.Agent, error) {
	query := `SELECT ` + agentCols + ` FROM agents`
	if enabledOnly {
		query += ` WHERE enabled = 1`
	}
	query += ` ORDER BY priority DESC, name ASC`
	rows, err := r.db.QueryContext(ctx, query)
	if err != nil {
		return nil, mapErr("agent.list", err)
	}
	defer rows.Close()
	out := make([]domain.Agent, 0, 12)
	for rows.Next() {
		a, err := scanAgent(rows)
		if err != nil {
			return nil, mapErr("agent.list_scan", err)
		}
		out = append(out, *a)
	}
	return out, mapErr("agent.list_iter", rows.Err())
}

// Delete removes an agent.
func (r *AgentRepo) Delete(ctx context.Context, id string) error {
	res, err := r.db.ExecContext(ctx, r.db.Rebind(`DELETE FROM agents WHERE id = ?`), id)
	if err != nil {
		return mapErr("agent.delete", err)
	}
	return ensureAffected(res, "agent.delete")
}

// Count counts agents.
func (r *AgentRepo) Count(ctx context.Context) (int, error) {
	var n int
	err := r.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM agents`).Scan(&n)
	return n, mapErr("agent.count", err)
}
