## Plan: Zero-Knowledge Identity — Remove Handles, Add Keyfiles & Invite Codes

**TL;DR**: Replace the plaintext `handle` (username) on the server with a keyfile-based identity system. Users download a keyfile at registration and prove ownership via challenge-response. Adding contacts moves from handle-search to one-time invite codes. Each user assigns their own custom name to every contact — stored encrypted client-side, editable anytime on a dedicated **Friends** page, never seen by the server.

---

## Phase 1: Keyfile-Based Registration & Authentication

### Step 1.1: Database migration — remove `handle` from `users` table

Create `server/db/migrations/003_remove_handle.sql`:
- Drop the UNIQUE index on `handle`
- Drop the `handle` column from `users`
- (SQLite doesn't support `DROP COLUMN` directly — use recreate-table pattern: create new table without handle, copy data, drop old, rename)

Update `server/db/schema.sql` — remove `handle` column from `CREATE TABLE users`.

*Depends on nothing. Parallel with Step 1.2.*

### Step 1.2: Update database queries to remove handle usage

Modify `server/db/queries.go`:
- `CreateUser`: remove `handle` parameter
- Remove `GetUserByHandle` (no longer needed)
- `GetUserByID`: remove `handle` from SELECT
- `GetMe`: remove handle from returned data
- Remove `SearchUsers` (no longer needed — replaced by invite system)
- Update `UserRow` struct: remove `Handle` field
- Update `scanUser` to not scan handle

*Depends on Step 1.1. Parallel with Step 1.3.*

### Step 1.3: Update registration handler — keyfile generation

Modify `server/handlers/register.go`:
- Remove `Handle` field from `RegisterRequest`
- Remove handle uniqueness check
- Remove `handle` from `CreateUser` call
- After creating user, generate a "keyfile" JSON: `{"user_id": "...", "auth_public_key": "..."}` and return it in the response (as `keyfile` field)
- The keyfile is NOT stored on the server — it's returned once and the client must save it

*Depends on Step 1.2. Parallel with Step 1.4.*

### Step 1.4: Update auth handlers — user_id instead of handle

Modify `server/handlers/auth.go`:
- `handleChallenge`: accept `user_id` instead of `handle`, use `GetUserByID` instead of `GetUserByHandle`
- `handleVerify`: accept `user_id` instead of `handle`
- The challenge entry already stores `UserID`, so verification doesn't need handle at all

*Depends on Step 1.2. Parallel with Step 1.3.*

### Step 1.5: Update recover handler — user_id instead of handle

Modify `server/handlers/recover.go`:
- Accept `user_id` in request (already supported, just remove handle fallback)
- Remove the `GetUserByHandle` fallback path

*Depends on Step 1.2.*

### Step 1.6: Remove user search endpoint

Modify `server/main.go`:
- Remove the `GET /api/users/search` endpoint registration — it's no longer needed
- Remove the `UserSearch` rate limit spec if not used elsewhere (keep the struct for future use)

*Depends on Step 1.2 and Phase 2 being planned. Can be done in parallel with Step 1.4.*

### Step 1.7: Update `/api/me` — remove handle from response

Modify `server/main.go` in the `/api/me` handler:
- Remove `"handle"` from the JSON response
- Keep `id`, `public_key_fingerprint`, `recovery_codes_remaining`

*Depends on Step 1.2.*

### Step 1.8: Update client registration flow

Modify `web/src/views/Login.svelte`:
- Remove the handle input from registration mode
- Remove `checkHandle`, `handleAvailable`, `handleDebounce` — no handle to check
- After key generation → server registration → receive keyfile from response
- **Auto-download the keyfile** as a `.json` file via a Blob download link
- Show a prominent warning: "Save this keyfile — you'll need it to log in. It cannot be recovered."
- Show the 10 recovery codes (unchanged flow)
- Store keys locally (unchanged)

*Depends on Step 1.3. Can be done in parallel with Steps 1.4-1.7.*

### Step 1.9: Update client login flow

Modify `web/src/views/Login.svelte`:
- Replace handle text input with a **file upload** button for the keyfile
- Parse keyfile JSON → extract `user_id`
- Use `user_id` (not handle) in challenge and verify API calls
- The challenge-response flow stays the same otherwise

Modify `web/src/lib/api.js`:
- Change `challenge(handle)` to `challenge(userId)`
- Change `verify(handle, challenge, signature)` to `verify(userId, challenge, signature)`
- Remove `searchUsers` method
- Update `register` to not send handle

Modify `web/src/lib/stores/auth.js`:
- `login()` now takes a keyfile object `{user_id, ...}` instead of a handle string
- `user` store no longer has `handle` field — just `uuid` and `key_fingerprint`

*Depends on Steps 1.4, 1.5, 1.8.*

---

## Phase 2: One-Time Invite Codes for "Add Friend"

### Step 2.1: Database — create `invite_codes` table

Create `server/db/migrations/004_invite_codes.sql`:
```sql
CREATE TABLE IF NOT EXISTS invite_codes (
    id              TEXT PRIMARY KEY,  -- UUID
    creator_id      TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    code            TEXT NOT NULL UNIQUE,  -- 8-char alphanumeric
    code_hash       TEXT NOT NULL,  -- SHA-256 of code for lookup
    created_at      TEXT NOT NULL DEFAULT (datetime('now')),
    expires_at      TEXT NOT NULL,  -- e.g., 24h from creation
    used            INTEGER NOT NULL DEFAULT 0,
    claimed_by      TEXT REFERENCES users(id),
    claimed_at      TEXT
);
```

Update `server/db/schema.sql` with the new table.

*Depends on nothing. Parallel with Phase 1 steps.*

### Step 2.2: Database queries for invite codes

Add to `server/db/queries.go`:
- `CreateInviteCode(id, creatorID, code, codeHash string, expiresAt time.Time) error`
- `GetInviteCodeByHash(codeHash string) (*InviteCodeRow, error)`
- `ClaimInviteCode(id, claimedBy string) error`
- `GetUserInviteCodes(creatorID string) ([]InviteCodeRow, error)` — list active invites
- `DeleteExpiredInviteCodes() (int64, error)` — cleanup
- `InviteCodeRow` struct

Add to `server/main.go`:
- Periodic cleanup goroutine for expired invite codes (alongside session cleanup)

*Depends on Step 2.1.*

### Step 2.3: Invite code API endpoints

Create `server/handlers/invites.go`:
- `POST /api/invites` — create a new invite code (generates random 8-char alphanumeric, stores hash, returns plaintext code once)
- `GET /api/invites` — list user's active (unused, unexpired) invite codes
- `POST /api/invites/claim` — claim an invite code {code: "ABC123XY"}, returns creator's user_id and public_key_x25519
- `DELETE /api/invites/:id` — revoke an invite code

Register in `server/main.go` on the authenticated mux.

Rate limit invite creation (e.g., 10 per hour) and claiming (e.g., 20 per minute).

*Depends on Step 2.2.*

### Step 2.4: Client-side invite code UI — generate

Modify `web/src/lib/api.js`:
- Add `createInvite()` → returns {id, code, expires_at}
- Add `listInvites()` → returns [{id, created_at, expires_at, used}]
- Add `claimInvite(code)` → returns {user_id, public_key_x25519}
- Add `revokeInvite(id)`

Create or modify `web/src/components/NewChatModal.svelte`:
- Replace the handle search UI with an **invite code entry** field
- Add a tab or section: "Enter Invite Code" (to add someone)
- Add a section: "My Invite Codes" — list active codes with copy/revoke buttons
- Button: "Generate New Invite Code" → calls API → shows code with copy-to-clipboard

*Depends on Step 2.3.*

### Step 2.5: Client-side invite claim flow — "name this person"

Modify `web/src/components/NewChatModal.svelte`:
- When user enters an invite code and clicks "Claim":
  1. Call `api.claimInvite(code)` → get back `{user_id, public_key_x25519}`
  2. Show a prompt: "What do you want to call this person?"
  3. User enters a display name (e.g., "Alice", "Bob-from-work")
  4. Store in IndexedDB contacts: `{uuid: user_id, custom_name: "Alice", public_key_x25519, ...}`
  5. Start the conversation (same as current flow after selecting a user)

*Depends on Step 2.4.*

---

## Phase 3: Friends Page & Encrypted Custom Names

### Step 3.1: Update IndexedDB contacts store

Modify `web/src/lib/db.js`:
- Keep `uuid` as primary key (already is)
- Change index from `by_handle` to `by_uuid`
- Add `custom_name` field (encrypted AES-256-GCM blob)
- Add `custom_name_nonce` field (12-byte nonce used for encryption)
- Add `public_key_x25519` field (the contact's X25519 key for ECDH)
- Add `added_at` field (when the contact was added)
- Remove `handle` field from contact schema

Add helper functions to `db.js`:
- `addContact(uuid, encryptedName, nonce, publicKeyX25519)` — creates a contact entry
- `getContact(uuid)` — fetch a single contact
- `getAllContacts()` — fetch all contacts (for Friends page)
- `updateContactName(uuid, encryptedName, nonce)` — rename a contact
- `removeContact(uuid)` — delete a contact
- `searchContacts(query)` — search locally by custom_name

*Depends on nothing. Parallel with Phase 1 & 2.*

### Step 3.2: Encrypt/decrypt contact names with user's master key

Create `web/src/lib/crypto/contact-encrypt.js`:
- `deriveContactKey(masterPrivateKey)` → HKDF-SHA256(masterPriv, salt=32-byte random, info="tailchat-contacts") → 32-byte AES key
- `encryptContactName(plaintext, contactKey)` → `{ciphertext, nonce}` using AES-256-GCM
- `decryptContactName(ciphertext, nonce, contactKey)` → plaintext name
- The derived contact key should be cached in memory (not stored in IndexedDB) during the session

*Depends on Step 3.1.*

### Step 3.3: Create dedicated Friends page

Create `web/src/components/FriendsPage.svelte`:
- **Layout**: Left panel shows contact list, right panel shows contact detail / edit form
- **Contact list**: Renders all contacts from IndexedDB showing their decrypted custom names; search bar at top for local filtering by name
- **Contact detail** (when a contact is selected):
  - Their custom name (editable inline or via "Edit" button — saves re-encrypted to IndexedDB)
  - Their key fingerprint (from stored `public_key_x25519`)
  - When they were added (`added_at`)
  - Conversation history link (opens chat with this contact)
  - "Remove Contact" button (with confirmation dialog before deleting from IndexedDB)
- **Empty state**: If no contacts yet, shows instructions on how to add someone via invite code (with link to NewChatModal)
- **Navigation**: Accessible from the left sidebar alongside Chats and Settings

Adding new contacts still happens via the invite code flow in `NewChatModal` (Phase 2). **Editing and managing** existing contacts happens on the Friends page.

*Depends on Steps 3.1, 3.2.*

### Step 3.4: Create shared contact-name resolution helper

Create `web/src/lib/contacts.js`:
- `getDisplayName(uuid)` — async: looks up contact in IndexedDB, decrypts custom name, returns it (or fallback `User_${uuid.slice(0, 8)}`)
- `getDisplayNameSync(uuid, contactsCache)` — sync version using a pre-loaded contacts map
- A Svelte writable store `contactNames` — maps uuid → display name, populated on app load and kept reactive
- `refreshContactNames()` — reloads all contacts from IndexedDB, decrypts names, updates the store
- `addContactToStore(uuid, customName)` — adds a single entry to the store after claiming an invite

*Depends on Steps 3.1, 3.2.*

### Step 3.5: Update all UI components to use custom names from contacts

Modify these components to resolve display names from the contacts store:
- `web/src/components/ChatListItem.svelte` — show contact's custom name (or fallback `User_${uuid.slice(0, 8)}`)
- `web/src/components/Conversation.svelte` — show custom name in conversation header
- `web/src/components/MessageBubble.svelte` — show message sender's custom name (for group messages where you see other people's names)
- `web/src/components/RightPanel.svelte` — show custom name in conversation info, with "View in Friends" link
- `web/src/components/NewGroupModal.svelte` — list contacts by custom name, search locally in IndexedDB for member selection
- `web/src/components/EmptyState.svelte` — remove all handle references

*Depends on Steps 3.3, 3.4.*

### Step 3.6: Update conversation store

Modify `web/src/lib/stores/chats.js`:
- When loading conversations, enrich each conversation object with `display_name` resolved from the contacts store
- Conversations whose user_id is not yet a contact show `User_${uuid.slice(0, 8)}`
- Remove all `handle`-based display logic

*Depends on Step 3.5.*

---

## Phase 4: Cleanup & Polish

### Step 4.1: Remove all handle references from server

- Verify no `handle` references remain in Go code
- Remove `handle` from rate limiter labels if applicable
- Update `server/handlers/messages.go` — conversations list already uses user_ids
- Update `server/handlers/groups.go` — verify no handle usage

### Step 4.2: Remove all handle references from client

- Verify no `handle` references remain in Svelte components
- Check `web/src/lib/stores/auth.js` — user object has no handle
- Check `web/src/lib/api.js` — no handle in any API calls
- Check all `.svelte` files for `handle` usage

### Step 4.3: Wire navigation — Friends page in sidebar

Modify app navigation (`LeftPanel.svelte` and/or `Main.svelte`):
- Add a "Friends" navigation item to the sidebar/panel
- Wire it to render `FriendsPage.svelte` when selected
- Add a route for `/friends` in the SPA router

*Depends on Step 3.3.*

### Step 4.4: Update config and documentation

- `scripts/config.yaml` — no changes needed (no hardcoded users)
- Verify `server/config.go` — no hardcoded user config exists (confirmed)
- Update README if it mentions handles or username-based auth

### Step 4.5: Testing & verification

- Manual registration flow test: create account, download keyfile, save recovery codes
- Manual login flow test: upload keyfile, challenge-response, get session
- Manual recovery flow test: enter recovery code without keyfile
- Manual invite flow test: create invite, share, claim, name contact
- Manual Friends page test: view contacts list, edit a contact's name (verify re-encrypted), remove a contact
- Manual local contact search test: type in Friends search bar, filter by custom name
- Manual group creation test: select contacts by custom name from local list
- Manual messaging test: 1:1 and group messages with custom names showing in UI
- Automated: run existing server tests if any

---

## Relevant Files

### Server — to modify:
- `server/db/schema.sql` — remove handle, add invite_codes
- `server/db/queries.go` — remove handle queries, add invite queries, update UserRow
- `server/db/migrations/003_remove_handle.sql` — new migration
- `server/db/migrations/004_invite_codes.sql` — new migration
- `server/handlers/register.go` — remove handle, return keyfile
- `server/handlers/auth.go` — user_id instead of handle
- `server/handlers/recover.go` — user_id instead of handle
- `server/handlers/invites.go` — new file for invite endpoints
- `server/handlers/messages.go` — verify no handle dependencies
- `server/handlers/groups.go` — verify no handle dependencies
- `server/main.go` — remove search endpoint, add invite routes, update /api/me

### Client — to modify/create:
- `web/src/views/Login.svelte` — keyfile upload for login, auto-download on register
- `web/src/lib/api.js` — remove handle params, add invite APIs
- `web/src/lib/stores/auth.js` — keyfile-based login, remove handle
- `web/src/lib/stores/chats.js` — custom names from contacts store
- `web/src/lib/db.js` — contacts schema update + CRUD helpers
- `web/src/lib/contacts.js` — **new** — shared display name resolution helpers + Svelte store
- `web/src/lib/crypto/contact-encrypt.js` — **new** — AES-256-GCM encryption for contact names
- `web/src/components/FriendsPage.svelte` — **new** — dedicated friends list management page
- `web/src/components/NewChatModal.svelte` — invite code entry + claim flow instead of handle search
- `web/src/components/NewGroupModal.svelte` — local contact search by custom name
- `web/src/components/ChatListItem.svelte` — custom name display from contacts
- `web/src/components/Conversation.svelte` — custom name display in header
- `web/src/components/MessageBubble.svelte` — custom name for sender labels
- `web/src/components/RightPanel.svelte` — custom name + link to Friends page
- `web/src/components/LeftPanel.svelte` — add Friends nav item
- `web/src/components/EmptyState.svelte` — remove handle references
- `web/src/components/Main.svelte` or router config — add `/friends` route

---

## Verification

1. **Registration**: `curl -X POST /api/register` → returns keyfile JSON; no handle field in response
2. **Keyfile login**: Upload keyfile → `POST /api/auth/challenge` with user_id → challenge returned → sign → `POST /api/auth/verify` → session token
3. **Recovery**: `POST /api/recover` with user_id + recovery_code_hash → encrypted key returned (unchanged logic)
4. **Invite create + claim**: `POST /api/invites` → returns 8-char code; another user claims it → prompted for custom name
5. **Friends page**: See all contacts with decrypted custom names; edit a name → saved re-encrypted; remove a contact → deleted from IndexedDB
6. **Local contact search**: Type in Friends search bar → filters by custom name (no server round-trip)
7. **Group creation**: NewGroupModal lists contacts by custom name; select members → create group
8. **Custom names in chat**: ChatListItem and Conversation show the custom name; MessageBubble shows sender's custom name in groups
9. **No handle leakage**: `SELECT * FROM users` → no handle column; grep codebase for `handle` → only in invite code context or legacy migration files
10. **Messaging still works**: Send/receive 1:1 and group messages end-to-end

---

## Decisions

- **Keyfile format**: JSON `{"user_id": "<uuid>", "auth_public_key": "<hex>"}` — human-readable, easy to parse
- **Keyfile-only login**: The keyfile is the sole login credential. Upload keyfile → server reads `user_id` → issues challenge → user signs with auth private key → session token. No backup code required for login. With Tailscale as the access-control perimeter, the primary risk is user losing their keyfile, not account takeover.
- **Backup codes = recovery only**: The 10 recovery codes are only for restoring the master private key on a new device (if keyfile is lost or switching devices). Not used for daily login.
- **Recovery code regeneration**: When a user uses their last remaining recovery code (going from 1 → 0 unused), the recovery flow forces regeneration of 10 new codes. The user's master private key is re-encrypted with each new code and uploaded.
- **Invite code format**: 8-character alphanumeric (e.g., `A3KX9M2P`) — 36^8 = 2.8 trillion space, sufficient with rate limiting
- **Invite code expiry**: 24 hours default, server-enforced
- **Contact names encryption**: AES-256-GCM with HKDF-SHA256-derived key from master key (info `"tailchat-contacts"`) — consistent with existing crypto patterns
- **Contact key caching**: Derived key held in memory during session, never stored in IndexedDB
- **Display name fallback**: `User_${uuid.slice(0, 8)}` shown for user_ids not yet in your contacts
- **No server-side contact sync** in this plan — contacts stay in IndexedDB. Can be added later as encrypted blob on server for cross-device sync.
- **Group member selection**: From local contacts list only (users you've added via invite codes)

## Further Considerations

1. **Cross-device migration via recovery codes**: Without handles, moving to a new device works like this: install fresh, choose "Recover Account", upload keyfile (or skip if lost), enter one recovery code → server returns encrypted master key → PBKDF2 decrypts it → keys re-derived and stored locally → session established. Contacts stay in IndexedDB (local), not synced server-side. A future phase could encrypt the contact book and store it as a server blob alongside key backups.

2. **Keyfile loss without recovery codes**: If a user loses their keyfile AND all recovery codes, the account is permanently inaccessible. The registration UI must make this extremely clear. Mitigations:
   - At registration: forced keyfile download + warning "Save this file or your account is unrecoverable"
   - In settings: option to re-download the keyfile (client can regenerate it from local keys while logged in)
   - The keyfile is NOT security-sensitive to lose alone (contains `user_id` + `auth_public_key` — both public). Someone who obtains it still can't log in without the private key stored locally.

3. **Recovery code depletion flow**: Step by step:
   - User has 10 unused recovery codes on server
   - User uses one (e.g. migrates to new device) → server marks it used → 9 remaining
   - When user uses the last unused code (0 remaining) → the recovery response includes a flag `recovery_codes_exhausted: true`
   - Client receives this flag → immediately generates 10 new random codes → re-encrypts master private key with each → uploads all 10 fresh encrypted backups via a new `POST /api/recover/regenerate` endpoint
   - Server replaces ALL old encrypted_key_backups for this user with the new batch
   - Client shows the new codes with the same "save these" warning
   - Rate-limited to once per 24h to prevent abuse
   - This makes recovery codes a renewable resource — you can always get more by using the last one, creating a natural cycle.

4. **Invite code replay attacks**: Invite codes are shared out-of-band in plaintext, so interception is possible. Mitigations:
   - One-time-use: once claimed, the code is immediately marked used and can never be reused
   - Claim requires authentication: only an authenticated user can claim, so the server always knows who claimed it
   - Creator visibility: `GET /api/invites` shows all codes with `used`, `claimed_by`, `claimed_at` — if a code is stolen, the creator can see who claimed it and revoke remaining codes
