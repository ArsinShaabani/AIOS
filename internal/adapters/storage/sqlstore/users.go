package sqlstore

import (
	"context"
	"database/sql"
	"strings"
	"time"

	"github.com/ArsinShaabani/AIOS/internal/domain"
	"github.com/ArsinShaabani/AIOS/internal/platform"
	"github.com/ArsinShaabani/AIOS/internal/ports"
)

// UserRepo implements ports.UserRepository.
type UserRepo struct{ db *DB }

func NewUserRepo(db *DB) *UserRepo { return &UserRepo{db: db} }

var _ ports.UserRepository = (*UserRepo)(nil)

const userCols = `id, email, display_name, password_hash, role, status, personality_id, locale, timezone, metadata, last_login_at, created_at, updated_at`

type rowScanner interface {
	Scan(dest ...any) error
}

func scanUser(row rowScanner) (*domain.User, error) {
	var (
		u        domain.User
		meta     string
		lastSeen sql.NullString
		created  string
		updated  string
	)
	if err := row.Scan(&u.ID, &u.Email, &u.DisplayName, &u.PasswordHash, &u.Role, &u.Status,
		&u.PersonalityID, &u.Locale, &u.Timezone, &meta, &lastSeen, &created, &updated); err != nil {
		return nil, err
	}
	decodeJSON(meta, &u.Metadata)
	u.LastLoginAt = ntp(lastSeen)
	u.CreatedAt = parseTime(created)
	u.UpdatedAt = parseTime(updated)
	return &u, nil
}

// Create inserts a user.
func (r *UserRepo) Create(ctx context.Context, u *domain.User) error {
	if err := u.Validate(); err != nil {
		return err
	}
	now := time.Now().UTC()
	if u.CreatedAt.IsZero() {
		u.CreatedAt = now
	}
	u.UpdatedAt = now
	_, err := r.db.ExecContext(ctx, r.db.Rebind(`INSERT INTO users (`+userCols+`)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`),
		u.ID, domain.NormalizeEmail(u.Email), u.DisplayName, u.PasswordHash, string(u.Role), u.Status,
		u.PersonalityID, u.Locale, u.Timezone, jsonText(u.Metadata), ftp(u.LastLoginAt), ft(u.CreatedAt), ft(u.UpdatedAt))
	return mapErr("user.create", err)
}

// GetByID loads a user by identifier.
func (r *UserRepo) GetByID(ctx context.Context, id string) (*domain.User, error) {
	row := r.db.QueryRowContext(ctx, r.db.Rebind(`SELECT `+userCols+` FROM users WHERE id = ?`), id)
	u, err := scanUser(row)
	if err != nil {
		return nil, mapErr("user.get", err)
	}
	return u, nil
}

// GetByEmail loads a user by normalised email.
func (r *UserRepo) GetByEmail(ctx context.Context, email string) (*domain.User, error) {
	row := r.db.QueryRowContext(ctx, r.db.Rebind(`SELECT `+userCols+` FROM users WHERE email = ?`), domain.NormalizeEmail(email))
	u, err := scanUser(row)
	if err != nil {
		return nil, mapErr("user.get_by_email", err)
	}
	return u, nil
}

// Update rewrites the mutable fields of a user.
func (r *UserRepo) Update(ctx context.Context, u *domain.User) error {
	if err := u.Validate(); err != nil {
		return err
	}
	u.UpdatedAt = time.Now().UTC()
	res, err := r.db.ExecContext(ctx, r.db.Rebind(`UPDATE users SET email = ?, display_name = ?, password_hash = ?,
		role = ?, status = ?, personality_id = ?, locale = ?, timezone = ?, metadata = ?, last_login_at = ?, updated_at = ?
		WHERE id = ?`),
		domain.NormalizeEmail(u.Email), u.DisplayName, u.PasswordHash, string(u.Role), u.Status, u.PersonalityID,
		u.Locale, u.Timezone, jsonText(u.Metadata), ftp(u.LastLoginAt), ft(u.UpdatedAt), u.ID)
	if err != nil {
		return mapErr("user.update", err)
	}
	return ensureAffected(res, "user.update")
}

// Delete removes a user and (via FK cascade) their sessions.
func (r *UserRepo) Delete(ctx context.Context, id string) error {
	res, err := r.db.ExecContext(ctx, r.db.Rebind(`DELETE FROM users WHERE id = ?`), id)
	if err != nil {
		return mapErr("user.delete", err)
	}
	return ensureAffected(res, "user.delete")
}

// List returns a filtered, paginated user list.
func (r *UserRepo) List(ctx context.Context, page platform.Page, filter ports.UserFilter) ([]domain.User, int, error) {
	page = page.Normalize()
	where := []string{"1=1"}
	args := []any{}
	if filter.Role != "" {
		where = append(where, "role = ?")
		args = append(args, string(filter.Role))
	}
	if filter.Status != "" {
		where = append(where, "status = ?")
		args = append(args, filter.Status)
	}
	if q := strings.TrimSpace(filter.Query); q != "" {
		where = append(where, "(LOWER(email) LIKE ? OR LOWER(display_name) LIKE ?)")
		like := "%" + strings.ToLower(q) + "%"
		args = append(args, like, like)
	}
	clause := strings.Join(where, " AND ")
	var total int
	if err := r.db.QueryRowContext(ctx, r.db.Rebind(`SELECT COUNT(*) FROM users WHERE `+clause), args...).Scan(&total); err != nil {
		return nil, 0, mapErr("user.count", err)
	}
	args = append(args, page.Limit, page.Offset)
	rows, err := r.db.QueryContext(ctx, r.db.Rebind(`SELECT `+userCols+` FROM users WHERE `+clause+`
		ORDER BY created_at DESC LIMIT ? OFFSET ?`), args...)
	if err != nil {
		return nil, 0, mapErr("user.list", err)
	}
	defer rows.Close()
	users := make([]domain.User, 0, page.Limit)
	for rows.Next() {
		u, err := scanUser(rows)
		if err != nil {
			return nil, 0, mapErr("user.list_scan", err)
		}
		users = append(users, *u)
	}
	return users, total, mapErr("user.list_iter", rows.Err())
}

// Count returns the total number of users.
func (r *UserRepo) Count(ctx context.Context) (int, error) {
	var n int
	err := r.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM users`).Scan(&n)
	return n, mapErr("user.count_all", err)
}
