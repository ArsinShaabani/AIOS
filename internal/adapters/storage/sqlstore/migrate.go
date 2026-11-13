package sqlstore

import (
	"context"
	"database/sql"
	"errors"
	"io/fs"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/ArsinShaabani/AIOS/internal/platform"
)

// Migrate applies embedded migrations in filename order. Each migration runs in
// its own transaction and is recorded in schema_migrations.
func (d *DB) Migrate(ctx context.Context) error {
	if _, err := d.ExecContext(ctx, `CREATE TABLE IF NOT EXISTS schema_migrations (version TEXT PRIMARY KEY, applied_at TEXT NOT NULL)`); err != nil {
		return platform.Wrap(platform.KindInternal, "db.migrate_init", "cannot create migration table", err)
	}
	applied := map[string]bool{}
	rows, err := d.QueryContext(ctx, `SELECT version FROM schema_migrations`)
	if err != nil {
		return platform.Wrap(platform.KindInternal, "db.migrate_read", "cannot read migrations", err)
	}
	for rows.Next() {
		var v string
		if err := rows.Scan(&v); err == nil {
			applied[v] = true
		}
	}
	_ = rows.Close()

	entries, err := fs.Glob(migrationFS, "migrations/*.sql")
	if err != nil {
		return err
	}
	sort.Strings(entries)
	for _, name := range entries {
		version := strings.TrimSuffix(filepath.Base(name), ".sql")
		if applied[version] {
			continue
		}
		body, err := migrationFS.ReadFile(name)
		if err != nil {
			return err
		}
		tx, err := d.BeginTx(ctx, nil)
		if err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, string(body)); err != nil {
			_ = tx.Rollback()
			return platform.Wrap(platform.KindInternal, "db.migrate_apply", "migration "+version+" failed", err)
		}
		if _, err := tx.ExecContext(ctx, d.Rebind(`INSERT INTO schema_migrations (version, applied_at) VALUES (?, ?)`), version, ft(time.Now())); err != nil {
			_ = tx.Rollback()
			return err
		}
		if err := tx.Commit(); err != nil {
			return err
		}
		d.Log.Info("migration applied", "version", version)
	}
	return nil
}

// withTx runs fn inside a transaction, rolling back on error/panic.
func (d *DB) withTx(ctx context.Context, fn func(tx *sql.Tx) error) error {
	tx, err := d.BeginTx(ctx, nil)
	if err != nil {
		return platform.Wrap(platform.KindUnavailable, "db.begin", "cannot begin transaction", err)
	}
	defer func() {
		if p := recover(); p != nil {
			_ = tx.Rollback()
			panic(p)
		}
	}()
	if err := fn(tx); err != nil {
		_ = tx.Rollback()
		return err
	}
	if err := tx.Commit(); err != nil {
		return platform.Wrap(platform.KindUnavailable, "db.commit", "cannot commit transaction", err)
	}
	return nil
}

// mapErr converts driver errors into typed platform errors.
func mapErr(op string, err error) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, sql.ErrNoRows) {
		return platform.NotFound(op+".not_found", "resource not found")
	}
	msg := strings.ToLower(err.Error())
	if strings.Contains(msg, "unique constraint") || strings.Contains(msg, "duplicate key") || strings.Contains(msg, "unique failed") {
		return platform.Wrap(platform.KindConflict, op+".duplicate", "resource already exists", err)
	}
	if strings.Contains(msg, "database is locked") || strings.Contains(msg, "deadlock") {
		return platform.Wrap(platform.KindUnavailable, op+".busy", "database busy", err)
	}
	return platform.Wrap(platform.KindInternal, op+".failed", "database operation failed", err)
}
