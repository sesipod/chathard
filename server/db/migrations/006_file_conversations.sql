-- 006_file_conversations.sql
-- Add conversation tracking columns to the files table.
-- Files uploaded from within a conversation are tagged with the conversation
-- ID and type so they can be listed via GET /api/conversations/:id/files
-- without needing to scan encrypted message bodies.

ALTER TABLE files ADD COLUMN target_id TEXT;
ALTER TABLE files ADD COLUMN target_type TEXT;
