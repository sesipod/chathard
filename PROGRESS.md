# Progress Log

## Completed

- [x] Task-001: Create `message_deletions` table + queries (commit: 5ecaab0)
- [x] Task-002: Add `POST /api/messages/hide` + `POST /api/messages/batch-hide` (commit: ee9a52f)
- [x] Task-003: Add client-side single message delete to MessageBubble
- [x] Task-004: Add multi-select message deletion mode

## Current Iteration

- Iteration: 4
- Working on: Task-004: Add multi-select message deletion mode
- Started: 2026-06-14

## Last Completed

- Task-004: Add multi-select message deletion mode
- Duration: ~5 minutes
- Tests: N/A
- Key decisions:
  - `api.batchHideMessages()` added to api.js
  - Conversation.svelte: `selectMode` (boolean) + `selectedMessages` (Set) state
  - "Select" button in header-right next to 3-dot menu, shows X icon in select mode
  - MessageBubble: new `selectMode`/`selected` props, checkbox on left, hide delete btn in select mode
  - `.selected` class on bubble-row, `border-left: 3px solid var(--color-accent)` highlight
  - Floating action bar between message list and input: "N selected", "Delete Selected (N)", "Cancel"
  - Confirmation dialog before batch hide: "Delete N messages from your feed?"
  - New `batchDelete` event chain: Conversation -> RightPanel -> Main.svelte (store removal)
  - Escape key exits select mode
  - Delete button hidden in MessageBubble when selectMode is active

## Blockers

- None

## Notes for Next Iteration

- Task-005: Add `GET /api/conversations/:id/files` endpoint
