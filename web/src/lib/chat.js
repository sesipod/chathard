/**
 * The only place that turns plaintext into ciphertext and back.
 * UI components call these functions; the server only ever sees what they
 * return to api.js.
 */
import api from './api.js';
import { getUser, getUsers } from './directory.js';
import { getIdentity } from './keyring.js';
import { fromB64 } from './crypto/bytes.js';
import {
  groupMessageEpoch, groupNameEpoch, openDirect, openGroup, openGroupKey, openGroupName,
  sealDirect, sealGroup, sealGroupKey, sealGroupName,
} from './crypto/messages.js';
import { decryptFile, encryptFile } from './crypto/files.js';
import { randomBytes } from './crypto/bytes.js';
import { toB64 } from './crypto/bytes.js';

const myId = () => sessionStorage.getItem('tailchat-user-id') || '';

async function identity() {
  const id = await getIdentity();
  if (!id) throw new Error('Your account key is not loaded. Sign in again.');
  return id;
}

// ── Group state (keys per epoch, roster, decrypted name) ──

/** @type {Map<string, {group: object, keys: Map<number, Uint8Array>, members: object[], name: string, untrusted: number}>} */
const groups = new Map();

export function forgetGroupState() {
  groups.clear();
}

/** Drop cached keys for a group so the next use re-fetches them. */
export function invalidateGroup(groupId) {
  groups.delete(groupId);
}

export async function loadGroup(groupId, force = false) {
  if (!force && groups.has(groupId)) return groups.get(groupId);
  const me = await identity();
  const data = await api.fetchGroupKeys(groupId);
  const group = data.group;
  const keys = new Map();
  let untrusted = 0;

  for (const e of data.envelopes) {
    // Only the group's admin may hand out keys. Anything else is ignored.
    if (e.signer_id !== group.created_by) {
      untrusted++;
      continue;
    }
    try {
      const signer = await getUser(e.signer_id);
      const key = await openGroupKey({
        envelope: fromB64(e.envelope), groupId, epoch: e.epoch, memberId: myId(),
        identity: me, signerId: e.signer_id, signerEdPub: signer.edPub,
      });
      keys.set(e.epoch, key);
    } catch (err) {
      untrusted++;
      console.warn(`group ${groupId} epoch ${e.epoch}: key rejected —`, err.message);
    }
  }

  // A server that wants a removed member to keep reading could claim the group is
  // still on an old key generation. Remember the newest epoch we have ever seen
  // and refuse to go backwards. (It cannot stop a server hiding a NEW epoch from
  // us entirely; see "Known limits" in the README.)
  const seenKey = `tailchat-epoch-${groupId}`;
  const seen = Number(localStorage.getItem(seenKey) || 0);
  if (group.epoch < seen) {
    throw new Error('The server offered an older group key than you have already seen. Not sending — possible tampering.');
  }
  localStorage.setItem(seenKey, String(Math.max(seen, group.epoch)));

  const state = { group, keys, members: data.members, name: 'Group', untrusted };
  state.name = await decryptGroupName(group, keys);
  groups.set(groupId, state);
  return state;
}

async function decryptGroupName(group, keys) {
  try {
    const blob = fromB64(group.encrypted_name);
    const key = keys.get(groupNameEpoch(blob));
    return key ? await openGroupName(blob, group.id, key) : '🔒 Group';
  } catch {
    return '🔒 Group';
  }
}

/** Decrypt names for the conversation list (loads keys for each group). */
export async function groupDisplayName(conv) {
  try {
    return (await loadGroup(conv.id)).name;
  } catch {
    return '🔒 Group';
  }
}

export async function createGroup(name, memberUsers) {
  const me = await identity();
  const meId = myId();
  const groupId = crypto.randomUUID();
  const groupKey = randomBytes(32);
  const roster = await getUsers([...new Set([...memberUsers.map((u) => u.id || u.uuid), meId])]);

  const members = [];
  for (const [userId, u] of roster) {
    members.push({
      userId,
      envelope: await sealGroupKey({
        groupKey, groupId, epoch: 0, memberId: userId, memberXPub: userId === meId ? me.xPub : u.xPub,
        adminId: meId, identity: me,
      }),
    });
  }
  await api.createGroup({ id: groupId, encryptedName: await sealGroupName(name, groupId, 0, groupKey), members });
  return groupId;
}

export async function addGroupMember(groupId, user) {
  const me = await identity();
  const state = await loadGroup(groupId, true);
  if (state.group.created_by !== myId()) throw new Error('Only the group admin can add people');
  const key = state.keys.get(state.group.epoch);
  if (!key) throw new Error('Group key unavailable');
  const u = await getUser(user.id || user.uuid);
  const envelope = await sealGroupKey({
    groupKey: key, groupId, epoch: state.group.epoch, memberId: u.uuid, memberXPub: u.xPub, adminId: myId(), identity: me,
  });
  await api.addMember(groupId, u.uuid, state.group.epoch, envelope);
  groups.delete(groupId);
}

/** Admin: drop `removeUserId` (optional) and issue a fresh key to everyone who remains. */
export async function rotateGroupKey(groupId, removeUserId = '', { onlyIfNeeded = false } = {}) {
  const me = await identity();
  const state = await loadGroup(groupId, true);
  if (state.group.created_by !== myId()) throw new Error('Only the group admin can change members');
  // Called from a possibly stale conversation list: if someone already rotated, do nothing.
  if (onlyIfNeeded && !state.group.needs_rotation) return;
  const remaining = state.members.filter((m) => m.id !== removeUserId);
  const roster = await getUsers(remaining.map((m) => m.id));
  const epoch = state.group.epoch + 1;
  const groupKey = randomBytes(32);
  const members = [];
  for (const [userId, u] of roster) {
    members.push({
      userId,
      envelope: await sealGroupKey({
        groupKey, groupId, epoch, memberId: userId, memberXPub: userId === myId() ? me.xPub : u.xPub,
        adminId: myId(), identity: me,
      }),
    });
  }
  const plainName = state.name.startsWith('🔒') ? 'Group' : state.name;
  await api.rotateGroup(groupId, {
    userId: removeUserId, epoch, encryptedName: await sealGroupName(plainName, groupId, epoch, groupKey), members,
  });
  groups.delete(groupId);
}

// ── Sending ──

async function sendBody(conv, body, expiresIn) {
  const me = await identity();
  const meId = myId();

  if (conv.type === 'group') {
    // The admin's browser may be rotating the key right now; refresh and retry a few times.
    for (let attempt = 0; attempt < 4; attempt++) {
      if (attempt > 0) await new Promise((r) => setTimeout(r, 250 * attempt));
      const state = await loadGroup(conv.id, attempt > 0);
      const key = state.keys.get(state.group.epoch);
      if (!key) throw new Error('You do not have the current group key yet');
      const { ciphertext, nonce } = await sealGroup({
        body, identity: me, senderId: meId, groupId: conv.id, epoch: state.group.epoch, groupKey: key,
      });
      try {
        return await api.sendMessage({ groupId: conv.id, ciphertext, nonce, expiresIn });
      } catch (err) {
        if (attempt < 3 && /epoch/i.test(err.message)) continue; // key rotated; refresh and retry
        throw err;
      }
    }
  }

  const recipientId = conv.user_id || conv.id;
  const recipient = await getUser(recipientId);
  const { ciphertext, nonce } = await sealDirect({
    body, identity: me, senderId: meId, recipientId, recipientXPub: recipient.xPub,
  });
  return api.sendMessage({ recipientId, ciphertext, nonce, expiresIn });
}

export const sendText = (conv, text, expiresIn) => sendBody(conv, { type: 'text', text }, expiresIn);

/**
 * Encrypt a file in the browser, upload the opaque blob, then send its key,
 * name and type to the recipients inside an encrypted message.
 */
export async function sendFile(conv, file, expiresIn, onProgress) {
  const targetId = conv.type === 'group' ? conv.id : conv.user_id || conv.id;
  const { blob, key } = await encryptFile(file, onProgress);
  const up = await api.uploadFile(blob, expiresIn, targetId, conv.type === 'group' ? 'group' : 'direct');
  const body = {
    type: 'file',
    file: { id: up.file_id, key: toB64(key), name: file.name || 'file', mime: file.type || '', size: file.size },
  };
  const res = await sendBody(conv, body, expiresIn);
  api.linkFileToMessage(up.file_id, res.id).catch(() => {});
  return { result: res, body };
}

// ── Receiving ──

const LOCKED = (why) => ({ kind: 'locked', text: why });

/** Decrypt one server message row. Never throws; failures become a locked placeholder. */
async function openRow(row, conv, me, users) {
  try {
    const sender = users.get(row.sender_id);
    const ciphertext = fromB64(row.ciphertext);
    const nonce = fromB64(row.nonce);
    let body;
    if (conv.type === 'group') {
      const state = await loadGroup(conv.id);
      let key = state.keys.get(groupMessageEpoch(ciphertext));
      if (!key) {
        const fresh = await loadGroup(conv.id, true);
        key = fresh.keys.get(groupMessageEpoch(ciphertext));
      }
      if (!key) return LOCKED('🔒 Sent before you joined this group');
      body = await openGroup({ ciphertext, nonce, senderId: row.sender_id, groupId: conv.id, groupKey: key, senderEdPub: sender.edPub });
    } else {
      body = await openDirect({
        ciphertext, nonce, identity: me, myId: myId(), senderId: row.sender_id, recipientId: row.recipient_id, senderEdPub: sender.edPub,
      });
    }
    return body;
  } catch (err) {
    console.warn('could not open message', row.id, err.message);
    return LOCKED('🔒 Could not be decrypted');
  }
}

/** Decrypt a page of messages for display. Returns UI-ready message objects. */
export async function openMessages(rows, conv) {
  const me = await identity();
  const users = await getUsers(rows.map((r) => r.sender_id).concat(myId() ? [myId()] : []));
  const seen = new Set();
  const out = [];
  for (const row of rows) {
    const body = await openRow(row, conv, me, users);
    if (body.mid) {
      if (seen.has(body.mid)) continue; // server replayed a message
      seen.add(body.mid);
    }
    out.push({
      id: row.id,
      sender_id: row.sender_id,
      created_at: row.created_at,
      expires_at: row.expires_at,
      status: row.read_at ? 'read' : 'sent',
      is_own: row.sender_id === myId(),
      locked: body.kind === 'locked',
      content: body.type === 'text' ? body.text : body.type === 'file' ? `📎 ${body.file?.name || 'file'}` : body.text || '',
      file: body.type === 'file' ? body.file : null,
    });
  }
  return out;
}

/** Download an encrypted file and decrypt it locally. */
export async function fetchFile(file) {
  const blob = await api.downloadFile(file.id);
  return decryptFile(blob, fromB64(file.key), file.size, file.mime || 'application/octet-stream');
}

/** Only these types are shown inline; everything else is download-only. */
export const isInlineImage = (file) => /^image\/(png|jpe?g|gif|webp|avif|bmp)$/i.test(file?.mime || '');

/** Strip path tricks and control characters from a name before using it as a download name. */
export function safeFileName(name) {
  return (String(name || 'file').replace(/[\\/\u0000-\u001f\u007f]/g, '_').replace(/^\.+/, '_').slice(0, 120)) || 'file';
}
