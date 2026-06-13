/**
 * Auth store — manages authentication state for the app.
 *
 * Stores:
 *   isAuthenticated  {boolean}  true when a valid session token exists
 *   user             {object|null}  { handle, uuid, key_fingerprint } or null
 *   sessionToken     {string}   the current Bearer token
 *
 * Actions:
 *   login(handle)        — challenge → sign → verify → store token
 *   logout()             — POST /api/auth/logout → clear → redirect
 *   restoreSession()     — rehydrate from sessionStorage on app load
 */
import { writable, derived } from 'svelte/store';
import { push } from 'svelte-spa-router';
import api from '../api.js';
import { loadKeyPair } from '../db.js';

// ── Local hex utilities (mirrored from Login.svelte) ──

function bytesToHex(bytes) {
  return Array.from(bytes).map((b) => b.toString(16).padStart(2, '0')).join('');
}

function hexToBytes(hex) {
  const bytes = new Uint8Array(hex.length / 2);
  for (let i = 0; i < hex.length; i += 2) {
    bytes[i / 2] = parseInt(hex.substring(i, i + 2), 16);
  }
  return bytes;
}

// ── Internal writables ──

/** @type {import('svelte/store').Writable<string>} */
const _token = writable(sessionStorage.getItem('tailchat-token') || '');

/** @type {import('svelte/store').Writable<object|null>} */
const _user = writable(null);

// ── Derived stores ──

/** Whether the user currently holds a valid session token. */
export const isAuthenticated = derived(_token, ($t) => $t.length > 0);

/** Current session Bearer token. */
export const sessionToken = { subscribe: _token.subscribe };

/** Current user profile (handle, uuid, key_fingerprint). */
export const user = { subscribe: _user.subscribe };

// ── Actions ──

/**
 * Perform the challenge-response login flow for the given handle.
 *
 * 1. Loads the derived auth keypair from IndexedDB.
 * 2. Calls POST /api/auth/challenge to get a random challenge.
 * 3. Signs the challenge with the derived auth Ed25519 key.
 * 4. Calls POST /api/auth/verify to get a session token.
 * 5. Stores the token in sessionStorage + store.
 * 6. Fetches /api/me for user profile.
 * 7. Navigates to /chat.
 */
async function login(handle) {
  const keys = await loadKeyPair();
  if (!keys) throw new Error('No cryptographic keys found. Please register or recover your account.');

  // 1. Get challenge (server generates random 32 bytes for this handle)
  const { challenge } = await api.challenge(handle);

  // 2. Sign challenge with derived auth key
  const challengeBytes = hexToBytes(challenge);
  let signature;
  try {
    const privKey = await crypto.subtle.importKey(
      'raw', keys.authPriv, { name: 'Ed25519' }, false, ['sign']
    );
    signature = await crypto.subtle.sign({ name: 'Ed25519' }, privKey, challengeBytes);
  } catch {
    const { ed25519 } = await import('@noble/curves/ed25519');
    signature = ed25519.sign(challengeBytes, keys.authPriv);
  }
  const sigHex = bytesToHex(new Uint8Array(signature));

  // 3. Verify signature against the challenge and get session token
  const { token, user_id } = await api.verify(handle, challenge, sigHex);

  // 4. Store in sessionStorage
  sessionStorage.setItem('tailchat-token', token);
  if (user_id) sessionStorage.setItem('tailchat-user-id', user_id);
  _token.set(token);

  // 5. Fetch profile
  try {
    const me = await api.getMe();
    _user.set({
      uuid: me.id || user_id,
      handle: me.handle || handle,
      key_fingerprint: me.public_key_fingerprint || '',
    });
  } catch {
    // Best-effort: fall back to provided handle
    _user.set({ uuid: user_id, handle, key_fingerprint: '' });
  }

  // 6. Navigate to chat
  push('/chat');
}

/**
 * Log out the current session.
 * Clears local state immediately, then attempts to invalidate server-side.
 */
async function logout() {
  try {
    await api.logout();
  } catch {
    // Server session may already be invalid — clear locally regardless
  }
  sessionStorage.removeItem('tailchat-token');
  sessionStorage.removeItem('tailchat-user-id');
  _token.set('');
  _user.set(null);
  push('/login');
}

/**
 * Restore session from sessionStorage on app initialisation.
 * Call this once from the root component's onMount.
 */
async function restoreSession() {
  const t = sessionStorage.getItem('tailchat-token');
  if (t) {
    _token.set(t);
    try {
      const me = await api.getMe();
      _user.set({
        uuid: me.id,
        handle: me.handle,
        key_fingerprint: me.public_key_fingerprint || '',
      });
    } catch {
      // Token expired or invalid — clear
      sessionStorage.removeItem('tailchat-token');
      sessionStorage.removeItem('tailchat-user-id');
      _token.set('');
    }
  }
}

export const auth = {
  subscribe: derived([_token, _user], ([$t, $u]) => ({
    isAuthenticated: $t.length > 0,
    sessionToken: $t,
    user: $u,
  })).subscribe,
  login,
  logout,
  restoreSession,
};
