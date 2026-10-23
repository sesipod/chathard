<p align="center">
  <h1 align="center">🔐 TailChat</h1>
  <p align="center"><em>Zero-knowledge, end-to-end encrypted messenger, reachable only over your own WireGuard VPN</em></p>
</p>

---

## What is TailChat?

TailChat is a **Signal-style encrypted messenger** that runs exclusively behind a self-hosted [WireGuard](https://www.wireguard.com) VPN managed with [wg-easy](https://github.com/wg-easy/wg-easy). New users get a QR code, scan it in the WireGuard app, and can then open the chat. Every byte stored on the server — messages, files, metadata, keys — is **client-side encrypted**. The server stores only opaque ciphertext blobs.

- **No passwords** — authenticate with Ed25519 keypairs stored in your browser
- **No phone numbers or usernames** — identity is keyfile-based (plaintext handles being removed)
- **Group chat** — encrypted group conversations with per-member key distribution and rotation
- **Real-time** — WebSocket push for instant message delivery and typing indicators
- **No public internet** — the only open port is WireGuard (UDP 51820); the app is served at `https://chat.your-domain` only to VPN peers
- **PWA** — install on mobile, works offline, sends notifications
- **Retention controls** — per-conversation auto-delete timers (1h, 24h, 7d, 30d, 90d)

<img src="https://via.placeholder.com/800x400/020617/2563eb?text=TailChat+Screenshot" alt="TailChat UI" />

---

## 🚧 Migration in Progress: Zero-Knowledge Identity

The last plaintext column in the database — `handle` (username) — is being removed. See [`new-plan.md`](./new-plan.md) for the full plan.

**What's changing:**
- **Login**: Handle text input → keyfile upload (download a `.json` keyfile at registration)
- **Add friends**: Handle search → one-time invite codes (8-char, 24h expiry, hashed on server)
- **Display names**: Server-side handles → client-side encrypted custom names (AES-256-GCM, never seen by server)
- **No handle column** in `users` table after migration

---

## Security Model

### Zero-Knowledge Server

```
┌──────────────────────────────┐
│       TailChat Server         │
│                               │
│  • handles (plain) ⚠️         │  ← Being removed — see migration below
│  • UUIDs (plain)              │  ← Message routing
│  • public_keys (plain)        │  ← Needed for encryption setup
│  ────────────────────────    │
│  • messages (ciphered)        │  ← AES-256-GCM, per-message keys
│  • group names (ciphered)     │  ← Encrypted with group key
│  • group metadata (ciphered)  │  ← Member names, encrypted per-member
│  • files (ciphered)           │  ← Unique AES key per file
│  • metadata (ciphered)        │  ← Filenames, MIME types encrypted
└──────────────────────────────┘

> ⚠️ **Migration in progress**: `handle` is the last plaintext column. After migration, the server stores no human-readable identifiers at all — only UUIDs and public keys.
```

### Key Hierarchy

```
Master Ed25519 Keypair (IndexedDB only — never leaves your device)
├── X25519 Keypair ──── Used for ECDH message encryption
├── Auth Keypair ────── HKDF-derived, signs login challenges
└── Recovery Codes ──── 10 one-time codes (PBKDF2-encrypted master key)
```

### Authentication

- **Dual-layer**: WireGuard peer IP (first gate, checked by the firewall, Caddy and the app) + Ed25519 challenge-response (second gate)
- **Key blinding**: The master signing key never leaves your browser. An HKDF-derived auth keypair (info `"tailchat-auth"`) handles login challenges
- **Session tokens**: SHA-256 hashed in the database — raw tokens are unrecoverable from a DB dump. Stored in `sessionStorage` (cleared on tab close)
- **Recovery codes**: 10 one-time codes protected by PBKDF2 (100K SHA-256 iterations) → AES-256-GCM. Constant-time hash comparison on server
- **No passwords**: Auth proves ownership of your keypair via cryptographic challenge
- **Keyfile login (v2)**: Upload downloaded keyfile → server identifies by `user_id` → challenge-response → session token

### Encryption

| Layer | Algorithm |
|-------|-----------|
| 1:1 Messages | X25519 ECDH → HKDF-SHA256 → AES-256-GCM (ephemeral keys per message) |
| Group Messages | AES-256-GCM with per-member encrypted group key (key rotation on member changes) |
| File Attachments | AES-256-GCM (unique key per file, 1MB chunked encryption) |
| Recovery Codes | PBKDF2 (100K iterations, SHA-256) → AES-256-GCM |
| Contact Names (v2) | HKDF-SHA256 → AES-256-GCM (derived from master key, stored client-side) |

### What an Attacker Sees (Database Dump)

```sql
-- users table: handles (plain — ⚠️ being removed) & public keys (designed to be public)
alice | \x8a3f... | \xb4e2... | \x9c1d...

-- messages table: all ciphertext
\xf3a1b2c8d4... | \xe5f6... | \x12ab...

-- groups table: encrypted names + encrypted member metadata
\x44dd88aa...

-- group_members table: per-member encrypted group keys
\xaabb11...

-- files table: encrypted metadata + opaque blob paths
\x9b2c3d... | \xab/shards/...

-- sessions table: hashed tokens (no raw tokens recoverable)
a1b2c3d4e5f6...

-- encrypted_key_backups: PBKDF2-protected master keys
\x7e8f...
```

> **No message content, no group names, no filenames, no private keys, no raw session tokens — ever.**
> The only human-readable text is the `handle` column, which is actively being migrated out.

### Rate Limiting

| Endpoint | Limit | Lockout |
|----------|-------|---------|
| Registration | 3 per hour | — |
| Messages | 60 per minute | — |
| User Search | 30 per minute | — |
| Recovery | 3 per hour | 1-hour lock after 3 failures |

---

## Current Version

**v1.1.0** (Iteration 14)
- All core features complete: registration, login, 1:1 messaging, group chat, file uploads, WebSocket real-time delivery
- Security hardening: constant-time comparison, rate limiting, session token hashing
- Upcoming v2: zero-knowledge identity migration (see [`new-plan.md`](./new-plan.md))

---

## Architecture

```
Browser (Svelte PWA)                     Docker host
┌─────────────────────┐                  ┌──────────────────────────────┐
│  IndexedDB          │                  │  wg-easy (WireGuard + QR UI) │
│  └─ Ed25519 keypair │   WireGuard      │  wg-firewall                 │
│  └─ X25519 keypair  │   tunnel         │  Caddy :443 (TLS)            │
│  └─ Auth keypair    │◄───────────────►│  Go server 127.0.0.1:3000    │
│                     │   HTTPS inside   │   ├─ REST API + WebSocket    │
│  sessionStorage     │                  │   ├─ SQLite (WAL mode)       │
│  └─ session token   │                  │   └─ Blob storage            │
└─────────────────────┘                  └──────────────────────────────┘
```

### Tech Stack

| Layer | Technology |
|-------|-----------|
| Server | Go 1.23, `net/http`, `modernc.org/sqlite` |
| Database | SQLite (single file, WAL mode, `_busy_timeout=5000`) |
| Real-time | `gorilla/websocket` |
| Frontend | Svelte 5, Vite 6, Tailwind CSS v4 |
| Router | `svelte-spa-router` (hash-based) |
| Crypto (primary) | Web Crypto API (Ed25519, X25519, AES-GCM, HKDF, PBKDF2) |
| Crypto (fallback) | `@noble/curves` (Firefox/Safari — Ed25519, X25519) |
| Deployment | Docker Compose: wg-easy, Caddy (Cloudflare DNS-01), distroless app image |
| Network | WireGuard via wg-easy (split tunnel) |

---

## Deploy (Docker + wg-easy)

### What you need
- A Linux host with Docker (the WireGuard kernel module is built into modern kernels)
- UDP **51820** forwarded from your router to that host
- A domain on **Cloudflare** DNS and an API token scoped to *Zone → DNS → Edit* for that zone

### 1. DNS (Cloudflare, proxy **off** / grey cloud for both)
| Record | Points to | Why |
|---|---|---|
| `vpn.your-domain` A | your public/home IP | where phones send WireGuard traffic (use DDNS if the IP changes) |
| `chat.your-domain` A | `10.8.0.1` | the app, only reachable inside the tunnel |

Caddy gets a real Let's Encrypt cert for `chat.your-domain` with the DNS-01 challenge, so nothing has to be reachable from the internet. A trusted cert is required: browsers only expose the Web Crypto API (and PWA install) on secure origins.

### 2. Configure and start
```bash
cp .env.example .env
nano .env            # CHAT_DOMAIN, ACME_EMAIL, CF_API_TOKEN, WG_ENDPOINT_HOST, WG_ADMIN_*
chmod 600 .env
docker compose up -d --build
```
After the first start, delete `WG_ADMIN_PASSWORD` from `.env` and set `WG_INIT_ENABLED=false`.

### 3. Add a user
1. Open the wg-easy admin UI at `http://<WG_ADMIN_BIND>:51821` (published on your Tailscale IP only).
2. **New client** → name it → show the **QR code** (or create a one-time link) and send it to the person.
3. They install the WireGuard app (iOS / Android / desktop), scan the QR code, switch the tunnel on, and open `https://chat.your-domain`.

To revoke someone, disable or delete their client in wg-easy; the app becomes unreachable for them immediately.

### Network layout
```
phone ──WireGuard UDP 51820──► wg-easy container (wg0 = 10.8.0.1)
                                 │ one shared network namespace:
                                 ├─ wg-firewall  wg0 → only tcp/443 on 10.8.0.1
                                 ├─ caddy :443   TLS (Let's Encrypt, DNS-01), drops non-peers
                                 └─ chat         127.0.0.1:3000, checks peer IP again
```
- Clients get a split tunnel (`AllowedIPs = 10.8.0.1/32`): only chat traffic uses the VPN.
- IP forwarding is off and `FORWARD` drops everything on `wg0`, so a peer cannot reach other peers, your LAN, or the internet even if they edit their config.

### Updating
```bash
git pull && docker compose up -d --build
```
A fresh database is created from `server/db/schema.sql`. Data lives in the `chat_data` volume; WireGuard keys and clients in `wg_data`.

---

## Development

```bash
# Clone
git clone https://github.com/YOUR_USER/tailchat.git
cd tailchat

# Install frontend deps
cd web && npm install

# Run Go server in dev mode (proxies to Vite)
cd ../server && go run . --dev

# Or run Vite dev server separately
cd ../web && npm run dev
```

### Project Structure

```
├── scripts/
│   └── config.yaml       # Config template for running outside Docker
├── deploy/
│   ├── caddy/            # Caddy image (Cloudflare DNS plugin) + Caddyfile
│   └── wg-firewall.sh    # Locks wg0 down to tcp/443
├── compose.yaml          # wg-easy + firewall + Caddy + app
├── Dockerfile            # App image (web build + Go server)
├── server/
│   ├── main.go           # HTTP server, routing, middleware chain
│   ├── config.go         # YAML config loading
│   ├── db/
│   │   ├── schema.sql    # Full schema reference
│   │   ├── queries.go    # All prepared SQL statements
│   │   └── migrations/   # 001_init, 002_user_retention
│   ├── handlers/         # auth, register, recover, messages, files, groups, ws, invites (v2)
│   ├── middleware/        # VPN gate (peer IP check), rate limiting
│   └── storage/          # Encrypted blob storage with hex-sharding
└── web/
    └── src/
        ├── views/           # Login.svelte, Main.svelte
        ├── components/      # App.svelte, ChatList, Conversation, MessageBubble, MessageInput,
        │                    # LeftPanel, RightPanel, SettingsPage, NewChatModal, NewGroupModal,
        │                    # EmptyState, FriendsPage (v2)
        │   └── common/      # Avatar, Modal, SearchInput
        └── lib/
            ├── api.js       # REST + WebSocket client
            ├── db.js        # IndexedDB wrapper (keys, contacts, drafts, group_keys, settings)
            ├── contacts.js  # (v2) Display name resolution + Svelte store
            ├── stores/      # auth.js, chats.js, settings.js
            └── crypto/
                ├── keygen.js       # Ed25519 → X25519 + HKDF auth derivation
                ├── encrypt.js      # X25519 ECDH → AES-256-GCM messaging
                ├── file-encrypt.js # Chunked AES-256-GCM file encryption
                ├── recover.js      # PBKDF2 recovery code encrypt/decrypt
                └── contact-encrypt.js # (v2) AES-256-GCM contact name encryption
```
    ├── src/
    │   ├── lib/crypto/    # Ed25519, X25519, AES-GCM, PBKDF2
    │   ├── lib/stores/    # Auth, chats, settings (Svelte)
    │   ├── components/    # Signal-style UI components
    │   └── views/         # Login, Main app
    ├── public/            # PWA manifest, icons, SW
    └── index.html         # Vite entry point
```

---

## Security Features

- [x] **Zero plaintext on disk** — SQLite stores only ciphertext, blob paths are UUIDs only
- [x] **Client-side E2E encryption** — messages, files, group names, filenames, MIME types
- [x] **Key blinding** — master Ed25519 key never leaves IndexedDB; derived key handles auth
- [x] **SHA-256 session hashes** — raw tokens unrecoverable from DB dumps
- [x] **Constant-time comparison** — timing-attack-resistant recovery code verification
- [x] **Rate limiting** — register 3/hr, messages 60/min, search 30/min, recovery lockout
- [x] **WireGuard-only access** — app only answers clients in the VPN subnet; CORS and WebSocket Origin restricted to the app's own domain
- [x] **Container hardening** — distroless non-root app image, read-only root FS, all capabilities dropped, `no-new-privileges`
- [x] **Body size limits** — 10MB max request body
- [x] **Atomic blob writes** — temp file + rename, no partial reads
- [x] **VPN firewall** — peers can reach only `10.8.0.1:443`; no peer-to-peer, no routing to the LAN or internet, admin UI unreachable over the tunnel

### Deliberately Not Implemented (Phase 5+)

- Forward secrecy (X3DH double ratchet)
- Push notifications via Web Push API (requires public relay)
- Federation / multi-server
- E2E encrypted voice/video calls

---

## FAQ

**Q: Why WireGuard-only?**  
A: The app is never on the public internet. Only people you hand a WireGuard QR code to can even load the page, and you can cut anyone off by disabling their client in wg-easy. It's the same idea as the old Tailscale setup, but fully self-hosted with no third-party coordination server or accounts.

**Q: Can the server admin read messages?**  
A: No. Messages are encrypted with AES-256-GCM client-side before transmission. The server stores only ciphertext. Even with full database and filesystem access, message content is unrecoverable without the recipient's private key.

**Q: What happens if I lose my device?**  
A: Use one of your 10 recovery codes with your handle to restore your keypair and all message history on a new device.

**Q: Can I self-host this?**  
A: Yes — that's the only deployment model. See **Deploy** above.

**Q: Does this work on mobile?**  
A: Yes, it's a PWA. With the WireGuard tunnel on, open the URL on your phone, tap "Add to Home Screen", and it installs as a standalone app with notifications.

## License

MIT
