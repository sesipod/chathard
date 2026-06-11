<script>
  /**
   * Debounced search input with clear button.
   *
   * Props:
   *   placeholder  {string}   Input placeholder text
   *   value        {string}   Controlled value
   *
   * Events:
   *   on:search    → { detail: value }  fired after 300ms debounce
   *   on:clear     fired when the clear button is clicked
   */
  export let placeholder = 'Search...';
  export let value = '';

  import { createEventDispatcher } from 'svelte';
  const dispatch = createEventDispatcher();

  let debounceTimer;

  function handleInput(e) {
    value = e.target.value;
    clearTimeout(debounceTimer);
    debounceTimer = setTimeout(() => {
      dispatch('search', value);
    }, 300);
  }

  function handleClear() {
    value = '';
    clearTimeout(debounceTimer);
    dispatch('clear');
    dispatch('search', '');
  }
</script>

<div class="search-root">
  <svg class="search-icon" width="16" height="16" viewBox="0 0 16 16" fill="none" stroke="currentColor" stroke-width="1.5">
    <circle cx="7" cy="7" r="5" />
    <line x1="11" y1="11" x2="14.5" y2="14.5" />
  </svg>
  <input
    type="search"
    class="search-input"
    {placeholder}
    bind:value
    on:input={handleInput}
    aria-label={placeholder}
  />
  {#if value}
    <button class="clear-btn" on:click={handleClear} aria-label="Clear search" title="Clear">
      <svg width="14" height="14" viewBox="0 0 14 14" fill="none" stroke="currentColor" stroke-width="1.5">
        <line x1="2" y1="2" x2="12" y2="12" />
        <line x1="12" y1="2" x2="2" y2="12" />
      </svg>
    </button>
  {/if}
</div>

<style>
  .search-root {
    position: relative;
    display: flex;
    align-items: center;
  }

  .search-icon {
    position: absolute;
    left: 10px;
    top: 50%;
    transform: translateY(-50%);
    color: var(--color-text-muted);
    pointer-events: none;
    flex-shrink: 0;
  }

  .search-input {
    width: 100%;
    padding: 0.5rem 2rem 0.5rem 2rem;
    background-color: var(--color-bg);
    border: 1px solid var(--color-border);
    border-radius: 8px;
    color: var(--color-text);
    font-size: 0.875rem;
    outline: none;
    transition: border-color 0.15s;
    appearance: none;
    -webkit-appearance: none;
  }

  .search-input:focus {
    border-color: var(--color-accent);
  }

  .search-input::placeholder {
    color: var(--color-text-muted);
    opacity: 0.6;
  }

  /* Remove native search clear in WebKit */
  .search-input::-webkit-search-decoration,
  .search-input::-webkit-search-cancel-button,
  .search-input::-webkit-search-results-button,
  .search-input::-webkit-search-results-decoration {
    display: none;
  }

  .clear-btn {
    position: absolute;
    right: 6px;
    top: 50%;
    transform: translateY(-50%);
    display: flex;
    align-items: center;
    justify-content: center;
    width: 24px;
    height: 24px;
    border: none;
    border-radius: 50%;
    background-color: var(--color-bg-tertiary);
    color: var(--color-text-muted);
    cursor: pointer;
    transition: background-color 0.15s;
    padding: 0;
  }

  .clear-btn:hover {
    background-color: var(--color-border);
    color: var(--color-text);
  }
</style>
