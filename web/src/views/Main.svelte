<script>
  import { push } from 'svelte-spa-router';
  import { onMount } from 'svelte';
  import {
    chatStore,
    invites,
    conversations,
    activeConversationId,
    activeConversation,
    activeMessages,
  } from '../lib/stores/chats.js';
  import { auth } from '../lib/stores/auth.js';
  import api from '../lib/api.js';
  import { createGroup, invalidateGroup, rotateGroupKey, sendFile, sendText } from '../lib/chat.js';
  import { getUser } from '../lib/directory.js';
  import LeftPanel from '../components/LeftPanel.svelte';
  import RightPanel from '../components/RightPanel.svelte';
  import NewChatModal from '../components/NewChatModal.svelte';
  import NewGroupModal from '../components/NewGroupModal.svelte';
  import SettingsPage from '../components/SettingsPage.svelte';
  import FilesModal from '../components/FilesModal.svelte';
  import InviteBanner from '../components/InviteBanner.svelte';

  // ── Auth guard ──
  let authenticated = false;
  let currentUser = null;

  // ── Modal visibility ──
  let showNewChat = false;
  let showNewGroup = false;
  let showSettings = false;
  let showFilesModal = false;
  let filesModalConvId = '';
  let filesModalIsGroup = false;

  // ── WebSocket ──
  let ws = null;
  let wsReady = false;

  // ── Typing state ──
  let typingSenders = {};       // { [convId]: Set<handle> }
  let typingTimers = {};        // { [convId]: timeoutId }
  let lastTypingSent = {};      // { [convId]: timestamp }
  const TYPING_INTERVAL = 2000;
  const TYPING_IDLE = 3000;
  $: typingHandles =
    $activeConversationId && typingSenders[$activeConversationId]
      ? [...typingSenders[$activeConversationId]]
      : [];
  $: typingUserStr = typingHandles.length > 0 ? typingHandles.join(', ') : null;

  // ── Mobile responsiveness ──
  let windowWidth = 1200;
  $: isMobile = windowWidth < 768;
  $: showConversation = isMobile && $activeConversationId ? true : false;

  // ── Toasts ──
  let toasts = [];
  let toastCounter = 0;

  // ── Failed message queue (for retry / background sync) ──
  let failedMessages = [];

  onMount(async () => {
    const token = sessionStorage.getItem('tailchat-token');
    if (!token) {
      push('/login');
      return;
    }
    authenticated = true;

    // Restore auth session
    await auth.restoreSession();
    const unsubAuth = auth.subscribe(($a) => {
      currentUser = $a.user;
    });

    // Initial data load
    await chatStore.loadConversations();
    chatStore.loadInvites();

    // Connect WebSocket (reconnects automatically; resyncs after a reconnect)
    if (token) {
      ws = api.connectWebSocket(token, {
        onMessage: handleWSMessage,
        onOpen: (reconnect) => {
          wsReady = true;
          if (reconnect) {
            chatStore.loadConversations();
            chatStore.loadInvites();
            if ($activeConversationId) chatStore.loadMessages($activeConversationId);
          }
        },
        onClose: () => {
          wsReady = false;
        },
      });
    }

    // Track window resize for mobile layout
    windowWidth = window.innerWidth;
    const onResize = () => {
      windowWidth = window.innerWidth;
    };
    window.addEventListener('resize', onResize);

    return () => {
      window.removeEventListener('resize', onResize);
      unsubAuth();
      if (ws) ws.close();
    };
  });

  // The group admin's client rotates the key after someone leaves.
  $: maybeRotate($conversations);
  const rotating = new Set();
  async function maybeRotate(list) {
    const me = sessionStorage.getItem('tailchat-user-id');
    for (const c of list) {
      if (c.type === 'group' && c.needs_rotation && c.created_by === me && !rotating.has(c.id)) {
        rotating.add(c.id);
        try {
          await rotateGroupKey(c.id, '', { onlyIfNeeded: true });
          await chatStore.loadConversations();
        } catch (e) {
          console.warn('group key rotation failed:', e.message);
        } finally {
          rotating.delete(c.id);
        }
      }
    }
  }

  // ── WebSocket event handler ──
  function handleWSMessage(event) {
    try {
      const data = JSON.parse(event.data);
      switch (data.type) {
        case 'new_message': {
          // The server only says WHICH conversation changed; we fetch and decrypt it here.
          const convId = data.conversation_id || data.group_id || data.sender_id;
          const isGroup = !!data.group_id;
          if (convId) {
            chatStore.loadConversations();
            chatStore.loadMessagesWithType(convId, isGroup);
          }
          break;
        }
        case 'group_changed': {
          // Roster or key changed: refresh (the admin's client rotates the key if someone left).
          if (data.group_id) invalidateGroup(data.group_id);
          chatStore.loadConversations();
          chatStore.loadInvites();
          break;
        }
        case 'read_receipt': {
          const convId = data.conversation_id || data.conversation_with;
          if (convId) {
            chatStore.messages.update((m) => {
              const msgs = m[convId];
              if (!msgs) return m;
              return {
                ...m,
                [convId]: msgs.map((msg) =>
                  msg.status === 'sent' || msg.status === 'delivered'
                    ? { ...msg, status: 'read' }
                    : msg,
                ),
              };
            });
          }
          break;
        }
        case 'typing':
          handleTypingReceive(data);
          break;
      }
    } catch (e) {
      console.warn('Failed to parse WS message:', e);
    }
  }

  // ── Typing: receive indicator ──
  async function handleTypingReceive(data) {
    const convId = data.conversation_id;
    if (!convId || !data.user_id) return;
    let handle = 'someone';
    try {
      handle = (await getUser(data.user_id)).handle;
    } catch {
      /* unknown or key-changed contact: still show that someone is typing */
    }

    if (!typingSenders[convId]) typingSenders[convId] = new Set();
    typingSenders[convId].add(handle);

    if (typingTimers[convId]) clearTimeout(typingTimers[convId]);
    typingTimers[convId] = setTimeout(() => {
      if (typingSenders[convId]) {
        typingSenders[convId].delete(handle);
        if (typingSenders[convId].size === 0) delete typingSenders[convId];
      }
      typingSenders = { ...typingSenders };
    }, TYPING_IDLE);

    typingSenders = { ...typingSenders };
  }

  // ── Typing: send indicator (debounced, called from MessageInput) ──
  function handleTypingSend() {
    if (!ws || !wsReady || !$activeConversationId) return;
    const now = Date.now();
    const last = lastTypingSent[$activeConversationId] || 0;
    if (now - last >= TYPING_INTERVAL) {
      lastTypingSent[$activeConversationId] = now;
      ws.send({
        type: 'typing',
        conversation_id: $activeConversationId,
        is_group: $activeConversation?.type === 'group',
        is_typing: true,
      });
    }
  }

  // ── Conversation selection ──
  function handleSelectConversation(e) {
    const conv = e.detail;
    const convId = conv.user_id || conv.id;
    chatStore.setActiveConversation(convId);
    chatStore.markAsRead(convId);
    chatStore.loadMessages(convId);
  }

  function handleBack() {
    chatStore.setActiveConversation('');
  }

  // ── Sending messages (optimistic UI with retry queue) ──
  const convKey = (conv) => conv.user_id || conv.id;

  async function handleSendMessage(e) {
    const text = e.detail;
    const conv = $activeConversation;
    if (!conv || !text) return;

    const convId = convKey(conv);
    const userId = currentUser?.uuid || sessionStorage.getItem('tailchat-user-id');

    const optimisticId = 'opt-' + Date.now() + '-' + Math.random().toString(36).slice(2, 6);
    chatStore.addMessage(convId, {
      id: optimisticId,
      content: text,
      created_at: new Date().toISOString(),
      status: 'sent',
      is_own: true,
      sender_id: userId,
    });

    const expiresIn = conv.expires_in && conv.expires_in !== 'Never' ? conv.expires_in : undefined;

    try {
      const result = await sendText(conv, text, expiresIn);
      if (result && result.id) {
        chatStore.messages.update((m) => ({
          ...m,
          [convId]: (m[convId] || [])
            .map((msg) => (msg.id === optimisticId ? { ...msg, id: result.id, is_own: true, content: text } : msg))
            // the server copy may already have arrived over the WebSocket: keep one
            .filter((msg, i, all) => all.findIndex((x) => x.id === msg.id) === i),
        }));
      }
    } catch (err) {
      chatStore.messages.update((m) => ({
        ...m,
        [convId]: (m[convId] || []).map((msg) =>
          msg.id === optimisticId ? { ...msg, status: 'failed' } : msg,
        ),
      }));
      failedMessages = [...failedMessages, { convId, text, timestamp: new Date().toISOString() }];
      showToast(err.name === 'KeyChangedError' ? err.message : 'Message failed to send.');
    }
  }

  async function handleAttachFile(e) {
    const conv = $activeConversation;
    if (!conv) return;
    const file = e.detail;
    const convId = convKey(conv);
    const expiresIn = conv.expires_in && conv.expires_in !== 'Never' ? conv.expires_in : undefined;
    try {
      showToast('Encrypting and sending…');
      const { result, body } = await sendFile(conv, file, expiresIn);
      const userId = currentUser?.uuid || sessionStorage.getItem('tailchat-user-id');
      chatStore.addMessage(convId, {
        id: result.id,
        content: `📎 ${body.file.name}`,
        file: body.file,
        created_at: new Date().toISOString(),
        status: 'sent',
        is_own: true,
        sender_id: userId,
      });
      chatStore.loadConversations();
      showToast('File sent');
    } catch (err) {
      showToast(err.message?.includes('too large') || err.message?.includes('exceeds') ? 'File is too large' : 'File upload failed');
    }
  }

  // ── Group invitations ──
  async function handleAcceptInvite(e) {
    try {
      await api.acceptInvite(e.detail.group_id);
      invalidateGroup(e.detail.group_id);
      await chatStore.loadInvites();
      await chatStore.loadConversations();
      showToast('Joined the group');
    } catch (err) {
      showToast('Could not accept: ' + (err.message || ''));
    }
  }

  async function handleDeclineInvite(e) {
    try {
      await api.declineInvite(e.detail.group_id);
      await chatStore.loadInvites();
    } catch (err) {
      showToast('Could not decline: ' + (err.message || ''));
    }
  }

  // ── Group creation ──
  async function handleCreateGroup(e) {
    const { name, members } = e.detail;
    try {
      await createGroup(name, members);
      showNewGroup = false;
      showToast('Group created — invitations sent');
      await chatStore.loadConversations();
    } catch (err) {
      showToast('Failed to create group: ' + (err.message || ''));
    }
  }

  // ── New 1:1 chat ──
  function handleNewChatSelect(e) {
    const user = e.detail.user;
    showNewChat = false;
    const userId = user.id || user.uuid;
    // Add the user as a synthetic conversation so it appears in the sidebar
    chatStore.conversations.update((list) => {
      const exists = list.some((c) => (c.id || c.user_id) === userId || c.id === '_syn_' + userId);
      if (!exists) {
        return [...list, {
          id: '_syn_' + userId,
          user_id: userId,
          handle: user.handle,
          last_message_at: new Date().toISOString(),
          unread_count: 0,
        }];
      }
      return list;
    });
    chatStore.setActiveConversation(userId);
    chatStore.loadMessages(userId);
  }

  // ── Settings ──
  function handleOpenSettings() {
    showSettings = true;
  }

  function handleCloseSettings() {
    showSettings = false;
  }

  async function handleRetentionChanged() {
    await chatStore.loadConversations();
  }

  // ── Retention ──
  async function handleRetention(e) {
    const { conversationId, expiresIn } = e.detail;
    const conv = $activeConversation;
    const isGroup = conv?.type === 'group';
    try {
      await api.updateRetention({
        conversationWith: isGroup ? null : conversationId,
        groupId: isGroup ? conversationId : null,
        expiresIn: expiresIn || '',
      });
      await chatStore.loadConversations();
      // Reload messages so the on-read retention filter takes effect immediately
      if (conversationId) {
        await chatStore.loadMessagesWithType(conversationId, isGroup);
      }
      showToast('Retention updated');
    } catch {
      showToast('Failed to update retention');
    }
  }

  async function handleLeaveGroup(e) {
    try {
      await api.leaveGroup(
        e.detail,
        currentUser?.uuid || sessionStorage.getItem('tailchat-user-id'),
      );
      showToast('Left group');
      await chatStore.loadConversations();
      chatStore.setActiveConversation('');
    } catch {
      showToast('Failed to leave group');
    }
  }

  function handleOpenFiles(e) {
    const conv = $activeConversation;
    if (!conv) return;
    filesModalConvId = conv.user_id || conv.id;
    filesModalIsGroup = conv.type === 'group';
    showFilesModal = true;
  }

  function handleCloseFilesModal() {
    showFilesModal = false;
  }

  function handleJumpToMessage(e) {
    const { messageId } = e.detail;
    if (!messageId) return;
    // The conversation is already active — we just close the modal.
    // The message list will need to scroll to this message.
    // The `loadConversations` and `loadMessages` already happen on conversation switch.
    showFilesModal = false;
    // Mark this message ID for scrolling after re-render
    scrollToMessageId = messageId;
  }

  let scrollToMessageId = '';

  function handleDeleteMessage(e) {
    const { messageId } = e.detail;
    const convId = $activeConversationId;
    if (!convId || !messageId) return;
    chatStore.messages.update((m) => {
      const msgs = (m[convId] || []).filter((msg) => msg.id !== messageId);
      return { ...m, [convId]: msgs };
    });
  }

  function handleBatchDelete(e) {
    const { messageIds } = e.detail;
    const convId = $activeConversationId;
    if (!convId || !messageIds || messageIds.length === 0) return;
    const idSet = new Set(messageIds);
    chatStore.messages.update((m) => {
      const msgs = (m[convId] || []).filter((msg) => !idSet.has(msg.id));
      return { ...m, [convId]: msgs };
    });
  }

  // ── Toast helper ──
  function showToast(message) {
    const id = ++toastCounter;
    toasts = [...toasts, { id, message }];
    setTimeout(() => {
      toasts = toasts.filter((t) => t.id !== id);
    }, 4000);
  }
</script>

{#if authenticated}
  <div class="app-shell">
    <!-- Left Panel -->
    <div
      class="panel-wrapper left-wrapper"
      class:hidden={isMobile && showConversation}
    >
      <LeftPanel
        conversations={$conversations}
        activeId={$activeConversationId}
        on:selectConversation={handleSelectConversation}
        on:openSettings={handleOpenSettings}
        on:newChat={() => (showNewChat = true)}
        on:newGroup={() => (showNewGroup = true)}
      />
    </div>

    <!-- Right Panel -->
    <div
      class="panel-wrapper right-wrapper"
      class:visible={!isMobile || showConversation}
    >
      <RightPanel
        activeConversation={$activeConversation}
        activeId={$activeConversationId}
        messages={$activeMessages}
        typingUser={typingUserStr}
        on:back={handleBack}
        on:sendMessage={handleSendMessage}
        on:attachFile={handleAttachFile}
        on:typing={handleTypingSend}
        on:retention={handleRetention}
        on:leaveGroup={handleLeaveGroup}
        on:openFiles={handleOpenFiles}
        on:delete={handleDeleteMessage}
        on:batchDelete={handleBatchDelete}
        on:newChat={() => (showNewChat = true)}
      />
    </div>
  </div>

  <!-- Modals -->
  <NewChatModal
    show={showNewChat}
    on:close={() => (showNewChat = false)}
    on:select={handleNewChatSelect}
  />
  <NewGroupModal
    show={showNewGroup}
    on:close={() => (showNewGroup = false)}
    on:create={handleCreateGroup}
  />
  <SettingsPage
    show={showSettings}
    on:close={handleCloseSettings}
    on:retention-changed={handleRetentionChanged}
    conversations={$conversations}
  />

  <FilesModal
    show={showFilesModal}
    convId={filesModalConvId}
    isGroup={filesModalIsGroup}
    on:close={handleCloseFilesModal}
    on:jumpToMessage={handleJumpToMessage}
  />

  <InviteBanner invites={$invites} on:accept={handleAcceptInvite} on:decline={handleDeclineInvite} />

  <!-- Toast notifications -->
  {#if toasts.length > 0}
    <div class="toast-container">
      {#each toasts as toast (toast.id)}
        <div class="toast">{toast.message}</div>
      {/each}
    </div>
  {/if}
{/if}

<style>
  .app-shell {
    display: flex;
    height: 100vh;
    overflow: hidden;
    background-color: var(--color-bg);
  }

  .panel-wrapper {
    display: flex;
    height: 100%;
  }

  .left-wrapper {
    width: 380px;
    min-width: 380px;
    flex-shrink: 0;
  }

  .right-wrapper {
    flex: 1;
    min-width: 0;
  }

  /* ── Toast container ── */
  .toast-container {
    position: fixed;
    top: 1rem;
    right: 1rem;
    z-index: 200;
    display: flex;
    flex-direction: column;
    gap: 0.5rem;
    max-width: 360px;
  }

  .toast {
    padding: 0.75rem 1rem;
    border-radius: 8px;
    background: var(--color-bg-secondary);
    border: 1px solid var(--color-border);
    color: var(--color-text);
    font-size: 0.8125rem;
    font-weight: 500;
    box-shadow: 0 4px 12px rgba(0, 0, 0, 0.3);
    animation: toast-in 0.25s ease-out;
  }

  @keyframes toast-in {
    from {
      opacity: 0;
      transform: translateY(-8px);
    }
    to {
      opacity: 1;
      transform: translateY(0);
    }
  }

  /* ── Mobile: single panel at a time ── */
  @media (max-width: 767px) {
    .left-wrapper {
      width: 100%;
      min-width: 0;
    }

    .left-wrapper.hidden {
      display: none;
    }

    .right-wrapper {
      position: absolute;
      inset: 0;
      z-index: 10;
      display: none;
    }

    .right-wrapper.visible {
      display: flex;
    }

    .toast-container {
      left: 1rem;
      right: 1rem;
      max-width: none;
    }
  }
</style>
