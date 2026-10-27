/**
 * Message, group-key and group-name encryption.
 *
 * Wire formats (all ciphertext; the server only checks the shape):
 *
 *   1:1 message  : 0x02 | wrapForRecipient(80) | wrapForSender(80) | AES-GCM(sig(64) | json)
 *   group message: 0x03 | epoch(4)                                 | AES-GCM(sig(64) | json)
 *   group key    : sig(64) | wrap(80)                (one per member per epoch)
 *   group name   : epoch(4) | nonce(12) | AES-GCM(name)
 *   wrap(80)     : ephemeralPub(32) | AES-GCM(key32) — ECDH → HKDF → AES-256-GCM
 *
 * Why the sender gets their own wrap: so you can re-read what YOU sent after
 * logging out or from another device. Why every message is signed: the server
 * (or the other party) cannot forge a message that appears to come from you.
 * Why ids are bound into the AEAD data: the server cannot move a ciphertext
 * into a different conversation.
 */
import { ed25519, x25519 } from '@noble/curves/ed25519';
import { hkdf } from '@noble/hashes/hkdf';
import { sha256 } from '@noble/hashes/sha256';
import { aesDecrypt, aesEncrypt, concat, fromUtf8, randomBytes, readU32, u32, utf8 } from './bytes.js';

export const VERSION_DIRECT = 0x02;
export const VERSION_GROUP = 0x03;
const WRAP_LEN = 80;
const SIG_LEN = 64;

// ── Key wrapping (encrypt a 32-byte key to someone's X25519 public key) ──

export async function wrapKey(key32, recipientXPub, context) {
  const eph = x25519.utils.randomPrivateKey();
  const ephPub = x25519.getPublicKey(eph);
  const shared = x25519.getSharedSecret(eph, recipientXPub);
  const okm = hkdf(sha256, shared, ephPub, concat(utf8('tailchat-v2-wrap'), recipientXPub), 44);
  const ct = await aesEncrypt(okm.subarray(0, 32), okm.subarray(32, 44), key32, utf8(context));
  return concat(ephPub, ct); // 32 + 32 + 16 = 80
}

export async function unwrapKey(wrap, myXPriv, context) {
  if (wrap.length !== WRAP_LEN) throw new Error('Bad key envelope');
  const ephPub = wrap.subarray(0, 32);
  const myXPub = x25519.getPublicKey(myXPriv);
  const shared = x25519.getSharedSecret(myXPriv, ephPub);
  const okm = hkdf(sha256, shared, ephPub, concat(utf8('tailchat-v2-wrap'), myXPub), 44);
  return aesDecrypt(okm.subarray(0, 32), okm.subarray(32, 44), wrap.subarray(32), utf8(context));
}

// ── Signing ──

function signBody(identity, ctx, payload) {
  return ed25519.sign(concat(utf8(ctx), new Uint8Array([0]), payload), identity.seed);
}

function verifyBody(edPub, ctx, payload, sig) {
  return ed25519.verify(sig, concat(utf8(ctx), new Uint8Array([0]), payload), edPub);
}

function encodePayload(obj) {
  return utf8(JSON.stringify({ v: 2, mid: crypto.randomUUID(), ts: Date.now(), ...obj }));
}

// ── 1:1 messages ──

const dmCtx = (sender, recipient) => `tailchat-v2|dm|${sender}|${recipient}`;

/**
 * @param {object} body  {type:'text', text} or {type:'file', file:{…}}
 * @returns {{ciphertext: Uint8Array, nonce: Uint8Array}}
 */
export async function sealDirect({ body, identity, senderId, recipientId, recipientXPub }) {
  const payload = encodePayload(body);
  const sig = signBody(identity, dmCtx(senderId, recipientId), payload);
  const key = randomBytes(32);
  const nonce = randomBytes(12);
  const aad = utf8(dmCtx(senderId, recipientId));
  const sealed = await aesEncrypt(key, nonce, concat(sig, payload), aad);
  const forRecipient = await wrapKey(key, recipientXPub, dmCtx(senderId, recipientId) + '|r');
  const forSender = await wrapKey(key, identity.xPub, dmCtx(senderId, recipientId) + '|s');
  return { ciphertext: concat(new Uint8Array([VERSION_DIRECT]), forRecipient, forSender, sealed), nonce };
}

/** Works for both the recipient and the original sender (any of their devices). */
export async function openDirect({ ciphertext, nonce, identity, myId, senderId, recipientId, senderEdPub }) {
  if (ciphertext[0] !== VERSION_DIRECT) throw new Error('Unsupported message version');
  const ctx = dmCtx(senderId, recipientId);
  const mine = myId === senderId;
  const wrap = mine ? ciphertext.subarray(1 + WRAP_LEN, 1 + 2 * WRAP_LEN) : ciphertext.subarray(1, 1 + WRAP_LEN);
  const key = await unwrapKey(wrap, identity.xPriv, ctx + (mine ? '|s' : '|r'));
  const plain = await aesDecrypt(key, nonce, ciphertext.subarray(1 + 2 * WRAP_LEN), utf8(ctx));
  return verifyAndParse(plain, ctx, senderEdPub);
}

function verifyAndParse(plain, ctx, senderEdPub) {
  const sig = plain.subarray(0, SIG_LEN);
  const payload = plain.subarray(SIG_LEN);
  if (!verifyBody(senderEdPub, ctx, payload, sig)) throw new Error('Signature check failed — message may be forged');
  return JSON.parse(fromUtf8(payload));
}

// ── Group messages ──

const groupCtx = (sender, groupId, epoch) => `tailchat-v2|group|${sender}|${groupId}|${epoch}`;

export async function sealGroup({ body, identity, senderId, groupId, epoch, groupKey }) {
  const payload = encodePayload(body);
  const ctx = groupCtx(senderId, groupId, epoch);
  const sig = signBody(identity, ctx, payload);
  const nonce = randomBytes(12);
  const sealed = await aesEncrypt(groupKey, nonce, concat(sig, payload), utf8(ctx));
  return { ciphertext: concat(new Uint8Array([VERSION_GROUP]), u32(epoch), sealed), nonce };
}

export function groupMessageEpoch(ciphertext) {
  if (ciphertext[0] !== VERSION_GROUP) throw new Error('Unsupported message version');
  return readU32(ciphertext, 1);
}

export async function openGroup({ ciphertext, nonce, senderId, groupId, groupKey, senderEdPub }) {
  const epoch = groupMessageEpoch(ciphertext);
  const ctx = groupCtx(senderId, groupId, epoch);
  const plain = await aesDecrypt(groupKey, nonce, ciphertext.subarray(5), utf8(ctx));
  return verifyAndParse(plain, ctx, senderEdPub);
}

// ── Group key envelopes (signed by the group admin) ──

const gkCtx = (groupId, epoch, memberId) => `tailchat-v2|gk|${groupId}|${epoch}|${memberId}`;

export async function sealGroupKey({ groupKey, groupId, epoch, memberId, memberXPub, adminId, identity }) {
  const wrap = await wrapKey(groupKey, memberXPub, gkCtx(groupId, epoch, memberId));
  const sig = signBody(identity, gkCtx(groupId, epoch, memberId) + '|' + adminId, wrap);
  return concat(sig, wrap);
}

export async function openGroupKey({ envelope, groupId, epoch, memberId, identity, signerId, signerEdPub }) {
  const sig = envelope.subarray(0, SIG_LEN);
  const wrap = envelope.subarray(SIG_LEN);
  if (!verifyBody(signerEdPub, gkCtx(groupId, epoch, memberId) + '|' + signerId, wrap, sig)) {
    throw new Error('Group key was not signed by the group admin');
  }
  return unwrapKey(wrap, identity.xPriv, gkCtx(groupId, epoch, memberId));
}

// ── Group names ──

export async function sealGroupName(name, groupId, epoch, groupKey) {
  const nonce = randomBytes(12);
  const ct = await aesEncrypt(groupKey, nonce, utf8(name), utf8(`tailchat-v2|gname|${groupId}|${epoch}`));
  return concat(u32(epoch), nonce, ct);
}

export function groupNameEpoch(blob) {
  return readU32(blob, 0);
}

export async function openGroupName(blob, groupId, groupKey) {
  const epoch = readU32(blob, 0);
  const pt = await aesDecrypt(groupKey, blob.subarray(4, 16), blob.subarray(16), utf8(`tailchat-v2|gname|${groupId}|${epoch}`));
  return fromUtf8(pt);
}
