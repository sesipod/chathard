/**
 * Chats store — manages conversations and messages.
 *
 * Stores:
 *   conversations        {Array}    list of conversation objects, sorted by recency
 *   activeConversationId {string}   the currently selected conversation id
 *   messages             {object}   keyed by conversation id → msg array
 *
 * Actions:
 *   loadConversations()        — GET /api/conversations
 *   loadMessages(convId)       — fetch & store messages for a conversation
 *   addMessage(convId, msg)    — optimistic insert into conversation + messages
 *   markAsRead(convId)         — POST /api/messages/read
 *   setActiveConversation(id)  — update active conversation id
 */
import { writable, derived } from 'svelte/store';
import api from '../api.js';
import { groupDisplayName, openMessages } from '../chat.js';
import { getUsers } from '../directory.js';

// ── Stores ──

/** Ordered list of conversation objects, most recent first. */
export const conversations = writable([]);

/** Id of the currently active/focused conversation. */
export const activeConversationId = writable('');

/** Pending group invitations: [{group_id, invited_by, handle, created_at}]. */
export const invites = writable([]);

/** Security warnings keyed by conversation id, e.g. a contact's key changed. */
export const securityAlerts = writable({});

/** Messages keyed by conversation id: { [convId]: msg[] }. */
export const messages = writable({});

// ── Actions ──

/** Fetch pending group invitations (group names stay hidden until you accept). */
async function loadInvites() {
  try {
    const list = await api.fetchInvites();
    let names = new Map();
    try {
      names = await getUsers(list.map((i) => i.invited_by));
    } catch {
      /* show "someone" if the inviter's key can't be verified */
    }
    invites.set(list.map((i) => ({ ...i, handle: names.get(i.invited_by)?.handle || 'someone' })));
  } catch (err) {
    console.error('Failed to load invitations:', err);
  }
}

/** Fetch all conversations from the server and sort by recency. */
async function loadConversations() {
  try {
    const list = await api.fetchConversations();
    // Group names are encrypted; decrypt them here (needs the group key).
    await Promise.all(
      list.map(async (c) => {
        if (c.type === 'group') c.name = await groupDisplayName(c);
      }),
    );
    // Deduplicate: if server list includes a user we already have as synthetic,
    // merge their data (server version wins for last_message_at, unread_count)
    conversations.update((existing) => {
      // The server has no previews (it only holds ciphertext), so keep the ones decrypted here.
      const previews = new Map(existing.map((c) => [c.user_id || c.id, c.last_message_preview]));
      const merged = list.map((c) => ({ ...c, last_message_preview: previews.get(c.user_id || c.id) }));
      for (const existingConv of existing) {
        const key = existingConv.user_id || existingConv.id;
        const synKey = existingConv.id && existingConv.id.startsWith('_syn_') ? existingConv.user_id : null;
        const alreadyInList = merged.some((c) => (c.user_id || c.id) === key || (synKey && (c.user_id || c.id) === synKey));
        if (!alreadyInList) {
          merged.push(existingConv);
        }
      }
      // Sort by last message timestamp descending
      merged.sort((a, b) => {
        const ta = a.last_message_at || a.last_active || a.created_at || '';
        const tb = b.last_message_at || b.last_active || b.created_at || '';
        return tb.localeCompare(ta);
      });
      return merged;
    });
  } catch (err) {
    console.error('Failed to load conversations:', err);
  }
}

/** Fetch messages for a conversation and store them. */
async function loadMessages(convId) {
  if (!convId) return;
  // Determine if this is a direct conversation or group
  const conv = getConversationSync(convId);
  await loadMessagesWithType(convId, conv && conv.type === 'group');
}

/** Fetch messages with explicit conversation type hint (from WS payload). */
async function loadMessagesWithType(convId, isGroup) {
  if (!convId) return;
  let msgs;
  try {
    const rows = isGroup ? await api.fetchGroupMessages(convId) : await api.fetchMessages({ withUserId: convId });
    // Everything below happens in this browser: the server only had ciphertext.
    msgs = await openMessages(rows, isGroup ? { type: 'group', id: convId } : { type: 'direct', id: convId, user_id: convId });
    // Keep messages that are still being sent (or failed) — a refresh must not erase them.
    messages.update((m) => {
      const pending = (m[convId] || []).filter((x) => String(x.id).startsWith('opt-'));
      const all = [...msgs, ...pending];
      all.sort((a, b) => (a.created_at || '').localeCompare(b.created_at || ''));
      msgs = all;
      return { ...m, [convId]: all };
    });
    const last = msgs[msgs.length - 1];
    if (last) {
      conversations.update((list) =>
        list.map((c) =>
          (c.user_id || c.id) === convId ? { ...c, last_message_preview: last.locked ? '🔒' : last.content } : c,
        ),
      );
    }
    securityAlerts.update((a) => {
      if (!(convId in a)) return a;
      const { [convId]: _gone, ...rest } = a;
      return rest;
    });
  } catch (err) {
    console.error('Failed to load messages:', err);
    if (err.name === 'KeyChangedError') {
      securityAlerts.update((a) => ({ ...a, [convId]: err.message }));
    }
  }
}

/** Optimistically insert a message into a conversation's message list. */
function addMessage(conversationId, msg) {
  messages.update((m) => {
    const existing = m[conversationId] || [];
    // Avoid duplicates by id
    if (msg.id && existing.some((x) => x.id === msg.id)) return m;
    return { ...m, [conversationId]: [...existing, msg] };
  });
  // Also bump the conversation to the top of the list
  conversations.update((list) => {
    const idx = list.findIndex((c) => c.id === conversationId || c.user_id === conversationId);
    if (idx !== -1) {
      const conv = list.splice(idx, 1)[0];
      conv.last_message_at = msg.created_at || new Date().toISOString();
      conv.last_message_preview = msg.content || '';
      list.unshift(conv);
    }
    return list;
  });
}

/** Mark all messages in a conversation as read. */
async function markAsRead(conversationId) {
  try {
    const conv = getConversationSync(conversationId);
    if (conv && conv.type === 'group') {
      await api.markRead({ groupId: conversationId });
    } else {
      await api.markRead({ conversationWith: conversationId });
    }
    // Update local unread count
    conversations.update((list) =>
      list.map((c) => {
        if (c.id === conversationId || c.user_id === conversationId) {
          return { ...c, unread_count: 0 };
        }
        return c;
      })
    );
  } catch (err) {
    console.error('Failed to mark as read:', err);
  }
}

/** Set the active conversation. */
function setActiveConversation(id) {
  activeConversationId.set(id);
}

/** Synchronously look up a conversation from the current store value. */
function getConversationSync(id) {
  let found = null;
  conversations.subscribe((list) => {
    found = list.find((c) => c.id === id || c.user_id === id);
  })();
  return found;
}

// ── Derived helpers ──

/** Messages for the active conversation. */
export const activeMessages = derived(
  [activeConversationId, messages],
  ([$activeId, $messages]) => ($activeId ? $messages[$activeId] || [] : [])
);

/** The active conversation object. */
export const activeConversation = derived(
  [activeConversationId, conversations],
  ([$activeId, $convs]) => $convs.find((c) => (c.user_id || c.id) === $activeId || c.id === $activeId) || null
);

export const chatStore = {
  securityAlerts,
  invites,
  loadInvites,
  conversations,
  activeConversationId,
  messages,
  activeMessages,
  activeConversation,
  loadConversations,
  loadMessages,
  loadMessagesWithType,
  addMessage,
  markAsRead,
  setActiveConversation,
};
