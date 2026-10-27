/**
 * Auth store.
 *
 * Signing in proves you hold the account key by signing a server challenge
 * with a login key derived from it. The account key itself never leaves the
 * browser.
 *
 *   login(handle, accountKey, remember)  — challenge → sign → verify → session
 *   logout()                             — end the session, drop keys from this tab
 *   restoreSession()                     — rehydrate after a page reload
 */
import { writable, derived } from 'svelte/store';
import { push } from 'svelte-spa-router';
import api from '../api.js';
import { clearCache } from '../directory.js';
import { forgetGroupState } from '../chat.js';
import { fromB64, fromHex, toHex, equal } from '../crypto/bytes.js';
import { deriveIdentity, fingerprint, signLoginChallenge } from '../crypto/identity.js';
import { getIdentity, lock, setAccountKey } from '../keyring.js';

const _token = writable(sessionStorage.getItem('tailchat-token') || '');
const _user = writable(null);

export const isAuthenticated = derived(_token, ($t) => $t.length > 0);
export const sessionToken = { subscribe: _token.subscribe };
export const user = { subscribe: _user.subscribe };

function profileFrom(me) {
  return {
    uuid: me.id,
    handle: me.handle,
    key_fingerprint: me.public_key_ed25519 ? fingerprint(fromB64(me.public_key_ed25519)) : '',
  };
}

/**
 * Sign in on this device. Throws friendly errors.
 * @param {string} handle
 * @param {Uint8Array} seed  the 32-byte account key
 * @param {boolean} remember keep the key on this device after logout
 */
async function login(handle, seed, remember = false, { navigate = true } = {}) {
  const identity = deriveIdentity(seed);

  const { challenge, derived_public_key } = await api.challenge(handle);
  if (!equal(fromHex(derived_public_key), identity.authPub)) {
    throw new Error(`That account key doesn't belong to "${handle}".`);
  }
  const sig = signLoginChallenge(identity, fromHex(challenge));
  const { token, user_id } = await api.verify(handle, challenge, toHex(sig));

  sessionStorage.setItem('tailchat-token', token);
  sessionStorage.setItem('tailchat-user-id', user_id);
  await setAccountKey(seed, remember);
  _token.set(token);

  try {
    _user.set(profileFrom(await api.getMe()));
  } catch {
    _user.set({ uuid: user_id, handle, key_fingerprint: '' });
  }
  if (navigate) push('/chat');
}

function clearLocal() {
  sessionStorage.removeItem('tailchat-token');
  sessionStorage.removeItem('tailchat-user-id');
  lock();
  clearCache();
  forgetGroupState();
  _token.set('');
  _user.set(null);
}

async function logout() {
  try {
    await api.logout();
  } catch {
    /* session may already be gone */
  }
  clearLocal();
  push('/login');
}

async function restoreSession() {
  const t = sessionStorage.getItem('tailchat-token');
  if (!t) return;
  _token.set(t);
  try {
    const identity = await getIdentity();
    if (!identity) throw new Error('no key');
    _user.set(profileFrom(await api.getMe()));
  } catch {
    clearLocal(); // expired session or missing key: start over at the login screen
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
