# Progress Log

## Completed

- [x] Phase 0: Automated Deployment (commit: 88d5ca5)
- [x] Phase 1: Core Server — Go + SQLite + Filesystem (commit: 71e7720)
- [x] Phase 2: Client-Side Crypto Layer (commit: pending)

## Current Iteration

- Iteration: 5
- Working on: Phase 2 — Client-Side Crypto Layer
- Started: 2026-06-11

## Last Completed

- Phase 2: Client-Side Crypto Layer
  - `web/src/lib/crypto/keygen.js` (260 lines) — Ed25519 keygen with Web Crypto + @noble/curves fallback, X25519 conversion, HKDF-derived auth keys, recovery codes
  - `web/src/lib/crypto/encrypt.js` (260 lines) — X25519 ECDH → HKDF → AES-256-GCM for 1:1 messages, symmetric group encryption, per-member group key distribution
  - `web/src/lib/crypto/file-encrypt.js` (188 lines) — Chunked AES-256-GCM file encryption (1MB chunks), encrypted metadata
  - `web/src/lib/crypto/recover.js` (85 lines) — PBKDF2 (100K iterations) → AES-GCM for master key recovery
  - `web/src/lib/db.js` (323 lines) — IndexedDB wrapper with keys, contacts, group_keys, drafts, settings stores
  - Total: 1,116 lines (under 1,200 limit)
  - Fixes: Added exported `ed25519ToX25519`, fixed `encryptGroupKeyForMember` public key bug and nonce embedding

## Blockers

- None

## Notes for Next Iteration

- Phase 3: Web App Frontend (Svelte + Vite + Tailwind CSS)
