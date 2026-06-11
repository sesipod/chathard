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
