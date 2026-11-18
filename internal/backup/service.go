// Package backup exports and restores the configuration-critical tables as a
// portable JSON document. It is admin-only and intentionally explicit about the
// tables it touches.
package backup

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/ArsinShaabani/AIOS/internal/adapters/storage/sqlstore"
	"github.com/ArsinShaabani/AIOS/internal/platform"
)

// Document is the portable backup format.
type Document struct {
	Format       string                      `json:"format"`
	Version      int                         `json:"version"`
	AppVersion   string                      `json:"app_version"`
	CreatedAt    time.Time                   `json:"created_at"`
	IncludesData bool                        `json:"includes_data"`
	Tables       map[string][]map[string]any `json:"tables"`
}

const format = "aios-backup"

var coreTables = []string{
	"users", "settings", "personalities", "agents", "devices", "plugins",
	"user_providers", "api_keys",
}

var dataTables = []string{
	"sessions", "messages", "memories", "documents", "document_chunks",
}

// Service reads/writes backups directly through the SQL handle.
type Service struct {
	db      *sqlstore.DB
	version string
}

func New(db *sqlstore.DB, version string) *Service {
	return &Service{db: db, version: version}
}

// Export serialises the selected tables. Secrets stay encrypted with the
// server's keyring; restore on another host requires copying that keyring.
func (s *Service) Export(ctx context.Context, includeData bool) (*Document, error) {
	tables := append(append([]string{}, coreTables...), dataTables...)
	if !includeData {
		tables = coreTables
	}
	doc := &Document{Format: format, Version: 1, AppVersion: s.version,
		CreatedAt: time.Now().UTC(), IncludesData: includeData, Tables: map[string][]map[string]any{}}
	for _, table := range tables {
		rows, err := s.exportTable(ctx, table)
		if err != nil {
			return nil, err
		}
		doc.Tables[table] = rows
	}
	return doc, nil
}

func (s *Service) exportTable(ctx context.Context, table string) ([]map[string]any, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT * FROM `+table)
	if err != nil {
		return nil, platform.Wrap(platform.KindInternal, "backup.export", "cannot read "+table, err)
	}
	defer rows.Close()
	cols, err := rows.Columns()
	if err != nil {
		return nil, err
	}
	out := make([]map[string]any, 0, 64)
	for rows.Next() {
		values := make([]any, len(cols))
		ptrs := make([]any, len(cols))
		for i := range values {
			ptrs[i] = &values[i]
		}
		if err := rows.Scan(ptrs...); err != nil {
			return nil, err
		}
		row := make(map[string]any, len(cols))
		for i, c := range cols {
			v := values[i]
			if b, ok := v.([]byte); ok {
				v = string(b)
			}
			row[c] = v
		}
		out = append(out, row)
	}
	return out, rows.Err()
}

// Restore replaces the backed-up tables inside one transaction.
func (s *Service) Restore(ctx context.Context, doc *Document) error {
	if doc == nil || doc.Format != format {
		return platform.Invalid("backup.format", "this is not an AIOS backup file")
	}
	if doc.Version != 1 {
		return platform.Invalid("backup.version", "unsupported backup version")
	}
	order := append(append([]string{}, coreTables...), dataTables...)
	// Only restore tables present in the document and known to us.
	known := map[string]bool{}
	for _, t := range order {
		known[t] = true
	}
	for t := range doc.Tables {
		if !known[t] {
			return platform.Invalid("backup.table", "unknown table in backup: "+t)
		}
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	for i := len(order) - 1; i >= 0; i-- {
		if _, ok := doc.Tables[order[i]]; !ok {
			continue
		}
		if _, err := tx.ExecContext(ctx, `DELETE FROM `+order[i]); err != nil {
			return platform.Wrap(platform.KindInternal, "backup.restore", "cannot clear "+order[i], err)
		}
	}
	for _, table := range order {
		rows, ok := doc.Tables[table]
		if !ok {
			continue
		}
		for _, row := range rows {
			if err := insertRow(ctx, tx, s.db, table, row); err != nil {
				return err
			}
		}
	}
	if err := tx.Commit(); err != nil {
		return platform.Wrap(platform.KindInternal, "backup.restore", "commit failed", err)
	}
	return nil
}

func insertRow(ctx context.Context, tx *sql.Tx, db *sqlstore.DB, table string, row map[string]any) error {
	cols := make([]string, 0, len(row))
	for c := range row {
		cols = append(cols, c)
	}
	sort.Strings(cols)
	quoted := make([]string, len(cols))
	marks := make([]string, len(cols))
	args := make([]any, len(cols))
	for i, c := range cols {
		quoted[i] = `"` + strings.ReplaceAll(c, `"`, ``) + `"`
		marks[i] = "?"
		args[i] = normalise(row[c])
	}
	q := fmt.Sprintf(`INSERT INTO %s (%s) VALUES (%s)`, table, strings.Join(quoted, ", "), strings.Join(marks, ", "))
	if _, err := tx.ExecContext(ctx, db.Rebind(q), args...); err != nil {
		return platform.Wrap(platform.KindInternal, "backup.restore", "cannot restore row in "+table, err)
	}
	return nil
}

func normalise(v any) any {
	switch n := v.(type) {
	case float64:
		if n == float64(int64(n)) {
			return int64(n)
		}
		return n
	case json.Number:
		if i, err := n.Int64(); err == nil {
			return i
		}
		return n.String()
	default:
		return v
	}
}
