# Progress Log

## Completed

- [x] Phase 0: Automated Deployment (commit: 88d5ca5)
- [x] Phase 1: Core Server — Go + SQLite + Filesystem (commits: 5392854, 71e7720)
- [x] Phase 2: Client-Side Crypto Layer (commits: 1a65bed, b47d1a6)
- [x] Phase 3a: Svelte SPA scaffold — layout, login, PWA (commit: d7b09c9)
- [x] Phase 3b: Chat list panel, API client, Svelte stores, common components (commit: 4c0be59)
- [x] Phase 3c: Conversation view, message components, and new chat/group modals (commit: 4451931)
- [x] Phase 3d: Settings page (commit: 7f5fade)
- [x] Phase 3e: Real-time WebSocket, mobile UX finalization (commit: 607f47b)
- [x] Phase 4: Security hardening (commit: 920304d)
- [x] Task-001: Mutual Permanent Deletion — 1:1 Conversations (commit: 5829a83)
- [x] Task-002: Update Cleanup Goroutine — Hide Instead of Delete (commit: 85a5869)

## Current Iteration

- Iteration: 2 (per PRD-PERMANENT-HIDE.md)
- Working on: Task-002: Update Cleanup Goroutine — Hide Instead of Delete
- Started: 2026-06-15

## Last Completed

- **Task-002: Update Cleanup Goroutine — Hide Instead of Delete**
  - `server/db/queries.go` — Renamed `DeleteExpiredMessages` → `HideExpiredMessages`; now finds expired messages and inserts per-user `message_deletions` entries (for sender + recipient of 1:1 messages) instead of hard-deleting from DB
  - `server/storage/cleanup.go` — Added `mutualDeleteFn` field to `Cleaner` struct, updated queries interface, updated `NewCleaner` to accept the callback, updated `run()` to call `mutualDeleteFn` for each hidden message
  - `server/handlers/messages.go` — Exported `CheckAndDeleteMutuallyHidden` (was `checkAndDeleteMutuallyHidden`) so `main.go` can reference it
  - `server/main.go` — Wired `handlers.CheckAndDeleteMutuallyHidden` as the mutual-delete callback passed to `NewCleaner`
  - Build: ✅ Compiles cleanly

## Blockers

- None

## Notes for Next Iteration

- The mutual-deletion pathway is now wired through both user-initiated hide and automatic cleanup expiry — if both users' retention hides the same message, it gets permanently deleted
