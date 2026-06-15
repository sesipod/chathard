# Progress Log

## Completed

- [x] Task-001: Mutual Permanent Deletion — 1:1 Conversations (commit: 5829a83)
- [x] Task-002: Update Cleanup Goroutine — Hide Instead of Delete (commit: 85a5869)
- [x] Task-003: Internal Language Cleanup — "Delete" → "Hide" (commit: a0e7cc3)

## Current Iteration

- Iteration: 3 (completed)
- Working on: (next task)

## Last Completed

- **Task-003**: Internal Language Cleanup — "Delete" → "Hide"
- **Duration**: ~3 minutes
- **Tests**: N/A (no test suite yet)
- **Build**: ✅ All passing
- **Key decisions**:
  - Updated comment in `server/db/queries.go` — "are deleted retroactively" → "are hidden retroactively" within `UpdateRetention`
  - No other changes needed: `DeleteExpiredMessages` already renamed to `HideExpiredMessages` (Task-002), log messages already say "hidden", `PermanentlyDeleteMutuallyHiddenMessage` and `DeleteExpiredFiles` keep "Delete" (they actually delete)

## Blockers

- None

## Notes for Next Iteration

- All internal references now use "hide" terminology where messages are hidden rather than physically deleted
- `CheckAndDeleteMutuallyHidden` keeps "Delete" — it performs physical deletion of mutually hidden 1:1 messages

- The mutual-deletion pathway is now wired through both user-initiated hide and automatic cleanup expiry
