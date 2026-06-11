# Progress Log

## Completed

- [x] Phase 0: Automated Deployment (commit: 88d5ca5)
- [x] Phase 1: Core Server — Go + SQLite + Filesystem (commit: pending)

## Current Iteration

- Iteration: 3
- Working on: Review Phase 1 artifacts
- Started: 2026-06-11T11:45:00Z

## Last Completed

- Phase 1: Core Server — Go + SQLite + Filesystem
- Duration: ~1 iteration
- Tests: N/A (Go not available on dev machine for compilation)
- Key decisions:
  - Dual-layer Tailscale auth: first layer rejects requests without Tailscale-User-Login (404), second layer validates SHA-256 session tokens
  - SQLite with WAL mode, pure-Go driver (modernc.org/sqlite) for zero CGO dependency
  - Blob storage with 2-hex-char sharding (<blob-dir>/<xx>/<uuid>.enc)
  - Chunked upload via temp file + atomic rename pattern
  - In-memory rate limiter with per-endpoint configs (register 3/hr, messages 60/min, search 30/min, recovery 3 failed → 1hr lock)
  - WebSocket hub with connection map per user_id, typing/read receipt relay
  - Using modernc.org/sqlite (CGO-free) over go-sqlite3 for simpler deployment

## Blockers

- Go not installed on dev machine — could not compile. Need to verify on target.

## Notes for Next Iteration

- Phase 2: Client-Side Crypto Layer
- Phase 3: Web App Frontend
