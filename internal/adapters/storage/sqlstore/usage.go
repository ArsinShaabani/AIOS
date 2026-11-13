package sqlstore

import (
	"context"
	"database/sql"
	"time"

	"github.com/ArsinShaabani/AIOS/internal/domain"
	"github.com/ArsinShaabani/AIOS/internal/ports"
)

// UsageRepo implements ports.UsageRepository.
type UsageRepo struct{ db *DB }

func NewUsageRepo(db *DB) *UsageRepo { return &UsageRepo{db: db} }

// Create appends a usage record.
func (r *UsageRepo) Create(ctx context.Context, u *domain.UsageRecord) error {
	if u.CreatedAt.IsZero() {
		u.CreatedAt = time.Now().UTC()
	}
	_, err := r.db.ExecContext(ctx, r.db.Rebind(`INSERT INTO usage_records
		(id, user_id, session_id, provider, model, kind, prompt_tokens, completion_tokens, cost_usd, latency_ms, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`),
		u.ID, u.UserID, u.SessionID, u.Provider, u.Model, u.Kind, u.PromptTokens, u.CompletionTokens,
		u.CostUSD, u.LatencyMS, ft(u.CreatedAt))
	return mapErr("usage.create", err)
}

// Totals aggregates usage for a period (empty userID = all users).
func (r *UsageRepo) Totals(ctx context.Context, userID string, from, to time.Time) (domain.UsageTotals, error) {
	query := `SELECT COUNT(*), COALESCE(SUM(prompt_tokens),0), COALESCE(SUM(completion_tokens),0),
		COALESCE(SUM(cost_usd),0), COALESCE(AVG(latency_ms),0) FROM usage_records
		WHERE created_at >= ? AND created_at < ?`
	args := []any{ft(from), ft(to)}
	if userID != "" {
		query += ` AND user_id = ?`
		args = append(args, userID)
	}
	var (
		t      domain.UsageTotals
		avgLat sql.NullFloat64
	)
	if err := r.db.QueryRowContext(ctx, r.db.Rebind(query), args...).Scan(
		&t.Requests, &t.PromptTokens, &t.CompletionTokens, &t.CostUSD, &avgLat); err != nil {
		return t, mapErr("usage.totals", err)
	}
	if avgLat.Valid {
		t.AvgLatencyMS = avgLat.Float64
	}
	return t, nil
}

// DailySeries returns one row per day for charts.
func (r *UsageRepo) DailySeries(ctx context.Context, userID string, from, to time.Time) ([]ports.UsagePoint, error) {
	query := `SELECT substr(created_at,1,10) AS day, COUNT(*),
		COALESCE(SUM(prompt_tokens + completion_tokens),0), COALESCE(SUM(cost_usd),0)
		FROM usage_records WHERE created_at >= ? AND created_at < ?`
	args := []any{ft(from), ft(to)}
	if userID != "" {
		query += ` AND user_id = ?`
		args = append(args, userID)
	}
	query += ` GROUP BY day ORDER BY day ASC`
	rows, err := r.db.QueryContext(ctx, r.db.Rebind(query), args...)
	if err != nil {
		return nil, mapErr("usage.daily", err)
	}
	defer rows.Close()
	out := make([]ports.UsagePoint, 0, 31)
	for rows.Next() {
		var p ports.UsagePoint
		if err := rows.Scan(&p.Day, &p.Requests, &p.Tokens, &p.CostUSD); err != nil {
			return nil, mapErr("usage.daily_scan", err)
		}
		out = append(out, p)
	}
	return out, mapErr("usage.daily_iter", rows.Err())
}

// ByProvider aggregates usage per provider.
func (r *UsageRepo) ByProvider(ctx context.Context, from, to time.Time) ([]ports.ProviderUsage, error) {
	rows, err := r.db.QueryContext(ctx, r.db.Rebind(`SELECT provider, COUNT(*),
		COALESCE(SUM(prompt_tokens + completion_tokens),0), COALESCE(SUM(cost_usd),0)
		FROM usage_records WHERE created_at >= ? AND created_at < ? GROUP BY provider ORDER BY 2 DESC`),
		ft(from), ft(to))
	if err != nil {
		return nil, mapErr("usage.providers", err)
	}
	defer rows.Close()
	out := make([]ports.ProviderUsage, 0, 8)
	for rows.Next() {
		var p ports.ProviderUsage
		if err := rows.Scan(&p.Provider, &p.Requests, &p.Tokens, &p.CostUSD); err != nil {
			return nil, mapErr("usage.providers_scan", err)
		}
		out = append(out, p)
	}
	return out, mapErr("usage.providers_iter", rows.Err())
}

// Count counts usage records.
func (r *UsageRepo) Count(ctx context.Context) (int, error) {
	var n int
	err := r.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM usage_records`).Scan(&n)
	return n, mapErr("usage.count", err)
}
