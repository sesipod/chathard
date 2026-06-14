-- 005_message_deletions.sql
-- Per-user message hiding table.
-- Messages are NEVER removed from the database. Instead, we track which messages
-- each user has chosen to hide. A user can hide ANY message in a conversation
-- (sent by them OR received from someone else). Other users are unaffected.

CREATE TABLE IF NOT EXISTS message_deletions (
    user_id    TEXT NOT NULL,
    message_id TEXT NOT NULL,
    deleted_at TEXT NOT NULL DEFAULT (datetime('now')),
    PRIMARY KEY (user_id, message_id)
);
