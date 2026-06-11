# Progress Log

## Completed

- [x] Phase 0: Automated Deployment (commit: 88d5ca5)
- [x] Phase 1: Core Server — Go + SQLite + Filesystem (commit: 71e7720)
- [x] Phase 2: Client-Side Crypto Layer (commit: pending)
- [x] Phase 3a: Svelte SPA scaffold — layout, login, PWA (commit: pending)
- [x] Phase 3b: Chat list panel, API client, Svelte stores, common components (commit: pending)
- [x] Phase 3c: Conversation view, message components, and new chat/group modals

## Current Iteration

- Iteration: 9
- Working on: Phase 3d — Settings page, finishes stores integration
- Started: 2026-06-11

## Last Completed

- **Phase 3c: Conversation view, message components, and new chat/group modals** (7 files, ~1,101 lines)
  - `web/src/components/EmptyState.svelte` (76 lines) — Placeholder when no chat selected, centered icon + "New Chat" button
  - `web/src/components/RightPanel.svelte` (78 lines) — Right panel container, shows EmptyState or Conversation, mobile slide-in with back arrow
  - `web/src/components/Conversation.svelte` (264 lines) — Active conversation: header (Avatar, retention badge, 3-dot menu), message list with date separators, auto-scroll, scroll-to-bottom FAB, MessageInput
  - `web/src/components/MessageBubble.svelte` (96 lines) — Message bubble: left/right alignment, status icons (sent/delivered/read), sender handle for groups, system messages, date separators
  - `web/src/components/MessageInput.svelte` (117 lines) — Sticky bottom bar: expandable textarea (max 4 lines), attachment button, send button, typing indicator display
  - `web/src/components/NewChatModal.svelte` (175 lines) — Modal overlay with debounced user search (min 3 chars), results with key fingerprint, selects to start 1:1 conversation
  - `web/src/components/NewGroupModal.svelte` (295 lines) — Multi-step wizard (name → members → review), member search with removable chips, prepares data for group key encryption + API call

## Blockers

- None

## Notes for Next Iteration

- Phase 3d: Settings page (handle, recovery codes, theme, retention, logout)
- Integrate stores with all new components
- Wire up Main.svelte to use RightPanel, Conversation, NewChatModal, NewGroupModal
