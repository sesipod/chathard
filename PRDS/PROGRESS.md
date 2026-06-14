# Progress Log

## Completed

- [x] Phase 0: Automated Deployment (commit: 88d5ca5)
- [x] Phase 1: Core Server — Go + SQLite + Filesystem (commits: 5392854, 71e7720)
- [x] Phase 2: Client-Side Crypto Layer (commits: 1a65bed, b47d1a6)
- [x] Phase 3a: Svelte SPA scaffold — layout, login, PWA (commit: d7b09c9)
- [x] Phase 3b: Chat list panel, API client, Svelte stores, common components (commit: 4c0be59)
- [x] Phase 3c: Conversation view, message components, and new chat/group modals (commit: 4451931)
- [x] Phase 3d: Settings page (commit: 7f5fade)
- [x] Phase 3e: Real-time WebSocket, mobile UX finalization (commit: 607f47b)
- [x] Phase 4: Security hardening (commit: 920304d)

## Current Iteration

- Iteration: 14
- Working on: Task-003/004: Fix group display in sidebar
- Started: 2026-06-13

## Last Completed

- **Task-003/004: Fix group display in sidebar**
  - `server/db/queries.go` — Added `GetUserGroupsWithActivity()` query with LEFT JOIN messages for last_active timestamp
  - `server/handlers/messages.go` — Modified `getConversations` to merge user's groups into the response with `type: "group"` field
  - Response now includes both 1:1 (`type: "direct"`) and group conversations
  - Groups appear with: `id` (group UUID), `name` (encrypted_name bytes), `last_active`, `type: "group"`
  - Client already handles `type === 'group'` in ChatListItem, Conversation, chats.js store
  - `handleCreateGroup` in Main.svelte already calls `loadConversations()` after creation
  - Build: ✅ Compiles cleanly

## Blockers

- None

## Notes for Next Iteration

- Task-005: End-to-end group chat test — verify groups appear in sidebar, messages can be sent/received
- Group WebSocket notification (`NotifyNewMessage`) has a gap: group members aren't notified when a group message is sent (code comment: "group handler would notify all members")
