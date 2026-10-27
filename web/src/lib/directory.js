/**
 * Who is who. Looks up other users' public keys and PINS the first key seen
 * for each person (trust-on-first-use). If the server ever presents a different
 * key for someone you already know, we refuse to use it — that is exactly what
 * a server-side key-swap attack looks like. Compare safety numbers (see
 * identity.safetyNumber) to rule out a swap at first contact.
 */
import api from './api.js';
import { equal, fromB64 } from './crypto/bytes.js';
import { keysConsistent } from './crypto/identity.js';
import { getContact, putContact } from './db.js';

export class KeyChangedError extends Error {
  constructor(userId, handle) {
    super(`The security key for ${handle || 'this contact'} changed. Messages are blocked until you verify them in person.`);
    this.name = 'KeyChangedError';
    this.userId = userId;
  }
}

const cache = new Map();

/** Turn a server user record into a verified, pinned contact. */
async function pin(u) {
  const edPub = fromB64(u.public_key_ed25519);
  const xPub = fromB64(u.public_key_x25519);
  if (!keysConsistent(edPub, xPub)) throw new Error(`Server returned inconsistent keys for ${u.handle}`);

  const known = (await getContact(u.id)) || null;
  if (known && !equal(known.edPub, edPub)) throw new KeyChangedError(u.id, known.handle);

  const contact = known || { uuid: u.id, handle: u.handle, edPub, xPub, verified: false, firstSeen: Date.now() };
  if (!known) await putContact(contact);
  cache.set(u.id, contact);
  return contact;
}

/** @returns {Promise<{uuid, handle, edPub, xPub, verified}>} */
export async function getUser(id) {
  return (await getUsers([id])).get(id);
}

export async function getUsers(ids) {
  const out = new Map();
  const missing = [];
  for (const id of new Set(ids)) {
    if (cache.has(id)) out.set(id, cache.get(id));
    else missing.push(id);
  }
  if (missing.length) {
    const fetched = await api.lookupUsers(missing);
    for (const u of fetched) out.set(u.id, await pin(u));
  }
  for (const id of ids) if (!out.has(id)) throw new Error('Unknown user');
  return out;
}

/** Pin a user object we already have (e.g. from search results). */
export const pinUser = pin;

export async function markVerified(id, verified = true) {
  const c = await getContact(id);
  if (!c) return;
  c.verified = verified;
  await putContact(c);
  cache.set(id, c);
}

export function clearCache() {
  cache.clear();
}
