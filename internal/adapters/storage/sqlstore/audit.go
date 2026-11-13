package sqlstore

import (
	"context"
	"strings"
	"time"

	"github.com/ArsinShaabani/AIOS/internal/domain"
	"github.com/ArsinShaabani/AIOS/internal/platform"
	"github.com/ArsinShaabani/AIOS/internal/ports"
)

// AuditRepo implements ports.AuditRepository.
type AuditRepo struct{ db *DB }

func NewAuditRepo(db *DB) *AuditRepo { return &AuditRepo{db: db} }

const auditCols = `id, actor_id, actor_type, action, resource, resource_id, status, ip, user_agent, detail, created_at`

func scanAudit(row rowScanner) (*domain.AuditEvent, error) {
	var (
		e       domain.AuditEvent
		detail  string
		created string
	)
	if err := row.Scan(&e.ID, &e.ActorID, &e.ActorType, &e.Action, &e.Resource, &e.ResourceID, &e.Status,
		&e.IP, &e.UserAgent, &detail, &created); err != nil {
		return nil, err
	}
	decodeJSON(detail, &e.Detail)
	e.CreatedAt = parseTime(created)
	return &e, nil
}

// Create appends an audit event.
func (r *AuditRepo) Create(ctx context.Context, e *domain.AuditEvent) error {
	if e.CreatedAt.IsZero() {
		e.CreatedAt = time.Now().UTC()
	}
	_, err := r.db.ExecContext(ctx, r.db.Rebind(`INSERT INTO audit_events (`+auditCols+`)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`),
		e.ID, e.ActorID, e.ActorType, e.Action, e.Resource, e.ResourceID, e.Status, e.IP, e.UserAgent,
		jsonText(e.Detail), ft(e.CreatedAt))
	return mapErr("audit.create", err)
}

// List returns filtered audit events, newest first.
func (r *AuditRepo) List(ctx context.Context, filter ports.AuditFilter, page platform.Page) ([]domain.AuditEvent, int, error) {
	page = page.Normalize()
	where := []string{"1=1"}
	args := []any{}
	if filter.ActorID != "" {
		where = append(where, "actor_id = ?")
		args = append(args, filter.ActorID)
	}
	if filter.Action != "" {
		where = append(where, "action = ?")
		args = append(args, filter.Action)
	}
	if filter.Status != "" {
		where = append(where, "status = ?")
		args = append(args, filter.Status)
	}
	if filter.From != nil {
		where = append(where, "created_at >= ?")
		args = append(args, ft(*filter.From))
	}
	if filter.To != nil {
		where = append(where, "created_at < ?")
		args = append(args, ft(*filter.To))
	}
	clause := strings.Join(where, " AND ")
	var total int
	if err := r.db.QueryRowContext(ctx, r.db.Rebind(`SELECT COUNT(*) FROM audit_events WHERE `+clause), args...).Scan(&total); err != nil {
		return nil, 0, mapErr("audit.count", err)
	}
	queryArgs := append(append([]any{}, args...), page.Limit, page.Offset)
	rows, err := r.db.QueryContext(ctx, r.db.Rebind(`SELECT `+auditCols+` FROM audit_events WHERE `+clause+`
		ORDER BY created_at DESC LIMIT ? OFFSET ?`), queryArgs...)
	if err != nil {
		return nil, 0, mapErr("audit.list", err)
	}
	defer rows.Close()
	out := make([]domain.AuditEvent, 0, page.Limit)
	for rows.Next() {
		e, err := scanAudit(rows)
		if err != nil {
			return nil, 0, mapErr("audit.list_scan", err)
		}
		out = append(out, *e)
	}
	return out, total, mapErr("audit.list_iter", rows.Err())
}

// DeleteBefore trims old audit rows according to retention policy.
func (r *AuditRepo) DeleteBefore(ctx context.Context, before time.Time) (int, error) {
	res, err := r.db.ExecContext(ctx, r.db.Rebind(`DELETE FROM audit_events WHERE created_at < ?`), ft(before))
	if err != nil {
		return 0, mapErr("audit.prune", err)
	}
	n, _ := res.RowsAffected()
	return int(n), nil
}

// Count counts audit events.
func (r *AuditRepo) Count(ctx context.Context) (int, error) {
	var n int
	err := r.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM audit_events`).Scan(&n)
	return n, mapErr("audit.count_all", err)
}
