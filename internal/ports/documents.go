package ports

import (
	"context"

	"github.com/ArsinShaabani/AIOS/internal/domain"
	"github.com/ArsinShaabani/AIOS/internal/platform"
)

// DocumentRepository stores RAG documents and their chunks.
type DocumentRepository interface {
	CreateDocument(ctx context.Context, d *domain.Document) error
	UpdateDocument(ctx context.Context, d *domain.Document) error
	GetDocument(ctx context.Context, id string) (*domain.Document, error)
	DeleteDocument(ctx context.Context, id string) error
	ListDocuments(ctx context.Context, userID string, page platform.Page) ([]domain.Document, int, error)
	CountDocuments(ctx context.Context) (int, error)
	CreateChunks(ctx context.Context, chunks []domain.DocumentChunk) error
	GetChunk(ctx context.Context, id string) (*domain.DocumentChunk, error)
	ListChunks(ctx context.Context, documentID string, limit int) ([]domain.DocumentChunk, error)
	DeleteChunks(ctx context.Context, documentID string) error
}
