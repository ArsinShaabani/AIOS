package sqlstore

import (
	"context"
	"database/sql"
	"time"

	"github.com/ArsinShaabani/AIOS/internal/domain"
	"github.com/ArsinShaabani/AIOS/internal/platform"
)

// DocumentRepo implements ports.DocumentRepository.
type DocumentRepo struct{ db *DB }

func NewDocumentRepo(db *DB) *DocumentRepo { return &DocumentRepo{db: db} }

const documentCols = `id, user_id, title, source, mime, size, status, chunk_count, error, created_at, updated_at`

func scanDocument(row rowScanner) (*domain.Document, error) {
	var (
		d       domain.Document
		created string
		updated string
	)
	if err := row.Scan(&d.ID, &d.UserID, &d.Title, &d.Source, &d.Mime, &d.Size, &d.Status,
		&d.ChunkCount, &d.Error, &created, &updated); err != nil {
		return nil, err
	}
	d.CreatedAt = parseTime(created)
	d.UpdatedAt = parseTime(updated)
	return &d, nil
}

// CreateDocument inserts document metadata.
func (r *DocumentRepo) CreateDocument(ctx context.Context, d *domain.Document) error {
	now := time.Now().UTC()
	if d.CreatedAt.IsZero() {
		d.CreatedAt = now
	}
	d.UpdatedAt = now
	if d.Status == "" {
		d.Status = domain.DocumentIndexed
	}
	_, err := r.db.ExecContext(ctx, r.db.Rebind(`INSERT INTO documents (`+documentCols+`)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`),
		d.ID, d.UserID, d.Title, d.Source, d.Mime, d.Size, string(d.Status), d.ChunkCount,
		d.Error, ft(d.CreatedAt), ft(d.UpdatedAt))
	return mapErr("document.create", err)
}

// UpdateDocument rewrites metadata/indexing state.
func (r *DocumentRepo) UpdateDocument(ctx context.Context, d *domain.Document) error {
	d.UpdatedAt = time.Now().UTC()
	res, err := r.db.ExecContext(ctx, r.db.Rebind(`UPDATE documents SET title = ?, source = ?, mime = ?,
		size = ?, status = ?, chunk_count = ?, error = ?, updated_at = ? WHERE id = ?`),
		d.Title, d.Source, d.Mime, d.Size, string(d.Status), d.ChunkCount, d.Error, ft(d.UpdatedAt), d.ID)
	if err != nil {
		return mapErr("document.update", err)
	}
	return ensureAffected(res, "document.update")
}

// GetDocument loads one document.
func (r *DocumentRepo) GetDocument(ctx context.Context, id string) (*domain.Document, error) {
	row := r.db.QueryRowContext(ctx, r.db.Rebind(`SELECT `+documentCols+` FROM documents WHERE id = ?`), id)
	d, err := scanDocument(row)
	if err != nil {
		return nil, mapErr("document.get", err)
	}
	return d, nil
}

// DeleteDocument removes a document; chunks cascade.
func (r *DocumentRepo) DeleteDocument(ctx context.Context, id string) error {
	res, err := r.db.ExecContext(ctx, r.db.Rebind(`DELETE FROM documents WHERE id = ?`), id)
	if err != nil {
		return mapErr("document.delete", err)
	}
	return ensureAffected(res, "document.delete")
}

// ListDocuments returns a user's documents newest first.
func (r *DocumentRepo) ListDocuments(ctx context.Context, userID string, page platform.Page) ([]domain.Document, int, error) {
	page = page.Normalize()
	var total int
	if err := r.db.QueryRowContext(ctx, r.db.Rebind(`SELECT COUNT(*) FROM documents WHERE user_id = ?`), userID).Scan(&total); err != nil {
		return nil, 0, mapErr("document.count", err)
	}
	rows, err := r.db.QueryContext(ctx, r.db.Rebind(`SELECT `+documentCols+` FROM documents
		WHERE user_id = ? ORDER BY created_at DESC LIMIT ? OFFSET ?`), userID, page.Limit, page.Offset)
	if err != nil {
		return nil, 0, mapErr("document.list", err)
	}
	defer rows.Close()
	out := make([]domain.Document, 0, page.Limit)
	for rows.Next() {
		d, err := scanDocument(rows)
		if err != nil {
			return nil, 0, mapErr("document.list_scan", err)
		}
		out = append(out, *d)
	}
	return out, total, mapErr("document.list_iter", rows.Err())
}

// CountDocuments counts all documents (admin metrics).
func (r *DocumentRepo) CountDocuments(ctx context.Context) (int, error) {
	var n int
	err := r.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM documents`).Scan(&n)
	return n, mapErr("document.count_all", err)
}

const chunkCols = `id, document_id, user_id, chunk_index, content, tokens, created_at`

func scanChunk(row rowScanner) (*domain.DocumentChunk, error) {
	var (
		c       domain.DocumentChunk
		created string
	)
	if err := row.Scan(&c.ID, &c.DocumentID, &c.UserID, &c.Index, &c.Content, &c.Tokens, &created); err != nil {
		return nil, err
	}
	c.CreatedAt = parseTime(created)
	return &c, nil
}

// CreateChunks inserts all chunks for a document atomically.
func (r *DocumentRepo) CreateChunks(ctx context.Context, chunks []domain.DocumentChunk) error {
	if len(chunks) == 0 {
		return nil
	}
	return r.db.withTx(ctx, func(tx *sql.Tx) error {
		stmt, err := tx.PrepareContext(ctx, r.db.Rebind(`INSERT INTO document_chunks (`+chunkCols+`)
			VALUES (?, ?, ?, ?, ?, ?, ?)`))
		if err != nil {
			return mapErr("chunk.prepare", err)
		}
		defer stmt.Close()
		for _, c := range chunks {
			if c.CreatedAt.IsZero() {
				c.CreatedAt = time.Now().UTC()
			}
			if _, err := stmt.ExecContext(ctx, c.ID, c.DocumentID, c.UserID, c.Index, c.Content,
				c.Tokens, ft(c.CreatedAt)); err != nil {
				return mapErr("chunk.create", err)
			}
		}
		return nil
	})
}

// GetChunk loads one chunk.
func (r *DocumentRepo) GetChunk(ctx context.Context, id string) (*domain.DocumentChunk, error) {
	row := r.db.QueryRowContext(ctx, r.db.Rebind(`SELECT `+chunkCols+` FROM document_chunks WHERE id = ?`), id)
	c, err := scanChunk(row)
	if err != nil {
		return nil, mapErr("chunk.get", err)
	}
	return c, nil
}

// ListChunks returns chunks in document order.
func (r *DocumentRepo) ListChunks(ctx context.Context, documentID string, limit int) ([]domain.DocumentChunk, error) {
	if limit <= 0 || limit > 1000 {
		limit = 100
	}
	rows, err := r.db.QueryContext(ctx, r.db.Rebind(`SELECT `+chunkCols+` FROM document_chunks
		WHERE document_id = ? ORDER BY chunk_index ASC LIMIT ?`), documentID, limit)
	if err != nil {
		return nil, mapErr("chunk.list", err)
	}
	defer rows.Close()
	out := make([]domain.DocumentChunk, 0, 16)
	for rows.Next() {
		c, err := scanChunk(rows)
		if err != nil {
			return nil, mapErr("chunk.list_scan", err)
		}
		out = append(out, *c)
	}
	return out, mapErr("chunk.list_iter", rows.Err())
}

// DeleteChunks removes all chunks of a document.
func (r *DocumentRepo) DeleteChunks(ctx context.Context, documentID string) error {
	_, err := r.db.ExecContext(ctx, r.db.Rebind(`DELETE FROM document_chunks WHERE document_id = ?`), documentID)
	return mapErr("chunk.delete", err)
}
