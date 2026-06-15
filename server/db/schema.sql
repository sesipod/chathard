-- schema.sql: Full SQLite schema for TailChat
-- Matches server/db/migrations/001_init.sql exactly.
-- Used as reference when creating a fresh database;
-- migrations handle upgrades for existing databases.

CREATE TABLE IF NOT EXISTS schema_migrations (
    version     INTEGER PRIMARY KEY,
    name        TEXT NOT NULL,
    applied_at  TEXT NOT NULL DEFAULT (datetime('now'))
);

CREATE TABLE IF NOT EXISTS users (
    id                          TEXT PRIMARY KEY,
    handle                      TEXT NOT NULL UNIQUE,
    public_key_ed25519          BLOB NOT NULL,
    public_key_x25519           BLOB NOT NULL,
    derived_public_key_ed25519  BLOB NOT NULL,
    created_at                  TEXT NOT NULL DEFAULT (datetime('now'))
);

CREATE TABLE IF NOT EXISTS sessions (
    token_hash  TEXT PRIMARY KEY,
    user_id     TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    created_at  TEXT NOT NULL DEFAULT (datetime('now')),
    expires_at  TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS encrypted_key_backups (
    user_id                TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    recovery_code_hash     TEXT NOT NULL,
    encrypted_private_key  BLOB NOT NULL,
    salt                   BLOB NOT NULL,
    auth_salt              BLOB NOT NULL,
    used                   INTEGER NOT NULL DEFAULT 0,
    PRIMARY KEY (user_id, recovery_code_hash)
);

CREATE TABLE IF NOT EXISTS messages (
    id                   TEXT PRIMARY KEY,
    sender_id            TEXT NOT NULL REFERENCES users(id),
    recipient_id         TEXT REFERENCES users(id),
    group_id             TEXT REFERENCES groups(id),
    ciphertext           BLOB NOT NULL,
    ephemeral_public_key BLOB NOT NULL,
    nonce                BLOB NOT NULL,
    created_at           TEXT NOT NULL DEFAULT (datetime('now')),
    expires_at           TEXT,
    read_at              TEXT
);

CREATE TABLE IF NOT EXISTS groups (
    id                      TEXT PRIMARY KEY,
    encrypted_name          BLOB NOT NULL,
    encrypted_symmetric_key BLOB NOT NULL,
    created_at              TEXT NOT NULL DEFAULT (datetime('now'))
);

CREATE TABLE IF NOT EXISTS group_members (
    group_id                  TEXT NOT NULL REFERENCES groups(id) ON DELETE CASCADE,
    user_id                   TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    encrypted_group_key       BLOB NOT NULL,
    encrypted_member_metadata BLOB NOT NULL,
    PRIMARY KEY (group_id, user_id)
);

CREATE TABLE IF NOT EXISTS files (
    id                  TEXT PRIMARY KEY,
    uploader_id         TEXT NOT NULL REFERENCES users(id),
    encrypted_blob_path TEXT NOT NULL,
    encrypted_metadata  BLOB NOT NULL,
    size_bytes          INTEGER NOT NULL,
    created_at          TEXT NOT NULL DEFAULT (datetime('now')),
    expires_at          TEXT,
    target_id           TEXT,
    target_type         TEXT,
    original_name       TEXT,
    message_id          TEXT
);

-- Per-user retention: each user controls what THEY see, without affecting others.
CREATE TABLE IF NOT EXISTS user_retention (
    user_id     TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    target_id   TEXT NOT NULL,
    target_type TEXT NOT NULL CHECK (target_type IN ('direct', 'group')),
    expires_in  TEXT,
    updated_at  TEXT NOT NULL DEFAULT (datetime('now')),
    PRIMARY KEY (user_id, target_id, target_type)
);

-- Per-user message hiding: messages stay in DB, hidden per-user via this table.
CREATE TABLE IF NOT EXISTS message_deletions (
    user_id    TEXT NOT NULL,
    message_id TEXT NOT NULL,
    deleted_at TEXT NOT NULL DEFAULT (datetime('now')),
    PRIMARY KEY (user_id, message_id)
);
