-- 001_init.sql
-- Initial schema for TailChat

-- Schema migrations tracking table
CREATE TABLE IF NOT EXISTS schema_migrations (
    version     INTEGER PRIMARY KEY,
    name        TEXT NOT NULL,
    applied_at  TEXT NOT NULL DEFAULT (datetime('now'))
);

-- Users table
CREATE TABLE IF NOT EXISTS users (
    id                          TEXT PRIMARY KEY,  -- UUID
    handle                      TEXT NOT NULL UNIQUE,
    public_key_ed25519          BLOB NOT NULL,
    public_key_x25519           BLOB NOT NULL,
    derived_public_key_ed25519  BLOB NOT NULL,     -- HKDF-blinded auth key
    created_at                  TEXT NOT NULL DEFAULT (datetime('now'))
);

-- Sessions table (token_hash = SHA-256 of session token)
CREATE TABLE IF NOT EXISTS sessions (
    token_hash  TEXT PRIMARY KEY,
    user_id     TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    created_at  TEXT NOT NULL DEFAULT (datetime('now')),
    expires_at  TEXT NOT NULL
);

-- Encrypted key backups (recovery codes)
CREATE TABLE IF NOT EXISTS encrypted_key_backups (
    user_id             TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    recovery_code_hash  TEXT NOT NULL,
    encrypted_private_key BLOB NOT NULL,
    salt                BLOB NOT NULL,
    auth_salt           BLOB NOT NULL,
    used                INTEGER NOT NULL DEFAULT 0,
    PRIMARY KEY (user_id, recovery_code_hash)
);

-- Messages table
CREATE TABLE IF NOT EXISTS messages (
    id                  TEXT PRIMARY KEY,  -- UUID
    sender_id           TEXT NOT NULL REFERENCES users(id),
    recipient_id        TEXT REFERENCES users(id),           -- NULL for group messages
    group_id            TEXT REFERENCES groups(id),          -- NULL for 1:1 messages
    ciphertext          BLOB NOT NULL,
    ephemeral_public_key BLOB NOT NULL,
    nonce               BLOB NOT NULL,
    created_at          TEXT NOT NULL DEFAULT (datetime('now')),
    expires_at          TEXT,
    read_at             TEXT
);

-- Groups table
CREATE TABLE IF NOT EXISTS groups (
    id                      TEXT PRIMARY KEY,  -- UUID
    encrypted_name          BLOB NOT NULL,
    encrypted_symmetric_key BLOB NOT NULL,
    created_at              TEXT NOT NULL DEFAULT (datetime('now'))
);

-- Group members table
CREATE TABLE IF NOT EXISTS group_members (
    group_id                TEXT NOT NULL REFERENCES groups(id) ON DELETE CASCADE,
    user_id                 TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    encrypted_group_key     BLOB NOT NULL,      -- encrypted with member's public key
    encrypted_member_metadata BLOB NOT NULL,
    PRIMARY KEY (group_id, user_id)
);

-- Files table
CREATE TABLE IF NOT EXISTS files (
    id                  TEXT PRIMARY KEY,  -- UUID
    uploader_id         TEXT NOT NULL REFERENCES users(id),
    encrypted_blob_path TEXT NOT NULL,
    encrypted_metadata  BLOB NOT NULL,
    size_bytes          INTEGER NOT NULL,
    created_at          TEXT NOT NULL DEFAULT (datetime('now')),
    expires_at          TEXT
);
