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

- Iteration: 12
- Working on: Phase 4 — Security hardening
- Started: 2026-06-11

## Last Completed

- **Phase 4: Security hardening** (3 files modified)
  - `server/handlers/auth.go` — Fixed session token expiry from 24h to 7 days, added periodic expired challenge cleanup goroutine (every 5 min)
  - `server/middleware/ratelimit.go` — Added periodic stale entry cleanup goroutine to prevent memory leak (every 5 min, removes entries past max window unless still locked)
  - `server/main.go` — Added periodic expired session cleanup goroutine (every 1 hour, deletes sessions where `expires_at < datetime('now')`)
  - Verification: SHA-256 session hashing ✅, challenge 5-min expiry ✅, constant-time recovery comparison ✅, HKDF key blinding ✅, unique AES-GCM per file ✅, no hardcoded secrets ✅

## Blockers

- None

## Notes for Next Iteration

- All phases complete. TailChat is fully built.
