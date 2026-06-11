<script>
  /**
   * New 1:1 chat modal — search users by handle, select to start a conversation.
   *
   * Props:
   *   show  {boolean}  Visibility toggle
   *
   * Events:
   *   on:close    — fired when the modal should be dismissed
   *   on:select   — fired with { detail: { user } } when a user is selected
   */
  import Modal from './common/Modal.svelte';
  import SearchInput from './common/SearchInput.svelte';
  import Avatar from './common/Avatar.svelte';
  import { createEventDispatcher } from 'svelte';
  import api from '../lib/api.js';

  export let show = false;

  const dispatch = createEventDispatcher();

  let query = '';
  let results = [];
  let searching = false;
  let error = '';

  /** Debounced search — fires when the search input emits its debounced value. */
  async function handleSearch(e) {
    query = e.detail;
    if (query.length < 3) {
      results = [];
      return;
    }

    searching = true;
    error = '';
    try {
      const data = await api.searchUsers(query);
      results = data.users || data || [];
    } catch (err) {
      error = err.message || 'Search failed';
      results = [];
    } finally {
      searching = false;
    }
  }

  function handleClear() {
    query = '';
    results = [];
    error = '';
  }

  function handleSelect(user) {
    dispatch('select', { user });
  }

  function handleClose() {
    dispatch('close');
  }

  /** Format a key fingerprint for display (first 16 chars + "..."). */
  function formatFingerprint(fp) {
    if (!fp) return '';
    return fp.length > 16 ? fp.slice(0, 16) + '…' : fp;
  }
</script>

<Modal title="New Chat" {show} on:close={handleClose}>
  <div class="search-section">
    <SearchInput
      placeholder="Search by handle (min 3 chars)..."
      value=""
      on:search={handleSearch}
      on:clear={handleClear}
    />
  </div>

  {#if error}
    <div class="error-message">{error}</div>
  {/if}

  {#if searching}
    <div class="status-text">Searching…</div>
  {/if}

  {#if !searching && query.length >= 3 && results.length === 0 && !error}
    <div class="status-text">No users found.</div>
  {/if}

  <div class="results-list">
    {#each results as user (user.id || user.uuid)}
      <button
        class="result-item"
        on:click={() => handleSelect(user)}
        aria-label="Start chat with {user.handle}"
      >
        <Avatar name={user.handle || '?'} size={40} />
        <div class="result-info">
          <span class="result-handle">{user.handle}</span>
          {#if user.key_fingerprint}
            <span class="result-fp">{formatFingerprint(user.key_fingerprint)}</span>
          {/if}
        </div>
      </button>
    {/each}
  </div>

  <div slot="footer">
    <button class="btn btn-cancel" on:click={handleClose}>Cancel</button>
  </div>
</Modal>

<style>
  .search-section {
    margin-bottom: 0.75rem;
  }

  .error-message {
    padding: 0.5rem 0;
    color: var(--color-danger);
    font-size: 0.8125rem;
  }

  .status-text {
    padding: 1rem 0;
    text-align: center;
    color: var(--color-text-muted);
    font-size: 0.8125rem;
  }

  .results-list {
    display: flex;
    flex-direction: column;
    gap: 2px;
    max-height: 300px;
    overflow-y: auto;
  }

  .result-item {
    display: flex;
    align-items: center;
    gap: 0.75rem;
    width: 100%;
    padding: 0.5rem 0.625rem;
    border: none;
    border-radius: 8px;
    background: none;
    color: var(--color-text);
    cursor: pointer;
    text-align: left;
    font-family: inherit;
    font-size: inherit;
    transition: background-color 0.12s;
  }

  .result-item:hover {
    background-color: var(--color-bg-tertiary);
  }

  .result-info {
    display: flex;
    flex-direction: column;
    gap: 1px;
    min-width: 0;
  }

  .result-handle {
    font-size: 0.875rem;
    font-weight: 500;
  }

  .result-fp {
    font-size: 0.75rem;
    color: var(--color-text-muted);
    font-family: monospace;
  }

  .btn {
    display: inline-flex;
    align-items: center;
    justify-content: center;
    gap: 0.5rem;
    padding: 0.5rem 1rem;
    font-size: 0.875rem;
    font-weight: 600;
    border: none;
    border-radius: 8px;
    cursor: pointer;
    font-family: inherit;
    transition: opacity 0.15s;
  }

  .btn-cancel {
    background-color: var(--color-bg-tertiary);
    color: var(--color-text);
  }

  .btn-cancel:hover {
    opacity: 0.85;
  }
</style>
