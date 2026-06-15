-- 007_file_details.sql
-- Add original filename and message_id tracking to files table

ALTER TABLE files ADD COLUMN original_name TEXT;
ALTER TABLE files ADD COLUMN message_id TEXT;
