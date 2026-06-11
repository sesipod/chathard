<script>
  /**
   * Reusable modal/dialog overlay.
   *
   * Props:
   *   title  {string}   Header text
   *   show   {boolean}  Visibility toggle
   *
   * Slots:
   *   default  — modal body content
   *   footer   — optional action buttons at the bottom
   *
   * Events:
   *   on:close  — emitted when the modal should be dismissed
   */
  export let title = '';
  export let show = false;

  import { createEventDispatcher } from 'svelte';
  const dispatch = createEventDispatcher();

  function close() {
    dispatch('close');
  }

  function onBackdropClick(e) {
    if (e.target === e.currentTarget) close();
  }

  function onKeydown(e) {
    if (e.key === 'Escape') close();
  }
</script>

<svelte:window on:keydown={onKeydown} />

{#if show}
  <div class="backdrop" role="dialog" aria-modal="true" aria-label={title} on:click={onBackdropClick}>
    <div class="modal">
      <!-- Header -->
      <div class="modal-header">
        <h2 class="modal-title">{title}</h2>
        <button class="close-btn" on:click={close} aria-label="Close" title="Close">
          <svg width="18" height="18" viewBox="0 0 18 18" fill="none" stroke="currentColor" stroke-width="2">
            <line x1="3" y1="3" x2="15" y2="15" />
            <line x1="15" y1="3" x2="3" y2="15" />
          </svg>
        </button>
      </div>

      <!-- Body -->
      <div class="modal-body">
        <slot />
      </div>

      <!-- Footer -->
      <div class="modal-footer">
        <slot name="footer" />
      </div>
    </div>
  </div>
{/if}

<style>
  .backdrop {
    position: fixed;
    inset: 0;
    z-index: 100;
    display: flex;
    align-items: center;
    justify-content: center;
    background-color: rgba(0, 0, 0, 0.6);
    backdrop-filter: blur(4px);
    padding: 1rem;
  }

  .modal {
    background-color: var(--color-bg-secondary);
    border: 1px solid var(--color-border);
    border-radius: 12px;
    width: 100%;
    max-width: 480px;
    max-height: 85vh;
    display: flex;
    flex-direction: column;
    box-shadow: 0 8px 32px rgba(0, 0, 0, 0.4);
    overflow: hidden;
  }

  .modal-header {
    display: flex;
    align-items: center;
    justify-content: space-between;
    padding: 1rem 1.25rem;
    border-bottom: 1px solid var(--color-border);
    flex-shrink: 0;
  }

  .modal-title {
    font-size: 1.125rem;
    font-weight: 700;
    color: var(--color-text);
  }

  .close-btn {
    display: flex;
    align-items: center;
    justify-content: center;
    width: 32px;
    height: 32px;
    border: none;
    border-radius: 50%;
    background-color: transparent;
    color: var(--color-text-muted);
    cursor: pointer;
    transition: background-color 0.15s;
  }

  .close-btn:hover {
    background-color: var(--color-bg-tertiary);
    color: var(--color-text);
  }

  .modal-body {
    padding: 1.25rem;
    overflow-y: auto;
    flex: 1;
  }

  .modal-footer {
    display: flex;
    align-items: center;
    justify-content: flex-end;
    gap: 0.5rem;
    padding: 0.75rem 1.25rem;
    border-top: 1px solid var(--color-border);
    flex-shrink: 0;
  }
</style>
