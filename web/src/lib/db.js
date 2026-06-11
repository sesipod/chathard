/**
 * IndexedDB wrapper for TailChat.
 *
 * Stores: keys (master Ed25519, X25519, derived auth), contacts (handle→UUID→keys),
 * drafts, group key history, and app settings.
 *
 * Database: "tailchat" (version 1)
 * Object stores: "keys", "contacts", "drafts", "group_keys", "settings"
 *
 * @module db
 */

/** @type {IDBDatabase|null} */
let db = null;

const DB_NAME = 'tailchat';
const DB_VERSION = 1;

/**
 * Open (or create) the IndexedDB database and ensure all object stores exist.
 *
 * Safe to call multiple times — returns the same connection on subsequent calls.
 *
 * @returns {Promise<IDBDatabase>}
 */
export async function initDB() {
  if (db) return db;

  return new Promise((resolve, reject) => {
    const req = indexedDB.open(DB_NAME, DB_VERSION);

    req.onupgradeneeded = (ev) => {
      const d = /** @type {IDBDatabase} */ (ev.target.result);

      // Keys store: single entry with master + derived keys
      if (!d.objectStoreNames.contains('keys')) {
        d.createObjectStore('keys', { keyPath: 'id' });
      }

      // Contacts: handle → UUID mapping + public keys
      if (!d.objectStoreNames.contains('contacts')) {
        const store = d.createObjectStore('contacts', { keyPath: 'uuid' });
        store.createIndex('by_handle', 'handle', { unique: true });
      }

      // Drafts: per-conversation unsent text
      if (!d.objectStoreNames.contains('drafts')) {
        d.createObjectStore('drafts', { keyPath: 'conversationId' });
      }

      // Group key history: ordered list of {key, validFrom} per group
      if (!d.objectStoreNames.contains('group_keys')) {
        d.createObjectStore('group_keys', { keyPath: 'groupId' });
      }

      // Settings: app preferences (theme, default retention, etc.)
      if (!d.objectStoreNames.contains('settings')) {
        d.createObjectStore('settings', { keyPath: 'key' });
      }
    };

    req.onsuccess = (ev) => {
      db = /** @type {IDBDatabase} */ (ev.target.result);
      resolve(db);
    };

    req.onerror = (ev) => {
      reject(new Error(`IndexedDB open failed: ${ev.target.error}`));
    };
  });
}

/**
 * Get a reference to an object store (readwrite).
 * @param {string} name
 * @param {'readwrite'|'readonly'} [mode='readwrite']
 * @returns {Promise<IDBObjectStore>}
 */
async function store(name, mode = 'readwrite') {
  const d = await initDB();
  const tx = d.transaction(name, mode);
  return tx.objectStore(name);
}

// ---------------------------------------------------------------------------
// Keys
// ---------------------------------------------------------------------------

/**
 * Store the full derived key set in IndexedDB.
 *
 * @param {{masterPub: Uint8Array, masterPriv: Uint8Array, x25519Pub: Uint8Array, x25519Priv: Uint8Array, authPub: Uint8Array, authPriv: Uint8Array, authSalt: Uint8Array}} keypair
 */
export async function storeKeyPair(keypair) {
  const s = await store('keys');
  return new Promise((resolve, reject) => {
    const req = s.put({
      id: 'default',
      ...keypair,
      masterPub: Array.from(keypair.masterPub),
      masterPriv: Array.from(keypair.masterPriv),
      x25519Pub: Array.from(keypair.x25519Pub),
      x25519Priv: Array.from(keypair.x25519Priv),
      authPub: Array.from(keypair.authPub),
      authPriv: Array.from(keypair.authPriv),
      authSalt: Array.from(keypair.authSalt),
    });
    req.onsuccess = () => resolve();
    req.onerror = () => reject(req.error);
  });
}

/**
 * Load keys from IndexedDB.
 *
 * @returns {Promise<{masterPub: Uint8Array, masterPriv: Uint8Array, x25519Pub: Uint8Array, x25519Priv: Uint8Array, authPub: Uint8Array, authPriv: Uint8Array, authSalt: Uint8Array}|null>}
 */
export async function loadKeyPair() {
  const s = await store('keys', 'readonly');
  return new Promise((resolve, reject) => {
    const req = s.get('default');
    req.onsuccess = () => {
      const row = req.result;
      if (!row) return resolve(null);
      resolve({
        masterPub: new Uint8Array(row.masterPub),
        masterPriv: new Uint8Array(row.masterPriv),
        x25519Pub: new Uint8Array(row.x25519Pub),
        x25519Priv: new Uint8Array(row.x25519Priv),
        authPub: new Uint8Array(row.authPub),
        authPriv: new Uint8Array(row.authPriv),
        authSalt: new Uint8Array(row.authSalt),
      });
    };
    req.onerror = () => reject(req.error);
  });
}

/**
 * Delete all keys from IndexedDB (logout / wipe).
 */
export async function deleteKeys() {
  const s = await store('keys');
  return new Promise((resolve, reject) => {
    const req = s.delete('default');
    req.onsuccess = () => resolve();
    req.onerror = () => reject(req.error);
  });
}

// ---------------------------------------------------------------------------
// Contacts
// ---------------------------------------------------------------------------

/**
 * Save or update a contact's public key mapping.
 *
 * @param {string} handle    - User's handle (e.g. "alice").
 * @param {string} uuid      - User's UUID.
 * @param {Uint8Array} x25519Pub - User's X25519 public key.
 */
export async function saveContact(handle, uuid, x25519Pub) {
  const s = await store('contacts');
  return new Promise((resolve, reject) => {
    const req = s.put({
      uuid,
      handle,
      x25519Pub: Array.from(x25519Pub),
      updatedAt: Date.now(),
    });
    req.onsuccess = () => resolve();
    req.onerror = () => reject(req.error);
  });
}

/**
 * Look up a contact by handle or UUID.
 *
 * @param {string} handleOrUuid
 * @returns {Promise<{uuid: string, handle: string, x25519Pub: Uint8Array}|null>}
 */
export async function getContact(handleOrUuid) {
  const d = await initDB();
  const tx = d.transaction('contacts', 'readonly');
  const s = tx.objectStore('contacts');

  return new Promise((resolve, reject) => {
    // Try UUID first (primary key)
    const reqByUuid = s.get(handleOrUuid);
    reqByUuid.onsuccess = () => {
      if (reqByUuid.result) {
        resolve(normalizeContact(reqByUuid.result));
        return;
      }
      // Fallback: search by handle index
      const idx = s.index('by_handle');
      const reqByHandle = idx.get(handleOrUuid);
      reqByHandle.onsuccess = () => {
        resolve(reqByHandle.result ? normalizeContact(reqByHandle.result) : null);
      };
      reqByHandle.onerror = () => reject(reqByHandle.error);
    };
    reqByUuid.onerror = () => reject(reqByUuid.error);
  });
}

/**
 * Get all cached contacts.
 *
 * @returns {Promise<Array<{uuid: string, handle: string, x25519Pub: Uint8Array}>>}
 */
export async function getAllContacts() {
  const s = await store('contacts', 'readonly');
  return new Promise((resolve, reject) => {
    const req = s.getAll();
    req.onsuccess = () => resolve((req.result || []).map(normalizeContact));
    req.onerror = () => reject(req.error);
  });
}

/** @param {{uuid:string, handle:string, x25519Pub:number[], updatedAt?:number}} row */
function normalizeContact(row) {
  return {
    uuid: row.uuid,
    handle: row.handle,
    x25519Pub: new Uint8Array(row.x25519Pub),
  };
}

// ---------------------------------------------------------------------------
// Group key history
// ---------------------------------------------------------------------------

/**
 * Store the key history for a group.
 *
 * @param {string} groupId
 * @param {Array<{key: Uint8Array, validFrom: string}>} keys
 */
export async function storeGroupKeyHistory(groupId, keys) {
  const s = await store('group_keys');
  return new Promise((resolve, reject) => {
    const serialized = keys.map((k) => ({
      key: Array.from(k.key),
      validFrom: k.validFrom,
    }));
    const req = s.put({ groupId, keys: serialized });
    req.onsuccess = () => resolve();
    req.onerror = () => reject(req.error);
  });
}

/**
 * Retrieve the key history for a group.
 *
 * @param {string} groupId
 * @returns {Promise<Array<{key: Uint8Array, validFrom: string}>|null>}
 */
export async function getGroupKeyHistory(groupId) {
  const s = await store('group_keys', 'readonly');
  return new Promise((resolve, reject) => {
    const req = s.get(groupId);
    req.onsuccess = () => {
      const row = req.result;
      if (!row) return resolve(null);
      resolve(
        row.keys.map((/** @type {{key:number[], validFrom:string}} */ k) => ({
          key: new Uint8Array(k.key),
          validFrom: k.validFrom,
        }))
      );
    };
    req.onerror = () => reject(req.error);
  });
}

// ---------------------------------------------------------------------------
// Drafts
// ---------------------------------------------------------------------------

/**
 * Save an unsent message draft for a conversation.
 *
 * @param {string} conversationId - The other user's UUID or group ID.
 * @param {string} text           - Draft text content.
 */
export async function saveDraft(conversationId, text) {
  const s = await store('drafts');
  return new Promise((resolve, reject) => {
    const req = s.put({ conversationId, text, updatedAt: Date.now() });
    req.onsuccess = () => resolve();
    req.onerror = () => reject(req.error);
  });
}

/**
 * Get the saved draft for a conversation.
 *
 * @param {string} conversationId
 * @returns {Promise<string|null>}
 */
export async function getDraft(conversationId) {
  const s = await store('drafts', 'readonly');
  return new Promise((resolve, reject) => {
    const req = s.get(conversationId);
    req.onsuccess = () => resolve(req.result ? req.result.text : null);
    req.onerror = () => reject(req.error);
  });
}

/**
 * Delete a draft after sending or discarding.
 *
 * @param {string} conversationId
 */
export async function deleteDraft(conversationId) {
  const s = await store('drafts');
  return new Promise((resolve, reject) => {
    const req = s.delete(conversationId);
    req.onsuccess = () => resolve();
    req.onerror = () => reject(req.error);
  });
}
