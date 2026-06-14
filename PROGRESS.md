# Progress Log

## Completed

- [x] Task-001: Create `message_deletions` table + queries (commit: 5ecaab0)
- [x] Task-002: Add `POST /api/messages/hide` + `POST /api/messages/batch-hide` (commit: ee9a52f)
- [x] Task-003: Add client-side single message delete to MessageBubble

## Current Iteration

- Iteration: 3
- Working on: Task-003: Add client-side single message delete to MessageBubble
- Started: 2026-06-14

## Last Completed

- Task-003: Add client-side single message delete to MessageBubble
- Duration: ~5 minutes
- Tests: N/A
- Build: Server ✅, Web (node not available)
- Key decisions:
  - `api.hideMessage()` added before Files section in api.js
  - Delete button (trash icon) in bubble-meta, visible on hover via CSS `.message-wrapper:hover .delete-btn`
  - Inline confirmation overlay positioned absolutely within the bubble
  - Event chain: MessageBubble -> Conversation -> RightPanel -> Main.svelte (store removal)
  - Used `chatStore.messages.update()` to filter out hidden message by id

## Blockers

- None

## Notes for Next Iteration

- Task-004: Multi-select message deletion mode
