# PRD: Auto-Delete Retention — Permanent Hide + Mutual Deletion

See `PRDS/PRD-PERMANENT-HIDE.md` for full PRD with acceptance criteria.

## Tasks

### Task-001: Mutual Permanent Deletion — 1:1 Conversations
When both participants in a 1:1 have hidden the same message, physically DELETE it from the `messages` table + clean up `message_deletions`. Trigger from `HideMessage`/`HideMessages` and cleanup goroutine.

### Task-002: Update Cleanup Goroutine — Hide Instead of Delete
Rename `DeleteExpiredMessages` to `HideExpiredMessages`. Move expired messages into `message_deletions` per-user instead of hard-deleting. Trigger mutual-deletion check.

### Task-003: Internal Language Cleanup — "Delete" → "Hide"
Rename internal function names, variables, and comments. Keep UI text and API paths unchanged.

### Task-004: Verify Exclusion in All Retrieval Queries
Audit all message SELECT queries to ensure `message_deletions` filtering is applied consistently.

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
