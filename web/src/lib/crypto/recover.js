/**
 * Recovery code key derivation for TailChat.
 *
 * Uses PBKDF2 (100K iterations, SHA-256) to derive an AES-256-GCM key from a
 * recovery code, enabling encryption/decryption of the master Ed25519 private key.
 *
 * @module recover
 */

/**
 * Derive an AES-256-GCM CryptoKey from a recovery code via PBKDF2.
 *
 * @param {string} recoveryCode - Formatted recovery code string (e.g. "X3KM-7FJ2-PQ9N-RT8B").
 * @param {Uint8Array} salt - 32-byte random salt (stored alongside the encrypted key).
 * @returns {Promise<CryptoKey>} AES-GCM CryptoKey usable for decrypt.
 */
export async function deriveKeyFromRecoveryCode(recoveryCode, salt) {
  // Normalize: strip dashes and convert to uppercase
  const normalized = recoveryCode.replace(/-/g, '').toUpperCase();

  const enc = new TextEncoder();
  const keyMaterial = await crypto.subtle.importKey(
    'raw',
    enc.encode(normalized),
    'PBKDF2',
    false,
    ['deriveBits', 'deriveKey']
  );

  return crypto.subtle.deriveKey(
    {
      name: 'PBKDF2',
      salt,
      iterations: 100000,
      hash: 'SHA-256',
    },
    keyMaterial,
    { name: 'AES-GCM', length: 256 },
    false,
    ['encrypt', 'decrypt']
  );
}

/**
 * Encrypt the master Ed25519 private key with a recovery code.
 *
 * @param {Uint8Array} masterPrivateKey - 32-byte Ed25519 master private key.
 * @param {string} recoveryCode - Formatted recovery code string.
 * @param {Uint8Array} salt - 32-byte random salt (generated once, stored on server).
 * @returns {Promise<Uint8Array>} Encrypted private key (ciphertext + 16-byte GCM tag).
 */
export async function encryptMasterKeyWithCode(masterPrivateKey, recoveryCode, salt) {
  const aesKey = await deriveKeyFromRecoveryCode(recoveryCode, salt);
  const nonce = crypto.getRandomValues(new Uint8Array(12));
  const ct = await crypto.subtle.encrypt(
    { name: 'AES-GCM', iv: nonce, tagLength: 128 },
    aesKey,
    masterPrivateKey
  );
  // Prepend nonce to output so decrypt can recover it
  const payload = new Uint8Array(12 + ct.byteLength);
  payload.set(nonce);
  payload.set(new Uint8Array(ct), 12);
  return payload;
}

/**
 * Decrypt the master Ed25519 private key with a recovery code.
 *
 * @param {Uint8Array} encryptedKey - Output from `encryptMasterKeyWithCode` (nonce-prefixed).
 * @param {string} recoveryCode - Formatted recovery code string.
 * @param {Uint8Array} salt - Same salt used during encryption.
 * @returns {Promise<Uint8Array>} 32-byte Ed25519 master private key.
 */
export async function decryptMasterKeyWithCode(encryptedKey, recoveryCode, salt) {
  const aesKey = await deriveKeyFromRecoveryCode(recoveryCode, salt);
  const nonce = encryptedKey.slice(0, 12);
  const ct = encryptedKey.slice(12);
  const pt = await crypto.subtle.decrypt(
    { name: 'AES-GCM', iv: nonce, tagLength: 128 },
    aesKey,
    ct
  );
  return new Uint8Array(pt);
}
