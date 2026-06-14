# Progress Log

## Completed

(none yet)

## Current Iteration

- Iteration: 1
- Working on: Task-001: Create `message_deletions` table + queries
- Started: 2026-06-14

## Last Completed

- Task-001: Create `message_deletions` table + queries
- Duration: ~5 minutes
- Build: ✅ All passing
- Key decisions:
  - `HideMessages` uses transaction + prepared statement for batch inserts
  - `GetHiddenMessageIDs` returns empty slice (not nil) to match existing patterns
  - `GetGroupMessagesWithRetention` signature changed: added `userID` as first param
  - Updated call sites in `messages.go` and `groups.go` handlers

## Blockers

- None

## Notes for Next Iteration

- Task-002 depends on Task-001 — handlers for hide/batch-hide endpoints
- `GetGroupMessagesWithRetention` now takes `userID` param — keep in mind for future callers
- New query functions available: `HideMessage`, `HideMessages`, `GetHiddenMessageIDs`
