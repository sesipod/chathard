# PRD: Auto-Delete Retention — Permanent Hide + Mutual Deletion

> **Source:** `delete-bug.md` — Bug report & requirements  
> **Status:** Draft — not yet executed  
> **Date:** 2026-06-15

---

## Overview

Fix the auto-delete retention system so that:
1. Once messages are hidden by a retention window, they stay hidden **permanently** — even if the user later widens the retention window.
2. When **both participants** in a 1:1 conversation have independently hidden the same message, that message is **physically deleted** from the database.
3. Internal code language shifts from "delete" to "hide" to accurately reflect behavior. User-facing UI keeps "Auto Delete" terminology.

---

## Current Bug

When a user changes retention from a narrow window (e.g., "1 hour") to a wider window (e.g., "24 hours"), `UpdateRetention()` rewrites `expires_at` on **all** messages. Messages that fell outside the narrow window and were hidden get a new future `expires_at` and **re-appear** in the UI.

This violates the core requirement: **expired messages must stay hidden forever, even if the retention window widens.**

---

## Available Retention Options

| Value | Label |
|-------|-------|
| `""` | Never |
| `"1h"` | 1 hour |
| `"24h"` | 24 hours |
| `"7d"` | 7 days |
| `"30d"` | 30 days |
| `"90d"` | 90 days |

---

## Tasks

### Task-001: Permanent Hide on Retention Change (Server)

**Priority**: High  
**Depends on**: Nothing (code already exists for message_deletions)

When a user sets retention to duration `D`:

1. Get all message IDs for this user in this conversation (existing logic)
2. Calculate cutoff: `now - D`
3. Split IDs into two groups via `GetExpiredMessageIDs(msgIDs, cutoff)`:
   - **Expired**: `created_at < cutoff` → insert into `message_deletions` (permanently hidden)
   - **Active**: `created_at >= cutoff` → set `expires_at = created_at + D` for future cleanup
4. Never modify `expires_at` on already-expired messages
5. If user clears to "Never", messages already in `message_deletions` must remain hidden

**Already done** (commit `1efc090`): The `updateRetention` handler and `GetExpiredMessageIDs` query exist. Verify they work correctly and handle edge cases.

**Acceptance Criteria**:
- [ ] User sets "1 hour" → messages older than 1 hour permanently hidden
- [ ] User changes to "24 hours" → messages older than 1 hour do NOT re-appear
- [ ] User changes to "Never" → previously hidden messages remain hidden
- [ ] Works for both 1:1 and group conversations

---

### Task-002: Mutual Permanent Deletion — 1:1 Conversations

**Priority**: High  
**Depends on**: Task-001

When both participants in a 1:1 conversation have **independently** hidden the same message, physically **DELETE** it from the `messages` table, then clean up both `message_deletions` entries.

**Trigger points**:
- Whenever `HideMessage` or `HideMessages` inserts into `message_deletions`
- After the cleanup goroutine hides expired messages

**Check logic**:
```sql
SELECT COUNT(*) FROM message_deletions WHERE message_id = ?
```
- If count = 2 (both participants hid it): 
  - `DELETE FROM messages WHERE id = ?`
  - `DELETE FROM message_deletions WHERE message_id = ?`
- Only for 1:1 messages (`recipient_id IS NOT NULL` and `group_id IS NULL`)
- Never for group messages (more than 2 participants)

**Implementation**:

1. Add query `GetMessageParticipants(messageID string) (senderID, recipientID string, err error)` to determine if a message is 1:1
2. Add query `CountMessageDeletions(messageID string) (int, error)` 
3. Add query `PermanentlyDeleteMutuallyHiddenMessage(messageID string) error` — deletes message + cleans up message_deletions
4. Add function `checkAndDeleteMutuallyHidden(messageID string)` that orchestrates the check
5. Call it from `HideMessage` and `HideMessages` after successful insert
6. Call it from cleanup goroutine after hiding expired messages

**Acceptance Criteria**:
- [ ] User1 hides message, User2 hides same message → message physically deleted from DB
- [ ] Only one user hides message → message stays in DB
- [ ] Group messages never physically deleted by this mechanism
- [ ] Both `message_deletions` entries cleaned up after physical deletion

---

### Task-003: Update Cleanup Goroutine — Hide Instead of Delete

**Priority**: Medium  
**Depends on**: Task-001, Task-002

Rename `DeleteExpiredMessages` to `HideExpiredMessages` and update its behavior:

1. Instead of `DELETE FROM messages WHERE expires_at < now()`, move expired messages into `message_deletions` for the user who set the retention
2. After hiding, trigger the mutual-deletion check (Task-002) for any affected 1:1 conversations

**Implementation**:
- For each expired message, determine which users have it in their retention window and insert into `message_deletions`
- Remove the hard `DELETE FROM messages` (mutual deletion in Task-002 handles physical cleanup)

**Acceptance Criteria**:
- [ ] Expired messages are moved to `message_deletions` per-user
- [ ] Mutual deletion check fires after hiding
- [ ] No orphaned data

---

### Task-004: Internal Language Cleanup — "Delete" → "Hide"

**Priority**: Low  
**Depends on**: Nothing (cosmetic/code quality)

Rename internal references from "delete" to "hide" to accurately reflect behavior:

| Current | Change To |
|---------|-----------|
| `DeleteExpiredMessages()` | `HideExpiredMessages()` |
| "deleted" in log messages | "hidden" (unless referring to physical deletion) |
| `deleted` variable names | `hidden` |
| Code comments referencing deletion | Reference "hiding from view" |

**Do NOT change**:
- `message_deletions` table name (avoids unnecessary migration)
- User-facing UI text (keep "Auto Delete", "retention")
- API endpoint paths
- HTTP response fields

**Do add**:
- Clear comments on `message_deletions` table: "Records messages hidden from a user's view"
- New function `PermanentlyDeleteMutuallyHiddenMessages()` for Task-002 physical deletion logic

---

### Task-005: Verify Exclusion in All Retrieval Queries

**Priority**: High  
**Depends on**: Nothing (already partially done)

Audit and ensure all message retrieval queries filter out `message_deletions`:

- [ ] `GetDirectMessagesWithRetention` — already filters
- [ ] `GetGroupMessagesWithRetention` — already filters
- [ ] `GetConversationMessageIDs` — verify it filters
- [ ] `GetUserGroupMessageIDs` — verify it filters
- [ ] `GetConversations` — already filters messages for last_message aggregation

Use consistent pattern:
```sql
LEFT JOIN message_deletions md ON m.id = md.message_id AND md.user_id = ?
WHERE md.message_id IS NULL
```

---

## Mutual Deletion Scenarios

| User1 Setting | User2 Setting | Messages older than User2's window | Messages between the two windows |
|---------------|---------------|-----------------------------------|----------------------------------|
| 1 hour | 24 hours | Physically deleted (both hidden) | Hidden from User1, visible to User2 |
| 1 hour | 7 days | Physically deleted (both hidden) | Hidden from User1, visible to User2 |
| 24 hours | 24 hours | Physically deleted (both hidden) | N/A |
| 30 days | 90 days | Physically deleted (both hidden) | Hidden from User1, visible to User2 |
| 7 days | 7 days | Physically deleted (both hidden) | N/A |
| Never | 24 hours | Hidden from User2 only | N/A |
| 1 hour | Never | Hidden from User1 only | N/A |
| 30 days | 30 days | Physically deleted (both hidden) | N/A |

**Key principle**: The narrower retention window determines what's hidden for that user. Messages hidden by **both** users get physically deleted. The wider-window participant can still see messages that only the narrower-window participant has hidden.

---

## Files Likely Affected

| File | Change |
|------|--------|
| `server/db/queries.go` | Add mutual-deletion queries (`GetMessageParticipants`, `CountMessageDeletions`, `PermanentlyDeleteMutuallyHiddenMessage`), rename `DeleteExpiredMessages` |
| `server/handlers/messages.go` | `updateRetention` handler — verify Task-001 fix, trigger mutual-deletion check |
| `server/storage/cleanup.go` | Update cleanup goroutine to use `HideExpiredMessages` + trigger mutual-deletion check |
| `server/db/migrations/005_message_deletions.sql` | Add clarifying comment (if migration file supports comments) |

---

## Out of Scope

- Delete-for-everyone vs delete-for-me distinction (not needed — current per-user model is correct)
- WebSocket notification for message deletion
- Undo functionality for hidden/deleted messages
- UI changes to retention picker options

---

## Verification

```bash
# Server builds
cd server && go build ./...

# Scenario: User sets 1h, then changes to 24h — old messages stay hidden
# (Manual test via UI)

# Scenario: Both users in 1:1 hide same message — physically deleted
# Check DB: SELECT * FROM messages WHERE id = 'test-msg' → empty
# Check DB: SELECT * FROM message_deletions WHERE message_id = 'test-msg' → empty
```
