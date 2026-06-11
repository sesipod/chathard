<script>
  import { push } from 'svelte-spa-router';
  import { onMount } from 'svelte';

  // ── Route guard: redirect to /login if not authenticated ──
  let authenticated = false;

  onMount(async () => {
    const token = sessionStorage.getItem('tailchat-token');
    if (!token) {
      push('/login');
      return;
    }
    authenticated = true;
  });

  // ── Active conversation state ──
  let activeConversationId = '';
  let activeConversationType = ''; // 'direct' | 'group'

  // ── Placeholder panel components (full implementation in later phases) ──
</script>

{#if authenticated}
  <div class="app-shell">
    <!-- Left Panel -->
    <aside class="left-panel">
      <header class="panel-header">
        <h1 class="panel-title">Chats</h1>
        <button class="icon-btn" title="Settings" aria-label="Settings">
          <svg width="20" height="20" viewBox="0 0 20 20" fill="none" stroke="currentColor" stroke-width="1.5">
            <circle cx="10" cy="10" r="3" />
            <path d="M10 1.5v2M10 16.5v2M1.5 10h2M16.5 10h2M3.4 3.4l1.4 1.4M15.2 15.2l1.4 1.4M3.4 16.6l1.4-1.4M15.2 4.8l1.4-1.4" />
          </svg>
        </button>
      </header>

      <button class="new-chat-btn" title="New conversation" aria-label="New conversation">
        <svg width="18" height="18" viewBox="0 0 18 18" fill="none" stroke="currentColor" stroke-width="2">
          <line x1="9" y1="2" x2="9" y2="16" />
          <line x1="2" y1="9" x2="16" y2="9" />
        </svg>
        <span>New Chat</span>
      </button>

      <!-- Search -->
      <div class="search-wrapper">
        <svg class="search-icon" width="16" height="16" viewBox="0 0 16 16" fill="none" stroke="currentColor" stroke-width="1.5">
          <circle cx="7" cy="7" r="5" />
          <line x1="11" y1="11" x2="14.5" y2="14.5" />
        </svg>
        <input
          type="search"
          class="search-input"
          placeholder="Search conversations..."
          aria-label="Search conversations"
        />
      </div>

      <!-- Conversation list placeholder -->
      <div class="conversation-list">
        <div class="empty-list-message">
          <p>No conversations yet.</p>
          <p class="text-sm">Tap + to start a new chat.</p>
        </div>
      </div>
    </aside>

    <!-- Right Panel -->
    <main class="right-panel">
      <div class="empty-state">
        <svg width="48" height="48" viewBox="0 0 48 48" fill="none" stroke="currentColor" stroke-width="1.5" style="color: var(--color-text-muted); opacity: 0.4;">
          <rect x="4" y="8" width="40" height="32" rx="4" />
          <line x1="12" y1="18" x2="36" y2="18" />
          <line x1="12" y1="24" x2="30" y2="24" />
          <line x1="12" y1="30" x2="26" y2="30" />
          <polyline points="34,28 40,34 34,36" />
        </svg>
        <h2 class="empty-title">Select a conversation</h2>
        <p class="empty-subtitle">or start a new one</p>
        <button class="btn btn-primary new-chat-btn-inline">
          <svg width="16" height="16" viewBox="0 0 18 18" fill="none" stroke="currentColor" stroke-width="2">
            <line x1="9" y1="2" x2="9" y2="16" />
            <line x1="2" y1="9" x2="16" y2="9" />
          </svg>
          New Chat
        </button>
      </div>
    </main>
  </div>
{/if}

<style>
  .app-shell {
    display: flex;
    height: 100vh;
    overflow: hidden;
    background-color: var(--color-bg);
  }

  /* ── Left Panel ── */
  .left-panel {
    width: 380px;
    min-width: 380px;
    display: flex;
    flex-direction: column;
    border-right: 1px solid var(--color-border);
    background-color: var(--color-bg-secondary);
  }

  .panel-header {
    display: flex;
    align-items: center;
    justify-content: space-between;
    padding: 1rem 1rem 0.5rem;
    flex-shrink: 0;
  }

  .panel-title {
    font-size: 1.25rem;
    font-weight: 700;
    color: var(--color-text);
  }

  .icon-btn {
    display: flex;
    align-items: center;
    justify-content: center;
    width: 36px;
    height: 36px;
    border: none;
    border-radius: 50%;
    background-color: transparent;
    color: var(--color-text-muted);
    cursor: pointer;
    transition: background-color 0.15s;
  }

  .icon-btn:hover {
    background-color: var(--color-bg-tertiary);
  }

  .new-chat-btn {
    display: flex;
    align-items: center;
    gap: 0.5rem;
    margin: 0.25rem 1rem 0.5rem;
    padding: 0.5rem 0.75rem;
    border: none;
    border-radius: 8px;
    background-color: var(--color-accent);
    color: #fff;
    font-size: 0.875rem;
    font-weight: 600;
    cursor: pointer;
    transition: opacity 0.15s;
  }

  .new-chat-btn:hover {
    opacity: 0.9;
  }

  .search-wrapper {
    position: relative;
    margin: 0 1rem 0.5rem;
  }

  .search-icon {
    position: absolute;
    left: 10px;
    top: 50%;
    transform: translateY(-50%);
    color: var(--color-text-muted);
    pointer-events: none;
  }

  .search-input {
    width: 100%;
    padding: 0.5rem 0.75rem 0.5rem 2rem;
    background-color: var(--color-bg);
    border: 1px solid var(--color-border);
    border-radius: 8px;
    color: var(--color-text);
    font-size: 0.875rem;
    outline: none;
    transition: border-color 0.15s;
  }

  .search-input:focus {
    border-color: var(--color-accent);
  }

  .search-input::placeholder {
    color: var(--color-text-muted);
    opacity: 0.6;
  }

  .conversation-list {
    flex: 1;
    overflow-y: auto;
  }

  .empty-list-message {
    padding: 2rem 1rem;
    text-align: center;
    color: var(--color-text-muted);
    font-size: 0.875rem;
  }

  .text-sm {
    font-size: 0.8125rem;
    margin-top: 0.25rem;
    opacity: 0.7;
  }

  /* ── Right Panel ── */
  .right-panel {
    flex: 1;
    display: flex;
    flex-direction: column;
    min-width: 0;
  }

  .empty-state {
    flex: 1;
    display: flex;
    flex-direction: column;
    align-items: center;
    justify-content: center;
    gap: 0.75rem;
    padding: 2rem;
  }

  .empty-title {
    font-size: 1.125rem;
    font-weight: 600;
    color: var(--color-text-muted);
  }

  .empty-subtitle {
    font-size: 0.875rem;
    color: var(--color-text-muted);
    opacity: 0.7;
  }

  .btn {
    display: inline-flex;
    align-items: center;
    justify-content: center;
    gap: 0.5rem;
    padding: 0.625rem 1.25rem;
    font-size: 0.875rem;
    font-weight: 600;
    border: none;
    border-radius: 8px;
    cursor: pointer;
    transition: opacity 0.15s;
  }

  .btn-primary {
    background-color: var(--color-accent);
    color: #fff;
  }

  .btn-primary:hover {
    opacity: 0.9;
  }

  .new-chat-btn-inline {
    margin-top: 0.5rem;
  }

  /* ── Mobile: single panel ── */
  @media (max-width: 767px) {
    .left-panel {
      width: 100%;
      min-width: 0;
    }

    .right-panel {
      display: none;
    }
  }
</style>
