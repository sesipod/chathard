/**
 * File encryption. Each file gets its own random 256-bit key, which travels
 * to the recipients INSIDE the encrypted chat message together with the file's
 * name, type and size. The server only ever stores the opaque blob.
 *
 * The blob is a sequence of AES-256-GCM frames of 1 MiB plaintext each.
 * Frame i uses the counter nonce i and authenticates (index, isLast) so frames
 * cannot be reordered, dropped, duplicated or truncated without detection.
 */
import { aesDecrypt, aesEncrypt, concat, randomBytes, u32, utf8 } from './bytes.js';

export const CHUNK = 1 << 20;
const TAG = 16;

const nonceFor = (i) => concat(new Uint8Array(4), u32(Math.floor(i / 2 ** 32)), u32(i >>> 0));
const aadFor = (i, last) => concat(utf8('tailchat-v2|file|'), u32(i), new Uint8Array([last ? 1 : 0]));

export const frameCount = (size) => Math.max(1, Math.ceil(size / CHUNK));
export const encryptedSize = (size) => size + frameCount(size) * TAG;

/** @param {Blob} file @returns {{blob: Blob, key: Uint8Array}} */
export async function encryptFile(file, onProgress) {
  const key = randomBytes(32);
  const n = frameCount(file.size);
  const frames = [];
  for (let i = 0; i < n; i++) {
    const plain = new Uint8Array(await file.slice(i * CHUNK, Math.min((i + 1) * CHUNK, file.size)).arrayBuffer());
    frames.push(await aesEncrypt(key, nonceFor(i), plain, aadFor(i, i === n - 1)));
    if (onProgress) onProgress(Math.round(((i + 1) / n) * 100));
  }
  return { blob: new Blob(frames, { type: 'application/octet-stream' }), key };
}

/** @param {Blob} blob @param {Uint8Array} key @param {number} size plaintext size from the message */
export async function decryptFile(blob, key, size, mime = 'application/octet-stream') {
  if (blob.size !== encryptedSize(size)) throw new Error('File is truncated or corrupted');
  const n = frameCount(size);
  const parts = [];
  for (let i = 0; i < n; i++) {
    const start = i * (CHUNK + TAG);
    const end = Math.min(start + CHUNK + TAG, blob.size);
    const frame = new Uint8Array(await blob.slice(start, end).arrayBuffer());
    parts.push(await aesDecrypt(key, nonceFor(i), frame, aadFor(i, i === n - 1)));
  }
  return new Blob(parts, { type: mime });
}
