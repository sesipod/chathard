# Plan: Tailscale-Only Encrypted Messenger (PWA)

## TL;DR
Build a zero-knowledge, passwordless messenger that runs exclusively over Tailscale. **Nothing on the server is ever in plaintext** — every byte stored (messages, files, metadata, keys) is client-side encrypted. Users authenticate via Ed25519 keypairs (stored in IndexedDB) and recover accounts using one-time recovery codes printed at registration. Accessible as a PWA on mobile via `https://*.ts.net`.

### Core principles
- **Zero plaintext on disk**: Server stores only ciphertext. If the server's database and filesystem are reviewed, an auditor sees only encrypted blobs with no way to determine message content, file contents, or conversation participants.
- **Handle is public-by-design** (users share it to connect), but the handle→UUID mapping is the only plaintext lookup table. Message routing uses opaque UUIDs — not handles.
- **User-chosen handles** with uniqueness enforcement.
- **Per-conversation retention policies** configurable by the user.

---

## Phase 0: Automated Deployment (Ubuntu 26 Server — Headless)

*This phase produces the scripts that provision a bare Ubuntu server into a running TailChat instance. Run `install.sh` once per server, then use `update.sh` for ongoing deploys.*

### Step 0a — `scripts/install.sh`: One-shot server provisioning

**What it does**: takes a fresh Ubuntu 26 headless server and installs everything needed to run TailChat. Idempotent — safe to re-run.

**Usage**: `curl -sSL https://raw.githubusercontent.com/you/tailchat/main/scripts/install.sh | sudo bash`

**Script steps** (in order):

1. **Pre-flight checks**
   - Must run as root (or sudo)
   - Must be Ubuntu 24.04+ (check `/etc/os-release`)
   - `curl` and `git` must be available (install via apt if missing)

2. **Install system dependencies**
   - `apt update && apt install -y build-essential sqlite3 curl git ufw`

3. **Install Go** (via official tarball, pinned version)
   - Download `go1.23.linux-amd64.tar.gz` from `go.dev/dl/`
   - Extract to `/usr/local/go`
   - Add to PATH via `/etc/profile.d/go.sh`
   - Verify: `go version`

4. **Install Node.js 22 LTS** (via NodeSource)
   - `curl -fsSL https://deb.nodesource.com/setup_22.x | bash -`
   - `apt install -y nodejs`
   - Verify: `node --version`, `npm --version`

5. **Install Tailscale**
   - `curl -fsSL https://tailscale.com/install.sh | sh`
   - `tailscale up --auth-key=$TS_AUTH_KEY --hostname=tailchat-server`
     - Falls back to interactive login URL if no auth key provided
     - Prints the tailnet IP/hostname for the admin to note
   - Enable Tailscale Serve: `tailscale serve --bg --https 443 localhost:3000`

6. **Configure firewall** (UFW)
   - `ufw allow in on tailscale0` — allow all traffic from tailnet
   - `ufw default deny incoming`
   - `ufw --force enable`

7. **Create system user**
   - `useradd --system --no-create-home --shell /usr/sbin/nologin tailchat`
   - This user runs the Go service — no login, no home dir

8. **Create directory structure**
   ```
   /opt/tailchat/
   ├── server/               # Go source + binary
   ├── web/                  # Frontend source
   ├── scripts/
   │   ├── install.sh        # This script (self)
   │   └── update.sh         # Update/deploy script
   ├── data/
   │   ├── blobs/            # Encrypted file storage (chmod 700)
   │   └── tailchat.db       # SQLite database (auto-created on first run)
   ├── config.yaml           # Server configuration
   └── .env                  # Environment variables (Tailscale auth key, etc.)
   ```

9. **Clone or copy application files**
   - If `$GIT_REPO` is set: `git clone $GIT_REPO /opt/tailchat/server-src`
   - Otherwise: print instructions for manual SCP of files
   - Sets proper ownership: `chown -R tailchat:tailchat /opt/tailchat`

10. **Initial build**
    - Frontend: `cd /opt/tailchat/web && npm ci && npm run build` → outputs to `web/dist/`
    - Backend: `cd /opt/tailchat/server && go build -o /opt/tailchat/tailchat-server .`
    - Verify binary exists and is runnable

11. **Create systemd service** (`/etc/systemd/system/tailchat.service`)
    ```ini
    [Unit]
    Description=TailChat Server
    After=network-online.target tailscaled.service
    Wants=network-online.target
    Requires=tailscaled.service

    [Service]
    Type=simple
    User=tailchat
    Group=tailchat
    WorkingDirectory=/opt/tailchat
    ExecStart=/opt/tailchat/tailchat-server \
      --db-path /opt/tailchat/data/tailchat.db \
      --blob-dir /opt/tailchat/data/blobs \
      --addr localhost:3000
    Restart=always
    RestartSec=5
    # Security hardening
    NoNewPrivileges=yes
    PrivateTmp=yes
    ProtectSystem=strict
    ProtectHome=yes
    ReadWritePaths=/opt/tailchat/data
    ReadOnlyPaths=/opt/tailchat/web/dist
    LimitNOFILE=65536

    [Install]
    WantedBy=multi-user.target
    ```

12. **Start service**
    - `systemctl daemon-reload`
    - `systemctl enable tailchat`
    - `systemctl start tailchat`
    - Health check: `curl -s http://localhost:3000/api/health` (add a `/api/health` endpoint returning `{"status":"ok"}` to Step 3)

13. **Print success summary**
    - Tailscale hostname: `tailchat-server.<tailnet>.ts.net`
    - Service status: `systemctl status tailchat`
    - Logs: `journalctl -u tailchat -f`

### Step 0b — `scripts/update.sh`: Deploy new versions

**What it does**: pulls latest code (or uses provided files), rebuilds, migrates, restarts. ~2 second downtime.

**Usage**: `sudo /opt/tailchat/scripts/update.sh [--from-dir /path/to/new-files]`

**Script steps**:

1. **Acquire new code**
   - If `--from-dir <path>` given: `rsync -a <path>/ /opt/tailchat/server-src/` (for SSH file drops)
   - Otherwise: `cd /opt/tailchat/server-src && git pull origin main`
   - Log the new commit hash: `git log -1 --oneline`

2. **Check for DB migrations**
   - Look for `.sql` files in `server/db/migrations/` newer than last applied
   - Apply in order with `sqlite3 /opt/tailchat/data/tailchat.db < migration.sql`
   - Track applied migrations in a `schema_migrations` table (created automatically)

3. **Build frontend**
   - `cd /opt/tailchat/web && npm ci && npm run build`
   - Verify `web/dist/index.html` exists after build

4. **Build backend**
   - `cd /opt/tailchat/server && go build -o /opt/tailchat/tailchat-server.new .`
   - Verify binary: `./tailchat-server.new --version` (add `--version` flag in Step 1)

5. **Atomic swap & restart**
   - Stop service: `systemctl stop tailchat`
   - Swap binary: `mv /opt/tailchat/tailchat-server.new /opt/tailchat/tailchat-server`
   - Start service: `systemctl start tailchat`
   - (Downtime is ~2 seconds — acceptable for a tailnet-only app)

6. **Health check**
   - `curl -s --retry 5 --retry-delay 2 http://localhost:3000/api/health`
   - If health check fails after retries: rollback to previous binary (`mv tailchat-server.prev tailchat-server`), print error, exit 1

7. **Log update**
   - Append to `/opt/tailchat/data/updates.log`: `[timestamp] <old-commit> → <new-commit> SUCCESS`

### Step 0c — `scripts/config.yaml`: Server configuration template

```yaml
# /opt/tailchat/config.yaml
server:
  addr: "localhost:3000"
  tailnet_domain: "example.ts.net"  # your tailnet name

storage:
  db_path: "/opt/tailchat/data/tailchat.db"
  blob_dir: "/opt/tailchat/data/blobs"
  max_blob_storage: "10GB"
  max_upload_size: "100MB"

cleanup:
  interval: "1h"  # how often to check for expired messages/blobs

logging:
  level: "info"   # debug, info, warn, error
  format: "json"  # json for journald
```

### Step 0d — Server-side additions to support deployment

Add these lightweight features to the server (referenced from Phase 1 steps):

- **`GET /api/health`** — returns `{"status":"ok","version":"1.0.0","uptime":"2h34m"}`. Used by update.sh health check and systemd `WatchdogSec`.
- **`--version` flag** — prints version string and exits. Used by update.sh to verify the compiled binary.
- **DB migrations directory**: `server/db/migrations/` with numbered `.sql` files (`001_init.sql`, `002_add_sessions.sql`, etc.). Applied idempotently via a `schema_migrations` tracking table.

### Update workflow (day-to-day operations)

| Method | Command |
|--------|---------|
| **Git-based update** | `ssh user@tailchat-server "sudo /opt/tailchat/scripts/update.sh"` |
| **SCP file drop** | `scp -r ./server ./web user@tailchat-server:/tmp/tailchat-new/ && ssh user@tailchat-server "sudo /opt/tailchat/scripts/update.sh --from-dir /tmp/tailchat-new"` |
| **Check logs** | `ssh user@tailchat-server "journalctl -u tailchat -f"` |
| **Check version** | `ssh user@tailchat-server "curl -s http://localhost:3000/api/health"` |
| **Restart service** | `ssh user@tailchat-server "sudo systemctl restart tailchat"` |

---

## Phase 1: Core Server (Go + SQLite + Filesystem)

*All steps in this phase can run in parallel, except Step 4 depends on Step 2.*

### Step 1 — Project scaffold & Tailscale-aware HTTP server
- Create `server/` with Go module (`go mod init tailchat-server`)
- Use `net/http` binding to `localhost:3000` (Tailscale Serve proxies inbound traffic)
- **Auth model (dual-layer)**:
  - **Outer layer (network)**: Tailscale Serve injects `Tailscale-User-Login` header on every request — this proves the request came from within the tailnet and identifies the tailnet user (e.g., `alice@mytailnet.ts.net`). All requests without this header are rejected at middleware level (404 or 403). This is the "you're on our tailnet" gate.
  - **Inner layer (app)**: Session tokens (Step 6, Step 18) provide app-level authentication. A tailnet user can have multiple browser sessions. The session token identifies *which device/browser* is making the request. The server maps `Tailscale-User-Login` → `user_id` for the first registration, then uses session tokens for subsequent requests.
  - **Registration**: the first time a tailnet user registers, the server links their `Tailscale-User-Login` to their `user_id`. One tailnet user = one TailChat account.
- Configure `tailscale serve --bg --https 443 localhost:3000` for automatic HTTPS certs via Let's Encrypt
- CORS middleware: only accept origins matching `*.ts.net` (tailnet domain)
- Production static serving: Go server serves `web/dist/` with SPA fallback (`/*` → `index.html`) when not running Vite dev server (detected via `--dev` flag or env var)
- Security headers middleware: `X-Content-Type-Options: nosniff`, `X-Frame-Options: DENY`, `Referrer-Policy: no-referrer`

### Step 2 — Database schema (SQLite via `modernc.org/sqlite`)
Tables:
- **`users`** — `id TEXT PK (UUID v4)`, `handle TEXT UNIQUE`, `public_key_ed25519 BLOB`, `public_key_x25519 BLOB`, `derived_public_key_ed25519 BLOB`, `created_at TEXT`
  - `public_key_ed25519`: master identity key (never used directly for auth — key blinding via Step 18)
  - `public_key_x25519`: pre-computed X25519 equivalent of the Ed25519 key, stored so recipients can encrypt without deriving per-message
  - `derived_public_key_ed25519`: HKDF-derived auth key (Step 18) — this is what the server verifies challenge signatures against
  - No display_name stored — display names are managed client-side only
  - Handle is the only human-readable identifier on the server
- **`sessions`** — `token_hash TEXT PK (SHA-256)`, `user_id TEXT FK`, `created_at TEXT`, `expires_at TEXT`
  - Server stores only the hash of the session token — if the DB is dumped, raw tokens are unrecoverable
- **`encrypted_key_backups`** — `user_id TEXT FK`, `recovery_code_hash TEXT`, `encrypted_private_key BLOB`, `salt BLOB`, `used INTEGER DEFAULT 0`
  - `recovery_code_hash` is a SHA-256 hash of the code — server verifies without knowing the code value
- **`messages`** — `id TEXT PK (UUID v4)`, `sender_id TEXT FK`, `recipient_id TEXT FK` (NULL for group), `group_id TEXT FK` (NULL for 1:1), `ciphertext BLOB`, `ephemeral_public_key BLOB`, `nonce BLOB`, `created_at TEXT`, `expires_at TEXT`, `read_at TEXT`
  - `expires_at` enables per-conversation retention (auto-delete)
  - `read_at` tracks when the recipient first read the message (powers read receipts / double-check)
  - Sender/recipient are UUIDs — an auditor sees "UUID→UUID" flows, not handles
- **`groups`** — `id TEXT PK`, `encrypted_name BLOB`, `encrypted_symmetric_key BLOB`, `created_at TEXT`
  - Group name is encrypted — server never sees it
- **`group_members`** — `group_id TEXT FK`, `user_id TEXT FK`, `encrypted_group_key BLOB`, `encrypted_member_metadata BLOB`, `PRIMARY KEY(group_id, user_id)`
- **`files`** — `id TEXT PK (UUID v4)`, `uploader_id TEXT FK`, `encrypted_blob_path TEXT`, `encrypted_metadata BLOB`, `size_bytes INTEGER`, `created_at TEXT`, `expires_at TEXT`
  - `encrypted_blob_path` is a server-controlled path (no user data in filename)
  - `encrypted_metadata` contains original filename, MIME type, etc. — encrypted client-side

### Step 3 — REST API endpoints
- **`POST /api/register`** — accepts `{handle, public_key_ed25519, public_key_x25519, derived_public_key_ed25519, encrypted_key_backups: [{recovery_code_hash, encrypted_private_key, salt}]}`. Client pre-computes X25519 key and derived auth key before registration. Handle uniqueness enforced server-side (UNIQUE index + HTTP 409 on conflict).
- **`POST /api/auth/challenge`** — server returns random challenge (32 random bytes, 5-min expiry); client signs with derived auth key (not master Ed25519 key); `POST /api/auth/verify` verifies signature against `derived_public_key_ed25519`. Returns session token (256-bit random).
- **`POST /api/auth/logout`** — deletes session from `sessions` table. Subsequent requests with that token return 401.
- **`POST /api/messages`** — stores ciphertext for recipient. Accepts optional `expires_in`. Returns message ID. Server cannot inspect the message.
- **`GET /api/messages?with=<user_id>&group_id=<id>&after=<ts>&before=<ts>&limit=50`** — fetches messages for authenticated user, filterable by 1:1 conversation partner or group. Auto-filters expired messages. Default limit 50, max 200.
- **`POST /api/messages/read`** — marks messages as read for a conversation. Body: `{conversation_with: UUID}` or `{group_id: UUID}`. Sets `read_at` on all unread messages from that sender/group up to the latest.
- **`PATCH /api/messages/retention`** — batch update expiry for a conversation. Body: `{conversation_with: UUID, expires_in: "30d"}` or `{group_id: UUID, expires_in: "7d"}`.
- **`GET /api/conversations`** — returns all conversations (1:1 + groups) with: partner handle (for 1:1), group name (decrypted by server? no — client must decrypt `encrypted_name`), last message preview (truncated ciphertext length + timestamp), unread count. For 1:1, includes the other user's UUID and handle. Powers the chat list.
- **`GET /api/users/search?handle=<query>`** — look up users by handle prefix. Returns `{id, handle, public_key_x25519, key_fingerprint}`. Includes X25519 key so the searcher can immediately encrypt. Rate-limited: 30/min.
- **`POST /api/recover`** — accepts `{user_id, recovery_code_hash}`. Returns `{encrypted_private_key, salt}`. Marks code as used.
- **`GET /api/me`** — returns authenticated user's own profile: `{id, handle, public_key_fingerprint, recovery_codes_remaining}`.
- **Group endpoints**: `POST /api/groups`, `POST /api/groups/:id/members`, `DELETE /api/groups/:id/members/:user_id` (self-removal = leave group), `GET /api/groups/:id/messages`, `GET /api/groups`
- **File endpoints**: `POST /api/files/upload` (chunked upload for >10MB, max 100MB), `GET /api/files/:id`, `DELETE /api/files/:id`
- **Rate limiting middleware** (built here, not deferred): register 3/hr per tailnet user, messages 60/min, user search 30/min, recovery 3 failed → 1hr lock. Challenge expires in 5 min.

### Step 4 — WebSocket hub for real-time delivery
- `GET /ws` — authenticated WebSocket upgrade
- In-memory map of `user_id → []*websocket.Conn`
- On new message: push notification over WS: `{"type": "new_message", "msg_id": "...", "sender_id": "...", "group_id": "..."|null}` — includes conversation context so client updates correct chat
- Typing indicators: `{"type": "typing", "user_id": "...", "group_id": "..."|null, "is_typing": true|false}` — encrypted boolean, no content leaked
- Read receipt relay: `{"type": "read_receipt", "from_user_id": "...", "conversation_with": "..."|null, "group_id": "..."|null, "up_to_msg_id": "..."}`
- Fallback: client polls `GET /api/messages` every 5s when WS disconnected

### Step 4b — Encrypted blob storage (Linux filesystem)
- Configurable directory: `--blob-dir /var/tailchat/blobs` (default `./data/blobs`)
- Files stored as: `<blob-dir>/<first-2-hex-of-uuid>/<full-uuid>.enc` — no user data in path
- `chmod 0600` on each blob file
- Periodic cleanup goroutine for expired blobs (disk + DB)
- Configurable max storage: `--max-blob-storage 10GB`
- Chunked upload writes to temp file, renames atomically on completion

---

## Phase 2: Client-Side Crypto Layer (Browser Web Crypto API)

*All steps depend on Phase 1 being stable for integration testing.*

### Step 5 — Key generation & storage
- Generate **Ed25519 keypair**. Primary: Web Crypto API `crypto.subtle.generateKey({name: "Ed25519"})` (supported in Chrome 113+). **Feature-detect at startup**: if `crypto.subtle.generateKey` rejects or Ed25519 is absent, fall back to `@noble/curves` (`ed25519.utils.randomPrivateKey()` + `ed25519.getPublicKey()`). Wrap both behind a unified `generateKeyPair()` function so the rest of the codebase is agnostic.
- **Pre-compute derived keys** (done once at key generation, stored in IndexedDB alongside the master key):
  - **X25519 keypair**: convert the Ed25519 keypair to X25519 (RFC 7748, via `@noble/curves` `ed25519ToX25519()`). This is the key used for ECDH in message encryption (Step 7) and group key encryption (Step 8). The X25519 public key is uploaded to the server at registration.
  - **Derived auth keypair**: derive a separate Ed25519 keypair via HKDF (SHA-256, info: `"tailchat-auth"`, salt: 32 random bytes stored alongside). Only this derived key signs auth challenges — the master key never leaves IndexedDB (key blinding, Step 18). The derived public key is uploaded to the server at registration.
- Generate 10 **recovery codes** (16 random bytes each, 4×4 base32 groups: `X3KM-7FJ2-PQ9N-RT8B`). Char set: `ABCDEFGHJKLMNPQRSTUVWXYZ23456789` (no 0/O, 1/I/L).
- For each code: PBKDF2 (100K iterations, SHA-256) → AES-GCM key → encrypt **master Ed25519 private key** (not derived keys — those can be re-derived after recovery).
- Store master private key + derived keys in **IndexedDB** — never leaves the device.
- Show recovery codes once with print/download (PDF).

### Step 6 — Authentication flow
- **Have key**: load from IndexedDB → sign server challenge with **derived auth key** (not master key — key blinding per Step 18) → `POST /api/auth/verify` → server verifies against `derived_public_key_ed25519` → returns session token (256-bit random) → client stores token in sessionStorage only
- **New user**: generate keypair + derived keys (Step 5) → choose handle (real-time availability via `GET /api/users/search?handle=`) → register via `POST /api/register` with `{handle, public_key_ed25519, public_key_x25519, derived_public_key_ed25519, encrypted_key_backups: [{recovery_code_hash, encrypted_private_key, salt}]}`. Tailscale identity header links the tailnet user to this account.
- **Lost key**: enter handle + recovery code → client hashes code (SHA-256) → `POST /api/recover` with `{user_id, recovery_code_hash}` → server returns `{encrypted_private_key, salt}` → client PBKDF2-derives key from recovery code → decrypts master private key → re-derives X25519 + auth keys → restores to IndexedDB
- Session token in sessionStorage only (not localStorage — XSS protection). Expires after 7 days.
- Session validation: every authenticated request checks `sessions` table for `SHA-256(token)` → if missing or expired → 401 → client redirects to login.

### Step 7 — 1:1 message encryption/decryption
- X25519 key agreement uses the **pre-computed** X25519 keypair (generated once at key creation, Step 5). No per-message derivation from Ed25519.
- **Send**: generate ephemeral X25519 keypair → compute shared secret with recipient's X25519 public key (fetched from `GET /api/users/search` or cached in IndexedDB contacts) → HKDF (SHA-256) → AES-256-GCM key → encrypt plaintext → send to `POST /api/messages`: `{recipient_id, ciphertext, ephemeral_public_key, nonce}`
- **Receive**: own X25519 private key + sender's ephemeral pubkey → same shared secret → decrypt
- Unique 12-byte nonce per message (via `crypto.getRandomValues`)

### Step 8 — Group chat encryption
- **Group creation**: creator generates a random 256-bit AES-GCM group key. Creator fetches each member's X25519 public key from `GET /api/users/search` (members are looked up by handle). Group key is encrypted per-member using ECDH: creator's X25519 private key + member's X25519 public key → shared secret → HKDF → AES-GCM → `encrypted_group_key`. Group name encrypted with the group key → `encrypted_name`. POST to `/api/groups` with the encrypted name + per-member encrypted keys.
- **Sending messages**: encrypt with the group key (symmetric AES-256-GCM). Unique nonce per message.
- **Member add**: the adding member generates a **new** group key, re-encrypts it for all remaining + new members, and sends the new encrypted keys + a "key rotated" system message. Old messages encrypted with the old key remain decryptable (clients keep key history).
- **Member remove / leave**: the removing member (or the leaver) generates a new group key, re-encrypts it for all remaining members except the removed user, and posts the new encrypted keys. The removed user can no longer decrypt new messages.
- **Key history**: clients maintain a list of `{group_key, valid_from_msg_id}` so they can decrypt older messages after key rotation.

### Step 8b — File encryption
- Random AES-GCM key per file
- Encrypt in 1MB chunks
- Encrypt metadata (filename, MIME type, size) → `encrypted_metadata`
- Upload encrypted blob → send decryption key + file ID inside an encrypted message
- Streaming decryption on download (no full file in RAM)

---

## Phase 3: Web App Frontend (Svelte + Vite + Tailwind CSS — Signal-Style UI)

*Depends on Phase 2 crypto layer.*

### Step 9 — Frontend project scaffold
- `npm create vite@latest web -- --template svelte` → SPA mode (not SvelteKit)
- Install dependencies: `tailwindcss @tailwindcss/vite svelte-spa-router`
- `svelte-spa-router` for hash-based client-side routing (lightweight, no SvelteKit needed). Routes: `/login` → `Login.svelte`, `/chat` → `Main.svelte` (guarded), `/chat/:id` → `Main.svelte` with conversation selected.
- Install Tailwind CSS v4 + `@tailwindcss/vite` plugin
- Tailwind config with Signal-inspired design tokens:
  - Background: `slate-950` (dark mode default), `slate-50` (light)
  - Chat bubbles: `blue-600` (sent), `slate-700` (received dark) / `slate-200` (received light)
  - Accent: `blue-500` for buttons, links, active states
  - Font: system-ui stack (matches Signal's native feel)
- Vite dev server proxies `/api` and `/ws` to `localhost:3000` (Go server) via `vite.config.js` `server.proxy`
- Directory structure:
  ```
  web/
  ├── src/
  │   ├── lib/
  │   │   ├── api.js          # REST + WebSocket client
  │   │   ├── db.js           # IndexedDB wrapper (contacts, keys, drafts)
  │   │   ├── crypto/
  │   │   │   ├── keygen.js
  │   │   │   ├── encrypt.js
  │   │   │   ├── file-encrypt.js
  │   │   │   └── recover.js
  │   │   └── stores/
  │   │       ├── auth.js     # Auth state (Svelte writable store)
  │   │       ├── chats.js    # Conversation list + messages
  │   │       └── settings.js # Theme, retention prefs
  │   ├── components/
  │   │   ├── App.svelte          # Root: two-panel layout shell
  │   │   ├── LeftPanel.svelte    # Chat list panel
  │   │   ├── ChatList.svelte     # Scrollable conversation list
  │   │   ├── ChatListItem.svelte # Single conversation row
  │   │   ├── RightPanel.svelte   # Conversation / empty state
  │   │   ├── Conversation.svelte # Active conversation view
  │   │   ├── MessageBubble.svelte
  │   │   ├── MessageInput.svelte
  │   │   ├── NewChatModal.svelte
  │   │   ├── NewGroupModal.svelte
  │   │   ├── SettingsPage.svelte
  │   │   ├── EmptyState.svelte   # "Select a chat or start a new one"
  │   │   └── common/
  │   │       ├── Avatar.svelte
  │   │       ├── Button.svelte
  │   │       ├── Modal.svelte
  │   │       └── SearchInput.svelte
  │   ├── views/
  │   │   ├── Login.svelte      # Register / Login / Recover
  │   │   └── Main.svelte       # Two-panel chat app (guarded)
  │   ├── app.css               # Tailwind imports + global styles
  │   └── main.js               # Svelte mount point + router init
  ├── public/
  │   ├── manifest.json
  │   └── icons/                # PWA icons
  └── vite.config.js
  ```

### Step 10 — Signal-style two-panel layout shell (`App.svelte`)
- Desktop (≥768px): Two columns side-by-side
  - Left panel: fixed 380px width, full height, border-right
  - Right panel: flex-1, fills remaining space
- Mobile (<768px): Single panel at a time, slide transitions
  - Default: chat list fills screen
  - Tapping a chat → slides to conversation view (back arrow in header)
- **Only one nav item: "Chats"** — no tabs, no bottom nav bar needed
- Header bar in left panel: app name "Chats" (bold) + gear icon (→ Settings)
- Settings icon opens Settings as a full-screen overlay (or replaces right panel)
- Dark mode by default (matches Signal), light mode toggle in Settings

### Step 11 — Left panel: Chat list (`LeftPanel.svelte` + `ChatList.svelte`)
- **Header**: App title "Chats" (bold), gear icon on the right → opens Settings
- **FAB / header button for new chat**: A "+" icon button (top-right or floating)
  - Tapping it shows a small menu or directly opens:
    - "New Chat" → opens `NewChatModal`
    - "New Group" → opens `NewGroupModal`
- **Search bar** (below header): filter conversations by handle or group name (client-side), debounced
- **Conversation list** (scrollable):
  - Each `ChatListItem` shows:
    - Circular avatar placeholder (generated color from handle hash, like Signal)
    - Handle (for 1:1) or encrypted-group-name-decrypted-locally (for groups)
    - Last message preview (truncated, "You: " prefix for own messages)
    - Relative timestamp ("2m ago", "Yesterday")
    - Unread badge (count, blue dot)
  - Sorted by most recent message (descending)
  - Active conversation highlighted with accent background
- **Empty state**: "No conversations yet. Tap + to start a new chat."

### Step 12 — New Chat & New Group flows

#### NewChatModal.svelte
- Opens as a modal/dialog overlay
- **Search input**: type handle (min 3 chars), debounced API call to `GET /api/users/search?handle=`
- Results list: handle + key fingerprint (truncated)
- Tapping a result → creates conversation locally (no server-side conversation object — 1:1 chats are implicit, defined by message sender_id/recipient_id pairs) → closes modal → navigates to the new conversation in right panel. The first message sent creates the conversation's server-side presence.
- "Cancel" button or tap outside to dismiss

#### NewGroupModal.svelte
- Opens as modal/dialog overlay
- **Step 1**: Enter group name (this will be encrypted before sending to server)
- **Step 2**: Search & add members (same search as NewChat, multi-select). Each search result includes `public_key_x25519` — the creator uses these to encrypt the group key per-member (Step 8).
  - Selected members shown as chips/tags (removable)
  - Min 1 other member required
- **Step 3**: Review → "Create Group" button
- On create: generate group key → encrypt per-member using their X25519 keys → POST /api/groups with encrypted name + per-member encrypted keys → navigate to new group conversation

### Step 13 — Right panel: Conversation view (`RightPanel.svelte` + `Conversation.svelte`)

#### Empty state (no chat selected)
- Centered Signal-style illustration or icon
- Text: "Select a conversation or start a new one"
- "New Chat" button (same as FAB action)

#### Active conversation
- **Header bar** (top of right panel):
  - Back arrow (mobile only)
  - Avatar + handle/group name
  - Retention indicator: small badge showing "Auto-delete: 7d" (if set)
  - Optional: 3-dot menu → "Files", "Retention settings", "Leave group"
- **Message list** (scrollable, fills remaining space):
  - Messages grouped by sender, with date separators ("Today", "Yesterday", "Jun 9")
  - Sent messages: right-aligned, blue bubble
  - Received messages: left-aligned, gray bubble (with sender handle prefix in groups)
  - Status indicators on sent messages: single check (sent), double check (delivered), filled double check (read)
  - Images/videos: blurred thumbnail placeholder → decrypt & render on tap
  - File attachments: file icon + encrypted filename (decrypted) + size
  - System messages: "Alice created the group", "Bob joined" — centered, muted
- **Message input bar** (bottom, sticky):
  - Attachment button (paperclip icon) → file picker
  - Text input (expandable textarea, max 4 lines then scroll)
  - Send button (arrow icon, disabled when empty)
  - Typing indicator: "Alice is typing..." in header or above input
- **Auto-scroll to bottom** on new messages, with "scroll to bottom" FAB if scrolled up

### Step 14 — Settings page (`SettingsPage.svelte`)
- Opens as full-screen overlay (or replaces right panel on desktop)
- Back arrow + "Settings" header
- Sections:

1. **Account**
   - "Your handle": `@username` (read-only, displayed prominently)
   - "Your UUID": truncated, copyable (for debugging)
   - "Public key fingerprint": truncated, copyable

2. **Security**
   - "Recovery codes": shows "X of 10 remaining"
     - Tapping opens recovery code management (view unused codes, generate new set warning)
   - "Export private key": button → saves encrypted key file (with warning dialog)
   - "Session": "Log out" button (red/destructive, with confirmation)

3. **Appearance**
   - Theme toggle: Dark / Light / System (Svelte store persisted to localStorage)

4. **Conversation Defaults**
   - "Default auto-delete": dropdown (Never / 1h / 24h / 7d / 30d / 90d) — applies to new conversations

5. **Per-Conversation Retention** (sub-page)
   - List of all conversations with current retention setting
   - Each row: avatar + handle/group name + current retention + change button
   - Tapping change → dropdown with options → PATCH /api/messages/retention

6. **About**
   - App version, Tailnet name, server info

### Step 15 — Real-time messaging UI
- WebSocket connection on auth, reconnect with exponential backoff
- Optimistic UI: message appears immediately in chat, status updates as server confirms
- Typing indicators: debounced WS event, encrypted boolean only (no content leaked)
- Read receipts: mark messages as read when conversation is visible
- Inline image/video preview: decrypt thumbnail on-demand, full-size on tap
- File download progress bar

### Step 16 — PWA manifest & Service Worker
- `manifest.json` with `display: "standalone"`, `theme_color: "#020617"` (slate-950)
- Service Worker: cache app shell (HTML, JS, CSS), Background Sync for failed sends
- Notifications: "New message from @handle" — zero content preview, click → focus PWA

### Step 17 — Mobile-specific UX
- 48px minimum tap targets
- `viewport` meta, `theme-color`, install prompt
- Pull-to-refresh on chat list
- Swipe right on conversation → back to chat list
- Safe area insets for notched phones

---

## Phase 4: Security Hardening

### Step 18 — Cryptographic hardening
- **Key blinding** (implemented in Step 5, enforced here): derived auth keypair via HKDF (SHA-256, info: `"tailchat-auth"`, random salt). Server stores `derived_public_key_ed25519` and verifies challenge signatures against it. Raw master Ed25519 key never used for auth — if the derived key is compromised, the master key (and thus message history) remains secure.
- **Session tokens**: 256-bit random (`crypto.getRandomValues`), stored in `sessions` table as `SHA-256(token)`. Server can verify tokens but cannot recover raw tokens from a DB dump. Tokens expire after 7 days (enforced server-side via `expires_at`). Logout deletes the row.
- **Constant-time comparison** for recovery code hash verification: use `crypto.subtle.timingSafeEqual` or a manual constant-time loop to prevent timing attacks on `recovery_code_hash`.
- **Unique random AES-GCM key per file** (already in Step 8b, reinforced here).
- **Rate limiting** is implemented in Phase 1 Step 3 alongside the handlers — not deferred.

---

## Verification

1. **Tailscale isolation**: `curl localhost:3000` works, `curl <public-ip>:3000` times out
2. **Zero-knowledge audit**: Query all SQLite tables + blob files — everything is ciphertext gibberish
3. **Registration flow**: generate keys → choose handle → register → recovery codes shown → logout → login → works
4. **Recovery flow**: clear IndexedDB → handle + recovery code → keys restored → read old messages
5. **1:1 messaging**: Alice → Bob → decrypt correctly → server DB has only ciphertext
6. **Group chat**: create → add members → send → all decrypt → non-member cannot → encrypted group name on server
7. **File sharing**: upload → encrypted blob on disk → recipient downloads + decrypts → original content verified
8. **Retention**: set 1h → send → wait → messages deleted from server + disk
9. **UI layout**: Desktop shows two-panel (380px left + flex right), mobile shows single-panel with back nav
10. **New chat flow**: "+" button → search handle → start conversation → appears in chat list
11. **New group flow**: "+" → "New Group" → name + add members → group appears in chat list
12. **Settings**: view handle via `GET /api/me`, recovery codes remaining, toggle theme, change retention
13. **Chat list**: `GET /api/conversations` returns 1:1 + groups with last message preview and unread count
14. **Read receipts**: `POST /api/messages/read` → read status relayed via WS → double-check turns filled
15. **Real-time**: WS push includes conversation context → correct chat updates without full poll
16. **Typing indicators**: WS `{"type": "typing", ...}` → shows "Alice is typing..." in correct conversation
17. **Logout**: `POST /api/auth/logout` → session invalidated → redirected to login
18. **Leave group**: `DELETE /api/groups/:id/members/@me` → removed from group → group disappears from chat list
19. **Ed25519 fallback**: Test in Firefox/Safari → `@noble/curves` fallback generates keys successfully
20. **Key blinding**: Auth uses derived key → raw Ed25519 key never leaves IndexedDB → server verifies derived public key
21. **PWA**: Lighthouse audit → install on Android/iOS → notification shows sender handle only → click focuses PWA
22. **Cross-browser**: Chrome + Firefox + Safari
23. **Production build**: `vite build` → Go server serves `web/dist/` → SPA fallback works on refresh
24. **Health endpoint**: `GET /api/health` returns `{"status":"ok","version":"..."}` → systemd watchdog + update.sh use it
25. **Install script**: `install.sh` on fresh Ubuntu 26 → all deps installed → service running → accessible at `tailchat-server.<tailnet>.ts.net`
26. **Update script**: `update.sh` → git pull → build → migrate → restart → health check passes → `updates.log` records success
27. **DB migrations**: numbered `.sql` files applied idempotently → `schema_migrations` table tracks applied migrations
28. **Systemd resilience**: `kill -9` the process → systemd restarts within 5s → health check recovers

---

## Files to create

| File | Purpose |
|------|---------|
| `scripts/install.sh` | One-shot Ubuntu 26 provisioning: deps, Tailscale, build, systemd |
| `scripts/update.sh` | Deploy updates: git pull or file drop → build → migrate → restart |
| `scripts/config.yaml` | Server configuration template |
| `scripts/tailchat.service` | systemd unit file (installed to /etc/systemd/system/ by install.sh) |
| `server/main.go` | Go HTTP server, Tailscale-aware routing |
| `server/config.go` | CLI flags (blob-dir, max-upload-size, max-blob-storage, db-path) |
| `server/db/schema.sql` | SQLite schema |
| `server/db/migrations/` | Numbered SQL migration files (001_init.sql, etc.) + schema_migrations tracking table |
| `server/db/queries.go` | Prepared SQL queries |
| `server/handlers/register.go` | Registration + handle uniqueness |
| `server/handlers/auth.go` | Challenge-response auth, session management, logout |
| `server/handlers/messages.go` | Message store/retrieve/read-receipts/retention + conversation list |
| `server/handlers/ws.go` | WebSocket hub + notification dispatch |
| `server/handlers/recover.go` | Recovery code verification |
| `server/handlers/files.go` | Encrypted blob upload/download/delete |
| `server/handlers/groups.go` | Group CRUD + member management (leave group via self-DELETE) |
| `server/middleware/ratelimit.go` | Rate limiting middleware (register 3/hr, messages 60/min, search 30/min) |
| `server/storage/blobs.go` | Filesystem blob read/write/cleanup |
| `server/storage/cleanup.go` | Periodic expired-message + expired-blob cleanup |
| `web/src/lib/crypto/keygen.js` | Ed25519 keypair gen, recovery code gen |
| `web/src/lib/crypto/encrypt.js` | X25519 + AES-GCM message encryption |
| `web/src/lib/crypto/file-encrypt.js` | Chunked file encrypt/decrypt |
| `web/src/lib/crypto/recover.js` | PBKDF2 key derivation from recovery codes |
| `web/src/lib/api.js` | REST + WebSocket client |
| `web/src/lib/db.js` | IndexedDB wrapper (contacts, keys, drafts) |
| `web/src/lib/stores/auth.js` | Auth state (Svelte writable store) |
| `web/src/lib/stores/chats.js` | Conversation list + messages store |
| `web/src/lib/stores/settings.js` | Theme, retention prefs store |
| `web/src/components/App.svelte` | **Root: two-panel Signal-style layout** |
| `web/src/components/LeftPanel.svelte` | **Chat list panel with "Chats" header + "+" button** |
| `web/src/components/ChatList.svelte` | **Scrollable conversation list** |
| `web/src/components/ChatListItem.svelte` | **Single conversation row (avatar, handle, preview, time, unread)** |
| `web/src/components/RightPanel.svelte` | **Conversation or empty state container** |
| `web/src/components/Conversation.svelte` | **Active chat view (messages + input + header)** |
| `web/src/components/MessageBubble.svelte` | **Message bubble (sent/received styling, status indicators)** |
| `web/src/components/MessageInput.svelte` | **Input bar (attach, text, send, typing indicator)** |
| `web/src/components/NewChatModal.svelte` | **Search users by handle → start 1:1 chat** |
| `web/src/components/NewGroupModal.svelte` | **Create group: name + search members + create** |
| `web/src/components/SettingsPage.svelte` | **Account, security, appearance, retention settings** |
| `web/src/components/EmptyState.svelte` | **"Select a chat or start a new one" placeholder** |
| `web/src/components/common/Avatar.svelte` | **Generated color avatar from handle hash** |
| `web/src/components/common/Button.svelte` | **Reusable button component** |
| `web/src/components/common/Modal.svelte` | **Reusable modal/dialog wrapper** |
| `web/src/components/common/SearchInput.svelte` | **Debounced search input** |
| `web/src/views/Login.svelte` | Registration / Login / Recovery flow |
| `web/src/views/Main.svelte` | Authenticated main app guard |
| `web/src/app.css` | Tailwind imports + Signal-style design tokens |
| `web/src/main.js` | Svelte mount point + router init |
| `web/public/manifest.json` | PWA manifest (`theme_color: "#020617"`) |
| `web/public/service-worker.js` | PWA service worker (sender-handle-only notifications) |
| `web/vite.config.js` | Vite config with Tailwind + SPA fallback + dev proxy to Go |
| `web/package.json` | Dependencies: svelte, vite, tailwindcss, @tailwindcss/vite, svelte-spa-router, @noble/curves |

**Bold items** are new or significantly expanded from the original plan.

---

## Decisions

| Area | Decision |
|------|----------|
| **Server language** | Go — stronger crypto ecosystem, Tailscale SDK, single binary |
| **Client crypto** | Web Crypto API (native, no deps). Fallback: `@noble/curves` for older browsers |
| **Network topology** | Single shared tailnet — all devices + server are nodes |
| **Handles** | User-chosen, unique, real-time availability check |
| **Recovery codes** | 10 codes at registration, 4×4 base32 (`X3KM-7FJ2-PQ9N-RT8B`), no ambiguous chars |
| **Retention** | Per-conversation configurable (1h to Never) |
| **Zero plaintext** | Only handles + UUIDs + public keys + encrypted blobs on server. Group names and filenames encrypted. |
| **Notifications** | "You have a new message from @handle" — zero content preview |
| **UI framework** | Svelte (SPA mode via Vite, not SvelteKit) |
| **CSS** | Tailwind CSS v4 with Signal-inspired design tokens (slate-950 dark, blue-600 sent, system-ui font) |
| **Layout** | Two-panel on desktop (380px left + flex right), single-panel on mobile with slide transitions |
| **Navigation** | "Chats" only — no tabs, no bottom nav. Settings via gear icon ⚙️ |
| **New chat/group** | "+" button → modal with user search. Group: multi-step modal (name → members → create) |
| **Settings** | Handle display, recovery codes remaining, export keys, dark/light/system theme, per-conversation retention, logout |
| **Theme** | Dark mode default (Signal-like), light mode toggle, system-follow option |

## Scope boundaries

### Included
- 1:1 E2E encrypted messaging with configurable retention
- Group E2E encrypted messaging with encrypted group names
- E2E encrypted file/attachment sharing (any type, up to 100MB)
- Passwordless Ed25519 keypair auth
- Recovery via 10 one-time codes
- Tailscale-only access (Tailscale Serve + HTTPS)
- PWA for mobile (installable, offline-capable, sender-handle-only notifications)
- WebSocket real-time delivery
- Linux filesystem blob storage with auto-cleanup
- Full zero-knowledge: server stores zero plaintext content
- **Signal-style two-panel UI** (Svelte + Tailwind CSS): chat list left, conversation right, single-panel mobile
- **"Chats" as sole navigation** with "+" button for new chat / new group
- **Settings page**: handle display, recovery codes, theme toggle, per-conversation retention, key export, logout
- **Automated deployment**: `install.sh` provisions Ubuntu 26 from scratch; `update.sh` handles git-pull or SCP-file-drop deploys with atomic binary swap, DB migrations, and health checks

### Deliberately excluded (Phase 5+)
- Forward secrecy (X3DH double ratchet)
- Push notifications via Web Push API (needs public relay)
- Federation / multi-tailnet
- Screen recording/screenshot protection
- E2E encrypted video/voice calls

---

## Architecture Notes

### Two-Panel CSS Structure (Signal-like)
```
┌──────────────────────┬─────────────────────────────┐
│ LeftPanel (w-95)     │ RightPanel (flex-1)          │
│ ┌──────────────────┐ │ ┌─────────────────────────┐ │
│ │ Chats       [⚙️] │ │ │ Avatar  @alice  [7d]   │ │ ← Header
│ │ [+ New]         │ │ ├─────────────────────────┤ │
│ ├──────────────────┤ │ │                         │ │
│ │ [Search...]      │ │ │  ┌───────────────────┐  │ │ ← Received
│ ├──────────────────┤ │ │  │ Hello!            │  │ │
│ │ ● @alice   2m   │ │ │  └───────────────────┘  │ │
│ │   Hey there!    │ │ │        ┌───────────────┐ │ │ ← Sent ✓✓
│ │                  │ │ │        │ Hi Alice!    │ │ │
│ │   @bob    1h    │ │ │        └───────────────┘ │ │
│ │   See you soon  │ │ │                         │ │
│ │                  │ │ ├─────────────────────────┤ │
│ │ # Team    Yest  │ │ │ [📎] [___________] [➤] │ │ ← Input
│ └──────────────────┘ │ └─────────────────────────┘ │
└──────────────────────┴─────────────────────────────┘
```

### Component State Machine (simplified)
```
App.svelte
├── NOT AUTHENTICATED → Login.svelte
│   ├── Register (handle + keys + recovery codes)
│   ├── Login (challenge-response)
│   └── Recover (handle + recovery code)
└── AUTHENTICATED → Main.svelte
    ├── LeftPanel
    │   ├── ChatList (default)
    │   └── SettingsPage (when gear tapped)
    └── RightPanel
        ├── EmptyState (no conversation selected)
        └── Conversation (active chat)
            ├── MessageBubble[] (scrollable)
            └── MessageInput (sticky bottom)
```

### Tailwind Design Tokens Reference
| Token | Dark Mode | Light Mode | Usage |
|-------|-----------|------------|-------|
| Page background | `slate-950` (#020617) | `slate-50` (#f8fafc) | `App.svelte` root |
| Left panel bg | `slate-900` (#0f172a) | `white` | `LeftPanel.svelte` |
| Right panel bg | `slate-950` | `slate-50` | `RightPanel.svelte` |
| Sent bubble | `blue-600` (#2563eb) | `blue-500` (#3b82f6) | `MessageBubble` (own) |
| Received bubble | `slate-700` (#334155) | `slate-200` (#e2e8f0) | `MessageBubble` (other) |
| Input bar bg | `slate-900` | `white` | `MessageInput` |
| Accent color | `blue-500` (#3b82f6) | `blue-600` (#2563eb) | Buttons, links |
| Text primary | `white` | `slate-900` | All text |
| Text muted | `slate-400` (#94a3b8) | `slate-500` (#64748b) | Timestamps, hints |
| Border | `slate-800` (#1e293b) | `slate-200` (#e2e8f0) | Panel dividers |
| Danger/destructive | `red-600` (#dc2626) | `red-600` | Logout button |
| Unread badge | `blue-500` | `blue-600` | Unread dot/count |