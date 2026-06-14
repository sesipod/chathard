<script>
  /**
   * Scrollable conversation list.
   *
   * Props:
   *   conversations  {Array}  Sorted list of conversation objects
   *   activeId       {string} Currently selected conversation id
   *
   * Events:
   *   on:select  — fired with { detail: conversation } when a row is clicked
   */
  import ChatListItem from './ChatListItem.svelte';
  import { createEventDispatcher } from 'svelte';

  export let conversations = [];
  export let activeId = '';

  const dispatch = createEventDispatcher();

  function handleSelect(e) {
    dispatch('select', e.detail);
  }
</script>

<div class="chat-list">
  {#if conversations.length === 0}
    <div class="empty-state">
      <p class="empty-text">No conversations yet.</p>
      <p class="empty-hint">Tap + to start a new chat.</p>
    </div>
  {:else}
    {#each conversations as conv (conv.id || conv.user_id)}
      <ChatListItem
        conversation={conv}
        isActive={(conv.id || conv.user_id) === activeId}
        on:select={handleSelect}
      />
    {/each}
  {/if}
</div>

<style>
  .chat-list {
    flex: 1;
    overflow-y: auto;
    padding: 0.25rem 0;
  }

  .empty-state {
    display: flex;
    flex-direction: column;
    align-items: center;
    justify-content: center;
    padding: 3rem 1.5rem;
    text-align: center;
  }

  .empty-text {
    color: var(--color-text-muted);
    font-size: 0.875rem;
    margin-bottom: 0.25rem;
  }

  .empty-hint {
    color: var(--color-text-muted);
    font-size: 0.8125rem;
    opacity: 0.7;
  }
</style>
