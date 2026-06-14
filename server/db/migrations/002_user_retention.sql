-- 002_user_retention.sql
-- Per-user conversation retention settings.
-- Each user can set their own retention duration per 1:1 conversation or group.
-- This does NOT delete messages from the database — it filters them on read.
-- Messages with individual expires_at (per-message TTL) are still cleaned up by the periodic cleaner.

CREATE TABLE IF NOT EXISTS user_retention (
    user_id     TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    target_id   TEXT NOT NULL,
    target_type TEXT NOT NULL CHECK (target_type IN ('direct', 'group')),
    expires_in  TEXT,  -- e.g. '1h', '7d', '30d' or NULL/empty for Never
    updated_at  TEXT NOT NULL DEFAULT (datetime('now')),
    PRIMARY KEY (user_id, target_id, target_type)
);
