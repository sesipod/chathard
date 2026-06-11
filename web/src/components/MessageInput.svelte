<script>
  /**
   * Message input bar — sticky bottom bar with attachment, textarea, and send.
   *
   * Props:
   *   typingUser  {string|null}  Handle of user currently typing (shown above input)
   *
   * Events:
   *   on:send     — fired with { detail: text } when the send button is clicked or Enter pressed
   *   on:attach   — fired with { detail: File } when a file is selected
   */
  import { createEventDispatcher } from 'svelte';

  export let typingUser = null;

  const dispatch = createEventDispatcher();

  let text = '';
  let fileInput;
  let textareaEl;

  /** Auto-resize textarea up to 4 lines (~96px). */
  function autoResize() {
    if (!textareaEl) return;
    textareaEl.style.height = 'auto';
    textareaEl.style.height = Math.min(textareaEl.scrollHeight, 96) + 'px';
  }

  function handleSend() {
    const trimmed = text.trim();
    if (!trimmed) return;
    dispatch('send', trimmed);
    text = '';
    // Reset height after clearing
    if (textareaEl) {
      textareaEl.style.height = 'auto';
    }
  }

  function handleKeydown(e) {
    if (e.key === 'Enter' && !e.shiftKey) {
      e.preventDefault();
      handleSend();
    }
  }

  function handleFileSelect(e) {
    const file = e.target.files?.[0];
    if (file) {
      dispatch('attach', file);
    }
    // Reset so the same file can be re-selected
    e.target.value = '';
  }

  function handleAttachClick() {
    fileInput?.click();
  }

  $: hasText = text.trim().length > 0;
</script>

<div class="message-input-wrapper">
  {#if typingUser}
    <div class="typing-indicator">{typingUser} is typing…</div>
  {/if}

  <div class="input-bar">
    <!-- Attachment button -->
    <button
      class="action-btn"
      on:click={handleAttachClick}
      title="Attach file"
      aria-label="Attach file"
    >
      <svg width="20" height="20" viewBox="0 0 20 20" fill="none" stroke="currentColor" stroke-width="1.5">
        <path d="M10 3v10a3 3 0 0 0 6 0V5a5 5 0 0 0-10 0v9a7 7 0 0 0 14 0V3" />
      </svg>
    </button>

    <input
      type="file"
      bind:this={fileInput}
      on:change={handleFileSelect}
      class="file-input"
      aria-hidden="true"
      tabindex="-1"
    />

    <!-- Text input -->
    <textarea
      bind:this={textareaEl}
      bind:value={text}
      on:keydown={handleKeydown}
      on:input={autoResize}
      class="text-input"
      placeholder="Type a message..."
      rows="1"
      aria-label="Message text"
    ></textarea>

    <!-- Send button -->
    <button
      class="send-btn"
      class:active={hasText}
      disabled={!hasText}
      on:click={handleSend}
      title="Send"
      aria-label="Send message"
    >
      <svg width="20" height="20" viewBox="0 0 20 20" fill="none" stroke="currentColor" stroke-width="2">
        <line x1="3" y1="10" x2="17" y2="10" />
        <polyline points="11,4 17,10 11,16" />
      </svg>
    </button>
  </div>
</div>

<style>
  .message-input-wrapper { border-top:1px solid var(--color-border); background:var(--color-bg-secondary); flex-shrink:0; }
  .typing-indicator { padding:.25rem 1rem; font-size:.75rem; color:var(--color-text-muted); font-style:italic; line-height:1.4; }
  .input-bar { display:flex; align-items:flex-end; gap:.5rem; padding:.5rem .75rem; }
  .action-btn { display:flex; align-items:center; justify-content:center; width:36px; height:36px; border:none; border-radius:50%; background:transparent; color:var(--color-text-muted); cursor:pointer; flex-shrink:0; transition:background .15s,color .15s; }
  .action-btn:hover { background:var(--color-bg-tertiary); color:var(--color-text); }
  .file-input { display:none; }
  .text-input { flex:1; padding:.5rem .75rem; border:1px solid var(--color-border); border-radius:8px; background:var(--color-bg); color:var(--color-text); font-size:.875rem; font-family:inherit; line-height:1.4; resize:none; outline:none; max-height:96px; transition:border-color .15s; }
  .text-input:focus { border-color:var(--color-accent); }
  .text-input::placeholder { color:var(--color-text-muted); opacity:.6; }
  .send-btn { display:flex; align-items:center; justify-content:center; width:36px; height:36px; border:none; border-radius:50%; background:transparent; color:var(--color-text-muted); cursor:pointer; flex-shrink:0; transition:background .15s,color .15s; }
  .send-btn.active { background:var(--color-accent); color:#fff; }
  .send-btn:disabled { opacity:.4; cursor:default; }
</style>
