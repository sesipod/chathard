# PRD: Fix File Uploads & Group Chat Creation

## Overview

Two production-blocking issues: file uploads return 500, and group chats are invisible after creation. Both features were untested during initial build and need end-to-end fixes.

---

## Success Criteria

- [ ] File upload returns 201 with file_id
- [ ] Uploaded files persist on disk and in DB
- [ ] Group creation returns 201 with group_id
- [ ] Created group appears in both creator's and members' sidebars
- [ ] Group messages can be sent and received

---

## Tasks

### Task-001: Diagnose file upload via server logs

**Priority**: Critical
**Estimated Iterations**: 1-2

**Background**: `POST /api/files/upload` returns 500 with "Failed to create file record". The handler logs were added in commit `5c827b3`. Need to capture the exact error.

**Acceptance Criteria**:

- [ ] `journalctl -u tailchat -f` shows `upload: insert file record` log line with the SQLite error
- [ ] Root cause identified (likely: nil `encrypted_metadata` causing NOT NULL constraint, or blob directory permissions, or files table missing)

**Verification**:

```bash
journalctl -u tailchat -f
# Trigger upload in browser, observe log output
```

---

### Task-002: Fix file upload root cause

**Priority**: Critical  
**Estimated Iterations**: 1-2

**Likely causes** (to be confirmed by Task-001 logs):

1. **`encrypted_metadata` is nil**: Client sends `form.append('encrypted_metadata', '')` (empty string). Server does `encryptedMeta = []byte(encryptedMetaStr)` → `[]byte("")` = empty non-nil slice. Should be fine for BLOB NOT NULL.

2. **Blob directory missing or wrong permissions**: `/opt/tailchat/data/blobs/` must exist with `tailchat:tailchat` ownership and mode 700.

3. **Files table missing from DB**: Schema migration not applied → table doesn't exist. Fix: delete DB and restart.

4. **Duplicate or invalid UUID**: `storage.GenerateUUID()` may produce colliding IDs.

**Acceptance Criteria**:

- [ ] Root cause fixed
- [ ] `curl` test upload succeeds:
  ```bash
  curl -X POST http://localhost:3000/api/files/upload \
    -H "Tailscale-User-Login: test@example.com" \
    -H "Authorization: Bearer $(cat token.txt)" \
    -F "file=@/tmp/test.txt" \
    -F "encrypted_metadata=test"
  ```
- [ ] File appears in `/opt/tailchat/data/blobs/`
- [ ] File record appears in `SELECT * FROM files`

**Verification**:

```bash
sudo sqlite3 /opt/tailchat/data/tailchat.db "SELECT id, size_bytes FROM files;"
ls -la /opt/tailchat/data/blobs/
```

---

### Task-003: Diagnose group creation flow

**Priority**: Critical
**Estimated Iterations**: 1-2

**Background**: Group creation may now succeed (after removing `encrypted_symmetric_key` length check), but groups don't appear in the sidebar.

**Acceptance Criteria**:

- [ ] Server returns 201 on POST /api/groups
- [ ] Group appears in `groups` and `group_members` tables
- [ ] Client receives the response and calls `loadConversations()`

**Verification**:

```bash
sudo sqlite3 /opt/tailchat/data/tailchat.db \
  "SELECT g.id, g.encrypted_name, gm.user_id FROM groups g JOIN group_members gm ON g.id = gm.group_id;"
```

---

### Task-004: Fix group display in sidebar

**Priority**: High
**Estimated Iterations**: 2-3

**Background**: Group conversations don't appear in the chat list. The `GetConversations` SQL query only returns 1:1 conversations (based on `INNER JOIN messages`). Groups need a separate query or a union.

**Acceptance Criteria**:

- [ ] User's groups appear in the sidebar with group name
- [ ] Clicking a group opens the group chat view
- [ ] Group messages can be sent and received (separate from 1:1)
- [ ] Group members receive WebSocket notifications

**Key code areas**:

| File | Function | Issue |
|------|----------|-------|
| `server/db/queries.go` | `GetConversations()` | Only queries 1:1 messages, not groups |
| `server/db/queries.go` | `GetUserGroups()` | Exists but not called from conversation listing |
| `web/src/lib/stores/chats.js` | `loadConversations()` | Calls GET /api/conversations which only returns 1:1 |
| `web/src/lib/api.js` | `fetchConversations()` | No group data in response |
| `web/src/components/ChatListItem.svelte` | Template | Handles `conversation.type === 'group'` but type is never set |

**Approach**: Merge groups into conversation endpoint response, or add a separate `/api/groups` call in `loadConversations`.

---

### Task-005: End-to-end group chat test

**Priority**: High
**Estimated Iterations**: 1-2

**Acceptance Criteria**:

- [ ] User A creates group with User B
- [ ] Group appears in both users' sidebars (via WebSocket reload)
- [ ] User B can open the group and send a message
- [ ] User A receives the message in real time
- [ ] Group handles display correctly in message bubbles

---

## Technical Constraints

- Language: Go 1.26 (server), JavaScript/Svelte 5 (client)
- Database: SQLite via modernc.org/sqlite
- API: REST + WebSocket
- Deployment: `sudo /opt/tailchat/scripts/update.sh`

## Architecture Notes

- File upload uses multipart form data → blob storage on disk → file record in SQLite
- Groups: encrypted name + symmetric key stored in `groups` table, per-member encrypted keys in `group_members`
- Conversations are derived from messages (INNER JOIN) — groups need separate handling
- WebSocket hub notifies via `sender_id`, `recipient_id`, `group_id` fields

## Out of Scope

- E2E encryption for files and group keys (placeholder empty values for now)
- File download/view UI
- Group member management (add/remove)
- Group retention/auto-delete settings
