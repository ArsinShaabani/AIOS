-- 0005_user_ai: per-user AI provider credentials and key routing.
ALTER TABLE api_keys ADD COLUMN provider TEXT NOT NULL DEFAULT '';
ALTER TABLE api_keys ADD COLUMN model TEXT NOT NULL DEFAULT '';

CREATE TABLE IF NOT EXISTS user_providers (
    id            TEXT PRIMARY KEY,
    user_id       TEXT NOT NULL,
    name          TEXT NOT NULL,
    kind          TEXT NOT NULL,
    base_url      TEXT NOT NULL DEFAULT '',
    default_model TEXT NOT NULL DEFAULT '',
    models        TEXT NOT NULL DEFAULT '[]',
    api_key_enc   TEXT NOT NULL DEFAULT '',
    enabled       INTEGER NOT NULL DEFAULT 1,
    max_tokens    INTEGER NOT NULL DEFAULT 8192,
    context_window INTEGER NOT NULL DEFAULT 32768,
    created_at    TEXT NOT NULL,
    updated_at    TEXT NOT NULL,
    UNIQUE(user_id, name)
);
CREATE INDEX IF NOT EXISTS idx_user_providers_user ON user_providers(user_id);
