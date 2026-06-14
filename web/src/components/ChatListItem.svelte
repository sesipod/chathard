<script>
  /**
   * Single conversation row in the chat list.
   *
   * Props:
   *   conversation  {object}  { id, type, handle, name, last_message, timestamp,
   *                             unread_count, last_message_at, last_message_preview }
   *   isActive      {boolean}
   *
   * Events:
   *   on:select  — fired (without payload) when the row is clicked
   */
  import Avatar from './common/Avatar.svelte';
  import { createEventDispatcher } from 'svelte';

  export let conversation = {};
  export let isActive = false;

  const dispatch = createEventDispatcher();

  $: displayName = conversation.handle || conversation.name || 'Unknown';
  $: unread = conversation.unread_count || 0;
  $: hasUnread = unread > 0;

  /** Format a relative timestamp string. */
  function formatTime(iso) {
    if (!iso) return '';
    const now = Date.now();
    const then = new Date(iso).getTime();
    const diffMs = now - then;
    const diffMin = Math.floor(diffMs / 60000);
    const diffHr = Math.floor(diffMs / 3600000);
    const diffDay = Math.floor(diffMs / 86400000);

    if (diffMin < 1) return 'now';
    if (diffMin < 60) return `${diffMin}m ago`;
    if (diffHr < 24) return `${diffHr}h ago`;
    if (diffDay === 1) return 'Yesterday';
    if (diffDay < 7) return `${diffDay}d ago`;
    const d = new Date(iso);
    return d.toLocaleDateString(undefined, { month: 'short', day: 'numeric' });
  }

  $: timeStr = formatTime(conversation.last_message_at || conversation.timestamp);

  /** Truncate and prefix the last message preview. */
  $: preview = conversation.last_message_preview
    ? conversation.last_message_preview.length > 60
      ? conversation.last_message_preview.slice(0, 60) + '…'
      : conversation.last_message_preview
    : conversation.last_message_at
      ? 'New message'
      : 'No messages yet';

  /** Determine group/1:1 type indicator. */
  $: typeIndicator = conversation.type === 'group' ? '# ' : '';

  function handleClick() {
    dispatch('select', conversation);
  }
</script>

<button
  class="chat-item"
  class:active={isActive}
  class:unread={hasUnread}
  on:click={handleClick}
  aria-label="Chat with {displayName}"
>
  <Avatar name={displayName} size={44} />

  <div class="chat-content">
    <div class="chat-top-row">
      <span class="chat-name" class:bold={hasUnread}>{typeIndicator}{displayName}</span>
      <span class="chat-time">{timeStr}</span>
    </div>
    <div class="chat-bottom-row">
      <span class="chat-preview" class:muted={!hasUnread}>
        {preview || 'No messages yet'}
      </span>
      {#if hasUnread}
        <span class="unread-badge">{unread > 99 ? '99+' : unread}</span>
      {/if}
    </div>
  </div>
</button>

<style>
  .chat-item {
    display: flex;
    align-items: center;
    gap: 0.75rem;
    width: 100%;
    padding: 0.625rem 1rem;
    border: none;
    background-color: transparent;
    color: var(--color-text);
    cursor: pointer;
    text-align: left;
    transition: background-color 0.12s;
    font-family: inherit;
    font-size: inherit;
  }

  .chat-item:hover {
    background-color: var(--color-bg-tertiary);
  }

  .chat-item.active {
    background-color: var(--color-accent);
    color: #fff;
  }

  .chat-item.active .chat-preview,
  .chat-item.active .chat-time {
    color: rgba(255, 255, 255, 0.75);
  }

  .chat-item.active .unread-badge {
    background-color: #fff;
    color: var(--color-accent);
  }

  .chat-content {
    flex: 1;
    min-width: 0;
    display: flex;
    flex-direction: column;
    gap: 2px;
  }

  .chat-top-row {
    display: flex;
    align-items: center;
    justify-content: space-between;
    gap: 0.5rem;
  }

  .chat-name {
    font-size: 0.9375rem;
    font-weight: 500;
    white-space: nowrap;
    overflow: hidden;
    text-overflow: ellipsis;
  }

  .chat-name.bold {
    font-weight: 700;
  }

  .chat-time {
    font-size: 0.75rem;
    color: var(--color-text-muted);
    white-space: nowrap;
    flex-shrink: 0;
  }

  .chat-bottom-row {
    display: flex;
    align-items: center;
    justify-content: space-between;
    gap: 0.5rem;
  }

  .chat-preview {
    font-size: 0.8125rem;
    white-space: nowrap;
    overflow: hidden;
    text-overflow: ellipsis;
    color: var(--color-text);
  }

  .chat-preview.muted {
    color: var(--color-text-muted);
  }

  .unread-badge {
    display: flex;
    align-items: center;
    justify-content: center;
    min-width: 20px;
    height: 20px;
    padding: 0 5px;
    border-radius: 10px;
    background-color: var(--color-accent);
    color: #fff;
    font-size: 0.6875rem;
    font-weight: 700;
    flex-shrink: 0;
    line-height: 1;
  }
</style>
