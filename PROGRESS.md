# Progress Log

## Completed

- [x] Task-001: Create `message_deletions` table + queries (commit: 5ecaab0)
- [x] Task-002: Add `POST /api/messages/hide` + `POST /api/messages/batch-hide` (commit: ee9a52f)
- [x] Task-003: Add client-side single message delete to MessageBubble
- [x] Task-004: Add multi-select message deletion mode (commit: 913e5e6)
- [x] Task-005: Add conversation tracking to files + `GET /api/conversations/:id/files` endpoint (commit: TBD)

## Current Iteration

- Iteration: 5
- Working on: Task-005: Add conversation tracking to files + `GET /api/conversations/:id/files` endpoint
- Started: 2026-06-14

## Last Completed

- Task-005: Add conversation tracking to files + `GET /api/conversations/:id/files` endpoint
- Duration: ~10 minutes
- Tests: N/A
- Key decisions:
  - Migration `006_file_conversations.sql` adds `target_id`/`target_type` columns to `files` table
  - `schema.sql` updated to include columns in CREATE TABLE
  - `InsertFile` signature extended with `targetID, targetType string` params
  - `uploadFile` reads `target_id`/`target_type` from form values (multipart) or JSON body
  - `GetConversationFiles` query filters files by `target_id` and `target_type`
  - `GET /api/conversations/{id}/files?type=direct|group` handler on FilesHandler
  - Auth check: group membership required for groups, conversation participant for direct
  - Route registered on `authMux` using Go 1.22+ pattern `GET /api/conversations/{id}/files`
  - `api.uploadFile(file, expiresIn, targetId, targetType)` updated
  - `api.fetchConversationFiles(convId, isGroup)` added
  - `Main.svelte` `handleAttachFile` passes `convId` and conversation type
  - Go build: clean

## Blockers

- None

## Notes for Next Iteration

- Task-006: Files Modal in chat UI — `FilesModal.svelte` component
