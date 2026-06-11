# Progress Log

## Completed

- [x] Phase 0: Automated Deployment (commit: 88d5ca5)
- [x] Phase 1: Core Server — Go + SQLite + Filesystem (commit: 71e7720)
- [x] Phase 2: Client-Side Crypto Layer (commit: pending)
- [x] Phase 3a: Svelte SPA scaffold — layout, login, PWA (commit: pending)

## Current Iteration

- Iteration: 8
- Working on: Phase 3b — Chat list panel, API client, Svelte stores, common components
- Started: 2026-06-11

## Last Completed

- **Phase 3b: Chat list panel, API client, Svelte stores, common components** (10 files, ~1,298 lines)
  - `web/src/lib/api.js` (271 lines) — REST + WebSocket client with Bearer auth, all 20+ endpoints
  - `web/src/lib/stores/auth.js` (140 lines) — Svelte writable store: challenge → sign → verify flow, restoreSession, logout
  - `web/src/lib/stores/chats.js` (138 lines) — Conversations + messages store with optimistic inserts, unread tracking, recency sorting
  - `web/src/lib/stores/settings.js` (81 lines) — Theme (dark/light/system) + defaultRetention, persisted to localStorage
  - `web/src/components/LeftPanel.svelte` (145 lines) — 380px chat list panel, gear icon → Settings, "+" button → New, SearchInput
  - `web/src/components/ChatList.svelte` (61 lines) — Scrollable conversation list with empty state, delegates to ChatListItem
  - `web/src/components/ChatListItem.svelte` (165 lines) — Conversation row: Avatar, display name, preview, relative timestamp, unread badge
  - `web/src/components/common/Avatar.svelte` (56 lines) — Circular avatar with deterministic HSL colour from text hash
  - `web/src/components/common/Modal.svelte` (123 lines) — Reusable modal: backdrop blur, click-outside/Escape dismiss, header/body/footer slots
  - `web/src/components/common/SearchInput.svelte` (118 lines) — 300ms debounced search with clear button, Signal-style styling

## Blockers

- None

## Notes for Next Iteration

- Phase 3c: Conversation view (RightPanel, Conversation, MessageBubble, MessageInput)
- Phase 3d: New chat/group modals, settings page, finish stores integration
