/**
 * TailChat REST + WebSocket client.
 *
 * All authenticated requests attach the session token from sessionStorage
 * as a Bearer token in the Authorization header.
 *
 * Usage:
 *   import api from '../lib/api.js';
 *   const conversations = await api.fetchConversations();
 *   const ws = api.connectWebSocket(token);
 */

import { toB64 } from './crypto/bytes.js';

// ── Helpers ──

/** Read the current session token. */
function getToken() {
  return sessionStorage.getItem('tailchat-token') || '';
}

/** Build headers with optional JSON content-type and auth. */
function headers(authenticated = true) {
  const h = { 'Content-Type': 'application/json' };
  if (authenticated) {
    const t = getToken();
    if (t) h['Authorization'] = `Bearer ${t}`;
  }
  return h;
}

/** Throw a readable error on non-OK responses (the server replies in plain text). */
async function throwIfNotOk(res) {
  if (!res.ok) {
    let msg = '';
    try {
      msg = (await res.text()).trim();
      if (msg.startsWith('{')) {
        const body = JSON.parse(msg);
        msg = body.error || body.message || msg;
      }
    } catch {
      /* fall through */
    }
    if (res.status === 401) {
      sessionStorage.removeItem('tailchat-token');
      sessionStorage.removeItem('tailchat-user-id');
    }
    throw new Error(msg || res.statusText || `HTTP ${res.status}`);
  }
}

const b64 = (bytes) => toB64(bytes);

// ── Public API ──

const api = {
  // ── Registration ──
  // Only public keys are sent. The account key itself never leaves the browser.
  async register(handle, edPub, xPub, authPub) {
    const res = await fetch('/api/register', {
      method: 'POST',
      headers: headers(false),
      body: JSON.stringify({
        handle,
        public_key_ed25519: b64(edPub),
        public_key_x25519: b64(xPub),
        derived_public_key_ed25519: b64(authPub),
      }),
    });
    await throwIfNotOk(res);
    return res.json();
  },

  // ── Auth ──
  async challenge(handle) {
    const res = await fetch('/api/auth/challenge', {
      method: 'POST',
      headers: headers(false),
      body: JSON.stringify({ handle }),
    });
    await throwIfNotOk(res);
    return res.json(); // { challenge }
  },

  async verify(handle, challenge, signature) {
    const res = await fetch('/api/auth/verify', {
      method: 'POST',
      headers: headers(false),
      body: JSON.stringify({ handle, challenge, signature }),
    });
    await throwIfNotOk(res);
    return res.json(); // { token, user_id }
  },

  async logout() {
    const res = await fetch('/api/auth/logout', {
      method: 'POST',
      headers: headers(),
    });
    // Always clear local state even if server fails
    sessionStorage.removeItem('tailchat-token');
    sessionStorage.removeItem('tailchat-user-id');
    return res;
  },

  // ── Messages ──
  async fetchMessages({ withUserId, groupId, after, before, limit } = {}) {
    const params = new URLSearchParams();
    if (withUserId) params.set('with', withUserId);
    if (groupId) params.set('group_id', groupId);
    if (after) params.set('after', after);
    if (before) params.set('before', before);
    if (limit) params.set('limit', String(limit));
    const qs = params.toString();
    const res = await fetch(`/api/messages${qs ? `?${qs}` : ''}`, { headers: headers() });
    await throwIfNotOk(res);
    const data = await res.json();
    return data.messages ?? []; // server wraps in { messages: [...] }, may be null
  },

  async sendMessage({ recipientId, groupId, ciphertext, nonce, expiresIn }) {
    const body = {
      recipient_id: recipientId || '',
      group_id: groupId || '',
      ciphertext: b64(ciphertext),
      nonce: b64(nonce),
    };
    if (expiresIn) body.expires_in = expiresIn;
    const res = await fetch('/api/messages', {
      method: 'POST',
      headers: headers(),
      body: JSON.stringify(body),
    });
    await throwIfNotOk(res);
    return res.json();
  },

  async markRead({ conversationWith, groupId } = {}) {
    const res = await fetch('/api/messages/read', {
      method: 'POST',
      headers: headers(),
      body: JSON.stringify({
        conversation_with: conversationWith || null,
        group_id: groupId || null,
      }),
    });
    await throwIfNotOk(res);
    // 204 No Content — no JSON body
  },

  async updateRetention({ conversationWith, groupId, expiresIn }) {
    const res = await fetch('/api/messages/retention', {
      method: 'PATCH',
      headers: headers(),
      body: JSON.stringify({
        conversation_with: conversationWith || null,
        group_id: groupId || null,
        expires_in: expiresIn,
      }),
    });
    await throwIfNotOk(res);
    // 204 No Content — no JSON body
  },

  // ── Conversations ──
  async fetchConversations() {
    const res = await fetch('/api/conversations', { headers: headers() });
    await throwIfNotOk(res);
    const data = await res.json();
    return data.conversations ?? []; // server wraps in { conversations: [...] }, may be null
  },

  // ── Users ──
  async searchUsers(handle) {
    const res = await fetch(`/api/users/search?handle=${encodeURIComponent(handle)}`, {
      headers: headers(),
    });
    await throwIfNotOk(res);
    return res.json();
  },

  async lookupUsers(ids) {
    const res = await fetch(`/api/users/lookup?ids=${ids.map(encodeURIComponent).join(',')}`, { headers: headers() });
    await throwIfNotOk(res);
    return (await res.json()).users ?? [];
  },

  // ── Me ──
  async getMe() {
    const res = await fetch('/api/me', { headers: headers() });
    await throwIfNotOk(res);
    return res.json();
  },

  // ── Groups ──
  async createGroup({ id, encryptedName, members }) {
    const res = await fetch('/api/groups', {
      method: 'POST',
      headers: headers(),
      body: JSON.stringify({
        id,
        encrypted_name: b64(encryptedName),
        members: members.map((m) => ({ user_id: m.userId, envelope: b64(m.envelope) })),
      }),
    });
    await throwIfNotOk(res);
    return res.json();
  },

  async fetchGroupKeys(groupId) {
    const res = await fetch(`/api/groups/${groupId}/keys`, { headers: headers() });
    await throwIfNotOk(res);
    return res.json(); // { group, envelopes:[{epoch, signer_id, envelope}], members:[…] }
  },

  async addMember(groupId, userId, epoch, envelope) {
    const res = await fetch(`/api/groups/${groupId}/members`, {
      method: 'POST',
      headers: headers(),
      body: JSON.stringify({ user_id: userId, epoch, envelope: b64(envelope) }),
    });
    await throwIfNotOk(res);
  },

  async leaveGroup(groupId, userId) {
    const res = await fetch(`/api/groups/${groupId}/members/${userId}`, { method: 'DELETE', headers: headers() });
    await throwIfNotOk(res);
  },

  /** Admin: remove `userId` (optional) and move the group to a new key epoch. */
  async rotateGroup(groupId, { userId, epoch, encryptedName, members }) {
    const res = await fetch(`/api/groups/${groupId}/${userId ? 'remove' : 'rotate'}`, {
      method: 'POST',
      headers: headers(),
      body: JSON.stringify({
        user_id: userId || '',
        epoch,
        encrypted_name: b64(encryptedName),
        members: members.map((m) => ({ user_id: m.userId, envelope: b64(m.envelope) })),
      }),
    });
    await throwIfNotOk(res);
  },

  async fetchGroupMessages(groupId, { before, limit } = {}) {
    const params = new URLSearchParams();
    if (before) params.set('before', before);
    if (limit) params.set('limit', String(limit));
    const qs = params.toString();
    const res = await fetch(`/api/groups/${groupId}/messages${qs ? `?${qs}` : ''}`, { headers: headers() });
    await throwIfNotOk(res);
    const data = await res.json();
    return data.messages ?? [];
  },

  async fetchMyGroups() {
    const res = await fetch('/api/groups', { headers: headers() });
    await throwIfNotOk(res);
    return res.json();
  },

  // ── Messages: Hide (per-user delete) ──
  async hideMessage(messageId) {
    const res = await fetch('/api/messages/hide', {
      method: 'POST',
      headers: headers(),
      body: JSON.stringify({ message_id: messageId }),
    });
    await throwIfNotOk(res);
  },

  async batchHideMessages(messageIds) {
    const res = await fetch('/api/messages/batch-hide', {
      method: 'POST',
      headers: headers(),
      body: JSON.stringify({ message_ids: messageIds }),
    });
    await throwIfNotOk(res);
  },

  // ── Files ──
  /** Upload an already-encrypted blob. Names/types/keys are NOT sent. */
  async uploadFile(blob, expiresIn, targetId, targetType) {
    const form = new FormData();
    form.append('file', blob, 'blob');
    form.append('target_id', targetId);
    form.append('target_type', targetType);
    if (expiresIn) form.append('expires_in', expiresIn);
    const res = await fetch('/api/files/upload', {
      method: 'POST',
      headers: { Authorization: `Bearer ${getToken()}` }, // no Content-Type — browser sets multipart
      body: form,
    });
    await throwIfNotOk(res);
    return res.json();
  },

  async linkFileToMessage(fileId, messageId) {
    const res = await fetch(`/api/files/${fileId}/link-message`, {
      method: 'PATCH',
      headers: headers(),
      body: JSON.stringify({ message_id: messageId }),
    });
    await throwIfNotOk(res);
  },

  async fetchConversationFiles(convId, isGroup) {
    const type = isGroup ? 'group' : 'direct';
    const res = await fetch(`/api/conversations/${convId}/files?type=${type}`, { headers: headers() });
    await throwIfNotOk(res);
    return res.json();
  },

  async downloadFile(fileId) {
    const res = await fetch(`/api/files/${fileId}`, { headers: headers() });
    await throwIfNotOk(res);
    return res.blob();
  },

  async deleteFile(fileId) {
    const res = await fetch(`/api/files/${fileId}`, {
      method: 'DELETE',
      headers: headers(),
    });
    await throwIfNotOk(res);
    return res.json();
  },

  // ── Health ──
  async health() {
    const res = await fetch('/api/health');
    return res.json();
  },

  // ── WebSocket ──
  /**
   * Open a WebSocket that reconnects with backoff and keeps its handlers across
   * reconnects. `onOpen(isReconnect)` lets the caller resync anything missed.
   * @returns {{send(obj): void, close(): void, readonly ready: boolean}}
   */
  connectWebSocket(token, { onMessage, onOpen, onClose } = {}, { maxRetries = 10, baseDelay = 1000 } = {}) {
    const wsPrefix = location.protocol === 'https:' ? 'wss' : 'ws';
    const url = `${wsPrefix}://${location.host}/ws?token=${encodeURIComponent(token)}`;
    let ws = null;
    let retries = 0;
    let closed = false;
    let everOpened = false;

    const open = () => {
      ws = new WebSocket(url);
      ws.addEventListener('open', () => {
        const reconnect = everOpened;
        everOpened = true;
        retries = 0;
        onOpen?.(reconnect);
      });
      ws.addEventListener('message', (ev) => onMessage?.(ev));
      ws.addEventListener('close', () => {
        onClose?.();
        if (closed || retries >= maxRetries) return;
        const delay = Math.min(baseDelay * Math.pow(2, retries), 30000);
        retries++;
        setTimeout(open, delay);
      });
    };
    open();

    return {
      send(obj) {
        if (ws && ws.readyState === WebSocket.OPEN) ws.send(JSON.stringify(obj));
      },
      close() {
        closed = true;
        ws?.close();
      },
      get ready() {
        return !!ws && ws.readyState === WebSocket.OPEN;
      },
    };
  },
};

export default api;
