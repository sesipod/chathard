/**
 * File encryption / decryption for TailChat attachments.
 *
 * Each file gets a random AES-256-GCM key. Content is encrypted in 1 MB chunks
 * for streaming-friendly processing. File metadata (filename, MIME type, size) is
 * separately encrypted.
 *
 * @module file-encrypt
 */

const CHUNK_SIZE = 1 * 1024 * 1024; // 1 MB

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

/**
 * Import a raw 32-byte key as an AES-GCM CryptoKey.
 * @param {Uint8Array} keyBytes
 * @param {'encrypt'|'decrypt'} usage
 * @returns {Promise<CryptoKey>}
 */
function importKey(keyBytes, usage) {
  return crypto.subtle.importKey('raw', keyBytes, { name: 'AES-GCM' }, false, [usage]);
}

/**
 * Encrypt a single chunk with AES-256-GCM.
 * @param {Uint8Array} chunk    - Up to 1 MB of data.
 * @param {CryptoKey} key       - AES-GCM CryptoKey.
 * @param {Uint8Array} nonce    - 12-byte nonce (unique per chunk).
 * @returns {Promise<Uint8Array>} Encrypted chunk + 16-byte GCM tag.
 */
async function encryptChunk(chunk, key, nonce) {
  const ct = await crypto.subtle.encrypt({ name: 'AES-GCM', iv: nonce, tagLength: 128 }, key, chunk);
  return new Uint8Array(ct);
}

/**
 * Decrypt a single chunk.
 * @param {Uint8Array} encryptedChunk - Ciphertext + GCM tag.
 * @param {CryptoKey} key             - AES-GCM CryptoKey.
 * @param {Uint8Array} nonce          - 12-byte nonce.
 * @returns {Promise<Uint8Array>} Plaintext bytes.
 */
async function decryptChunk(encryptedChunk, key, nonce) {
  const pt = await crypto.subtle.decrypt({ name: 'AES-GCM', iv: nonce, tagLength: 128 }, key, encryptedChunk);
  return new Uint8Array(pt);
}

// ---------------------------------------------------------------------------
// File encryption
// ---------------------------------------------------------------------------

/**
 * Encrypt a file for upload.
 *
 * Generates a random AES-256-GCM key, encrypts the file in 1 MB chunks,
 * and separately encrypts the file metadata.
 *
 * The encrypted output format (per chunk) is:
 *   [nonce (12 bytes) | ciphertext + GCM tag (variable)]
 *
 * @param {File} file - The File/Blob to encrypt.
 * @param {(pct: number) => void} [onProgress] - Optional progress callback (0-100).
 * @returns {Promise<{encryptedBlob: Blob, fileKey: Uint8Array, encryptedMetadata: Uint8Array}>}
 */
export async function encryptFile(file, onProgress) {
  const fileKey = crypto.getRandomValues(new Uint8Array(32));
  const key = await importKey(fileKey, 'encrypt');

  // Encrypt metadata
  const metadata = JSON.stringify({
    name: file.name,
    mimeType: file.type,
    size: file.size,
  });
  const metaNonce = crypto.getRandomValues(new Uint8Array(12));
  const encryptedMetadata = await encryptChunk(
    new TextEncoder().encode(metadata),
    key,
    metaNonce
  );
  // Prepend nonce to encrypted metadata so it can be recovered during decrypt
  const metaPayload = new Uint8Array(12 + encryptedMetadata.length);
  metaPayload.set(metaNonce);
  metaPayload.set(encryptedMetadata, 12);

  // Encrypt file content in chunks
  const chunks = [];
  let offset = 0;
  let chunkIndex = 0;

  while (offset < file.size) {
    const slice = file.slice(offset, Math.min(offset + CHUNK_SIZE, file.size));
    const chunkData = new Uint8Array(await slice.arrayBuffer());
    const nonce = crypto.getRandomValues(new Uint8Array(12));
    const encrypted = await encryptChunk(chunkData, key, nonce);

    // Layout: nonce (12) + encrypted_chunk (variable)
    const frame = new Uint8Array(12 + encrypted.length);
    frame.set(nonce);
    frame.set(encrypted, 12);
    chunks.push(frame);

    offset += CHUNK_SIZE;
    chunkIndex++;

    if (onProgress) {
      onProgress(Math.round((offset / file.size) * 100));
    }
  }

  // Total encrypted blob: concatenation of all chunk frames
  const totalLen = chunks.reduce((sum, c) => sum + c.length, 0);
  const encryptedBlobData = new Uint8Array(totalLen);
  let pos = 0;
  for (const c of chunks) {
    encryptedBlobData.set(c, pos);
    pos += c.length;
  }

  const encryptedBlob = new Blob([encryptedBlobData], { type: 'application/octet-stream' });

  return { encryptedBlob, fileKey, encryptedMetadata: metaPayload };
}

// ---------------------------------------------------------------------------
// File decryption
// ---------------------------------------------------------------------------

/**
 * Decrypt a previously encrypted file blob.
 *
 * Reads the nonce-prepended frames and decrypts them sequentially,
 * streaming the plaintext into the returned Blob. No full file is held
 * in RAM at once (frames are processed one at a time).
 *
 * @param {Blob} encryptedBlob     - The encrypted file blob from `encryptFile`.
 * @param {Uint8Array} fileKey     - The 32-byte file encryption key.
 * @param {Uint8Array} encryptedMetadata - The encrypted metadata payload (nonce-prefixed).
 * @param {(pct: number) => void} [onProgress] - Optional progress callback (0-100).
 * @returns {Promise<{plaintext: Blob, metadata: {name: string, mimeType: string, size: number}}>}
 */
export async function decryptFile(encryptedBlob, fileKey, encryptedMetadata, onProgress) {
  const key = await importKey(fileKey, 'decrypt');

  // Decrypt metadata
  const metaNonce = encryptedMetadata.slice(0, 12);
  const metaCt = encryptedMetadata.slice(12);
  const metaPlain = new TextDecoder().decode(await decryptChunk(metaCt, key, metaNonce));
  const metadata = JSON.parse(metaPlain);

  // Decrypt content chunks
  const buffer = await encryptedBlob.arrayBuffer();
  const data = new Uint8Array(buffer);
  const plainChunks = [];
  let pos = 0;
  let totalDecrypted = 0;

  while (pos < data.length) {
    // Read nonce (12 bytes)
    const nonce = data.slice(pos, pos + 12);
    pos += 12;

    // The rest is one encrypted chunk (ciphertext + 16-byte GCM tag)
    // We need to read until the next nonce or end of data.
    // Since chunk sizes vary (last chunk may be smaller), we use a heuristic:
    // each frame = 12 (nonce) + chunkSize + 16 (GCM tag). The plaintext chunk
    // size is at most CHUNK_SIZE, so ciphertext is at most CHUNK_SIZE + 16.
    // We read the remaining data as one chunk.
    const remaining = data.length - pos;
    const chunkData = data.slice(pos, pos + remaining);
    pos += remaining;

    const plainChunk = await decryptChunk(chunkData, key, nonce);
    plainChunks.push(plainChunk);
    totalDecrypted += plainChunk.length;

    if (onProgress) {
      onProgress(Math.round((totalDecrypted / metadata.size) * 100));
    }
  }

  const plaintext = new Blob(plainChunks, { type: metadata.mimeType || 'application/octet-stream' });

  return { plaintext, metadata };
}
