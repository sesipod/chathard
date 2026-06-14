<script>
  /**
   * Active conversation view — header, message list, message input.
   *
   * Props:
   *   conversation  {object}  { id, type, handle, name, retention, ... }
   *   messages      {Array}   Messages to display (from store)
   *   typingUser    {string|null}  Handle of user currently typing
   *
   * Events:
   *   on:back         — fired when the mobile back arrow is clicked
   *   on:sendMessage  — fired with { detail: text } to send a message
   *   on:attachFile   — fired with { detail: File }
   *   on:typing       — fired when the user types (for WS typing indicator)
   *   on:retention    — fired with { detail: expiresIn }
   *   on:leaveGroup   — fired when "Leave group" is selected
   *   on:openFiles    — fired when "Files" is selected in the menu
   */
  import { createEventDispatcher, afterUpdate } from 'svelte';
  import MessageBubble from './MessageBubble.svelte';
  import MessageInput from './MessageInput.svelte';
  import Avatar from './common/Avatar.svelte';

  export let conversation = {};
  export let messages = [];
  export let typingUser = null;

  const dispatch = createEventDispatcher();

  let messageListEl;
  let isScrolledUp = false;
  let showMenu = false;
  let showRetentionPicker = false;

  const retentionOptions = ['Never', '1h', '24h', '7d', '30d', '90d'];

  $: displayName = conversation.handle || conversation.name || 'Unknown';
  $: isGroup = conversation.type === 'group';
  $: currentUserId = sessionStorage.getItem('tailchat-user-id') || '';
  $: currentRetention = conversation.expires_in || 'Never';
  $: retentionLabel = currentRetention && currentRetention !== 'Never'
    ? `Auto-delete: ${currentRetention}`
    : '';

  /** Group messages by day and generate date separator data. */
  $: messageRows = buildMessageRows(messages);

  /** Build message rows with date separators injected. */
  function buildMessageRows(msgs) {
    if (!msgs || msgs.length === 0) return [];

    const rows = [];
    let lastDate = '';

    for (const msg of msgs) {
      const msgDate = msg.created_at ? new Date(msg.created_at).toDateString() : '';
      let dateText = '';

      if (msgDate && msgDate !== lastDate) {
        dateText = formatDateLabel(msg.created_at);
        lastDate = msgDate;
      }

      rows.push({
        message: msg,
        showDateSeparator: !!dateText && !msg.is_system,
        dateText,
      });
    }

    return rows;
  }

  /** Format a date into "Today", "Yesterday", or "Jun 9". */
  function formatDateLabel(iso) {
    if (!iso) return '';
    const now = new Date();
    const date = new Date(iso);
    const today = now.toDateString();
    const yesterday = new Date(now);
    yesterday.setDate(yesterday.getDate() - 1);

    if (date.toDateString() === today) return 'Today';
    if (date.toDateString() === yesterday.toDateString()) return 'Yesterday';
    return date.toLocaleDateString(undefined, { month: 'short', day: 'numeric' });
  }

  /** Scroll the message list to the bottom. */
  function scrollToBottom(smooth = false) {
    if (!messageListEl) return;
    messageListEl.scrollTo({
      top: messageListEl.scrollHeight,
      behavior: smooth ? 'smooth' : 'auto',
    });
    isScrolledUp = false;
  }

  /** Auto-scroll when new messages arrive, unless user is scrolled up. */
  afterUpdate(() => {
    if (!isScrolledUp) {
      scrollToBottom(false);
    }
  });

  /** Track whether the user has scrolled up. */
  function handleScroll() {
    if (!messageListEl) return;
    const { scrollTop, scrollHeight, clientHeight } = messageListEl;
    const distanceFromBottom = scrollHeight - scrollTop - clientHeight;
    isScrolledUp = distanceFromBottom > 100;
  }

  function handleScrollToBottom() {
    scrollToBottom(true);
  }

  function handleSend(e) {
    dispatch('sendMessage', e.detail);
  }

  function handleAttach(e) {
    dispatch('attachFile', e.detail);
  }

  function handleBack() {
    dispatch('back');
  }

  function handleTyping() {
    dispatch('typing');
  }

  function toggleMenu() {
    showMenu = !showMenu;
  }

  function handleMenuAction(action) {
    showMenu = false;
    if (action === 'files') dispatch('openFiles', conversation.id);
    if (action === 'retention') showRetentionPicker = !showRetentionPicker;
    if (action === 'leave') dispatch('leaveGroup', conversation.id);
  }

  function handleRetentionSelect(value) {
    showRetentionPicker = false;
    const convId = conversation.user_id || conversation.id;
    if (value === currentRetention) return;
    dispatch('retention', { conversationId: convId, expiresIn: value === 'Never' ? '' : value });
  }

  function handleRetentionChange(e) {
    const value = e.target.value;
    if (value && value !== currentRetention) {
      const convId = conversation.user_id || conversation.id;
      dispatch('retention', { conversationId: convId, expiresIn: value === 'Never' ? '' : value });
    }
  }

</script>

<div class="conversation">
  <!-- Header -->
  <header class="conv-header">
    <div class="header-left">
      <!-- Back arrow (mobile only) -->
      <button
        class="back-btn"
        on:click={handleBack}
        title="Back to chats"
        aria-label="Back to chats"
      >
        <svg width="20" height="20" viewBox="0 0 20 20" fill="none" stroke="currentColor" stroke-width="2">
          <polyline points="14,3 7,10 14,17" />
        </svg>
      </button>

      <Avatar name={displayName} size={36} />

      <div class="header-info">
        <span class="conv-name">{displayName}</span>
        {#if retentionLabel}
          <span class="retention-badge">{retentionLabel}</span>
        {/if}
      </div>
    </div>

    <div class="header-right">
      <!-- 3-dot menu -->
      <div class="menu-wrapper">
        <button
          class="menu-btn"
          on:click={toggleMenu}
          title="More"
          aria-label="More options"
          aria-expanded={showMenu}
        >
          <svg width="20" height="20" viewBox="0 0 20 20" fill="currentColor">
            <circle cx="10" cy="4" r="1.5" />
            <circle cx="10" cy="10" r="1.5" />
            <circle cx="10" cy="16" r="1.5" />
          </svg>
        </button>

        {#if showMenu}
          <!-- eslint-disable-next-line svelte/no-at-html-tags -->
          <div class="menu-dropdown" role="menu">
            <button class="menu-item" role="menuitem" on:click={() => handleMenuAction('files')}>
              <svg width="16" height="16" viewBox="0 0 16 16" fill="none" stroke="currentColor" stroke-width="1.5">
                <path d="M9 1H4a1 1 0 0 0-1 1v12a1 1 0 0 0 1 1h8a1 1 0 0 0 1-1V5l-4-4z"/>
                <line x1="9" y1="1" x2="13" y2="5"/>
              </svg>
              Files
            </button>

            {#if isGroup}
              <button class="menu-item" role="menuitem" on:click={() => handleMenuAction('retention')}>
                <svg width="16" height="16" viewBox="0 0 16 16" fill="none" stroke="currentColor" stroke-width="1.5">
                  <circle cx="8" cy="8" r="6" />
                  <polyline points="8,4 8,8 10,10" />
                </svg>
                Retention
              </button>
              <div class="menu-divider"></div>
              <button class="menu-item danger" role="menuitem" on:click={() => handleMenuAction('leave')}>
                <svg width="16" height="16" viewBox="0 0 16 16" fill="none" stroke="currentColor" stroke-width="1.5">
                  <path d="M6 2H3a1 1 0 0 0-1 1v10a1 1 0 0 0 1 1h3" />
                  <polyline points="10,11 14,8 10,5" />
                  <line x1="14" y1="8" x2="6" y2="8" />
                </svg>
                Leave group
              </button>
            {:else}
              <button class="menu-item" role="menuitem" on:click={() => handleMenuAction('retention')}>
                <svg width="16" height="16" viewBox="0 0 16 16" fill="none" stroke="currentColor" stroke-width="1.5">
                  <circle cx="8" cy="8" r="6" />
                  <polyline points="8,4 8,8 10,10" />
                </svg>
                Auto-delete
              </button>
            {/if}
          </div>
        {/if}
      </div>
    </div>
  </header>

  <!-- Retention picker -->
  {#if showRetentionPicker}
    <div class="retention-picker">
      <span class="retention-label">Auto-delete messages after:</span>
      <div class="retention-options">
        {#each retentionOptions as opt}
          <button
            class="retention-btn"
            class:active={opt === currentRetention}
            on:click={() => handleRetentionSelect(opt)}
          >{opt}</button>
        {/each}
      </div>
    </div>
  {/if}

  <!-- Message list -->
  <div
    class="message-list"
    bind:this={messageListEl}
    on:scroll={handleScroll}
  >
    {#if messageRows.length === 0}
      <div class="empty-messages">
        <p>No messages yet. Send the first one!</p>
      </div>
    {:else}
      {#each messageRows as row, i (row.message.id || i)}
        <MessageBubble
          message={row.message}
          isOwn={row.message.is_own ?? (row.message.sender_id === currentUserId)}
          senderHandle={row.message.is_own ? '' : (conversation.handle || '')}
          convId={conversation.user_id || conversation.id || ''}
          showDateSeparator={row.showDateSeparator}
          dateText={row.dateText}
        />
      {/each}
    {/if}
  </div>

  <!-- Scroll to bottom FAB -->
  {#if isScrolledUp}
    <button
      class="scroll-fab"
      on:click={handleScrollToBottom}
      title="Scroll to bottom"
      aria-label="Scroll to bottom"
    >
      <svg width="20" height="20" viewBox="0 0 20 20" fill="none" stroke="currentColor" stroke-width="2">
        <polyline points="6,8 10,12 14,8" />
      </svg>
    </button>
  {/if}

  <!-- Message input -->
  <MessageInput
    typingUser={typingUser}
    on:send={handleSend}
    on:attach={handleAttach}
    on:typing={handleTyping}
  />
</div>

<style>
  .conversation { display:flex; flex-direction:column; height:100%; position:relative; }
  .conv-header { display:flex; align-items:center; justify-content:space-between; padding:.5rem .75rem; border-bottom:1px solid var(--color-border); background:var(--color-bg-secondary); flex-shrink:0; min-height:56px; }
  .header-left { display:flex; align-items:center; gap:.625rem; min-width:0; }
  .back-btn { display:none; align-items:center; justify-content:center; width:36px; height:36px; border:none; border-radius:50%; background:transparent; color:var(--color-text); cursor:pointer; flex-shrink:0; transition:background .15s; }
  .back-btn:hover { background:var(--color-bg-tertiary); }
  .header-info { display:flex; flex-direction:column; gap:1px; min-width:0; }
  .conv-name { font-size:.9375rem; font-weight:600; color:var(--color-text); white-space:nowrap; overflow:hidden; text-overflow:ellipsis; }
  .retention-badge { font-size:.6875rem; color:var(--color-text-muted); white-space:nowrap; }
  .header-right { display:flex; align-items:center; flex-shrink:0; }
  .menu-wrapper { position:relative; }
  .menu-btn { display:flex; align-items:center; justify-content:center; width:36px; height:36px; border:none; border-radius:50%; background:transparent; color:var(--color-text-muted); cursor:pointer; transition:background .15s; }
  .menu-btn:hover { background:var(--color-bg-tertiary); }
  .menu-dropdown { position:absolute; right:0; top:100%; z-index:20; min-width:180px; padding:.375rem 0; border:1px solid var(--color-border); border-radius:8px; background:var(--color-bg-secondary); box-shadow:0 4px 16px rgba(0,0,0,.3); }
  .menu-item { display:flex; align-items:center; gap:.5rem; width:100%; padding:.5rem .875rem; border:none; background:none; color:var(--color-text); font-size:.8125rem; font-family:inherit; cursor:pointer; text-align:left; transition:background .12s; }
  .menu-item:hover { background:var(--color-bg-tertiary); }
  .menu-item.danger { color:var(--color-danger); }
  .menu-divider { height:1px; margin:.25rem .5rem; background:var(--color-border); }
  .retention-picker { display:flex; flex-direction:column; gap:.375rem; padding:.5rem .75rem; border-bottom:1px solid var(--color-border); background:var(--color-bg-secondary); flex-shrink:0; }
  .retention-label { font-size:.6875rem; color:var(--color-text-muted); font-weight:600; text-transform:uppercase; letter-spacing:.04em; }
  .retention-options { display:flex; gap:.25rem; flex-wrap:wrap; }
  .retention-btn { padding:.25rem .5rem; border:1px solid var(--color-border); border-radius:6px; background:var(--color-bg); color:var(--color-text-muted); font-size:.75rem; font-family:inherit; cursor:pointer; transition:all .12s; }
  .retention-btn:hover { border-color:var(--color-accent); color:var(--color-text); }
  .retention-btn.active { background:var(--color-accent); color:#fff; border-color:var(--color-accent); }
  .message-list { flex:1; overflow-y:auto; padding:.5rem 0; display:flex; flex-direction:column; }
  .empty-messages { flex:1; display:flex; align-items:center; justify-content:center; padding:2rem; color:var(--color-text-muted); font-size:.875rem; text-align:center; }
  .scroll-fab { position:absolute; bottom:64px; right:1.25rem; z-index:5; display:flex; align-items:center; justify-content:center; width:40px; height:40px; border:none; border-radius:50%; background:var(--color-accent); color:#fff; cursor:pointer; box-shadow:0 2px 8px rgba(0,0,0,.3); transition:opacity .15s,transform .15s; }
  .scroll-fab:hover { opacity:.9; transform:scale(1.05); }
  @media (max-width:767px) { .back-btn { display:flex; } .scroll-fab { bottom:72px; } }
</style>
