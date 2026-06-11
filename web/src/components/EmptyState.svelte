<script>
  /**
   * Placeholder view when no conversation is selected.
   *
   * Events:
   *   on:newChat  — fired when the "New Chat" button is clicked
   */
  import { createEventDispatcher } from 'svelte';

  const dispatch = createEventDispatcher();

  function handleNewChat() {
    dispatch('newChat');
  }
</script>

<div class="empty-state">
  <!-- Signal-style illustration: chat bubble outline -->
  <svg width="64" height="64" viewBox="0 0 64 64" fill="none" stroke="currentColor" stroke-width="1.5" class="empty-icon">
    <rect x="6" y="12" width="52" height="40" rx="6" />
    <line x1="16" y1="24" x2="48" y2="24" />
    <line x1="16" y1="32" x2="40" y2="32" />
    <line x1="16" y1="40" x2="34" y2="40" />
    <polyline points="44,36 52,44 44,48" />
  </svg>

  <h2 class="empty-title">Select a conversation</h2>
  <p class="empty-subtitle">or start a new one</p>

  <button class="new-chat-btn" on:click={handleNewChat}>
    <svg width="16" height="16" viewBox="0 0 18 18" fill="none" stroke="currentColor" stroke-width="2">
      <line x1="9" y1="2" x2="9" y2="16" />
      <line x1="2" y1="9" x2="16" y2="9" />
    </svg>
    New Chat
  </button>
</div>

<style>
  .empty-state {
    flex: 1;
    display: flex;
    flex-direction: column;
    align-items: center;
    justify-content: center;
    gap: 0.75rem;
    padding: 2rem;
    color: var(--color-text-muted);
  }

  .empty-icon {
    opacity: 0.35;
    margin-bottom: 0.25rem;
  }

  .empty-title {
    font-size: 1.125rem;
    font-weight: 600;
    color: var(--color-text-muted);
  }

  .empty-subtitle {
    font-size: 0.875rem;
    opacity: 0.7;
  }

  .new-chat-btn {
    display: inline-flex;
    align-items: center;
    gap: 0.5rem;
    margin-top: 0.5rem;
    padding: 0.625rem 1.25rem;
    border: none;
    border-radius: 8px;
    background-color: var(--color-accent);
    color: #fff;
    font-size: 0.875rem;
    font-weight: 600;
    cursor: pointer;
    transition: opacity 0.15s;
    font-family: inherit;
  }

  .new-chat-btn:hover {
    opacity: 0.9;
  }
</style>
