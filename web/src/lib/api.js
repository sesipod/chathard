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

/** Throw a structured error on non-OK responses. */
async function throwIfNotOk(res) {
  if (!res.ok) {
    let msg;
    try {
      const body = await res.json();
      msg = body.error || body.message || res.statusText;
    } catch {
      msg = res.statusText || `HTTP ${res.status}`;
    }
    // Redirect to login on 401
    if (res.status === 401) {
      sessionStorage.removeItem('tailchat-token');
      sessionStorage.removeItem('tailchat-user-id');
    }
    throw new Error(msg);
  }
}

// ── Public API ──

const api = {
  // ── Registration ──
  async register(handle, publicKeyEd25519, publicKeyX25519, derivedPublicKey, encryptedKeyBackups) {
    const res = await fetch('/api/register', {
      method: 'POST',
      headers: headers(false),
      body: JSON.stringify({
        handle,
        public_key_ed25519: Array.from(publicKeyEd25519),
        public_key_x25519: Array.from(publicKeyX25519),
        derived_public_key_ed25519: Array.from(derivedPublicKey),
        encrypted_key_backups: encryptedKeyBackups,
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

  async sendMessage({ recipientId, groupId, ciphertext, ephemeralPub, nonce, expiresIn }) {
    const body = {
      recipient_id: recipientId || null,
      group_id: groupId || null,
      ciphertext: Array.from(ciphertext),
      ephemeral_public_key: Array.from(ephemeralPub),
      nonce: Array.from(nonce),
    };
    if (expiresIn) {
      body.expires_in = expiresIn;
    }
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

  // ── Recovery ──
  async recover(userId, recoveryCodeHash) {
    const res = await fetch('/api/recover', {
      method: 'POST',
      headers: headers(),
      body: JSON.stringify({ user_id: userId, recovery_code_hash: recoveryCodeHash }),
    });
    await throwIfNotOk(res);
    return res.json();
  },

  // ── Me ──
  async getMe() {
    const res = await fetch('/api/me', { headers: headers() });
    await throwIfNotOk(res);
    return res.json();
  },

  // ── Groups ──
  async createGroup(encryptedName, members) {
    const res = await fetch('/api/groups', {
      method: 'POST',
      headers: headers(),
      body: JSON.stringify({
        encrypted_name: Array.from(encryptedName),
        encrypted_symmetric_key: [],
        members: members.map(id => ({
          user_id: id,
          encrypted_group_key: [],
          encrypted_member_metadata: [],
        })),
      }),
    });
    await throwIfNotOk(res);
    return res.json();
  },

  async addMember(groupId, userId, encryptedGroupKey) {
    const res = await fetch(`/api/groups/${groupId}/members`, {
      method: 'POST',
      headers: headers(),
      body: JSON.stringify({
        user_id: userId,
        encrypted_group_key: Array.from(encryptedGroupKey),
      }),
    });
    await throwIfNotOk(res);
    return res.json();
  },

  async removeMember(groupId, userId) {
    const res = await fetch(`/api/groups/${groupId}/members/${userId}`, {
      method: 'DELETE',
      headers: headers(),
    });
    await throwIfNotOk(res);
    return res.json();
  },

  async fetchGroupMessages(groupId) {
    const res = await fetch(`/api/groups/${groupId}/messages`, { headers: headers() });
    await throwIfNotOk(res);
    const data = await res.json();
    return data.messages ?? []; // server wraps in { messages: [...] }, may be null
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
  async uploadFile(file, expiresIn, targetId, targetType) {
    const form = new FormData();
    form.append('file', file);
    form.append('encrypted_metadata', '');
    if (expiresIn) {
      form.append('expires_in', expiresIn);
    }
    if (targetId) {
      form.append('target_id', targetId);
    }
    if (targetType) {
      form.append('target_type', targetType);
    }
    const res = await fetch('/api/files/upload', {
      method: 'POST',
      headers: { Authorization: `Bearer ${getToken()}` }, // no Content-Type — browser sets multipart
      body: form,
    });
    await throwIfNotOk(res);
    return res.json();
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
   * Open a WebSocket connection with automatic reconnect.
   * @param {string} token  Session token for authentication.
   * @param {object} [opts]
   * @param {number} [opts.maxRetries=10]
   * @param {number} [opts.baseDelay=1000]
   * @returns {WebSocket}
   */
  connectWebSocket(token, { maxRetries = 10, baseDelay = 1000 } = {}) {
    const wsPrefix = location.protocol === 'https:' ? 'wss' : 'ws';
    const url = `${wsPrefix}://${location.host}/ws?token=${encodeURIComponent(token)}`;
    let ws = new WebSocket(url);
    let retries = 0;

    ws.addEventListener('close', () => {
      if (retries >= maxRetries) return;
      const delay = Math.min(baseDelay * Math.pow(2, retries), 30000);
      retries++;
      setTimeout(() => {
        ws = new WebSocket(url);
      }, delay);
    });

    return ws;
  },
};

export default api;
