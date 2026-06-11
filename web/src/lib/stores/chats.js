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

// ── Stores ──

/** Ordered list of conversation objects, most recent first. */
export const conversations = writable([]);

/** Id of the currently active/focused conversation. */
export const activeConversationId = writable('');

/** Messages keyed by conversation id: { [convId]: msg[] }. */
export const messages = writable({});

// ── Actions ──

/** Fetch all conversations from the server and sort by recency. */
async function loadConversations() {
  try {
    const list = await api.fetchConversations();
    // Ensure sorted by last message timestamp descending
    list.sort((a, b) => {
      const ta = a.last_message_at || a.created_at || '';
      const tb = b.last_message_at || b.created_at || '';
      return tb.localeCompare(ta);
    });
    conversations.set(list);
  } catch (err) {
    console.error('Failed to load conversations:', err);
    conversations.set([]);
  }
}

/** Fetch messages for a conversation and store them. */
async function loadMessages(convId) {
  if (!convId) return;
  // Determine if this is a direct conversation or group
  const conv = getConversationSync(convId);
  let msgs;
  try {
    if (conv && conv.type === 'group') {
      msgs = await api.fetchGroupMessages(convId);
    } else {
      msgs = await api.fetchMessages({ withUserId: convId });
    }
    msgs.sort((a, b) => (a.created_at || '').localeCompare(b.created_at || ''));
    messages.update((m) => ({ ...m, [convId]: msgs }));
  } catch (err) {
    console.error('Failed to load messages:', err);
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
      conv.last_message_preview = msg.ciphertext
        ? 'Encrypted message'
        : (msg.content || '');
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
  ([$activeId, $convs]) => $convs.find((c) => c.id === $activeId || c.user_id === $activeId) || null
);

export const chatStore = {
  conversations,
  activeConversationId,
  messages,
  activeMessages,
  activeConversation,
  loadConversations,
  loadMessages,
  addMessage,
  markAsRead,
  setActiveConversation,
};
