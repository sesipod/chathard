# Progress Log

## Completed

- [x] Phase 0: Automated Deployment (commit: 88d5ca5)
- [x] Phase 1: Core Server — Go + SQLite + Filesystem (commit: 71e7720)
- [x] Phase 2: Client-Side Crypto Layer (commit: pending)
- [x] Phase 3a: Svelte SPA scaffold — layout, login, PWA (commit: pending)

## Current Iteration

- Iteration: 7
- Working on: Phase 3a — Svelte SPA scaffold
- Started: 2026-06-11

## Last Completed

- **Phase 3a: Svelte SPA scaffold** (9 files, ~782 lines)
  - `web/package.json` — Svelte 5 + Vite 6 + Tailwind CSS v4 + svelte-spa-router + @noble/curves
  - `web/vite.config.js` — Svelte + Tailwind plugins, dev proxy (/api, /ws → :3000), SPA fallback
  - `web/src/main.js` — Svelte mount to #app, imports app.css
  - `web/src/app.css` — Tailwind imports, dark/light CSS custom properties, system-ui font, custom scrollbars
  - `web/src/components/App.svelte` — Root component with hash-based svelte-spa-router, theme provider (localStorage/dark default)
  - `web/src/views/Login.svelte` — 3-mode form (register/login/recover), handle availability check, keygen → register, challenge → sign → verify, recovery code decrypt → restore
  - `web/src/views/Main.svelte` — Route guard (redirect if no session token), two-panel shell (380px left + flex right), empty state, mobile single-panel
  - `web/public/manifest.json` — PWA manifest (standalone, slate-950 theme, 192/512 icons)
  - `web/public/service-worker.js` — App shell caching, Background Sync for failed sends, push notifications (sender-handle-only), click-to-focus PWA

## Blockers

- None

## Notes for Next Iteration

- Phase 3b: Chat list components (LeftPanel, ChatList, ChatListItem, avatar)
- Phase 3c: Conversation view (RightPanel, Conversation, MessageBubble, MessageInput)
- Phase 3d: New chat/group modals, settings, stores (api.js, stores/)
