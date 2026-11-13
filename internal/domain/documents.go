package domain

import "time"

// DocumentStatus reflects indexing state.
type DocumentStatus string

const (
	DocumentIndexed DocumentStatus = "indexed"
	DocumentError   DocumentStatus = "error"
)

// Document is an ingested knowledge file for retrieval-augmented answering.
// Chunks live separately and are embedded into the vector index.
type Document struct {
	ID         string         `json:"id"`
	UserID     string         `json:"user_id"`
	Title      string         `json:"title"`
	Source     string         `json:"source,omitempty"`
	Mime       string         `json:"mime"`
	Size       int64          `json:"size"`
	Status     DocumentStatus `json:"status"`
	ChunkCount int            `json:"chunk_count"`
	Error      string         `json:"error,omitempty"`
	CreatedAt  time.Time      `json:"created_at"`
	UpdatedAt  time.Time      `json:"updated_at"`
}

// DocumentChunk is one retrievable slice of a document.
type DocumentChunk struct {
	ID         string    `json:"id"`
	DocumentID string    `json:"document_id"`
	UserID     string    `json:"user_id"`
	Index      int       `json:"index"`
	Content    string    `json:"content"`
	Tokens     int       `json:"tokens"`
	CreatedAt  time.Time `json:"created_at"`
}

// DocumentMatch is a retrieval hit with citation metadata.
type DocumentMatch struct {
	DocumentID string  `json:"document_id"`
	Title      string  `json:"title"`
	ChunkID    string  `json:"chunk_id"`
	ChunkIndex int     `json:"chunk_index"`
	Content    string  `json:"content"`
	Score      float64 `json:"score"`
}
