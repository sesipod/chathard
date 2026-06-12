<p align="center">
  <h1 align="center">🔐 TailChat</h1>
  <p align="center"><em>Zero-knowledge, end-to-end encrypted messenger for Tailscale networks</em></p>
</p>

---

## What is TailChat?

TailChat is a **Signal-style encrypted messenger** that runs exclusively over your [Tailscale](https://tailscale.com) network. Every byte stored on the server — messages, files, metadata, keys — is **client-side encrypted**. The server stores only opaque ciphertext blobs. No plaintext ever touches disk.

- **No passwords** — authenticate with Ed25519 keypairs stored in your browser
- **No phone numbers** — just choose a username handle
- **No public internet** — accessible only to devices on your tailnet via `https://tailchat-server.your-tailnet.ts.net`
- **PWA** — install on mobile, works offline, sends notifications

<img src="https://via.placeholder.com/800x400/020617/2563eb?text=TailChat+Screenshot" alt="TailChat UI" />

---

## Security Model

### Zero-Knowledge Server

```
┌─────────────────────────┐
│     TailChat Server      │
│                          │
│  • handles (plain)       │  ← Only plaintext lookup table
│  • UUIDs (plain)         │  ← Message routing
│  • public_keys (plain)   │  ← Needed for encryption setup
│  ─────────────────────  │
│  • messages (ciphered)   │  ← AES-256-GCM, per-message keys
│  • group names (ciphered)│  ← Encrypted with group key
│  • files (ciphered)      │  ← Unique AES key per file
│  • metadata (ciphered)   │  ← Filenames, MIME types encrypted
└─────────────────────────┘
```

### Key Hierarchy

```
Master Ed25519 Keypair (IndexedDB only — never leaves your device)
├── X25519 Keypair ──── Used for ECDH message encryption
├── Auth Keypair ────── HKDF-derived, signs login challenges
└── Recovery Codes ──── 10 one-time codes (PBKDF2-encrypted master key)
```

### Authentication

- **Dual-layer**: Tailscale network identity + Ed25519 challenge-response
- **Key blinding**: The master signing key never leaves your browser. A derived auth key handles login
- **Session tokens**: SHA-256 hashed in the database — raw tokens are unrecoverable from a DB dump
- **No passwords**: Auth proves ownership of your keypair via cryptographic challenge

### Encryption

| Layer | Algorithm |
|-------|-----------|
| 1:1 Messages | X25519 ECDH → HKDF-SHA256 → AES-256-GCM (ephemeral keys per message) |
| Group Messages | AES-256-GCM with per-member encrypted group key (key rotation on member changes) |
| File Attachments | AES-256-GCM (unique key per file, 1MB chunked encryption) |
| Recovery Codes | PBKDF2 (100K iterations, SHA-256) → AES-256-GCM |

### What an Attacker Sees (Database Dump)

```sql
-- users table: handles & public keys (designed to be public)
alice | \x8a3f... | \xb4e2... | \x9c1d...

-- messages table: all ciphertext
\xf3a1b2c8d4... | \xe5f6... | \x12ab...

-- groups table: encrypted names
\x44dd88aa...

-- files table: encrypted metadata
\x9b2c3d...
```

**No message content, no group names, no filenames, no private keys — ever.**

---

## Architecture

```
Browser (Svelte PWA)                    Ubuntu 26 Server
┌─────────────────────┐                 ┌──────────────────────────┐
│  IndexedDB          │                 │  systemd: tailchat       │
│  └─ Ed25519 keypair │                 │  └─ Go HTTP server :3000 │
│  └─ X25519 keypair  │    HTTPS        │     ├─ REST API (16+ ep) │
│  └─ Auth keypair    │◄──────────────►│     ├─ WebSocket hub     │
│                     │  Tailscale      │     ├─ SQLite (WAL mode) │
│  sessionStorage     │  Serve          │     ├─ Blob storage      │
│  └─ session token   │                 │     └─ Rate limiter      │
└─────────────────────┘                 └──────────────────────────┘
                                               │
                                        Tailscale Serve
                                        :443 → :3000 + TLS
```

### Tech Stack

| Layer | Technology |
|-------|-----------|
| Server | Go 1.26, `net/http`, `modernc.org/sqlite` |
| Database | SQLite (single file, WAL mode) |
| Real-time | `gorilla/websocket` |
| Frontend | Svelte 5, Vite 6, Tailwind CSS v4 |
| Router | `svelte-spa-router` (hash-based) |
| Crypto (primary) | Web Crypto API (Ed25519, AES-GCM, HKDF) |
| Crypto (fallback) | `@noble/curves` (Firefox/Safari) |
| Deployment | `install.sh`, `update.sh`, systemd |
| OS | Ubuntu 26 Server (headless) |

---

## Quick Deploy

### Prerequisites
- Ubuntu 26 server
- Tailscale installed and authenticated
- A GitHub repo containing the app code

### One-Command Install

```bash
# Clone the repo
git clone https://github.com/YOUR_USER/tailchat.git /tmp/tailchat
cd /tmp/tailchat

# Edit your tailnet domain
nano scripts/config.yaml  # change "example.ts.net" to your tailnet

# Run the installer
sudo bash scripts/install.sh
```

The script installs Go, Node.js, builds the app, creates a systemd service, and configures the firewall.

### Verify

```bash
curl http://localhost:3000/api/health
# {"status":"ok","version":"1.0.0","uptime":"7s"}
```

Then open `https://tailchat-server.your-tailnet.ts.net` from any device on your tailnet.

---

## Updating

```bash
# Git-based update
ssh user@tailchat-server "sudo /opt/tailchat/scripts/update.sh"

# Or SCP file drop
scp -r ./server ./web user@tailchat-server:/tmp/tailchat-new/
ssh user@tailchat-server "sudo /opt/tailchat/scripts/update.sh --from-dir /tmp/tailchat-new"
```

`update.sh` handles DB migrations, atomic binary swap (~2s downtime), and automatic health-check rollback.

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
│   ├── install.sh        # One-shot server provisioning
│   ├── update.sh         # Deploy updates
│   ├── config.yaml       # Server config template
│   └── tailchat.service  # systemd unit
├── server/
│   ├── main.go           # HTTP server, routing, middleware
│   ├── config.go         # YAML config loading
│   ├── db/               # Schema, queries, migrations
│   ├── handlers/         # REST + WebSocket handlers
│   ├── middleware/        # Rate limiting
│   └── storage/          # Encrypted blob storage
└── web/
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
- [x] **Tailscale-only access** — no public internet exposure; CORS restricted to `*.ts.net`
- [x] **Systemd hardening** — `NoNewPrivileges`, `ProtectSystem=strict`, `PrivateTmp`
- [x] **Body size limits** — 10MB max request body
- [x] **Atomic blob writes** — temp file + rename, no partial reads
- [x] **UFW firewall** — only tailscale0 interface allowed

### Deliberately Not Implemented (Phase 5+)

- Forward secrecy (X3DH double ratchet)
- Push notifications via Web Push API (requires public relay)
- Federation / multi-tailnet
- E2E encrypted voice/video calls

---

## FAQ

**Q: Why Tailscale-only?**  
A: Tailscale provides automatic WireGuard networking, Let's Encrypt TLS, and identity headers — no need to manage certificates, VPNs, or auth infrastructure. TailChat is accessible only to devices you've added to your tailnet.

**Q: Can the server admin read messages?**  
A: No. Messages are encrypted with AES-256-GCM client-side before transmission. The server stores only ciphertext. Even with full database and filesystem access, message content is unrecoverable without the recipient's private key.

**Q: What happens if I lose my device?**  
A: Use one of your 10 recovery codes with your handle to restore your keypair and all message history on a new device.

**Q: Can I self-host this?**  
A: Yes — that's the only deployment model. Run the `install.sh` script on any Ubuntu 26 server with Tailscale.

**Q: Does this work on mobile?**  
A: Yes, it's a PWA. Open the URL on your phone, tap "Add to Home Screen", and it installs as a standalone app with notifications.

## License

MIT
