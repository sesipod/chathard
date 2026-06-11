# Progress Log

## Completed

- [x] Phase 0: Automated Deployment (commit: pending)

## Current Iteration

- Iteration: 1
- Working on: Phase 0 — Automated Deployment (finished)
- Started: 2026-06-11T00:00:00Z
- Completed: 2026-06-11T11:06:00Z

## Last Completed

- Phase 0: Automated Deployment
- Duration: ~1 iteration
- Tests: N/A (scripts only)
- Key decisions:
  - Go 1.23 pinned via official tarball
  - Node.js 22 LTS via NodeSource
  - Tailscale serve --https 443 for PWA access
  - UFW locked to tailscale0 only
  - Atomic swap pattern for zero-downtime-ish updates
  - schema_migrations table for idempotent DB migrations
  - systemd unit with full security hardening (ProtectSystem, NoNewPrivileges, etc.)

## Blockers

- None

## Notes for Next Iteration

- Phase 1: Core Server — Go + SQLite + Filesystem
- Server needs to implement GET /api/health and --version flag (referenced by install.sh/update.sh)
- 14 server files expected, ~2500 lines max
