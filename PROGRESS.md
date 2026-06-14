# Progress Log

## Completed

- [x] Task-001: Create `message_deletions` table + queries (commit: 5ecaab0)
- [x] Task-002: Add `POST /api/messages/hide` + `POST /api/messages/batch-hide` (commit: ee9a52f)

## Current Iteration

- Iteration: 2
- Working on: Task-002: Add `POST /api/messages/hide` + `POST /api/messages/batch-hide`
- Started: 2026-06-14

## Last Completed

- Task-002: Add `POST /api/messages/hide` + `POST /api/messages/batch-hide`
- Duration: ~3 minutes
- Tests: N/A
- Build: ✅ All passing
- Key decisions:
  - Routes added BEFORE `/api/messages` catch-all in ServeHTTP switch
  - Request structs `hideMessageReq` and `batchHideMessageReq` follow existing patterns
  - Both return 204 No Content on success
  - Input validation for empty message_id/message_ids

## Blockers

- None

## Notes for Next Iteration

- Task-003 depends on Task-002 — client-side single message delete
- `HideMessage` / `HideMessages` query functions are now wired to HTTP endpoints
