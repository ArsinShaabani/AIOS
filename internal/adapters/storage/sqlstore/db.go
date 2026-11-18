// Package sqlstore implements every repository port on top of database/sql.
// The SQL is deliberately dialect-neutral (TEXT timestamps, JSON blobs, ?
// placeholders rebound for Postgres) so switching SQLite <-> Postgres never
// requires touching the core or rewriting migrations.
package sqlstore

import (
	"context"
	"database/sql"
	"embed"
	"fmt"
	"log/slog"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/ArsinShaabani/AIOS/internal/config"
	"github.com/ArsinShaabani/AIOS/internal/platform"

	_ "github.com/lib/pq"
	_ "modernc.org/sqlite"
)

//go:embed migrations/*.sql
var migrationFS embed.FS

// Dialect identifies the SQL flavour in use.
type Dialect string

const (
	DialectSQLite   Dialect = "sqlite"
	DialectPostgres Dialect = "postgres"
)

// DB wraps *sql.DB with dialect-aware helpers.
type DB struct {
	*sql.DB
	Dialect Dialect
	Log     *slog.Logger
}

// Open connects, configures the pool and (optionally) migrates.
func Open(ctx context.Context, cfg config.DatabaseConfig, dataDir string, log *slog.Logger) (*DB, error) {
	if log == nil {
		log = slog.Default()
	}
	var (
		driver  string
		dsn     string
		dialect Dialect
	)
	switch cfg.Driver {
	case "postgres":
		driver, dsn, dialect = "postgres", cfg.DSN, DialectPostgres
	default:
		var err error
		dsn, err = sqliteDSN(cfg.DSN, dataDir)
		if err != nil {
			return nil, err
		}
		driver, dialect = "sqlite", DialectSQLite
	}

	raw, err := sql.Open(driver, dsn)
	if err != nil {
		return nil, platform.Wrap(platform.KindInternal, "db.open", "cannot open database", err)
	}
	raw.SetMaxOpenConns(max(cfg.MaxOpenConns, 1))
	raw.SetMaxIdleConns(max(cfg.MaxIdleConns, 1))
	if cfg.ConnMaxLifetime.Std() > 0 {
		raw.SetConnMaxLifetime(cfg.ConnMaxLifetime.Std())
	}
	if dialect == DialectSQLite {
		// A single writer keeps memory predictable on 1 GB boxes; WAL still
		// allows concurrent readers.
		raw.SetMaxOpenConns(1)
	}
	pingCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	if err := raw.PingContext(pingCtx); err != nil {
		_ = raw.Close()
		return nil, platform.Wrap(platform.KindUnavailable, "db.ping", "database unreachable", err)
	}
	db := &DB{DB: raw, Dialect: dialect, Log: log}
	if cfg.AutoMigrate {
		if err := db.Migrate(ctx); err != nil {
			_ = raw.Close()
			return nil, err
		}
	}
	return db, nil
}

// sqliteDSN resolves relative paths against the data directory and appends the
// pragmas required for safe concurrent reads and durable writes.
func sqliteDSN(dsn, dataDir string) (string, error) {
	if dsn == "" {
		dsn = "aios.db"
	}
	path := dsn
	query := url.Values{}
	if idx := strings.Index(dsn, "?"); idx >= 0 {
		path = dsn[:idx]
		if parsed, err := url.ParseQuery(dsn[idx+1:]); err == nil {
			query = parsed
		}
	}
	path = strings.TrimPrefix(path, "file:")
	if !filepath.IsAbs(path) {
		path = filepath.Join(dataDir, path)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
		return "", fmt.Errorf("create data dir: %w", err)
	}
	pragmas := []string{
		"busy_timeout(5000)",
		"journal_mode(WAL)",
		"synchronous(NORMAL)",
		"foreign_keys(ON)",
		"temp_store(MEMORY)",
		"mmap_size(67108864)",
		"cache_size(-8000)",
	}
	for _, p := range pragmas {
		query.Add("_pragma", p)
	}
	return "file:" + path + "?" + query.Encode(), nil
}

// Rebind converts ? placeholders to $n for Postgres.
func (d *DB) Rebind(query string) string {
	if d.Dialect != DialectPostgres {
		return query
	}
	var b strings.Builder
	b.Grow(len(query) + 8)
	n := 0
	for i := 0; i < len(query); i++ {
		if query[i] == '?' {
			n++
			b.WriteByte('$')
			b.WriteString(fmt.Sprint(n))
			continue
		}
		b.WriteByte(query[i])
	}
	return b.String()
}
