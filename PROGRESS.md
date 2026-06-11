# Progress Log

## Completed

- [x] Phase 0: Automated Deployment (commit: 88d5ca5)
- [x] Phase 1: Core Server — Go + SQLite + Filesystem (commit: 71e7720)
- [x] Phase 2: Client-Side Crypto Layer (commit: pending)

## Current Iteration

- Iteration: 6
- Working on: Phase 2 — Fix 3 critical crypto bugs from Reviewer
- Started: 2026-06-11

## Last Completed

- Bug fixes for Phase 2 crypto layer:
  - Bug 1: `encryptMessage` — Removed Web Crypto try/catch that generated a mismatched ephemeral keypair. Now always derives `ephemeralPub` from `ephemeralPriv` using `@noble/curves` X25519.
  - Bug 2: `encryptGroupKeyForMember` — Same pattern: removed Web Crypto try/catch that generated a random unrelated keypair. Now derives `senderEphemeralPub` from `ownX25519Priv` using `@noble/curves` X25519.
  - Bug 3: `decryptFile` — `decryptFile` was consuming all remaining data as one chunk (`const remaining = data.length - pos`). Now calculates exact frame sizes per chunk using `metadata.size`: each frame = 12 (nonce) + `chunkPlainSize` + 16 (GCM tag), where `chunkPlainSize = Math.min(CHUNK_SIZE, remainingSize)`. This properly processes multi-chunk files.

## Blockers

- None

## Notes for Next Iteration

- Phase 3: Web App Frontend (Svelte + Vite + Tailwind CSS)
