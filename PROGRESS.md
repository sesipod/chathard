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

- Iteration: 13
- Working on: Task-001/002: Fix file upload 500 error
- Started: 2026-06-13

## Last Completed

- **Task-001/002: Fix file upload 500 error**
  - `server/handlers/files.go` — Removed `if encryptedMetaStr != ""` guard that caused `encryptedMeta` to stay `nil` when the client sends an empty-string `encrypted_metadata` form field
  - Root cause: `nil` `[]byte` → SQL `NULL` → violated `encrypted_metadata BLOB NOT NULL` constraint
  - Fix: Always set `encryptedMeta = []byte(encryptedMetaStr)`, producing a valid zero-length blob `X''` for empty strings
  - Build: ✅ Compiles cleanly

## Blockers

- None

## Notes for Next Iteration

- Task-003/004: Fix group creation + display in sidebar
- File upload should now return 201; verify with curl test per PRD
