# PRD: Files Modal, Message Deletion & Multi-Select

See `PRDS/PRD-FILES-MESSAGE-DELETE.md` for full PRD with acceptance criteria.

## Tasks

### Task-001: Create `message_deletions` table + queries
Migration, schema, query functions, update message fetches to filter hidden messages.

### Task-002: Add `POST /api/messages/hide` + `POST /api/messages/batch-hide`
Server handlers for single and batch per-user message hiding.

### Task-003: Add client-side single message delete to MessageBubble
Delete button on all messages (sent AND received), confirmation dialog, hide API call, store removal.

### Task-004: Add multi-select message deletion mode
Select button in header, checkboxes, floating action bar, batch hide via API.

### Task-005: Add `GET /api/conversations/:id/files` endpoint
Scan messages for 📎 pattern, return file metadata.

### Task-006: Files Modal in chat UI
FilesModal.svelte component, wire up to conversation header Files button.

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
