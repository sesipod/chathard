<script>
  import { createEventDispatcher, onMount } from 'svelte';
  import api from '../lib/api.js';

  export let show = false;
  export let convId = '';
  export let isGroup = false;

  const dispatch = createEventDispatcher();

  let files = [];
  let loading = false;
  let error = '';

  $: if (show && convId) {
    loadFiles();
  }

  async function loadFiles() {
    loading = true;
    error = '';
    try {
      const data = await api.fetchConversationFiles(convId, isGroup);
      files = data.files || [];
    } catch (e) {
      error = e.message || 'Failed to load files';
      files = [];
    } finally {
      loading = false;
    }
  }

  function close() {
    dispatch('close');
  }

  function handleBackdropClick(e) {
    if (e.target === e.currentTarget) close();
  }

  function formatSize(bytes) {
    if (!bytes) return '—';
    if (bytes < 1024) return bytes + ' B';
    if (bytes < 1024 * 1024) return (bytes / 1024).toFixed(1) + ' KB';
    return (bytes / (1024 * 1024)).toFixed(1) + ' MB';
  }

  function formatDate(iso) {
    if (!iso) return '';
    const d = new Date(iso);
    return d.toLocaleDateString(undefined, { month: 'short', day: 'numeric', year: 'numeric' });
  }

  async function handleDownload(fileId) {
    try {
      const blob = await api.downloadFile(fileId);
      const url = URL.createObjectURL(blob);
      const a = document.createElement('a');
      a.href = url;
      a.download = fileId + '.enc';
      a.click();
      URL.revokeObjectURL(url);
    } catch (e) {
      console.error('Download failed:', e);
    }
  }
</script>

{#if show}
<!-- svelte-ignore a11y-click-events-have-key-events -->
<div class="modal-overlay" on:click={handleBackdropClick} role="dialog">
  <div class="modal">
    <header class="modal-header">
      <h2>Files</h2>
      <button class="close-btn" on:click={close} aria-label="Close">&times;</button>
    </header>
    <div class="modal-body">
      {#if loading}
        <div class="state-msg">Loading files…</div>
      {:else if error}
        <div class="state-msg error">{error}</div>
      {:else if files.length === 0}
        <div class="state-msg">No files shared in this conversation.</div>
      {:else}
        <div class="file-list">
          {#each files as file (file.id)}
            <div class="file-row">
              <div class="file-icon">
                <svg width="24" height="24" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.5">
                  <path d="M14 2H6a2 2 0 0 0-2 2v16a2 2 0 0 0 2 2h12a2 2 0 0 0 2-2V8l-6-6z"/>
                  <polyline points="14,2 14,8 20,8"/>
                </svg>
              </div>
              <div class="file-info">
                <span class="file-id">{file.id.slice(0, 12)}…</span>
                <span class="file-meta">{formatSize(file.size_bytes)} · {formatDate(file.created_at)}</span>
              </div>
              <button class="download-btn" on:click={() => handleDownload(file.id)} title="Download">
                <svg width="18" height="18" viewBox="0 0 18 18" fill="none" stroke="currentColor" stroke-width="2">
                  <path d="M4 12v2a2 2 0 002 2h6a2 2 0 002-2v-2M9 3v9M6 9l3 3 3-3"/>
                </svg>
              </button>
            </div>
          {/each}
        </div>
      {/if}
    </div>
  </div>
</div>
{/if}

<style>
  .modal-overlay { position:fixed; inset:0; z-index:100; display:flex; align-items:center; justify-content:center; background:rgba(0,0,0,.5); padding:1rem; }
  .modal { max-width:480px; width:100%; max-height:80vh; display:flex; flex-direction:column; border-radius:12px; background:var(--color-bg-secondary); border:1px solid var(--color-border); box-shadow:0 8px 32px rgba(0,0,0,.4); }
  .modal-header { display:flex; align-items:center; justify-content:space-between; padding:.75rem 1rem; border-bottom:1px solid var(--color-border); flex-shrink:0; }
  .modal-header h2 { font-size:1rem; font-weight:700; margin:0; }
  .close-btn { display:flex; align-items:center; justify-content:center; width:32px; height:32px; border:none; border-radius:6px; background:none; color:var(--color-text-muted); font-size:1.25rem; cursor:pointer; font-family:inherit; }
  .close-btn:hover { background:var(--color-bg-tertiary); color:var(--color-text); }
  .modal-body { flex:1; overflow-y:auto; padding:.5rem 0; }
  .state-msg { padding:2rem 1rem; text-align:center; color:var(--color-text-muted); font-size:.875rem; }
  .state-msg.error { color:var(--color-danger); }
  .file-list { display:flex; flex-direction:column; }
  .file-row { display:flex; align-items:center; gap:.75rem; padding:.625rem 1rem; border-bottom:1px solid var(--color-border); transition:background .12s; }
  .file-row:last-child { border-bottom:none; }
  .file-row:hover { background:var(--color-bg-tertiary); }
  .file-icon { flex-shrink:0; color:var(--color-text-muted); }
  .file-info { flex:1; min-width:0; display:flex; flex-direction:column; gap:2px; }
  .file-id { font-size:.8125rem; font-weight:600; color:var(--color-text); font-family:'SF Mono','Cascadia Code',monospace; }
  .file-meta { font-size:.6875rem; color:var(--color-text-muted); }
  .download-btn { display:flex; align-items:center; justify-content:center; width:36px; height:36px; border:none; border-radius:8px; background:none; color:var(--color-text-muted); cursor:pointer; flex-shrink:0; transition:all .12s; }
  .download-btn:hover { background:var(--color-accent); color:#fff; }
</style>
