-- schema.sql: full SQLite schema for TailChat (applied with IF NOT EXISTS on start).
--
-- Zero-knowledge rule: every column that holds message content, file content,
-- file names, group names or private keys holds CIPHERTEXT produced in the
-- browser. The server only ever sees public keys, random ids, sizes, timing
-- and who talks to whom.

CREATE TABLE IF NOT EXISTS users (
    id                          TEXT PRIMARY KEY,
    handle                      TEXT NOT NULL UNIQUE,
    public_key_ed25519          BLOB NOT NULL,   -- identity key (signs messages)
    public_key_x25519           BLOB NOT NULL,   -- encryption key (derived from the identity key)
    derived_public_key_ed25519  BLOB NOT NULL,   -- login key (challenge-response only)
    created_at                  TEXT NOT NULL DEFAULT (datetime('now'))
);

CREATE TABLE IF NOT EXISTS sessions (
    token_hash  TEXT PRIMARY KEY,
    user_id     TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    created_at  TEXT NOT NULL DEFAULT (datetime('now')),
    expires_at  TEXT NOT NULL
);

-- NOTE: there is deliberately no table holding any key material. The user's
-- 256-bit account key never leaves their devices; losing it means losing the
-- account, and nobody (including the server admin) can recover it.

CREATE TABLE IF NOT EXISTS groups (
    id              TEXT PRIMARY KEY,
    encrypted_name  BLOB NOT NULL,
    created_by      TEXT NOT NULL REFERENCES users(id),
    epoch           INTEGER NOT NULL DEFAULT 0,   -- current group-key generation
    needs_rotation  INTEGER NOT NULL DEFAULT 0,   -- set when a member leaves
    created_at      TEXT NOT NULL DEFAULT (datetime('now'))
);

CREATE TABLE IF NOT EXISTS group_members (
    group_id  TEXT NOT NULL REFERENCES groups(id) ON DELETE CASCADE,
    user_id   TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    PRIMARY KEY (group_id, user_id)
);

-- One row per (group, key generation, member): the group key sealed to that
-- member's public key and signed by the group admin.
CREATE TABLE IF NOT EXISTS group_key_envelopes (
    group_id   TEXT NOT NULL REFERENCES groups(id) ON DELETE CASCADE,
    epoch      INTEGER NOT NULL,
    user_id    TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    signer_id  TEXT NOT NULL REFERENCES users(id),
    envelope   BLOB NOT NULL,
    PRIMARY KEY (group_id, epoch, user_id)
);

CREATE TABLE IF NOT EXISTS messages (
    id            TEXT PRIMARY KEY,
    sender_id     TEXT NOT NULL REFERENCES users(id),
    recipient_id  TEXT REFERENCES users(id),
    group_id      TEXT REFERENCES groups(id),
    ciphertext    BLOB NOT NULL,   -- versioned envelope, see web/src/lib/crypto/messages.js
    nonce         BLOB NOT NULL,
    created_at    TEXT NOT NULL DEFAULT (datetime('now')),
    expires_at    TEXT,
    read_at       TEXT
);

CREATE INDEX IF NOT EXISTS idx_messages_direct ON messages (sender_id, recipient_id, created_at);
CREATE INDEX IF NOT EXISTS idx_messages_group  ON messages (group_id, created_at);

-- Encrypted file blobs live on disk. Name, type, size and the file key travel
-- inside the (encrypted) chat message, never here.
CREATE TABLE IF NOT EXISTS files (
    id                  TEXT PRIMARY KEY,
    uploader_id         TEXT NOT NULL REFERENCES users(id),
    encrypted_blob_path TEXT NOT NULL,
    size_bytes          INTEGER NOT NULL,
    created_at          TEXT NOT NULL DEFAULT (datetime('now')),
    expires_at          TEXT,
    target_id           TEXT,
    target_type         TEXT,
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
