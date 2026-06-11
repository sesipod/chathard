---
plan-meta:
  goal: "Build TailChat — a zero-knowledge, passwordless, E2E-encrypted messenger running exclusively over Tailscale, accessible as a PWA"
  context: >
    Tech stack: Go (server), Svelte + Vite + Tailwind CSS (frontend SPA/PWA), SQLite (database),
    Web Crypto API + @noble/curves (client crypto). Deployed on Ubuntu 26 headless server via
    install.sh/update.sh scripts. Tailscale serves as both network layer and identity provider.
    No plaintext ever stored on server. Signal-style two-panel UI.
  current_phase: 0
  phases:
    - id: 0
      name: "Automated Deployment"
      description: >
        Provisioning scripts that take a bare Ubuntu 26 headless server to a running TailChat instance.
        Includes install.sh (one-shot setup), update.sh (ongoing deploys), config.yaml template,
        systemd service unit, and DB migration framework.
      expected_artifacts:
        - "scripts/install.sh"
        - "scripts/update.sh"
        - "scripts/config.yaml"
        - "scripts/tailchat.service"
        - "server/db/migrations/"
      entry_checks:
        - "repo:exists"
      exit_checks:
        - "file:scripts/install.sh"
        - "file:scripts/update.sh"
        - "file:scripts/config.yaml"
        - "file:scripts/tailchat.service"
        - "grep:systemctl daemon-reload:scripts/install.sh"
        - "grep:atomic swap:scripts/update.sh"
        - "grep:schema_migrations:scripts/update.sh"
      max_files: 5
      max_lines: 600

    - id: 1
      name: "Core Server — Go + SQLite + Filesystem"
      description: >
        Go HTTP server with Tailscale-aware identity verification (dual-layer auth),
        SQLite database (users, sessions, messages, groups, files), REST API with 16+ endpoints,
        WebSocket hub for real-time delivery, encrypted blob storage, and rate limiting middleware.
        Serves built SPA in production with SPA fallback. Includes /api/health and --version
        for deployment support.
      expected_artifacts:
        - "server/main.go"
        - "server/config.go"
        - "server/db/schema.sql"
        - "server/db/queries.go"
        - "server/handlers/register.go"
        - "server/handlers/auth.go"
        - "server/handlers/messages.go"
        - "server/handlers/ws.go"
        - "server/handlers/recover.go"
        - "server/handlers/files.go"
        - "server/handlers/groups.go"
        - "server/middleware/ratelimit.go"
        - "server/storage/blobs.go"
        - "server/storage/cleanup.go"
      entry_checks:
        - "file:scripts/install.sh"
        - "file:scripts/config.yaml"
      exit_checks:
        - "file:server/main.go"
        - "file:server/handlers/auth.go"
        - "file:server/handlers/messages.go"
        - "grep:POST /api/register:server/handlers/register.go"
        - "grep:GET /api/conversations:server/handlers/messages.go"
        - "grep:GET /api/health:server/main.go"
        - "grep:Tailscale-User-Login:server/main.go"
        - "grep:sessions:server/db/schema.sql"
        - "grep:public_key_x25519:server/db/schema.sql"
        - "grep:read_at:server/db/schema.sql"
      max_files: 14
      max_lines: 2500

    - id: 2
      name: "Client-Side Crypto Layer — Web Crypto API"
      description: >
        Ed25519 keypair generation with Web Crypto API + @noble/curves fallback (feature-detected).
        Pre-computed X25519 keypair (for ECDH) and HKDF-derived auth keypair (for key blinding).
        10 recovery codes with PBKDF2 + AES-GCM encryption of master key.
        Challenge-response auth using derived auth key, session tokens in sessionStorage.
        1:1 E2E encryption via X25519 ECDH + AES-256-GCM. Group chat with per-member encrypted
        group keys and key rotation on member changes. Chunked file encryption.
      expected_artifacts:
        - "web/src/lib/crypto/keygen.js"
        - "web/src/lib/crypto/encrypt.js"
        - "web/src/lib/crypto/file-encrypt.js"
        - "web/src/lib/crypto/recover.js"
        - "web/src/lib/db.js"
      entry_checks:
        - "file:server/main.go"
      exit_checks:
        - "file:web/src/lib/crypto/keygen.js"
        - "file:web/src/lib/crypto/encrypt.js"
        - "file:web/src/lib/crypto/file-encrypt.js"
        - "file:web/src/lib/crypto/recover.js"
        - "grep:generateKeyPair:web/src/lib/crypto/keygen.js"
        - "grep:ed25519ToX25519:web/src/lib/crypto/keygen.js"
        - "grep:HKDF:web/src/lib/crypto/encrypt.js"
        - "grep:PBKDF2:web/src/lib/crypto/recover.js"
      max_files: 5
      max_lines: 1200

    - id: 3
      name: "Web App Frontend — Signal-Style Svelte PWA"
      description: >
        Svelte SPA with Vite + Tailwind CSS v4. Signal-style two-panel layout (380px left,
        flex right on desktop; single-panel on mobile). svelte-spa-router for hash-based routing.
        "Chats" as sole navigation. Components: chat list, conversation view, message bubbles,
        new chat/new group modals, settings page (handle, recovery codes, theme, retention, logout).
        WebSocket real-time with optimistic UI, typing indicators, read receipts.
        PWA manifest + service worker with background sync and sender-handle-only notifications.
        Mobile-specific UX (48px taps, swipe, safe areas).
      expected_artifacts:
        - "web/vite.config.js"
        - "web/package.json"
        - "web/src/main.js"
        - "web/src/app.css"
        - "web/src/lib/api.js"
        - "web/src/lib/stores/auth.js"
        - "web/src/lib/stores/chats.js"
        - "web/src/lib/stores/settings.js"
        - "web/src/components/App.svelte"
        - "web/src/components/LeftPanel.svelte"
        - "web/src/components/ChatList.svelte"
        - "web/src/components/ChatListItem.svelte"
        - "web/src/components/RightPanel.svelte"
        - "web/src/components/Conversation.svelte"
        - "web/src/components/MessageBubble.svelte"
        - "web/src/components/MessageInput.svelte"
        - "web/src/components/NewChatModal.svelte"
        - "web/src/components/NewGroupModal.svelte"
        - "web/src/components/SettingsPage.svelte"
        - "web/src/components/EmptyState.svelte"
        - "web/src/components/common/Avatar.svelte"
        - "web/src/components/common/Button.svelte"
        - "web/src/components/common/Modal.svelte"
        - "web/src/components/common/SearchInput.svelte"
        - "web/src/views/Login.svelte"
        - "web/src/views/Main.svelte"
        - "web/public/manifest.json"
        - "web/public/service-worker.js"
      entry_checks:
        - "file:web/src/lib/crypto/keygen.js"
        - "file:web/src/lib/crypto/encrypt.js"
      exit_checks:
        - "file:web/src/components/App.svelte"
        - "file:web/src/components/LeftPanel.svelte"
        - "file:web/src/components/Conversation.svelte"
        - "file:web/src/components/NewChatModal.svelte"
        - "file:web/src/components/NewGroupModal.svelte"
        - "file:web/src/components/SettingsPage.svelte"
        - "file:web/src/views/Login.svelte"
        - "file:web/public/manifest.json"
        - "grep:svelte-spa-router:web/package.json"
        - "grep:standalone:web/public/manifest.json"
        - "grep:two-panel:web/src/components/App.svelte"
        - "grep:Chats:web/src/components/LeftPanel.svelte"
      max_files: 28
      max_lines: 4000

    - id: 4
      name: "Security Hardening"
      description: >
        Key blinding enforcement (HKDF-derived auth key, server verifies derived public key).
        Session tokens as SHA-256 hashes in DB. Constant-time recovery code comparison.
        Confirms rate limiting is in Phase 1, not deferred. Unique AES-GCM keys per file.
      expected_artifacts: []
      entry_checks:
        - "file:web/src/components/App.svelte"
        - "file:server/main.go"
      exit_checks:
        - "grep:timingSafeEqual:server/handlers/recover.go"
        - "grep:token_hash:server/db/schema.sql"
        - "grep:derived_public_key:server/db/schema.sql"
      max_files: 3
      max_lines: 200

  next_invocation:
    - "Begin Phase 0: Create scripts/install.sh, scripts/update.sh, scripts/config.yaml, scripts/tailchat.service, and server/db/migrations/ directory"
---

# PRD: TailChat — Tailscale-Only Encrypted Messenger

## Overview

TailChat is a zero-knowledge, passwordless, end-to-end encrypted messenger that runs exclusively over Tailscale. Every byte stored on the server — messages, files, metadata, keys — is client-side encrypted. Users authenticate via Ed25519 keypairs stored in IndexedDB and recover accounts using one-time recovery codes. Accessible as a PWA on any device at `https://tailchat-server.<tailnet>.ts.net`.

### Core Principles

- **Zero plaintext on disk**: Server stores only ciphertext. An auditor sees encrypted blobs — no message content, file contents, or conversation participants are discernible.
- **Handle is public-by-design**: Users share handles to connect. The handle→UUID mapping is the only plaintext lookup table. Message routing uses opaque UUIDs.
- **Passwordless**: No passwords. Ed25519 keypairs with 10 recovery codes printed at registration.
- **Per-conversation retention**: Auto-delete configurable per conversation (1h to Never).
- **Signal-style UI**: Two-panel desktop, single-panel mobile. "Chats" only. Dark mode default.

## Success Criteria

- [ ] All 5 phases complete with passing exit checks
- [ ] `install.sh` provisions fresh Ubuntu 26 server to running app
- [ ] `update.sh` deploys updates with atomic binary swap and health check
- [ ] All 16+ REST API endpoints implemented with rate limiting
- [ ] WebSocket real-time delivery with conversation context
- [ ] E2E encryption works for 1:1, group, and file sharing
- [ ] Recovery flow: lost key → recovery code → full restore
- [ ] Zero-knowledge audit: all DB tables + blobs are ciphertext
- [ ] PWA installable, notifications show sender handle only, zero content preview
- [ ] Cross-browser: Chrome, Firefox, Safari
- [ ] Build succeeds for both Go server and Svelte frontend

## Technical Constraints

- **Server**: Go 1.23+, SQLite (modernc.org/sqlite), net/http, Ubuntu 26
- **Frontend**: Svelte (SPA, not SvelteKit), Vite, Tailwind CSS v4, svelte-spa-router
- **Crypto**: Web Crypto API primary, @noble/curves fallback for non-Chrome browsers
- **Network**: Tailscale only — no public internet access. Dual-layer auth (Tailscale identity + session tokens).
- **Deployment**: install.sh provisions, update.sh deploys, systemd manages process
- **Testing**: Manual verification checklist (23 items). Automated tests deferred to Phase 5+.

## Tasks

### Phase 0: Automated Deployment

**Priority**: Critical — must exist before any app code can run on the server.
**Estimated Iterations**: 2-3

**Acceptance Criteria**:

- [ ] `scripts/install.sh` provisions a fresh Ubuntu 26 server with Go, Node.js 22, Tailscale, systemd service
- [ ] `scripts/update.sh` handles git-pull and SCP-file-drop deploys with atomic binary swap
- [ ] `scripts/config.yaml` template with all server configuration knobs
- [ ] `scripts/tailchat.service` systemd unit with security hardening (NoNewPrivileges, ProtectSystem, etc.)
- [ ] `server/db/migrations/` directory with numbered SQL files and schema_migrations tracking
- [ ] Server exposes `GET /api/health` and `--version` flag for deployment tooling

**Verification**:
```bash
# On fresh Ubuntu 26 server:
sudo bash scripts/install.sh
systemctl status tailchat    # active (running)
curl http://localhost:3000/api/health  # {"status":"ok","version":"..."}
```

---

### Phase 1: Core Server — Go + SQLite + Filesystem

**Priority**: Critical — the backend everything depends on.
**Estimated Iterations**: 5-8

**Acceptance Criteria**:

- [ ] Tailscale-aware HTTP server with dual-layer auth (Tailscale identity headers + session tokens)
- [ ] SQLite schema: users, sessions, encrypted_key_backups, messages, groups, group_members, files
- [ ] 16+ REST endpoints: register, auth (challenge/verify/logout), messages (CRUD + read + retention), conversations, user search, recovery, groups (CRUD + members), files (upload/download/delete), health, me
- [ ] WebSocket hub: new_message (with conversation context), typing indicators, read receipt relay
- [ ] Encrypted blob storage: chunked upload, atomic rename, periodic cleanup, configurable max storage
- [ ] Rate limiting middleware: register 3/hr, messages 60/min, search 30/min, recovery lockout
- [ ] Production static serving of `web/dist/` with SPA fallback
- [ ] Security headers: X-Content-Type-Options, X-Frame-Options, Referrer-Policy
- [ ] UFW firewall: allow only tailscale0 interface

**Verification**:
```bash
cd server && go build -o tailchat-server .
./tailchat-server --version
# In another terminal:
curl -X POST http://localhost:3000/api/register -d '{"handle":"alice",...}'
curl http://localhost:3000/api/health
```

---

### Phase 2: Client-Side Crypto Layer

**Priority**: Critical — all E2E encryption depends on this.
**Estimated Iterations**: 3-5

**Acceptance Criteria**:

- [ ] Ed25519 keypair generation with Web Crypto API + @noble/curves fallback (feature-detected)
- [ ] Pre-computed X25519 keypair (RFC 7748 conversion) and HKDF-derived auth keypair
- [ ] 10 recovery codes (4×4 base32, PBKDF2-encrypted master key) with print/download
- [ ] Challenge-response login using derived auth key (key blinding — master key never leaves IndexedDB)
- [ ] Registration: uploads all 3 public keys + encrypted backups
- [ ] Recovery: handle + code → PBKDF2 decrypt → re-derive X25519 + auth keys → restore
- [ ] 1:1 E2E encryption: ephemeral X25519 ECDH → HKDF → AES-256-GCM
- [ ] Group encryption: random group key, encrypted per-member via ECDH, key rotation on member changes
- [ ] File encryption: random key per file, 1MB chunks, streaming decrypt
- [ ] Session tokens in sessionStorage only, 7-day expiry
- [ ] IndexedDB wrapper for key storage + contact cache

**Verification**:
```javascript
// In browser console:
const kp = await generateKeyPair(); // uses Web Crypto or @noble/curves
// Register flow, login, send encrypted message, decrypt received message
```

---

### Phase 3: Web App Frontend — Signal-Style UI

**Priority**: High — user-facing.
**Estimated Iterations**: 8-12

**Acceptance Criteria**:

- [ ] Vite + Svelte SPA scaffold with Tailwind CSS v4 and svelte-spa-router
- [ ] Two-panel layout: 380px left (chat list) + flex right (conversation) on desktop; single-panel on mobile
- [ ] Left panel: "Chats" header, gear icon → Settings, "+" button → New Chat/New Group menu, search bar
- [ ] Chat list: sorted by recency, avatar (color from handle hash), handle, last message preview, timestamp, unread badge
- [ ] NewChatModal: search users by handle (min 3 chars, debounced) → start 1:1 conversation
- [ ] NewGroupModal: group name → multi-select members (with X25519 keys from search) → create
- [ ] Right panel: empty state ("Select a conversation") or active conversation with header (avatar, retention badge, 3-dot menu)
- [ ] Message bubbles: sent (right, blue) / received (left, gray), status checks (✓/✓✓/✓✓ filled), date separators
- [ ] Message input: attachment button, expandable textarea, send button, typing indicator
- [ ] Settings page: handle display, recovery codes remaining, export key, theme toggle (dark/light/system), per-conversation retention, logout
- [ ] WebSocket real-time: optimistic UI, typing indicators (encrypted boolean), read receipts
- [ ] PWA: manifest.json (theme_color #020617), service worker with background sync
- [ ] Desktop notifications: "New message from @handle" — zero content preview
- [ ] Mobile UX: 48px tap targets, pull-to-refresh, swipe back, safe area insets
- [ ] Dark mode default, light mode toggle

**Verification**:
```bash
cd web && npm ci && npm run build
# Open https://tailchat-server.<tailnet>.ts.net
# Register → Login → New Chat → Send message → Check real-time delivery
# Open Settings → Toggle theme → Change retention → Logout
```

---

### Phase 4: Security Hardening

**Priority**: Medium — integrity and defense-in-depth.
**Estimated Iterations**: 1-2

**Acceptance Criteria**:

- [ ] Key blinding enforced: server only stores/verifies derived_public_key_ed25519
- [ ] Session tokens stored as SHA-256 hashes in DB (raw tokens unrecoverable from dump)
- [ ] Constant-time comparison for recovery code hash verification
- [ ] Unique AES-GCM key per file confirmed
- [ ] Rate limiting confirmed as implemented in Phase 1 (not deferred)

**Verification**:
```bash
# Audit: sqlite3 data/tailchat.db "SELECT * FROM sessions;" # only hashes
# Audit: sqlite3 data/tailchat.db "SELECT handle, public_key_ed25519 FROM users;" # no private keys
# Audit: grep -r "timingSafeEqual\|constantTimeCompare" server/
```

---

## Architecture Notes

### Technology Stack

| Layer | Technology |
|-------|-----------|
| Server | Go 1.23, net/http, modernc.org/sqlite |
| Database | SQLite (single file, WAL mode) |
| Real-time | gorilla/websocket or nhooyr.io/websocket |
| Frontend | Svelte 5, Vite, Tailwind CSS v4 |
| Router | svelte-spa-router (hash-based) |
| Crypto (primary) | Web Crypto API (Ed25519, AES-GCM, HKDF, PBKDF2) |
| Crypto (fallback) | @noble/curves (ed25519, x25519) |
| Network | Tailscale Serve (HTTPS + identity headers) |
| Deployment | install.sh, update.sh, systemd |
| OS | Ubuntu 26 Server (headless) |

### Two-Panel Layout (Signal-Style)

```
┌──────────────────────┬─────────────────────────────┐
│ LeftPanel (380px)    │ RightPanel (flex-1)          │
│ ┌──────────────────┐ │ ┌─────────────────────────┐ │
│ │ Chats       [⚙️] │ │ │ Avatar  @alice  [7d]   │ │
│ │ [+ New]         │ │ ├─────────────────────────┤ │
│ ├──────────────────┤ │ │  ┌───────────────────┐  │ │
│ │ ● @alice   2m   │ │ │  │ Hello!            │  │ │
│ │   Hey there!    │ │ │  └───────────────────┘  │ │
│ │   @bob    1h    │ │ │        ┌───────────────┐ │ │
│ │ # Team   Yest   │ │ │        │ Hi!       ✓✓ │ │ │
│ └──────────────────┘ │ ├─────────────────────────┤ │
│                      │ │ 📎 [___________]  ➤     │ │
└──────────────────────┴─────────────────────────────┘
```

### Key Hierarchy

```
Master Ed25519 Keypair (IndexedDB only, never leaves device)
├── X25519 Keypair (pre-computed, stored in IndexedDB)
│   └── Used for: 1:1 ECDH, group key encryption
├── Derived Auth Keypair (HKDF, stored in IndexedDB)
│   └── Used for: challenge-response login
└── Recovery Codes (10x, PBKDF2-encrypted master private key)
    └── Used for: account recovery
```

### Data Flow: Sending a 1:1 Message

```
Alice (sender)                          Server                    Bob (recipient)
─────────────                          ──────                    ────────────────
1. Fetch Bob's X25519 pubkey
   (from /api/users/search or
    cached in IndexedDB)

2. Generate ephemeral X25519 kp

3. ECDH(ephemeral_priv, Bob_X25519)
   → shared secret → HKDF → AES key

4. Encrypt plaintext → ciphertext

5. POST /api/messages ──────────────→ Store ciphertext ──────→ WS push to Bob
   {recipient_id,                    in messages table          {type:"new_message",
    ciphertext,                                                 sender_id, msg_id}
    ephemeral_pubkey,
    nonce}
                                                              6. ECDH(Bob_X25519_priv,
                                                                 ephemeral_pubkey)
                                                                 → shared secret
                                                                 → decrypt
```

## Out of Scope (Phase 5+)

- Forward secrecy (X3DH double ratchet)
- Push notifications via Web Push API (needs public relay)
- Federation / multi-tailnet support
- Screen recording/screenshot protection
- E2E encrypted video/voice calls
- Automated test suite (pytest/jest/vitest)
