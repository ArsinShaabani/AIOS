-- 0004_documents: document RAG storage (metadata + chunks).
CREATE TABLE IF NOT EXISTS documents (
    id          TEXT PRIMARY KEY,
    user_id     TEXT NOT NULL,
    title       TEXT NOT NULL,
    source      TEXT NOT NULL DEFAULT '',
    mime        TEXT NOT NULL DEFAULT 'text/plain',
    size        INTEGER NOT NULL DEFAULT 0,
    status      TEXT NOT NULL DEFAULT 'indexed',
    chunk_count INTEGER NOT NULL DEFAULT 0,
    error       TEXT NOT NULL DEFAULT '',
    created_at  TEXT NOT NULL,
    updated_at  TEXT NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_documents_user ON documents(user_id, created_at);

CREATE TABLE IF NOT EXISTS document_chunks (
    id          TEXT PRIMARY KEY,
    document_id TEXT NOT NULL REFERENCES documents(id) ON DELETE CASCADE,
    user_id     TEXT NOT NULL,
    chunk_index INTEGER NOT NULL,
    content     TEXT NOT NULL,
    tokens      INTEGER NOT NULL DEFAULT 0,
    created_at  TEXT NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_document_chunks_doc ON document_chunks(document_id, chunk_index);
CREATE INDEX IF NOT EXISTS idx_document_chunks_user ON document_chunks(user_id);
