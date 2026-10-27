/**
 * Account identity.
 *
 * Everything a user is comes from ONE random 256-bit secret, the "account key".
 * It is shown to the user once at sign-up and is what they enter to sign in on
 * any device. The server never receives it or anything derived from it that
 * could unlock data, so a server operator cannot read messages.
 *
 *   accountKey (32 bytes)
 *     ├─ Ed25519 identity key      signs every message / group-key envelope
 *     ├─ X25519 encryption key     receives message keys and group keys
 *     └─ Ed25519 login key (HKDF)  proves identity to the server (never the identity key)
 */
import { ed25519, x25519, edwardsToMontgomeryPriv, edwardsToMontgomeryPub } from '@noble/curves/ed25519';
import { hkdf } from '@noble/hashes/hkdf';
import { sha256 } from '@noble/hashes/sha256';
import { sha512 } from '@noble/hashes/sha512';
import { concat, equal, randomBytes, utf8 } from './bytes.js';

const ALPHABET = 'ABCDEFGHJKLMNPQRSTUVWXYZ23456789'; // no 0/O/1/I/L

export function newAccountKey() {
  return randomBytes(32);
}

/** Derive every key from the account key. Pure and deterministic. */
export function deriveIdentity(seed) {
  if (!(seed instanceof Uint8Array) || seed.length !== 32) throw new Error('Account key must be 32 bytes');
  const edPub = ed25519.getPublicKey(seed);
  const xPriv = edwardsToMontgomeryPriv(seed);
  const xPub = x25519.getPublicKey(xPriv);
  const authPriv = hkdf(sha256, seed, utf8('tailchat-v2'), utf8('login-key'), 32);
  const authPub = ed25519.getPublicKey(authPriv);
  return { seed, edPub, xPriv, xPub, authPriv, authPub };
}

/** True if an X25519 key really is the Montgomery form of an Ed25519 key. */
export function keysConsistent(edPub, xPub) {
  try {
    return equal(edwardsToMontgomeryPub(edPub), xPub);
  } catch {
    return false;
  }
}

// ── Account key text format ──
// 32 key bytes + 2 checksum bytes → base32 → "XXXXX-XXXXX-…" (11 groups of 5).

function toBase32(bytes) {
  let out = '';
  let buf = 0;
  let bits = 0;
  for (const b of bytes) {
    buf = (buf << 8) | b;
    bits += 8;
    while (bits >= 5) {
      bits -= 5;
      out += ALPHABET[(buf >> bits) & 31];
    }
    buf &= (1 << bits) - 1;
  }
  if (bits > 0) out += ALPHABET[(buf << (5 - bits)) & 31];
  return out;
}

function fromBase32(str, byteLen) {
  const out = new Uint8Array(byteLen);
  let buf = 0;
  let bits = 0;
  let o = 0;
  for (const ch of str) {
    const v = ALPHABET.indexOf(ch);
    if (v < 0) throw new Error('Invalid character in account key');
    buf = (buf << 5) | v;
    bits += 5;
    if (bits >= 8) {
      bits -= 8;
      if (o < byteLen) out[o++] = (buf >> bits) & 255;
      buf &= (1 << bits) - 1;
    }
  }
  return out;
}

export function formatAccountKey(seed) {
  const check = sha256(concat(utf8('tailchat-key-check'), seed)).subarray(0, 2);
  return toBase32(concat(seed, check)).match(/.{1,5}/g).join('-');
}

/** Parse what the user typed/pasted; throws a friendly error on typos. */
export function parseAccountKey(text) {
  const clean = String(text).toUpperCase().replace(/[\s-]/g, '');
  if (clean.length !== 55) throw new Error('That account key is the wrong length');
  const raw = fromBase32(clean, 34);
  const seed = raw.subarray(0, 32);
  const check = sha256(concat(utf8('tailchat-key-check'), seed)).subarray(0, 2);
  if (!equal(check, raw.subarray(32))) throw new Error('Account key has a typo — check it and try again');
  return new Uint8Array(seed);
}

// ── Fingerprints & safety numbers ──

export function fingerprint(edPub) {
  return Array.from(sha256(concat(utf8('tailchat-fp'), edPub)).subarray(0, 8), (b) => b.toString(16).padStart(2, '0'))
    .join('')
    .match(/.{4}/g)
    .join(' ');
}

/**
 * 60-digit number both people compute identically from their two identity
 * keys. If it matches on both screens (read aloud / compared in person), no
 * one — including the server — swapped a key.
 */
export function safetyNumber(edPubA, edPubB) {
  const [lo, hi] = Buffer_compare(edPubA, edPubB) <= 0 ? [edPubA, edPubB] : [edPubB, edPubA];
  const h = sha512(concat(utf8('tailchat-safety-v1'), lo, hi));
  const groups = [];
  for (let i = 0; i < 12; i++) {
    const n = ((h[i * 4] << 24) | (h[i * 4 + 1] << 16) | (h[i * 4 + 2] << 8) | h[i * 4 + 3]) >>> 0;
    groups.push(String(n % 100000).padStart(5, '0'));
  }
  return groups.join(' ');
}

function Buffer_compare(a, b) {
  for (let i = 0; i < Math.min(a.length, b.length); i++) if (a[i] !== b[i]) return a[i] - b[i];
  return a.length - b.length;
}

export function signLoginChallenge(identity, challengeBytes) {
  return ed25519.sign(challengeBytes, identity.authPriv);
}
