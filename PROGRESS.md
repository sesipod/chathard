# Progress Log

## Completed

- [x] Phase 0: Automated Deployment (commit: 88d5ca5)
- [x] Phase 1: Core Server — Go + SQLite + Filesystem (commit: 71e7720)
- [x] Phase 2: Client-Side Crypto Layer (commit: pending)
- [x] Phase 3a: Svelte SPA scaffold — layout, login, PWA (commit: pending)
- [x] Phase 3b: Chat list panel, API client, Svelte stores, common components (commit: pending)
- [x] Phase 3c: Conversation view, message components, and new chat/group modals
- [x] **Phase 3 final: Real-time WebSocket integration, mobile UX finalization** (commit: pending)

## Current Iteration

- Iteration: 11
- Working on: Phase 4 — Security hardening
- Started: 2026-06-11

## Last Completed

- **Phase 3 final: WebSocket integration + mobile UX finalization** (5 files modified)
  - `web/src/views/Main.svelte` — Complete rewrite: store wiring (chats, auth, settings), WebSocket connect on mount with auto-reconnect, real-time message/read_receipt/typing WS event dispatch, LeftPanel↔RightPanel conversation data flow, NewChatModal/NewGroupModal visibility, SettingsPage overlay, optimistic message send with retry queue and background sync registration, mobile responsive single-panel layout with slide transitions (768px breakpoint), toast notification system, failed message queue for retry
  - `web/src/components/LeftPanel.svelte` — Added "+" dropdown menu (New Chat / New Group) with `on:newGroup` event, click-outside-to-close behavior
  - `web/src/components/RightPanel.svelte` — Added `messages` and `typingUser` props, forwarded `on:typing` event for WS typing indicator
  - `web/src/components/Conversation.svelte` — Accepts `messages` as external prop (from store) and `typingUser` prop for "X is typing..." display, forwards `on:typing` event
  - `web/src/components/MessageInput.svelte` — Added `on:typing` event dispatch on each keystroke, wired to debounced WS typing send

## Blockers

- None

## Notes for Next Iteration

- Phase 4: Security hardening — key blinding, SHA-256 session tokens, constant-time comparisons, unique AES-GCM keys per file
