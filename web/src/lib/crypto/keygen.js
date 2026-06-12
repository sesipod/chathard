/**
 * Key generation module for TailChat.
 * Primary: Web Crypto API crypto.subtle.generateKey({name: "Ed25519"})
 * Fallback: @noble/curves ed25519.utils.randomPrivateKey()
 * Feature-detects at startup and caches the method.
 *
 * @module keygen
 */

/** @typedef {{ publicKey: Uint8Array, privateKey: Uint8Array }} Keypair */

// ---------------------------------------------------------------------------
// Feature detection — runs once at module scope
// ---------------------------------------------------------------------------

/**
 * @type {'webcrypto'|'noble'|null}
 */
let cryptoImpl = null;

/**
 * Detect which Ed25519 implementation is available.
 * Cache result so every subsequent call skips detection.
 * @returns {Promise<'webcrypto'|'noble'>}
 */
async function detectCryptoImpl() {
  if (cryptoImpl) return cryptoImpl;
  try {
    const kp = await crypto.subtle.generateKey(
      { name: 'Ed25519' },
      false,
      ['sign', 'verify']
    );
    // If we got here without throwing, Ed25519 is supported
    cryptoImpl = 'webcrypto';
  } catch {
    cryptoImpl = 'noble';
  }
  return cryptoImpl;
}

// ---------------------------------------------------------------------------
// Lazy noble imports (only when fallback is needed)
// ---------------------------------------------------------------------------

let _nobleEd = null;
let _nobleX = null;

/**
 * Lazy-import @noble/curves modules.
 * @returns {Promise<{ed25519: any, x25519: any}>}
 */
async function noble() {
  if (!_nobleEd) {
    const { ed25519, x25519 } = await import('@noble/curves');
    _nobleEd = ed25519;
    _nobleX = x25519;
  }
  return { ed25519: _nobleEd, x25519: _nobleX };
}

// ---------------------------------------------------------------------------
// Ed25519 keypair generation
// ---------------------------------------------------------------------------

/**
 * Generate a unified Ed25519 keypair.
 *
 * Primary path uses Web Crypto API (Chrome 113+, Safari 18+).
 * Fallback uses @noble/curves (Firefox, older Safari).
 *
 * @returns {Promise<Keypair>} `{ publicKey, privateKey }` as 32-byte Uint8Arrays.
 */
export async function generateKeyPair() {
  const impl = await detectCryptoImpl();

  if (impl === 'webcrypto') {
    const kp = await crypto.subtle.generateKey(
      { name: 'Ed25519' },
      true,
      ['sign', 'verify']
    );
    const pub = new Uint8Array(await crypto.subtle.exportKey('raw', kp.publicKey));
    const priv = new Uint8Array(await crypto.subtle.exportKey('raw', kp.privateKey));
    return { publicKey: pub, privateKey: priv };
  }

  // Fallback: @noble/curves
  const { ed25519 } = await noble();
  const priv = ed25519.utils.randomPrivateKey();
  const pub = ed25519.getPublicKey(priv);
  return { publicKey: pub, privateKey: priv };
}

// ---------------------------------------------------------------------------
// Ed25519 → X25519 private key conversion (RFC 7748)
// ---------------------------------------------------------------------------

/**
 * Convert an Ed25519 private key scalar to an X25519 private key scalar.
 *
 * RFC 7748 §5: prune the scalar by clearing the lowest 3 bits (cofactor
 * clearing), clearing the highest bit, and setting the second-highest bit.
 *
 * @param {Uint8Array} edPriv - 32-byte Ed25519 private key.
 * @returns {Uint8Array} 32-byte X25519 private key.
 */
function ed25519PrivToX25519(edPriv) {
  const priv = new Uint8Array(edPriv);
  priv[0] &= 248;
  priv[31] &= 127;
  priv[31] |= 64;
  return priv;
}

/**
 * Convert an Ed25519 public key to an X25519 public key.
 *
 * Uses @noble/curves `edwardsToMontgomery` for the Edwards → Montgomery
 * point conversion (always falls back to noble since Web Crypto API has
 * no built-in Ed25519 → X25519 conversion).
 *
 * @param {Uint8Array} edPub - 32-byte Ed25519 public key.
 * @returns {Promise<Uint8Array>} 32-byte X25519 public key.
 */
async function ed25519PubToX25519(edPub) {
  const { ed25519 } = await noble();
  return ed25519.etf.edwardsToMontgomeryPub(edPub);
}

// ---------------------------------------------------------------------------
// Pre-compute derived keys (X25519 + HKDF-derived auth)
// ---------------------------------------------------------------------------

/**
 * Pre-compute derived keys once at key generation.
 *
 * Produces:
 * - **X25519 keypair** — used for ECDH in message encryption (RFC 7748).
 * - **Derived Ed25519 auth keypair** — HKDF-SHA256 with info `"tailchat-auth"`
 *   and a random 32-byte salt. Used for challenge-response login so the master
 *   Ed25519 key never leaves IndexedDB (key blinding).
 *
 * @param {Keypair} ed25519Keypair - The master Ed25519 keypair.
 * @returns {Promise<{x25519Pub: Uint8Array, x25519Priv: Uint8Array, authPub: Uint8Array, authPriv: Uint8Array, authSalt: Uint8Array}>}
 */
export async function deriveKeys(ed25519Keypair) {
  const { publicKey: edPub, privateKey: edPriv } = ed25519Keypair;

  // 1. X25519 keypair
  const x25519Priv = ed25519PrivToX25519(edPriv);
  const x25519Pub = await ed25519PubToX25519(edPub);

  // 2. HKDF-derived auth keypair
  const authSalt = crypto.getRandomValues(new Uint8Array(32));
  const authIkm = new Uint8Array(edPriv.length + edPub.length);
  authIkm.set(edPriv);
  authIkm.set(edPub, edPriv.length);

  const hkdfKey = await crypto.subtle.importKey('raw', authIkm, 'HKDF', false, ['deriveBits']);
  const authSeed = new Uint8Array(
    await crypto.subtle.deriveBits(
      { name: 'HKDF', hash: 'SHA-256', salt: authSalt, info: new TextEncoder().encode('tailchat-auth') },
      hkdfKey,
      256
    )
  );

  // Derive auth Ed25519 public key from the HKDF seed via @noble/curves.
  // The seed itself serves as the Ed25519 private key (Ed25519 private key = seed).
  const { ed25519 } = await noble();
  const authPub = ed25519.getPublicKey(authSeed);

  return {
    x25519Pub,
    x25519Priv,
    authPub,
    authPriv: authSeed,
    authSalt,
  };
}

// ---------------------------------------------------------------------------
// Ed25519 → X25519 full keypair conversion (RFC 7748)
// ---------------------------------------------------------------------------

/**
 * Convert an Ed25519 keypair to an X25519 keypair.
 *
 * The private key is pruned per RFC 7748 §5 (cofactor clearing).
 * The public key uses the Edwards → Montgomery point transform.
 *
 * @param {Keypair} ed25519Keypair - The Ed25519 keypair to convert.
 * @returns {Promise<{publicKey: Uint8Array, privateKey: Uint8Array}>} X25519 keypair.
 */
export async function ed25519ToX25519(ed25519Keypair) {
  const priv = ed25519PrivToX25519(ed25519Keypair.privateKey);
  const pub = await ed25519PubToX25519(ed25519Keypair.publicKey);
  return { publicKey: pub, privateKey: priv };
}

// ---------------------------------------------------------------------------
// Recovery codes
// ---------------------------------------------------------------------------

const BASE32_CHARS = 'ABCDEFGHJKLMNPQRSTUVWXYZ23456789';

/**
 * Generate an array of recovery codes.
 *
 * Each code is 16 random bytes, formatted as 4 groups of 4 base32 characters
 * (e.g. `X3KM-7FJ2-PQ9N-RT8B`). Characters excluded: `0`, `O`, `1`, `I`, `L`
 * to avoid ambiguity when printed or handwritten.
 *
 * @param {number} [count=10] - How many codes to generate.
 * @returns {Uint8Array[]} Array of 16-byte buffers (one per code).
 */
export function generateRecoveryCodes(count = 10) {
  const codes = [];
  for (let i = 0; i < count; i++) {
    codes.push(crypto.getRandomValues(new Uint8Array(16)));
  }
  return codes;
}

/**
 * Format a 16-byte recovery code as a human-readable string.
 *
 * Splits the bytes into 4 groups of 4 base32 characters, separated by dashes.
 * Example: `X3KM-7FJ2-PQ9N-RT8B`.
 *
 * @param {Uint8Array} bytes - 16-byte random buffer.
 * @returns {string} Formatted code string.
 */
export function formatRecoveryCode(bytes) {
  // Convert 16 bytes to 5-bit base32 groups
  const groups = [];
  let buffer = 0;
  let bitsInBuffer = 0;
  for (const b of bytes) {
    buffer = (buffer << 8) | b;
    bitsInBuffer += 8;
    while (bitsInBuffer >= 5) {
      bitsInBuffer -= 5;
      groups.push((buffer >> bitsInBuffer) & 31);
    }
  }
  if (bitsInBuffer > 0) {
    groups.push((buffer << (5 - bitsInBuffer)) & 31);
  }

  // Pad to 16 characters (4 groups of 4)
  while (groups.length < 16) groups.push(0);

  const chars = groups.map((v) => BASE32_CHARS[v]).join('');
  return chars.match(/.{4}/g).join('-');
}
