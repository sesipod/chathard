# PRD: Fix Auto-Delete Retention System

## Goal

Fix the per-user conversation auto-delete retention system so messages are actually deleted after the configured period, and UI state is consistent between the conversation header and Settings page.

## Background

The retention system has two mechanisms:
1. **On-read filtering** — hides old messages from GET responses via `created_at >= datetime('now', ?)` in `GetDirectMessagesWithRetention` / `GetGroupMessagesWithRetention`
2. **Per-message `expires_at` deletion** — cleanup goroutine (`DeleteExpiredMessages()`) deletes messages where `expires_at IS NOT NULL AND expires_at < now()`

**Bug**: The `updateRetention` handler only calls `SetUserRetention()` (stores in `user_retention` table) but NEVER calls `UpdateRetention()` to set `expires_at` on existing messages. Since the cleanup goroutine only deletes messages with `expires_at` set, nothing ever gets cleaned up.

Additionally, the client-side `handleRetention` function doesn't reload conversations after updating, so UI state stays stale.

## Scope

- Server-side Go code in `server/handlers/messages.go` and `server/db/queries.go`
- Client-side Svelte code in `web/src/views/Main.svelte`
- Optional: add `m` (minutes) support to `parseDuration` for easier testing

## Tasks

### Task-001: Add `GetUserGroupMessageIDs` query

**Priority**: High
**Depends on**: Nothing

Add a new query `GetUserGroupMessageIDs(userID, groupID string) ([]string, error)` that returns only message IDs where `sender_id = userID AND group_id = groupID`. This is needed by Task-002 for group retention — the existing `GetGroupMessageIDs` returns ALL messages in a group.

**Acceptance Criteria**:
- [ ] New function exists in `server/db/queries.go`
- [ ] Returns message IDs where `sender_id = userID AND group_id = groupID`
- [ ] Returns empty slice (not nil) when no messages found
- [ ] Builds without errors
- [ ] Follows existing patterns (deferred rows.Close, scan loop, etc.)

### Task-002: Wire `updateRetention` to set `expires_at` on existing messages

**Priority**: High
**Depends on**: Task-001

Modify `updateRetention` handler in `server/handlers/messages.go` to:
1. After `SetUserRetention()` succeeds, get the user's own message IDs for this conversation
2. For 1:1 conversations: use `GetConversationMessageIDs(userID, targetID)`
3. For groups: use `GetUserGroupMessageIDs(userID, targetID)`
4. Call `UpdateRetention(msgIDs, expiresIn)` to set/clear `expires_at` on existing messages

**Acceptance Criteria**:
- [ ] When user sets retention to `"1h"` on a 1:1 conversation, `expires_at` is set on ALL the user's outgoing messages to `created_at + 1h`
- [ ] When user sets retention to `"7d"` on a group, `expires_at` is set on ALL the user's messages in that group to `created_at + 7d`
- [ ] When user clears retention (`""`), `expires_at` is set to `NULL` on the user's messages
- [ ] Messages from the OTHER party in a 1:1 conversation are NEVER affected
- [ ] Builds without errors (`cd server && go build ./...`)
- [ ] Uses existing `UpdateRetention` and `GetConversationMessageIDs` functions correctly

### Task-003: Refresh conversations store after retention change from header

**Priority**: Medium
**Depends on**: Nothing

Modify `handleRetention` in `web/src/views/Main.svelte` to reload conversations after successfully updating retention, so the `expires_in` badge in the conversation header and Settings page reflect the new value immediately.

**Acceptance Criteria**:
- [ ] After setting retention from the conversation header, `expires_in` badge updates immediately
- [ ] Settings page shows updated value without page reload
- [ ] No double-toast or race conditions
- [ ] Works for both 1:1 and group conversations

## Out of Scope

- Cross-device sync of retention settings
- Server-side default retention enforcement
- UI changes to add minutes-based options
- Server deployment (user will handle server restart)

## Verification

After all tasks, verify with:
```bash
cd server && go build ./...
cd ../web && npm run build
```
