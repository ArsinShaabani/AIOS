package sqlstore

import (
	"context"
	"strings"

	"github.com/ArsinShaabani/AIOS/internal/domain"
	"github.com/ArsinShaabani/AIOS/internal/platform"
	"github.com/ArsinShaabani/AIOS/internal/ports"
)

func (r *DeviceRepo) list(ctx context.Context, baseWhere string, baseArgs []any, filter ports.DeviceFilter, page platform.Page, order string) ([]domain.Device, int, error) {
	page = page.Normalize()
	where := []string{baseWhere}
	args := append([]any{}, baseArgs...)
	if filter.Kind != "" {
		where = append(where, "kind = ?")
		args = append(args, string(filter.Kind))
	}
	if filter.Status != "" {
		where = append(where, "status = ?")
		args = append(args, filter.Status)
	}
	if filter.Room != "" {
		where = append(where, "room = ?")
		args = append(args, filter.Room)
	}
	if q := strings.TrimSpace(filter.Query); q != "" {
		where = append(where, "(LOWER(name) LIKE ? OR LOWER(room) LIKE ? OR LOWER(hardware) LIKE ?)")
		like := "%" + strings.ToLower(q) + "%"
		args = append(args, like, like, like)
	}
	clause := strings.Join(where, " AND ")
	var total int
	if err := r.db.QueryRowContext(ctx, r.db.Rebind(`SELECT COUNT(*) FROM devices WHERE `+clause), args...).Scan(&total); err != nil {
		return nil, 0, mapErr("device.count", err)
	}
	queryArgs := append(append([]any{}, args...), page.Limit, page.Offset)
	rows, err := r.db.QueryContext(ctx, r.db.Rebind(`SELECT `+deviceCols+` FROM devices WHERE `+clause+`
		ORDER BY `+order+` LIMIT ? OFFSET ?`), queryArgs...)
	if err != nil {
		return nil, 0, mapErr("device.list", err)
	}
	defer rows.Close()
	out := make([]domain.Device, 0, page.Limit)
	for rows.Next() {
		d, err := scanDevice(rows)
		if err != nil {
			return nil, 0, mapErr("device.list_scan", err)
		}
		out = append(out, *d)
	}
	return out, total, mapErr("device.list_iter", rows.Err())
}

// ListByUser lists a user's devices.
func (r *DeviceRepo) ListByUser(ctx context.Context, userID string, filter ports.DeviceFilter, page platform.Page) ([]domain.Device, int, error) {
	return r.list(ctx, "user_id = ?", []any{userID}, filter, page, "name ASC")
}

// ListAll lists every device (admin).
func (r *DeviceRepo) ListAll(ctx context.Context, filter ports.DeviceFilter, page platform.Page) ([]domain.Device, int, error) {
	return r.list(ctx, "1=1", nil, filter, page, "created_at DESC")
}

// Count counts devices.
func (r *DeviceRepo) Count(ctx context.Context) (int, error) {
	var n int
	err := r.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM devices`).Scan(&n)
	return n, mapErr("device.count_all", err)
}

// CountByStatus groups devices by presence state.
func (r *DeviceRepo) CountByStatus(ctx context.Context) (map[string]int, error) {
	rows, err := r.db.QueryContext(ctx, `SELECT status, COUNT(*) FROM devices GROUP BY status`)
	if err != nil {
		return nil, mapErr("device.status_counts", err)
	}
	defer rows.Close()
	out := map[string]int{}
	for rows.Next() {
		var status string
		var n int
		if err := rows.Scan(&status, &n); err != nil {
			return nil, mapErr("device.status_scan", err)
		}
		out[status] = n
	}
	return out, mapErr("device.status_iter", rows.Err())
}
