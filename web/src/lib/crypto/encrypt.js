/**
 * Message encryption / decryption for TailChat.
 *
 * Uses X25519 ECDH for key agreement, HKDF-SHA256 for key derivation, and
 * AES-256-GCM for symmetric encryption. Supports 1:1 messages, group messages,
 * and encrypted group key distribution.
 *
 * @module encrypt
 */

// ---------------------------------------------------------------------------
// Shared ECDH helper
// ---------------------------------------------------------------------------

/**
 * Perform X25519 ECDH key agreement.
 *
 * Primary: Web Crypto API `crypto.subtle.deriveBits({name:"X25519"})`.
 * Fallback: @noble/curves `x25519.getSharedSecret()`.
 *
 * @param {Uint8Array} privKey - 32-byte X25519 private key.
 * @param {Uint8Array} pubKey  - 32-byte X25519 public key.
 * @returns {Promise<Uint8Array>} 32-byte shared secret.
 */
async function ecdh(privKey, pubKey) {
  // Web Crypto API supports X25519 in Chrome 113+ / Safari 18+
  try {
    // Try Web Crypto path first
    const priv = await crypto.subtle.importKey('raw', privKey, { name: 'X25519' }, false, ['deriveBits']);
    const pub = await crypto.subtle.importKey('raw', pubKey, { name: 'X25519' }, false, []);
    const secret = await crypto.subtle.deriveBits({ name: 'X25519', public: pub }, priv, 256);
    return new Uint8Array(secret);
  } catch {
    // Fallback to @noble/curves
    const { x25519 } = await import('@noble/curves/x25519');
    return x25519.getSharedSecret(privKey, pubKey);
  }
}

// ---------------------------------------------------------------------------
// HKDF-SHA256
// ---------------------------------------------------------------------------

/**
 * Derive a 256-bit AES-GCM key using HKDF-SHA256.
 *
 * @param {Uint8Array} ikm  - Input key material (shared secret).
 * @param {Uint8Array} salt - Salt (typically nonce or 32-byte random).
 * @param {string|Uint8Array} info - Context / application-specific info.
 * @returns {Promise<Uint8Array>} 32-byte derived AES-GCM key.
 */
export async function HKDF(ikm, salt, info) {
  const infoBytes = typeof info === 'string' ? new TextEncoder().encode(info) : info;
  const hkdfKey = await crypto.subtle.importKey('raw', ikm, 'HKDF', false, ['deriveBits']);
  const derived = await crypto.subtle.deriveBits(
    { name: 'HKDF', hash: 'SHA-256', salt, info: infoBytes },
    hkdfKey,
    256
  );
  return new Uint8Array(derived);
}

// ---------------------------------------------------------------------------
// AES-256-GCM helpers
// ---------------------------------------------------------------------------

/**
 * Import a raw 32-byte key as an AES-GCM CryptoKey.
 * @param {Uint8Array} keyBytes
 * @param {'encrypt'|'decrypt'} usage
 * @returns {Promise<CryptoKey>}
 */
async function importAesKey(keyBytes, usage) {
  return crypto.subtle.importKey('raw', keyBytes, { name: 'AES-GCM' }, false, [usage]);
}

/**
 * Encrypt plaintext with AES-256-GCM.
 * @param {Uint8Array} plaintext
 * @param {Uint8Array} keyBytes - 32-byte key.
 * @param {Uint8Array} nonce    - 12-byte nonce.
 * @returns {Promise<Uint8Array>} Ciphertext + 16-byte GCM tag.
 */
async function aesEncrypt(plaintext, keyBytes, nonce) {
  const key = await importAesKey(keyBytes, 'encrypt');
  const ct = await crypto.subtle.encrypt({ name: 'AES-GCM', iv: nonce, tagLength: 128 }, key, plaintext);
  return new Uint8Array(ct);
}

/**
 * Decrypt AES-256-GCM ciphertext.
 * @param {Uint8Array} ciphertext - Includes 16-byte GCM tag.
 * @param {Uint8Array} keyBytes   - 32-byte key.
 * @param {Uint8Array} nonce      - 12-byte nonce.
 * @returns {Promise<Uint8Array>} Plaintext.
 */
async function aesDecrypt(ciphertext, keyBytes, nonce) {
  const key = await importAesKey(keyBytes, 'decrypt');
  const pt = await crypto.subtle.decrypt({ name: 'AES-GCM', iv: nonce, tagLength: 128 }, key, ciphertext);
  return new Uint8Array(pt);
}

// ---------------------------------------------------------------------------
// 1:1 message encryption / decryption
// ---------------------------------------------------------------------------

/**
 * Encrypt a plaintext message for a specific recipient.
 *
 * 1. Generate an ephemeral X25519 keypair.
 * 2. ECDH(ephemeral_priv, recipient_x25519_pub) → shared secret.
 * 3. HKDF-SHA256(shared_secret, salt=nonce, info="tailchat-message") → AES-GCM key.
 * 4. AES-256-GCM encrypt with a random 12-byte nonce.
 *
 * @param {Uint8Array} plaintext         - UTF-8 encoded message bytes.
 * @param {Uint8Array} recipientX25519Pub - Recipient's X25519 public key (32 bytes).
 * @param {Uint8Array} ownX25519Priv      - Sender's X25519 private key (32 bytes).
 * @returns {Promise<{ciphertext: Uint8Array, ephemeralPub: Uint8Array, nonce: Uint8Array}>}
 */
export async function encryptMessage(plaintext, recipientX25519Pub, ownX25519Priv) {
  // Generate ephemeral X25519 keypair
  const ephemeralPriv = crypto.getRandomValues(new Uint8Array(32));
  // Clamp ephemeral private key (RFC 7748)
  ephemeralPriv[0] &= 248;
  ephemeralPriv[31] &= 127;
  ephemeralPriv[31] |= 64;

  let ephemeralPub;
  try {
    const kp = await crypto.subtle.generateKey({ name: 'X25519' }, true, ['deriveBits']);
    const raw = await crypto.subtle.exportKey('raw', kp.publicKey);
    ephemeralPub = new Uint8Array(raw);
  } catch {
    const { x25519 } = await import('@noble/curves/x25519');
    ephemeralPub = x25519.getPublicKey(ephemeralPriv);
  }

  // ECDH → shared secret
  const sharedSecret = await ecdh(ephemeralPriv, recipientX25519Pub);

  // Derive AES-GCM key
  const nonce = crypto.getRandomValues(new Uint8Array(12));
  const aesKey = await HKDF(sharedSecret, nonce, 'tailchat-message');

  // Encrypt
  const ciphertext = await aesEncrypt(plaintext, aesKey, nonce);

  return { ciphertext, ephemeralPub, nonce };
}

/**
 * Decrypt a 1:1 message received from a sender.
 *
 * 1. ECDH(own_x25519_priv, sender_ephemeral_pub) → shared secret.
 * 2. HKDF-SHA256(shared_secret, salt=nonce, info="tailchat-message") → AES-GCM key.
 * 3. AES-256-GCM decrypt.
 *
 * @param {Uint8Array} ciphertext   - Encrypted message body.
 * @param {Uint8Array} ephemeralPub - Sender's ephemeral X25519 public key (32 bytes).
 * @param {Uint8Array} nonce        - 12-byte nonce used during encryption.
 * @param {Uint8Array} ownX25519Priv - Recipient's X25519 private key (32 bytes).
 * @returns {Promise<Uint8Array>} Decrypted plaintext bytes.
 */
export async function decryptMessage(ciphertext, ephemeralPub, nonce, ownX25519Priv) {
  const sharedSecret = await ecdh(ownX25519Priv, ephemeralPub);
  const aesKey = await HKDF(sharedSecret, nonce, 'tailchat-message');
  return aesDecrypt(ciphertext, aesKey, nonce);
}

// ---------------------------------------------------------------------------
// Group message encryption / decryption (symmetric)
// ---------------------------------------------------------------------------

/**
 * Encrypt a message for a group using the shared symmetric group key.
 *
 * @param {Uint8Array} plaintext - UTF-8 encoded message.
 * @param {Uint8Array} groupKey  - 32-byte AES-256-GCM group key.
 * @returns {Promise<{ciphertext: Uint8Array, nonce: Uint8Array}>}
 */
export async function encryptGroupMessage(plaintext, groupKey) {
  const nonce = crypto.getRandomValues(new Uint8Array(12));
  const ciphertext = await aesEncrypt(plaintext, groupKey, nonce);
  return { ciphertext, nonce };
}

/**
 * Decrypt a group message.
 *
 * @param {Uint8Array} ciphertext - Encrypted message.
 * @param {Uint8Array} nonce      - 12-byte nonce.
 * @param {Uint8Array} groupKey   - 32-byte AES-256-GCM group key.
 * @returns {Promise<Uint8Array>} Decrypted plaintext.
 */
export async function decryptGroupMessage(ciphertext, nonce, groupKey) {
  return aesDecrypt(ciphertext, groupKey, nonce);
}

// ---------------------------------------------------------------------------
// Group key distribution (per-member encrypted)
// ---------------------------------------------------------------------------

/**
 * Encrypt a group key for a specific group member using ECDH.
 *
 * @param {Uint8Array} groupKey       - 32-byte AES-GCM group key.
 * @param {Uint8Array} memberX25519Pub - Member's X25519 public key.
 * @param {Uint8Array} ownX25519Priv   - Encryptor's X25519 private key.
 * @returns {Promise<{encryptedKey: Uint8Array, senderEphemeralPub: Uint8Array, nonce: Uint8Array}>}
 */
export async function encryptGroupKeyForMember(groupKey, memberX25519Pub, ownX25519Priv) {
  const sharedSecret = await ecdh(ownX25519Priv, memberX25519Pub);
  const nonce = crypto.getRandomValues(new Uint8Array(12));
  const aesKey = await HKDF(sharedSecret, nonce, 'tailchat-group-key');
  const encryptedKey = await aesEncrypt(groupKey, aesKey, nonce);

  // Generate ephemeral pubkey for the recipient to re-derive shared secret
  let senderEphemeralPub;
  try {
    const kp = await crypto.subtle.generateKey({ name: 'X25519' }, true, ['deriveBits']);
    senderEphemeralPub = new Uint8Array(await crypto.subtle.exportKey('raw', kp.publicKey));
  } catch {
    // For non-WebCrypto ECDH, the sender's X25519 public key is used
    const { x25519 } = await import('@noble/curves/x25519');
    senderEphemeralPub = x25519.getPublicKey(ownX25519Priv);
  }

  // Embed nonce as first 12 bytes of encryptedKey so decrypt can recover it
  const payload = new Uint8Array(12 + encryptedKey.length);
  payload.set(nonce);
  payload.set(encryptedKey, 12);

  return { encryptedKey: payload, senderEphemeralPub, nonce };
}

/**
 * Decrypt a group key that was encrypted for this member.
 *
 * @param {Uint8Array} encryptedKey     - Encrypted group key.
 * @param {Uint8Array} ownX25519Priv    - Recipient's X25519 private key.
 * @param {Uint8Array} senderEphemeralPub - Sender's X25519 public key (used as ephemeral).
 * @returns {Promise<Uint8Array>} Decrypted 32-byte group key.
 */
export async function decryptGroupKeyForMember(encryptedKey, ownX25519Priv, senderEphemeralPub) {
  const sharedSecret = await ecdh(ownX25519Priv, senderEphemeralPub);
  // Nonce is embedded as first 12 bytes of encryptedKey by encryptGroupKeyForMember
  const nonce = encryptedKey.slice(0, 12);
  const ct = encryptedKey.slice(12);
  const aesKey = await HKDF(sharedSecret, nonce, 'tailchat-group-key');
  return aesDecrypt(ct, aesKey, nonce);
}

/**
 * Generate a new random AES-256-GCM group key (32 bytes).
 *
 * @returns {Uint8Array} 32-byte random key.
 */
export function generateGroupKey() {
  return crypto.getRandomValues(new Uint8Array(32));
}
