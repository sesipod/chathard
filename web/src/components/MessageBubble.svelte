<script>
  /**
   * Individual message bubble in a conversation.
   *
   * Props:
   *   message          {object}  { id, sender_id, ciphertext, created_at, status,
   *                               content, is_system, ephemeral_public_key, nonce }
   *   isOwn            {boolean} Whether this message was sent by the current user
   *   senderHandle     {string}  Sender's handle (shown in groups for received msgs)
   *   showDateSeparator {boolean} Whether to render a date header before this bubble
   *   dateText          {string}  Text for the date header ("Today", "Yesterday", "Jun 9")
   */
  export let message = {};
  export let isOwn = false;
  export let senderHandle = '';
  export let showDateSeparator = false;
  export let dateText = '';
  export let convId = '';

  import { getAutoShowImages, imageSettingsVersion } from '../lib/stores/settings.js';

  /** Format a timestamp into a short time string like "10:42 AM". */
  function formatTime(iso) {
    if (!iso) return '';
    const d = new Date(iso);
    return d.toLocaleTimeString(undefined, { hour: 'numeric', minute: '2-digit' });
  }

  /** Get display content from a message, decoding base64 ciphertext if possible. */
  function getContent(msg) {
    if (msg.content) return msg.content;
    if (!msg.ciphertext) return '';
    try {
      const bytes = typeof msg.ciphertext === 'string'
        ? Uint8Array.from(atob(msg.ciphertext), c => c.charCodeAt(0))
        : new Uint8Array(msg.ciphertext);
      return new TextDecoder('utf-8', { fatal: true }).decode(bytes);
    } catch {
      return '🔒 Encrypted';
    }
  }

  /** Detect file reference: 📎 filename (file_id) */
  $: fileMatch = content.match(/^📎\s*(.+?)\s*\(([a-f0-9]{32})\)$/);
  $: isFile = !!fileMatch;
  $: fileName = fileMatch ? fileMatch[1] : '';
  $: fileId = fileMatch ? fileMatch[2] : '';
  $: displayContent = isFile ? `📎 ${fileName}` : content;
  $: isImage = fileMatch && /\.(png|jpg|jpeg|gif|webp|svg|bmp)$/i.test(fileName);
  // Use comma operator so store subscription forces re-evaluation on toggle
  $: showImages = ($imageSettingsVersion, getAutoShowImages(convId || 'default'));
  $: shouldAutoLoad = isImage && showImages;
  let imageLoaded = false;
  let imageError = false;
  let imageUrl = '';
  let lightbox = false;

  // Load image blob when auto-show is enabled
  $: if (shouldAutoLoad && fileId && !imageLoaded && !imageError) {
    imageLoaded = true;
    loadImage(fileId);
  }

  async function loadImage(id) {
    try {
      const { default: api } = await import('../lib/api.js');
      const blob = await api.downloadFile(id);
      imageUrl = URL.createObjectURL(blob);
    } catch {
      imageError = true;
    }
  }

  async function downloadFile() {
    if (!fileId) return;
    try {
      const { default: api } = await import('../lib/api.js');
      const blob = await api.downloadFile(fileId);
      const url = URL.createObjectURL(blob);
      const a = document.createElement('a');
      a.href = url;
      a.download = fileName || 'download';
      a.click();
      URL.revokeObjectURL(url);
    } catch (e) {
      console.error('Download failed:', e);
    }
  }

  $: timeStr = formatTime(message.created_at);
  $: content = getContent(message);
  $: isSystem = !!message.is_system;
  $: status = message.status || 'sent';

  /** Determine display label for message status. */
  $: statusLabel = status === 'sent' ? 'Sent' : status === 'delivered' ? 'Delivered' : 'Read';
</script>

<div class="message-wrapper" class:own={isOwn} class:system={isSystem}>
  {#if showDateSeparator}
    <div class="date-separator">
      <span class="date-label">{dateText}</span>
    </div>
  {/if}

  {#if isSystem}
    <!-- System messages: centered, muted -->
    <div class="system-message">{content}</div>
  {:else}
    <div class="bubble-row">
      <div class="bubble" class:sent={isOwn} class:received={!isOwn}>
        {#if !isOwn && senderHandle}
          <div class="sender-handle">{senderHandle}</div>
        {/if}
        <div class="bubble-content">
          {#if isFile}
            {#if imageUrl}
              <img class="inline-image" src={imageUrl} alt={fileName} on:click={() => { lightbox = true; }} on:error={() => { imageError = true; imageUrl = ''; }} />
              <span class="file-caption">{fileName}</span>
              {#if lightbox}
                <div class="lightbox-bg" on:click={() => { lightbox = false; }} role="dialog">
                  <img class="lightbox-img" src={imageUrl} alt={fileName} on:click|stopPropagation />
                </div>
              {/if}
            {:else if imageError}
              <button class="file-download" on:click={downloadFile}>
                <svg width="18" height="18" viewBox="0 0 18 18" fill="none" stroke="currentColor" stroke-width="2"><path d="M4 12v2a2 2 0 002 2h6a2 2 0 002-2v-2M9 3v9M6 9l3 3 3-3"/></svg>
                <span>{displayContent}</span>
              </button>
            {:else}
              <button class="file-download" on:click={downloadFile}>
                <svg width="18" height="18" viewBox="0 0 18 18" fill="none" stroke="currentColor" stroke-width="2"><path d="M4 12v2a2 2 0 002 2h6a2 2 0 002-2v-2M9 3v9M6 9l3 3 3-3"/></svg>
                <span>{displayContent}</span>
              </button>
            {/if}
          {:else}
            <span class="bubble-text">{displayContent}</span>
          {/if}
        </div>
        <div class="bubble-meta">
          <span class="bubble-time">{timeStr}</span>
          {#if isOwn}
            <span class="status-icon" title={statusLabel} aria-label={statusLabel}>
              {#if status === 'sent'}
                <svg width="14" height="14" viewBox="0 0 14 14" fill="currentColor">
                  <path d="M5.5 8.5L3 6l-.7.7L5.5 10l6-6-.7-.7z"/>
                </svg>
              {:else if status === 'delivered'}
                <svg width="16" height="14" viewBox="0 0 16 14" fill="currentColor">
                  <path d="M5.5 8.5L3 6l-.7.7L5.5 10l6-6-.7-.7z"/>
                  <path d="M8.5 8.5L6 6l-.7.7L8.5 10l6-6-.7-.7z" opacity="0.6"/>
                </svg>
              {:else if status === 'read'}
                <svg width="16" height="14" viewBox="0 0 16 14" fill="var(--color-accent)">
                  <path d="M5.5 8.5L3 6l-.7.7L5.5 10l6-6-.7-.7z"/>
                  <path d="M8.5 8.5L6 6l-.7.7L8.5 10l6-6-.7-.7z"/>
                </svg>
              {/if}
            </span>
          {/if}
        </div>
      </div>
    </div>
  {/if}
</div>

<style>
  .message-wrapper { padding:.125rem 1rem; }
  .message-wrapper.own { display:flex; flex-direction:column; align-items:flex-end; }
  .message-wrapper.system { display:flex; flex-direction:column; align-items:center; }
  .date-separator { display:flex; align-items:center; justify-content:center; margin:1rem 0 .5rem; width:100%; }
  .date-label { padding:.25rem .75rem; border-radius:4px; background:var(--color-bg-tertiary); color:var(--color-text-muted); font-size:.75rem; font-weight:600; text-transform:uppercase; letter-spacing:.025em; }
  .system-message { font-size:.75rem; color:var(--color-text-muted); text-align:center; padding:.5rem 0; opacity:.75; max-width:80%; }
  .bubble-row { max-width:75%; min-width:80px; width:fit-content; }
  .bubble { padding:.5rem .75rem; border-radius:12px; position:relative; word-wrap:break-word; overflow-wrap:break-word; }
  .bubble.sent { background:var(--color-sent); color:#fff; border-bottom-right-radius:4px; }
  .bubble.received { background:var(--color-received); color:var(--color-text); border-bottom-left-radius:4px; }
  .sender-handle { font-size:.75rem; font-weight:600; color:var(--color-accent); margin-bottom:.125rem; }
  .bubble-content { display:flex; flex-direction:column; }
  .bubble-text { font-size:.875rem; line-height:1.4; white-space:pre-wrap; }
  .file-download { display:flex; align-items:center; gap:.5rem; background:none; border:none; color:inherit; cursor:pointer; padding:.25rem 0; font-size:.875rem; font-family:inherit; text-decoration:underline; text-underline-offset:3px; }
  .file-download:hover { opacity:.8; }
  .inline-image { max-width:280px; max-height:320px; border-radius:8px; cursor:pointer; object-fit:cover; }
  .inline-image:hover { opacity:.9; }
  .file-caption { font-size:.6875rem; color:inherit; opacity:.6; margin-top:2px; }
  .lightbox-bg { position:fixed; inset:0; z-index:200; display:flex; align-items:center; justify-content:center; background:rgba(0,0,0,.85); cursor:pointer; padding:1rem; }
  .lightbox-img { max-width:90vw; max-height:90vh; object-fit:contain; border-radius:8px; cursor:default; }
  .bubble-meta { display:flex; align-items:center; justify-content:flex-end; gap:.25rem; margin-top:.125rem; }
  .bubble.sent .bubble-meta { justify-content:flex-end; }
  .bubble-time { font-size:.6875rem; opacity:.7; line-height:1; }
  .bubble.sent .bubble-time { color:rgba(255,255,255,.75); }
  .bubble.received .bubble-time { color:var(--color-text-muted); }
  .status-icon { display:inline-flex; align-items:center; line-height:1; flex-shrink:0; }
  .bubble.sent .status-icon { color:rgba(255,255,255,.75); }
</style>
