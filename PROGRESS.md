# Progress Log

## Completed

- [x] Task-001: Mutual Permanent Deletion — 1:1 Conversations (commit: 5829a83)
- [x] Task-002: Update Cleanup Goroutine — Hide Instead of Delete (commit: pending)

## Current Iteration

- Iteration: 2
- Working on: Task-002: Update Cleanup Goroutine — Hide Instead of Delete
- Started: 2026-06-15

## Last Completed

- **Task-002**: Update Cleanup Goroutine — Hide Instead of Delete
- **Duration**: ~5 minutes
- **Tests**: N/A (no test suite yet)
- **Build**: ✅ All passing
- **Key decisions**:
  - `server/db/queries.go` — Renamed `DeleteExpiredMessages` → `HideExpiredMessages`; finds expired messages and inserts per-user `message_deletions` entries instead of hard-deleting
  - `server/storage/cleanup.go` — Added `mutualDeleteFn` callback to `Cleaner` struct; triggers mutual-deletion check after hiding expired messages
  - `server/handlers/messages.go` — Exported `CheckAndDeleteMutuallyHidden` for use by `main.go` and `cleanup.go`
  - `server/main.go` — Wired `handlers.CheckAndDeleteMutuallyHidden` as the callback

## Blockers

- None

## Notes for Next Iteration

- The mutual-deletion pathway is now wired through both user-initiated hide and automatic cleanup expiry
