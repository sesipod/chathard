# Progress Log

## Completed

- [x] Task-001: Mutual Permanent Deletion — 1:1 Conversations (commit: 5829a83)
- [x] Task-002: Update Cleanup Goroutine — Hide Instead of Delete (commit: 85a5869)
- [x] Task-003: Internal Language Cleanup — "Delete" → "Hide" (commit: a0e7cc3)
- [x] Task-004: Verify Exclusion in All Retrieval Queries (commit: TBD)

## Current Iteration

- Iteration: 4
- Working on: Task-004 — Verify Exclusion in All Retrieval Queries
- Started: 2026-06-15

## Last Completed

- **Task-004**: Verify Exclusion in All Retrieval Queries
- **Duration**: ~5 minutes
- **Tests**: N/A (no test suite yet)
- **Build**: ✅ All passing
- **Key decisions**:
  - `GetConversationMessageIDs` and `GetUserGroupMessageIDs` — added `AND id NOT IN (SELECT message_id FROM message_deletions WHERE user_id = ?)` filter using existing `userID` parameter
  - No signature changes needed — both functions already took `userID` as first parameter
  - `GetDirectMessages`, `GetMessages`, `GetGroupMessages` — verified dead code (not called from any handler), left unchanged
  - `GetDirectMessagesWithRetention`, `GetGroupMessagesWithRetention`, `GetConversations` — already had the filter from previous work

## Blockers

- None

## Notes for Next Iteration

- All internal references now use "hide" terminology where messages are hidden rather than physically deleted
- `CheckAndDeleteMutuallyHidden` keeps "Delete" — it performs physical deletion of mutually hidden 1:1 messages

- The mutual-deletion pathway is now wired through both user-initiated hide and automatic cleanup expiry
