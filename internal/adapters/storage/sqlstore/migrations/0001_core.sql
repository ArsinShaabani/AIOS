-- 0001_core: portable schema for SQLite and PostgreSQL.
-- Conventions: TEXT ids/timestamps (fixed-width UTC), INTEGER booleans and
-- TEXT JSON payloads keep both engines identical without dialect branching.

CREATE TABLE IF NOT EXISTS users (
    id             TEXT PRIMARY KEY,
    email          TEXT NOT NULL UNIQUE,
    display_name   TEXT NOT NULL DEFAULT '',
    password_hash  TEXT NOT NULL,
    role           TEXT NOT NULL DEFAULT 'user',
    status         TEXT NOT NULL DEFAULT 'active',
    personality_id TEXT NOT NULL DEFAULT '',
    locale         TEXT NOT NULL DEFAULT 'en',
    timezone       TEXT NOT NULL DEFAULT 'UTC',
    metadata       TEXT NOT NULL DEFAULT '{}',
    last_login_at  TEXT,
    created_at     TEXT NOT NULL,
    updated_at     TEXT NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_users_role ON users(role);
CREATE INDEX IF NOT EXISTS idx_users_created ON users(created_at);

CREATE TABLE IF NOT EXISTS sessions (
    id             TEXT PRIMARY KEY,
    user_id        TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    title          TEXT NOT NULL DEFAULT '',
    personality_id TEXT NOT NULL DEFAULT '',
    agent_id       TEXT NOT NULL DEFAULT '',
    channel        TEXT NOT NULL DEFAULT 'api',
    metadata       TEXT NOT NULL DEFAULT '{}',
    message_count  INTEGER NOT NULL DEFAULT 0,
    created_at     TEXT NOT NULL,
    updated_at     TEXT NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_sessions_user ON sessions(user_id, updated_at);
CREATE INDEX IF NOT EXISTS idx_sessions_updated ON sessions(updated_at);

CREATE TABLE IF NOT EXISTS messages (
    id           TEXT PRIMARY KEY,
    session_id   TEXT NOT NULL REFERENCES sessions(id) ON DELETE CASCADE,
    user_id      TEXT NOT NULL DEFAULT '',
    role         TEXT NOT NULL,
    content      TEXT NOT NULL DEFAULT '',
    name         TEXT NOT NULL DEFAULT '',
    tool_calls   TEXT NOT NULL DEFAULT '[]',
    tool_call_id TEXT NOT NULL DEFAULT '',
    agent_id     TEXT NOT NULL DEFAULT '',
    provider     TEXT NOT NULL DEFAULT '',
    model        TEXT NOT NULL DEFAULT '',
    tokens_in    INTEGER NOT NULL DEFAULT 0,
    tokens_out   INTEGER NOT NULL DEFAULT 0,
    latency_ms   INTEGER NOT NULL DEFAULT 0,
    cached       INTEGER NOT NULL DEFAULT 0,
    created_at   TEXT NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_messages_session ON messages(session_id, created_at);
CREATE INDEX IF NOT EXISTS idx_messages_created ON messages(created_at);

CREATE TABLE IF NOT EXISTS memories (
    id           TEXT PRIMARY KEY,
    user_id      TEXT NOT NULL,
    session_id   TEXT NOT NULL DEFAULT '',
    tier         TEXT NOT NULL DEFAULT 'long',
    kind         TEXT NOT NULL DEFAULT 'fact',
    content      TEXT NOT NULL,
    summary      TEXT NOT NULL DEFAULT '',
    tags         TEXT NOT NULL DEFAULT '[]',
    importance   REAL NOT NULL DEFAULT 0.5,
    pinned       INTEGER NOT NULL DEFAULT 0,
    source       TEXT NOT NULL DEFAULT '',
    embedding    TEXT NOT NULL DEFAULT '[]',
    use_count    INTEGER NOT NULL DEFAULT 0,
    expires_at   TEXT,
    last_used_at TEXT,
    created_at   TEXT NOT NULL,
    updated_at   TEXT NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_memories_user ON memories(user_id, tier, created_at);
CREATE INDEX IF NOT EXISTS idx_memories_session ON memories(session_id);
CREATE INDEX IF NOT EXISTS idx_memories_expires ON memories(expires_at);

CREATE TABLE IF NOT EXISTS personalities (
    id              TEXT PRIMARY KEY,
    owner_id        TEXT NOT NULL DEFAULT '',
    name            TEXT NOT NULL,
    description     TEXT NOT NULL DEFAULT '',
    tone            TEXT NOT NULL DEFAULT 'friendly',
    style           TEXT NOT NULL DEFAULT 'mixed',
    humor           INTEGER NOT NULL DEFAULT 3,
    emotional_level INTEGER NOT NULL DEFAULT 5,
    response_length TEXT NOT NULL DEFAULT 'medium',
    custom_prompt   TEXT NOT NULL DEFAULT '',
    is_preset       INTEGER NOT NULL DEFAULT 0,
    created_at      TEXT NOT NULL,
    updated_at      TEXT NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_personalities_owner ON personalities(owner_id);

CREATE TABLE IF NOT EXISTS agents (
    id            TEXT PRIMARY KEY,
    owner_id      TEXT NOT NULL DEFAULT '',
    name          TEXT NOT NULL,
    description   TEXT NOT NULL DEFAULT '',
    system_prompt TEXT NOT NULL DEFAULT '',
    tools         TEXT NOT NULL DEFAULT '[]',
    keywords      TEXT NOT NULL DEFAULT '[]',
    model_hint    TEXT NOT NULL DEFAULT '',
    priority      INTEGER NOT NULL DEFAULT 0,
    enabled       INTEGER NOT NULL DEFAULT 1,
    builtin       INTEGER NOT NULL DEFAULT 0,
    created_at    TEXT NOT NULL,
    updated_at    TEXT NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_agents_enabled ON agents(enabled, priority);
