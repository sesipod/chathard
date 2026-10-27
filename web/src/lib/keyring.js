/**
 * Holds the signed-in user's keys in memory.
 *
 * Where the 32-byte account key lives between page loads:
 *   - "Remember this device" ticked → IndexedDB (stays after logout until you
 *     choose "Forget this device").
 *   - otherwise → sessionStorage (survives a reload, dies with the tab).
 * The server never sees it.
 */
import { deriveIdentity } from './crypto/identity.js';
import { fromB64, toB64 } from './crypto/bytes.js';
import { deleteSeed, loadSeed, saveSeed } from './db.js';

const SESSION_KEY = 'tailchat-seed';
let current = null;

export async function setAccountKey(seed, remember) {
  current = deriveIdentity(seed);
  sessionStorage.setItem(SESSION_KEY, toB64(seed));
  if (remember) await saveSeed(seed);
  return current;
}

/** Identity for this tab, restoring it from sessionStorage / the remembered device key. */
export async function getIdentity() {
  if (current) return current;
  const s = sessionStorage.getItem(SESSION_KEY);
  if (s) {
    current = deriveIdentity(fromB64(s));
    return current;
  }
  const remembered = await loadSeed();
  if (remembered) {
    current = deriveIdentity(remembered);
    sessionStorage.setItem(SESSION_KEY, toB64(remembered));
  }
  return current;
}

export async function hasRememberedKey() {
  return !!(await loadSeed());
}

/** Log out: forget keys in this tab. A remembered device key stays in IndexedDB. */
export function lock() {
  current = null;
  sessionStorage.removeItem(SESSION_KEY);
}

/** Also remove the remembered device key (shared / lost devices). */
export async function forgetDevice() {
  lock();
  await deleteSeed();
}
