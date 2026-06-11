<script>
  /**
   * Chat list panel — the left sidebar showing conversations.
   *
   * 380px on desktop, full-width on mobile.
   *
   * Props:
   *   conversations  {Array}   Passed through to ChatList
   *   activeId       {string}  Currently active conversation id
   *
   * Events:
   *   on:selectConversation  — fired with { detail: conversation }
   *   on:openSettings        — fired when the gear icon is clicked
   *   on:newChat             — fired when "+" is clicked
   */
  import ChatList from './ChatList.svelte';
  import SearchInput from './common/SearchInput.svelte';
  import { createEventDispatcher } from 'svelte';

  export let conversations = [];
  export let activeId = '';

  const dispatch = createEventDispatcher();

  /** Locally filtered conversation list based on search query. */
  let searchQuery = '';
  let showNewMenu = false;

  $: filteredConversations = searchQuery
    ? conversations.filter((c) => {
        const name = (c.handle || c.name || '').toLowerCase();
        return name.includes(searchQuery.toLowerCase());
      })
    : conversations;

  function handleSearch(e) {
    searchQuery = e.detail;
  }

  function handleClear() {
    searchQuery = '';
  }

  function handleSelect(e) {
    dispatch('selectConversation', e.detail);
  }

  function handleSettings() {
    dispatch('openSettings');
  }

  function toggleNewMenu() {
    showNewMenu = !showNewMenu;
  }

  function handleNewChat() {
    showNewMenu = false;
    dispatch('newChat');
  }

  function handleNewGroup() {
    showNewMenu = false;
    dispatch('newGroup');
  }

  /** Close the new-menu dropdown when clicking outside. */
  function handleClickOutside() {
    if (showNewMenu) showNewMenu = false;
  }
</script>

<!-- svelte-ignore a11y-no-static-element-interactions -->
<aside class="left-panel" on:click={handleClickOutside}>
  <!-- Header -->
  <header class="panel-header">
    <h1 class="panel-title">Chats</h1>
    <div class="header-actions">
      <!-- New conversation dropdown -->
      <div class="new-menu-wrapper">
        <button
          class="icon-btn"
          title="New conversation"
          aria-label="New conversation"
          on:click|stopPropagation={toggleNewMenu}
        >
          <svg width="20" height="20" viewBox="0 0 20 20" fill="none" stroke="currentColor" stroke-width="2">
            <line x1="10" y1="4" x2="10" y2="16" />
            <line x1="4" y1="10" x2="16" y2="10" />
          </svg>
        </button>
        {#if showNewMenu}
          <div class="new-menu-dropdown" on:click|stopPropagation>
            <button class="new-menu-item" on:click={handleNewChat}>
              <svg width="16" height="16" viewBox="0 0 18 18" fill="none" stroke="currentColor" stroke-width="2">
                <line x1="9" y1="2" x2="9" y2="16" /><line x1="2" y1="9" x2="16" y2="9" />
              </svg>
              New Chat
            </button>
            <button class="new-menu-item" on:click={handleNewGroup}>
              <svg width="16" height="16" viewBox="0 0 18 18" fill="none" stroke="currentColor" stroke-width="1.5">
                <circle cx="6" cy="6" r="3" /><circle cx="13" cy="6" r="3" />
                <path d="M2 16c0-3 2-5 5-5h5c3 0 5 2 5 5" />
              </svg>
              New Group
            </button>
          </div>
        {/if}
      </div>
      <button
        class="icon-btn"
        title="Settings"
        aria-label="Settings"
        on:click={handleSettings}
      >
        <svg width="20" height="20" viewBox="0 0 20 20" fill="none" stroke="currentColor" stroke-width="1.5">
          <circle cx="10" cy="10" r="3" />
          <path d="M10 1.5v2M10 16.5v2M1.5 10h2M16.5 10h2M3.4 3.4l1.4 1.4M15.2 15.2l1.4 1.4M3.4 16.6l1.4-1.4M15.2 4.8l1.4-1.4" />
        </svg>
      </button>
    </div>
  </header>

  <!-- Search -->
  <div class="search-wrapper">
    <SearchInput
      placeholder="Search conversations..."
      value={searchQuery}
      on:search={handleSearch}
      on:clear={handleClear}
    />
  </div>

  <!-- Conversation list -->
  <ChatList
    conversations={filteredConversations}
    activeId={activeId}
    on:select={handleSelect}
  />
</aside>

<style>
  .left-panel {
    width: 380px;
    min-width: 380px;
    display: flex;
    flex-direction: column;
    border-right: 1px solid var(--color-border);
    background-color: var(--color-bg-secondary);
    height: 100%;
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

  .header-actions {
    display: flex;
    align-items: center;
    gap: 0.25rem;
  }

  .new-menu-wrapper {
    position: relative;
  }

  .new-menu-dropdown {
    position: absolute;
    right: 0;
    top: 100%;
    z-index: 30;
    min-width: 170px;
    padding: 0.25rem 0;
    border: 1px solid var(--color-border);
    border-radius: 8px;
    background: var(--color-bg-secondary);
    box-shadow: 0 4px 16px rgba(0, 0, 0, 0.35);
  }

  .new-menu-item {
    display: flex;
    align-items: center;
    gap: 0.5rem;
    width: 100%;
    padding: 0.5rem 0.875rem;
    border: none;
    background: none;
    color: var(--color-text);
    font-size: 0.875rem;
    font-family: inherit;
    cursor: pointer;
    text-align: left;
    transition: background-color 0.12s;
  }

  .new-menu-item:hover {
    background-color: var(--color-bg-tertiary);
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

  .search-wrapper {
    padding: 0 1rem 0.5rem;
    flex-shrink: 0;
  }

  /* Mobile: full width */
  @media (max-width: 767px) {
    .left-panel {
      width: 100%;
      min-width: 0;
    }
  }
</style>
