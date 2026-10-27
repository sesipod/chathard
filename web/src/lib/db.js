/**
 * IndexedDB wrapper.
 *
 * Stores:
 *   keys      – the account key, ONLY if the user ticked "remember this device"
 *   contacts  – pinned public keys of people you talk to (trust-on-first-use)
 *
 * No message plaintext is ever written here.
 *
 * @module db
 */

const DB_NAME = 'tailchat';
const DB_VERSION = 2;

/** @type {Promise<IDBDatabase>|null} */
let opening = null;

export function initDB() {
  if (opening) return opening;
  opening = new Promise((resolve, reject) => {
    const req = indexedDB.open(DB_NAME, DB_VERSION);
    req.onupgradeneeded = () => {
      const d = req.result;
      for (const name of Array.from(d.objectStoreNames)) d.deleteObjectStore(name); // v1 layout is obsolete
      d.createObjectStore('keys', { keyPath: 'id' });
      d.createObjectStore('contacts', { keyPath: 'uuid' });
    };
    req.onsuccess = () => resolve(req.result);
    req.onerror = () => {
      opening = null;
      reject(new Error(`IndexedDB open failed: ${req.error}`));
    };
  });
  return opening;
}

async function run(storeName, mode, fn) {
  const d = await initDB();
  return new Promise((resolve, reject) => {
    const tx = d.transaction(storeName, mode);
    const req = fn(tx.objectStore(storeName));
    tx.oncomplete = () => resolve(req?.result);
    tx.onerror = () => reject(tx.error);
    tx.onabort = () => reject(tx.error);
  });
}

// ── Account key (remember-this-device) ──

export const saveSeed = (seed) => run('keys', 'readwrite', (s) => s.put({ id: 'default', seed: Array.from(seed) }));

export async function loadSeed() {
  const row = await run('keys', 'readonly', (s) => s.get('default'));
  return row?.seed ? new Uint8Array(row.seed) : null;
}

export const deleteSeed = () => run('keys', 'readwrite', (s) => s.delete('default'));

// ── Pinned contacts ──

export const putContact = (c) =>
  run('contacts', 'readwrite', (s) =>
    s.put({ ...c, edPub: Array.from(c.edPub), xPub: Array.from(c.xPub) }),
  );

export async function getContact(uuid) {
  const row = await run('contacts', 'readonly', (s) => s.get(uuid));
  return row ? { ...row, edPub: new Uint8Array(row.edPub), xPub: new Uint8Array(row.xPub) } : null;
}

export const clearContacts = () => run('contacts', 'readwrite', (s) => s.clear());
