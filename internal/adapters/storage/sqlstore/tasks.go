package sqlstore

import (
	"context"
	"time"

	"github.com/ArsinShaabani/AIOS/internal/domain"
	"github.com/ArsinShaabani/AIOS/internal/platform"
)

// TaskRepo implements ports.TaskRepository with atomic claims.
type TaskRepo struct{ db *DB }

func NewTaskRepo(db *DB) *TaskRepo { return &TaskRepo{db: db} }

const taskCols = `id, kind, payload, status, priority, attempts, max_attempts, run_at, started_at,
	finished_at, last_error, created_at`

func scanTask(row rowScanner) (*domain.Task, error) {
	var (
		t        domain.Task
		payload  string
		started  sqlNullString
		finished sqlNullString
		runAt    string
		created  string
	)
	if err := row.Scan(&t.ID, &t.Kind, &payload, &t.Status, &t.Priority, &t.Attempts, &t.MaxAttempts,
		&runAt, &started, &finished, &t.LastError, &created); err != nil {
		return nil, err
	}
	t.Payload = []byte(payload)
	t.RunAt = parseTime(runAt)
	t.StartedAt = started.Ptr()
	t.FinishedAt = finished.Ptr()
	t.CreatedAt = parseTime(created)
	return &t, nil
}

// Create inserts a task.
func (r *TaskRepo) Create(ctx context.Context, t *domain.Task) error {
	now := time.Now().UTC()
	if t.CreatedAt.IsZero() {
		t.CreatedAt = now
	}
	if t.RunAt.IsZero() {
		t.RunAt = now
	}
	if t.Status == "" {
		t.Status = "pending"
	}
	if t.MaxAttempts <= 0 {
		t.MaxAttempts = 3
	}
	if len(t.Payload) == 0 {
		t.Payload = []byte("{}")
	}
	_, err := r.db.ExecContext(ctx, r.db.Rebind(`INSERT INTO tasks (`+taskCols+`)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`),
		t.ID, t.Kind, string(t.Payload), t.Status, t.Priority, t.Attempts, t.MaxAttempts, ft(t.RunAt),
		ftp(t.StartedAt), ftp(t.FinishedAt), t.LastError, ft(t.CreatedAt))
	return mapErr("task.create", err)
}

// ClaimDue atomically claims due tasks for execution.
func (r *TaskRepo) ClaimDue(ctx context.Context, now time.Time, limit int) ([]domain.Task, error) {
	if limit <= 0 {
		limit = 10
	}
	rows, err := r.db.QueryContext(ctx, r.db.Rebind(`SELECT id FROM tasks
		WHERE status = 'pending' AND run_at <= ? ORDER BY priority DESC, run_at ASC LIMIT ?`), ft(now), limit)
	if err != nil {
		return nil, mapErr("task.claim_select", err)
	}
	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err == nil {
			ids = append(ids, id)
		}
	}
	_ = rows.Close()

	out := make([]domain.Task, 0, len(ids))
	for _, id := range ids {
		res, err := r.db.ExecContext(ctx, r.db.Rebind(`UPDATE tasks SET status = 'running', started_at = ?,
			attempts = attempts + 1 WHERE id = ? AND status = 'pending'`), ft(now), id)
		if err != nil {
			return out, mapErr("task.claim", err)
		}
		n, _ := res.RowsAffected()
		if n == 0 {
			continue
		}
		t, err := r.Get(ctx, id)
		if err != nil {
			continue
		}
		out = append(out, *t)
	}
	return out, nil
}

// Complete marks a task done.
func (r *TaskRepo) Complete(ctx context.Context, id string, at time.Time) error {
	_, err := r.db.ExecContext(ctx, r.db.Rebind(`UPDATE tasks SET status = 'done', finished_at = ? WHERE id = ?`),
		ft(at), id)
	return mapErr("task.complete", err)
}

// Fail records a failure, scheduling a retry or marking the task dead.
func (r *TaskRepo) Fail(ctx context.Context, id string, errMsg string, retryAt time.Time, dead bool, at time.Time) error {
	status := "pending"
	var finished any
	if dead {
		status = "dead"
		finished = ft(at)
	}
	_, err := r.db.ExecContext(ctx, r.db.Rebind(`UPDATE tasks SET status = ?, last_error = ?, run_at = ?,
		finished_at = ? WHERE id = ?`), status, platform.Truncate(errMsg, 500), ft(retryAt), finished, id)
	return mapErr("task.fail", err)
}

// ReclaimStale returns tasks stuck in "running" (e.g. a crashed worker) to the
// pending state. Attempts are preserved so retry limits still apply.
func (r *TaskRepo) ReclaimStale(ctx context.Context, startedBefore time.Time) (int, error) {
	res, err := r.db.ExecContext(ctx, r.db.Rebind(`UPDATE tasks SET status = 'pending', started_at = NULL
		WHERE status = 'running' AND started_at IS NOT NULL AND started_at < ?`), ft(startedBefore))
	if err != nil {
		return 0, mapErr("task.reclaim", err)
	}
	n, _ := res.RowsAffected()
	return int(n), nil
}

// Get loads one task.
func (r *TaskRepo) Get(ctx context.Context, id string) (*domain.Task, error) {
	row := r.db.QueryRowContext(ctx, r.db.Rebind(`SELECT `+taskCols+` FROM tasks WHERE id = ?`), id)
	t, err := scanTask(row)
	if err != nil {
		return nil, mapErr("task.get", err)
	}
	return t, nil
}

// List returns tasks filtered by status.
func (r *TaskRepo) List(ctx context.Context, status string, page platform.Page) ([]domain.Task, int, error) {
	page = page.Normalize()
	where := "1=1"
	args := []any{}
	if status != "" {
		where = "status = ?"
		args = append(args, status)
	}
	var total int
	if err := r.db.QueryRowContext(ctx, r.db.Rebind(`SELECT COUNT(*) FROM tasks WHERE `+where), args...).Scan(&total); err != nil {
		return nil, 0, mapErr("task.count", err)
	}
	queryArgs := append(append([]any{}, args...), page.Limit, page.Offset)
	rows, err := r.db.QueryContext(ctx, r.db.Rebind(`SELECT `+taskCols+` FROM tasks WHERE `+where+`
		ORDER BY created_at DESC LIMIT ? OFFSET ?`), queryArgs...)
	if err != nil {
		return nil, 0, mapErr("task.list", err)
	}
	defer rows.Close()
	out := make([]domain.Task, 0, page.Limit)
	for rows.Next() {
		t, err := scanTask(rows)
		if err != nil {
			return nil, 0, mapErr("task.list_scan", err)
		}
		out = append(out, *t)
	}
	return out, total, mapErr("task.list_iter", rows.Err())
}

// CountByStatus groups tasks by status.
func (r *TaskRepo) CountByStatus(ctx context.Context) (map[string]int, error) {
	rows, err := r.db.QueryContext(ctx, `SELECT status, COUNT(*) FROM tasks GROUP BY status`)
	if err != nil {
		return nil, mapErr("task.status_counts", err)
	}
	defer rows.Close()
	out := map[string]int{}
	for rows.Next() {
		var status string
		var n int
		if err := rows.Scan(&status, &n); err != nil {
			return nil, err
		}
		out[status] = n
	}
	return out, mapErr("task.status_iter", rows.Err())
}

// DeleteBefore removes finished tasks older than a cutoff.
func (r *TaskRepo) DeleteBefore(ctx context.Context, before time.Time) (int, error) {
	res, err := r.db.ExecContext(ctx, r.db.Rebind(`DELETE FROM tasks WHERE status IN ('done','dead') AND created_at < ?`), ft(before))
	if err != nil {
		return 0, mapErr("task.prune", err)
	}
	n, _ := res.RowsAffected()
	return int(n), nil
}
