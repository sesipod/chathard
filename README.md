<p align="center">
  <h1 align="center">🔐 TailChat</h1>
  <p align="center"><em>Zero-knowledge, end-to-end encrypted messenger, reachable only over your own WireGuard VPN</em></p>
</p>

---

## What is TailChat?

TailChat is a **Signal-style encrypted messenger** that runs exclusively behind a self-hosted [WireGuard](https://www.wireguard.com) VPN managed with [wg-easy](https://github.com/wg-easy/wg-easy). New users get a QR code, scan it in the WireGuard app, and can then open the chat. Messages, files, file names and group names are **encrypted in your browser** before they are sent. The server stores only ciphertext and public keys, so someone with full access to the server cannot read what people said.

- **No passwords** — you sign in with a random 256-bit **account key** (text, QR or file) that never leaves your devices
- **Any device** — enter your handle and account key on a new phone, tablet or laptop and your whole history decrypts there
- **No phone numbers or email** — just a handle
- **Group chat** — encrypted group conversations with per-member key distribution and rotation
- **Real-time** — WebSocket push for instant message delivery and typing indicators
- **No public internet** — the only open port is WireGuard (UDP 51820); the app is served at `https://chat.your-domain` only to VPN peers
- **PWA** — install on mobile, works offline, sends notifications
- **Retention controls** — per-conversation auto-delete timers (1h, 24h, 7d, 30d, 90d)

<img src="https://via.placeholder.com/800x400/020617/2563eb?text=TailChat+Screenshot" alt="TailChat UI" />

---

## Security Model

**Goal:** someone with full access to the server (disk, database, backups, logs, even a live shell) cannot read anyone's messages, files, file names or group names.

### What the server stores

| Data | Stored as |
|---|---|
| Messages (1:1 and group) | AES-256-GCM ciphertext |
| Files and images | AES-256-GCM ciphertext blobs; **name, type, size and key travel inside the encrypted message** |
| Group names | AES-256-GCM ciphertext |
| Group keys | sealed to each member's public key and signed by the group admin |
| Accounts | handle (plaintext), three **public** keys |
| Private keys / account keys | **never** — there is no table, field or log line that holds them |

**What the server can still see (metadata):** handles, who talks to whom, which groups exist and who is in them, when messages are sent, and roughly how large they are.

### Key hierarchy

```
Account key (256 random bits) — shown once at sign-up; you keep it, the server never sees it
├── Ed25519 identity key ─── signs every message and group-key envelope
├── X25519 encryption key ── receives message keys and group keys (derived from the identity key)
└── Ed25519 login key ────── HKDF-derived; proves who you are to the server (the identity key never does)
```

Because everything derives from the one account key, **any device that has it can decrypt all of your history**, including messages you sent yourself.

### Signing in (no passwords, nothing to guess)

1. The browser asks the server for a random challenge for your handle.
2. It signs the challenge with the login key derived from your account key.
3. The server verifies the signature and issues a session token (stored hashed).

The server holds no password hash, vault or recovery blob, so there is nothing to brute-force. **If you lose your account key, nobody can recover your account or history — including the server admin.** Store it in a password manager or print the QR code.

Optional *Remember this device* keeps the key in that browser's IndexedDB so you can tap "Continue" instead of re-entering it. Use it only on devices you own; *Settings → Forget* removes it.

### Encryption

| Layer | Construction |
|---|---|
| 1:1 message | random per-message key → AES-256-GCM; that key is wrapped (X25519 ECDH → HKDF → AES-GCM) **for the recipient and again for the sender**, so you can re-read what you sent on any device |
| Group message | AES-256-GCM under the group key for the current *epoch* |
| Group key | one random key per epoch, sealed to each member and **signed by the admin**; a new epoch is issued whenever someone is removed or leaves, so ex-members cannot read new messages. Invitees only receive the key once they accept |
| Group name | AES-256-GCM under the group key |
| Files | per-file random key, 1 MiB AES-256-GCM frames with index + "last frame" authenticated (reorder/truncate/tamper is detected) |
| Authenticity | every message is Ed25519-signed; ids are bound into the encryption so the server cannot move a ciphertext to another conversation or change its sender |

### What an attacker with server access sees

```sql
users:                 alice | <32-byte pubkey> | <32-byte pubkey> | <32-byte pubkey>
messages:              <sender uuid> | <recipient/group uuid> | 02a931a7da0c… (381 random-looking bytes)
groups:                000000000e107c… (encrypted name) | admin uuid | epoch 1
group_key_envelopes:   per member, 144 bytes sealed to that member's key
files:                 uuid | size | <conversation uuid>        (blob on disk is ciphertext; no name)
```

The automated browser test searches the whole database, write-ahead log, blob directory and server log for every message, file content, file name, group name and account key used in the test: **none appears**.

### What the server (or an attacker controlling it) can still try, and how it is handled

| Attack | Defence |
|---|---|
| Read stored data | Everything sensitive is ciphertext; see above |
| Forge a message "from" someone | Messages are Ed25519-signed; the signature is verified against the sender's pinned key |
| Move/relabel/replay a ciphertext | Sender, recipient/group, and epoch are authenticated data; duplicate message ids are dropped |
| Hand a user a fake group key | Group keys must be signed by the group's admin; others are ignored |
| Swap a contact's public key | First-seen keys are **pinned**; a later change blocks messaging with a red warning. To rule out a swap at *first contact*, compare the **safety number** (chat menu → *Verify security*) in person or on a call |
| Read a removed member's future messages | Admin's browser issues a new group key (new epoch) for the remaining members |

### Browser and server hardening

- **Content-Security-Policy** (`default-src 'none'`, scripts from `self` only) so injected or third-party script cannot run and steal the account key; plus `no-store` on every API response, `frame-ancestors 'none'`, COOP/CORP, no referrer.
- **DNS-rebinding guard**: requests whose `Host` is not the configured domain (or a bare IP) are refused.
- **Session tokens are header-only.** WebSockets use **one-time, 30-second tickets**, so no long-lived secret ever appears in a URL or log.
- **Handles are case-insensitive-unique** (`Alice` ≠ `alice` impersonation is impossible); two accounts cannot share identity keys.
- **User search needs a login**, matches by prefix and escapes wildcards; sign-up uses a separate yes/no handle check; sign-in challenges look the same for real and unknown handles (no account enumeration).
- **Per-connection WebSocket queues, write deadlines, keep-alive and a per-user cap**, so one stuck or hostile client cannot freeze the server for everyone.
- **Rate limits** on every authenticated request and on uploads; a **per-account upload quota** (default 2 GB) stops one user filling the disk.
- Read receipts only cover messages actually addressed to you; static directories are never listed.
- **Group invitations must be accepted.** Being added to a group does nothing until you accept: you hold no key, see no messages and get no notifications. Declining deletes the key that was addressed to you. At most 20 invitations can be pending per person.

### Known limits (read these)

- **The server serves the app's JavaScript.** A malicious server operator could ship a modified web app that leaks keys to them. End-to-end encryption in a web app can never fully remove this; it protects against stolen data, database access, backups and passive snooping, but not against an operator who actively replaces the code. Mitigations: keep the server patched, restrict who has admin access, and consider shipping the app as a separately signed package later.
- **No forward secrecy, and an account key cannot be rotated.** Anyone who has your account key *and* a copy of the encrypted data (a server backup, or a hostile admin) can decrypt your history. **If a device is lost or stolen: delete that device's WireGuard peer in wg-easy first.** Without a VPN route the thief cannot reach the server at all, so the key on the device is useless for live access (give every device its own peer so this only affects the stolen one). Then treat the account as burned if the thief could also obtain server data, and create a new one.
- **A hostile server can censor.** It cannot read or forge messages, but it can delete or delay them, withhold a group key rotation (letting a removed member keep a stale key), or show wrong timestamps. The app refuses to *go back* to an older group key than it has seen, but cannot detect a rotation it was never told about.
- **Metadata is visible** (who talks to whom, when, group membership).
- **Anyone who gets your account key *is* you.**
- Handles are plaintext, and anyone signed in can search them.
- **A sender's auto-delete timer also removes their messages from the recipient's view** (by design, like disappearing messages).
- **Operational secrets:** the Cloudflare token in `.env` can change your DNS (scope it to a single zone, ideally a dedicated one). A WireGuard QR code contains that person's private VPN key: send it over a channel you trust, prefer wg-easy's one-time links, and delete the client when someone leaves.

### Rate Limiting

| Endpoint | Limit |
|----------|-------|
| Registration | 3 per hour per IP |
| Messages / sign-in | 60 per minute per IP |
| User search / handle check | 30 per minute per IP |
| All authenticated requests | 900 per minute per IP |
| File uploads | 30 per minute per IP, 2 GB per account |

---

## Status

**v2.0 — end-to-end encryption.** The earlier code had the crypto helpers but never called them (messages and files went to the server in plain text). This version wires real encryption through every path and tests it, including in a real browser (see *Testing*).

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
│   ├── handlers/         # auth, register, messages, files, groups, ws
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
- [x] **Signed messages & group keys** — forgery by the server or a peer is detected
- [x] **Authorization on every route** — group data, files and hide/retention require membership; covered by Go tests
- [x] **Rate limiting** — register 3/hr, messages 60/min, search 30/min (per real client IP)
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
A: No. Messages are encrypted with AES-256-GCM client-side before transmission. The server stores only ciphertext. Even with full database and filesystem access, message content is unrecoverable without an account key, which the server never has. See *Known limits* for the one thing a hostile server operator could still attempt (serving modified app code).

**Q: What happens if I lose my device?**  
A: Nothing is lost as long as you still have your **account key**: enter it with your handle on any new device and your full history appears. If you lose both the device and the key, the account cannot be recovered by anyone.

**Q: Can I self-host this?**  
A: Yes — that's the only deployment model. See **Deploy** above.

**Q: Does this work on mobile?**  
A: Yes, it's a PWA. With the WireGuard tunnel on, open the URL on your phone, tap "Add to Home Screen", and it installs as a standalone app with notifications.

## Testing

```bash
# Go: authorization rules (who may read/send/add/remove/download what)
cd server && go test ./...

# JavaScript: the real crypto — round trips, multi-device, forgery, tampering, truncation
cd web && npm install && node --test test/crypto.test.mjs

# Browser end-to-end (real server, real built app, headless Chromium, 3 users):
#   register, 1:1 chat, image + file, logout/login, brand-new device, groups,
#   invitations (accept / decline), key rotation on leave, simulated server key-swap, and a search of all server
#   storage for plaintext.  See e2e/README.md
```

## License

MIT
