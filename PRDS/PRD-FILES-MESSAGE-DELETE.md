# PRD: Files Modal, Message Deletion & Multi-Select

## Overview

Three features to complete the conversation experience: a working Files browser per conversation, ability to delete individual messages, and multi-select bulk delete for both 1:1 and group chats.

## Success Criteria

- [ ] All three features implemented and working
- [ ] Server build passes
- [ ] Deletion is per-user — other participants never see your deletions
- [ ] Confirmation dialogs for destructive actions
- [ ] Files correctly scoped to the conversation they were shared in

---

## Design Decisions

### Per-User Deletion Model

Messages are **never removed from the database**. Instead, we track which messages each user has hidden:

- New table `message_deletions (user_id, message_id)` — a user inserts a row here when they want to hide a message
- When fetching messages (`GET /api/messages`), filter out messages where the current user has a deletion record
- A user can delete **any message** in a conversation — their own sent messages AND messages the other person sent
- Other users are completely unaffected — they still see the message
- The message stays in the DB forever (or until the cleanup goroutine removes it based on `expires_at`)

This applies to both single-message delete and batch delete.

---

## Tasks

### Task-001: Create `message_deletions` table + queries

**Priority**: High
**Depends on**: Nothing

Create the database infrastructure for per-user message deletion.

**Implementation**:

1. Create `server/db/migrations/005_message_deletions.sql`:
   ```sql
   CREATE TABLE IF NOT EXISTS message_deletions (
       user_id    TEXT NOT NULL,
       message_id TEXT NOT NULL,
       deleted_at TEXT NOT NULL DEFAULT (datetime('now')),
       PRIMARY KEY (user_id, message_id)
   );
   ```

2. Update `server/db/schema.sql` with the same table.

3. Add query functions to `server/db/queries.go`:
   - `HideMessage(userID, messageID string) error` — inserts into `message_deletions`
   - `HideMessages(userID string, messageIDs []string) error` — batch insert for multi-select
   - `GetHiddenMessageIDs(userID string) ([]string, error)` — returns all message IDs a user has hidden

4. Update `GetDirectMessagesWithRetention` and `GetGroupMessagesWithRetention` to filter out hidden messages:
   ```sql
   AND m.id NOT IN (SELECT message_id FROM message_deletions WHERE user_id = ?)
   ```
   Pass `userID` as an additional parameter.

**Acceptance Criteria**:
- [ ] Migration file exists
- [ ] Schema updated
- [ ] `HideMessage` inserts into `message_deletions`
- [ ] `HideMessages` batch inserts correctly
- [ ] `GetHiddenMessageIDs` returns hidden IDs
- [ ] Message queries filter out hidden messages
- [ ] Build passes (`go build ./...`)

---

### Task-002: Add `POST /api/messages/hide` endpoint

**Priority**: High
**Depends on**: Task-001

Add a server-side endpoint to hide a single message from the current user's view. Works for ANY message in the conversation — sent by the user OR received from someone else.

**Implementation**:

1. Add `POST /api/messages/hide` handler in `server/handlers/messages.go`:
   - Accepts `{ message_id: "..." }`
   - Calls `HideMessage(userID, messageID)`
   - Returns 204 No Content

2. Register in `ServeHTTP` routing alongside other message sub-paths.

3. Add `POST /api/messages/batch-hide` for batch operations:
   - Accepts `{ message_ids: [...] }`
   - Calls `HideMessages(userID, messageIDs)`
   - Returns 204 No Content

**Acceptance Criteria**:
- [ ] `POST /api/messages/hide` with `{ message_id }` hides the message for the current user
- [ ] `POST /api/messages/batch-hide` with `{ message_ids }` hides multiple messages
- [ ] Works for own messages AND messages from other users
- [ ] Other users still see the message
- [ ] Build passes (`go build ./...`)

---

### Task-003: Add client-side single message delete to MessageBubble

**Priority**: High
**Depends on**: Task-002

Add a delete button to every message bubble (both sent and received). When clicked, confirm, then call the hide API and remove from the local store.

**Implementation**:

1. Add `api.hideMessage(messageId)` to `web/src/lib/api.js`:
   ```javascript
   async hideMessage(messageId) {
     const res = await fetch('/api/messages/hide', {
       method: 'POST',
       headers: headers(),
       body: JSON.stringify({ message_id: messageId }),
     });
     await throwIfNotOk(res);
   }
   ```

2. Add delete button to `MessageBubble.svelte`:
   - Visible on hover for ALL messages (both sent and received)
   - Trash icon in the bubble meta area
   - On click: show confirmation tooltip/dialog ("Delete this message from your feed? Other people will still see it.")
   - On confirm: call `api.hideMessage(message.id)`, then dispatch `'delete'` event

3. Add event handling in `Main.svelte` or `Conversation.svelte`:
   - `on:delete` event from MessageBubble
   - Remove the message from the local messages store
   - Show toast on success

**Acceptance Criteria**:
- [ ] Delete icon appears on hover for ALL messages
- [ ] Confirmation before deletion ("Delete this message from your feed?")
- [ ] Message removed from UI immediately after hiding
- [ ] Server records the hide (does NOT delete from DB)
- [ ] Message still visible to other users

### Task-004: Add multi-select message deletion mode

**Priority**: Medium
**Depends on**: Task-002, Task-003

Add a multi-select mode to the conversation view that lets users select multiple messages (sent AND received) and hide them all at once.

**Implementation**:

1. **Enter multi-select mode**: A "Select" button in the conversation header that toggles multi-select mode. On mobile, long-press a message to enter.

2. **Selection UI**: Checkbox overlay on each message bubble when in multi-select mode. Selected messages get a highlighted border. A floating action bar at the bottom shows "Delete Selected (N)" and "Cancel".

3. **Batch hide API**: Uses `POST /api/messages/batch-hide` with `{ message_ids: [...] }`.

4. **Confirmation**: "Delete N messages from your feed? Other people will not be affected."

5. **Exit**: After deletion, or pressing Cancel/Escape, multi-select mode ends.

**Acceptance Criteria**:
- [ ] Multi-select mode can be entered from conversation header
- [ ] Checkboxes appear on ALL messages in multi-select mode
- [ ] Any messages can be selected (sent OR received)
- [ ] Floating bar shows "Delete Selected (N)" with count
- [ ] Confirmation dialog before batch hide
- [ ] After hiding, messages removed from UI and store
- [ ] Server handles batch hide correctly
- [ ] Multi-select mode exits after action

---

### Task-005: Add conversation tracking to files + `GET /api/conversations/:id/files` endpoint

**Priority**: Medium
**Depends on**: Nothing (no dependency on Tasks 001-004)

**Important**: Messages are stored as encrypted ciphertext — the server CANNOT read file references from message content. Instead, we must track which conversation a file belongs to at upload time.

Add conversation tracking to file uploads and a new endpoint to list files by conversation.

**Implementation**:

1. **Create `server/db/migrations/006_file_conversations.sql`:**
   ```sql
   ALTER TABLE files ADD COLUMN target_id TEXT;
   ALTER TABLE files ADD COLUMN target_type TEXT CHECK(target_type IN ('direct', 'group'));
   ```
   Note: SQLite doesn't support CHECK with ALTER TABLE on older versions — verify. If it fails, skip CHECK and enforce in application code.

2. **Update `server/db/schema.sql`** to include `target_id` and `target_type` columns in the `files` CREATE TABLE.

3. **Update upload handler** (`server/handlers/files.go`):
   - Add `target_id` and `target_type` fields to both multipart and JSON upload paths
   - Pass them to `InsertFile`

4. **Update `InsertFile` query** to accept and store `targetID` and `targetType`.

5. **Add query functions to `server/db/queries.go`:**
   - `GetConversationFiles(targetID, targetType string) ([]FileRow, error)` — returns files for a conversation
   - Must verify the requesting user is a participant (check via messages table or group membership)

6. **Add handler** routing in `server/handlers/messages.go` or `server/handlers/files.go`:
   - `GET /api/conversations/{id}/files?type=direct|group`
   - Auth check: user must have messages with the target (1:1) or be a group member (group)
   - Returns `{ files: [...] }`

7. **Register in `server/main.go`** on the authenticated mux.

8. **Update `web/src/lib/api.js`**:
   - `uploadFile(file, expiresIn, targetId, targetType)` — add `target_id` and `target_type` to form data
   - `fetchConversationFiles(convId, isGroup)` — new method

**Acceptance Criteria**:
- [ ] Migration file exists with target_id and target_type columns
- [ ] Files uploaded from a conversation are tagged with the conversation ID
- [ ] `GET /api/conversations/{id}/files?type=direct` returns files for a 1:1 conversation
- [ ] `GET /api/conversations/{id}/files?type=group` returns files for a group
- [ ] Auth check: only conversation participants can list files
- [ ] Hidden files (message_deletions) are excluded
- [ ] Build passes

---

### Task-006: Files Modal in chat UI

**Priority**: Medium
**Depends on**: Task-005

Wire up the "Files" button in the conversation header menu to open a modal showing all files shared in that conversation.

**Current state**: `handleOpenFiles` in `Main.svelte` is a no-op placeholder. The menu item exists in `Conversation.svelte`.

**Implementation**:

1. Create `web/src/components/FilesModal.svelte`:
   - Receives `show`, `convId`, `isGroup` props
   - On mount: calls `api.fetchConversationFiles(convId, isGroup)`
   - Shows loading state while fetching
   - Renders list of files with: filename (from message text), file icon, size, date uploaded, download button
   - Each file row has a download button that calls `api.downloadFile(fileId)`
   - Close button / click-outside to dismiss
   - Empty state: "No files shared in this conversation."

2. Add `fetchConversationFiles(convId, isGroup)` to `web/src/lib/api.js`:
   ```javascript
   async fetchConversationFiles(convId, isGroup) {
     const type = isGroup ? 'group' : 'direct';
     const res = await fetch(`/api/conversations/${convId}/files?type=${type}`, { headers: headers() });
     await throwIfNotOk(res);
     return res.json(); // { files: [...] }
   }
   ```

3. Wire up in `Main.svelte`:
   - Add `showFilesModal` state and `filesModalConvId`/`filesModalIsGroup`
   - `handleOpenFiles(e)` sets these and shows the modal
   - Render `FilesModal` component when `showFilesModal` is true

4. Add `downloadFile` link to each file row (reuse logic from `MessageBubble.svelte`)

**Acceptance Criteria**:
- [ ] Clicking "Files" in conversation header opens the modal
- [ ] Modal shows all files shared in that conversation
- [ ] Each file shows name, size, upload date
- [ ] Download button works for each file
- [ ] Empty state shown when no files
- [ ] Modal can be closed via X button or click outside
- [ ] Loading state shown while fetching

---

## Technical Constraints

- Language: Go (server), JavaScript/Svelte (client)
- Database: SQLite (no schema changes for Option A)
- Auth: Session token required for all new endpoints
- Pattern: Follow existing patterns in `queries.go`, `handlers/`, `api.js`, components

## Architecture Notes

### File Discovery (Option A — Parse Messages)

```
User clicks "Files" in chat header
  → Main.svelte opens FilesModal
  → FilesModal calls GET /api/conversations/{id}/files?type=direct|group
  → Server scans messages in conversation
  → Matches 📎 filename (file_id) pattern
  → Returns array of file objects with metadata
  → FilesModal renders download links
```

The file_id from the message text is a 32-char hex UUID. The server fetches file metadata from the `files` table using these IDs.

### Message Deletion

```
User clicks delete on own message
  → Confirmation dialog
  → DELETE /api/messages/:id (server checks sender_id)
  → 204 → message removed from store
  → WebSocket notification for real-time sync (optional, v2)
```

## Out of Scope

- Edit messages (future)
- WebSocket notification for message deletion (future)
- File preview in Files Modal (just download)
- Server-side denormalization of file→conversation mapping

## Verification

```bash
# Server builds
cd server && go build ./...

# Test message hide
curl -X POST /api/messages/hide -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"message_id": "some-msg-uuid"}'
# Should return 204

# Test batch hide
curl -X POST /api/messages/batch-hide -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"message_ids": ["id1", "id2"]}'
# Should return 204

# Verify message still exists in DB (other user can see it)
sqlite3 /opt/tailchat/data/tailchat.db \
  "SELECT id FROM messages WHERE id = 'some-msg-uuid';"
# Should return the message still

# Verify hidden messages excluded from GET
curl "/api/messages?with=other-user" -H "Authorization: Bearer $TOKEN"
# Hidden messages should not appear

# Test conversation files
curl "/api/conversations/other-user-id/files?type=direct" -H "Authorization: Bearer $TOKEN"
# Should return files shared with that user

curl "/api/conversations/group-id/files?type=group" -H "Authorization: Bearer $TOKEN"
# Should return files shared in that group
```
