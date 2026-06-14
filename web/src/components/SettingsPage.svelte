<script>
  import { createEventDispatcher, onMount } from 'svelte';
  import Avatar from './common/Avatar.svelte';
  import { auth } from '../lib/stores/auth.js';
  import { settings, theme, defaultRetention } from '../lib/stores/settings.js';
  import api from '../lib/api.js';

  export let show = false;
  export let conversations = [];

  const dispatch = createEventDispatcher();

  let userProfile = null;
  let recoveryRemaining = 0;
  let serverInfo = null;
  let showRecoveryView = false;
  let confirmLogout = false;
  let confirmExportKey = false;
  let editingRetentionConv = null;
  let editingRetentionValue = 'Never';

  const themeOptions = ['Dark', 'Light', 'System'];
  const retentionOptions = ['Never', '1h', '24h', '7d', '30d', '90d'];

  onMount(async () => {
    const unsub = auth.subscribe(($a) => { userProfile = $a.user; });
    try { const me = await api.getMe(); recoveryRemaining = me.recovery_codes_remaining || 0; } catch {}
    try { serverInfo = await api.health(); } catch {}
    return unsub;
  });

  function back() { dispatch('close'); }
  function copy(t) { navigator.clipboard.writeText(t).catch(() => {}); }
  function trunc(s, n) { return s?.length > n ? s.slice(0, n) + '…' : s || ''; }
  function setTheme(m) { settings.setTheme(m.toLowerCase()); }
  function setDefRet(e) { settings.setDefaultRetention(e.target.value); }
  function editRet(c) { editingRetentionConv = c; editingRetentionValue = 'Never'; editingRetentionConv = editingRetentionConv; }
  async function saveRet() {
    if (!editingRetentionConv) return;
    try { await api.updateRetention({ conversationWith: editingRetentionConv.user_id || editingRetentionConv.id, expiresIn: editingRetentionValue === 'Never' ? '' : editingRetentionValue }); } catch {}
    editingRetentionConv = null;
    editingRetentionConv = editingRetentionConv; // trigger reactivity
  }
  function cancelRet() { editingRetentionConv = null; editingRetentionConv = editingRetentionConv; }
  async function doLogout() { await auth.logout(); }
  function doExport() { confirmExportKey = false; }
  function convName(c) { return c.handle || c.name || 'Unknown'; }
  function convKey(c) { return c.user_id || c.id; }

  // Per-conversation image toggle state (reactive, synced with localStorage)
  let imageToggles = {};
  function getShowImages(c) {
    const k = convKey(c);
    if (!(k in imageToggles)) imageToggles[k] = settings.getAutoShowImages(k);
    return imageToggles[k];
  }
  function toggleImages(c) {
    const k = convKey(c);
    imageToggles[k] = !getShowImages(c);
    settings.setAutoShowImages(k, imageToggles[k]);
    imageToggles = { ...imageToggles }; // trigger Svelte reactivity
  }
</script>

{#if show}
<div class="overlay"><div class="page">
<header class="hdr"><button class="back" on:click={back}><svg width="18" height="18" viewBox="0 0 20 20" fill="none" stroke="currentColor" stroke-width="2"><polyline points="12,4 6,10 12,16"/></svg> Settings</button></header>
<div class="scroll">
{#if showRecoveryView}
<section class="sec">
  <div class="sec-hdr"><button class="back" on:click={() => { showRecoveryView = false; }}><svg width="16" height="16" viewBox="0 0 20 20" fill="none" stroke="currentColor" stroke-width="2"><polyline points="12,4 6,10 12,16"/></svg> Back</button><h3 class="st">Recovery Codes</h3></div>
  <div class="card">
    <p class="p-3">You have <strong>{recoveryRemaining}</strong> of 10 recovery codes remaining.</p>
    <div class="warn"><svg width="16" height="16" viewBox="0 0 16 16" fill="none" stroke="currentColor" stroke-width="1.5"><circle cx="8" cy="8" r="6"/><line x1="8" y1="5" x2="8" y2="9"/><line x1="8" y1="11" x2="8" y2="11"/></svg><span>Generating a new set invalidates all existing codes.</span></div>
    <button class="btn btn-danger-outline w-full m-3" disabled>Generate New Codes</button>
  </div>
</section>
{:else}
<section class="sec"><h3 class="st">Account</h3><div class="card">
  <div class="row"><span>Your handle</span><span class="mono">@{userProfile?.handle || '…'}</span></div>
  <div class="row"><span>Your UUID</span><button class="cpy" on:click={() => copy(userProfile?.uuid || '')}><span class="mono">{trunc(userProfile?.uuid, 8)}</span>📋</button></div>
  <div class="row"><span>Public key fingerprint</span><button class="cpy" on:click={() => copy(userProfile?.key_fingerprint || '')}><span class="mono">{trunc(userProfile?.key_fingerprint, 16)}</span>📋</button></div>
</div></section>

<section class="sec"><h3 class="st">Security</h3><div class="card">
  <button class="row clickable" on:click={() => { showRecoveryView = true; }}><span>Recovery codes</span><span class="flex-row"><span class="badge">{recoveryRemaining} of 10 remaining</span>▶</span></button>
  <div class="row"><span>Export private key</span><button class="btn btn-sm" on:click={() => { confirmExportKey = true; }}>Export</button></div>
  <div class="row"><span>Session</span><button class="btn btn-sm btn-danger" on:click={() => { confirmLogout = true; }}>Log out</button></div>
</div></section>

<section class="sec"><h3 class="st">Appearance</h3><div class="card"><div class="row">
  <span>Theme</span>
  <div class="seg" role="radiogroup">{#each themeOptions as opt}<button class="seg-btn" class:active={$theme === opt.toLowerCase()} on:click={() => setTheme(opt)}>{opt}</button>{/each}</div>
</div></div></section>

<section class="sec"><h3 class="st">Conversation Defaults</h3><div class="card">
  <div class="row">
    <span>Default auto-delete</span>
    <select class="sel" value={$defaultRetention} on:change={setDefRet}>{#each retentionOptions as opt}<option value={opt}>{opt}</option>{/each}</select>
  </div>
</div></section>

<section class="sec"><h3 class="st">Per-Conversation Settings</h3>
{#if conversations.length === 0}<div class="card"><p class="muted">No conversations yet.</p></div>
{:else}{#each conversations as conv (conv.user_id || conv.id)}
<div class="card conv-card">
  <div class="conv-head">
    <Avatar name={convName(conv)} size={28} />
    <span class="conv-name">{convName(conv)}</span>
  </div>
  <div class="row">
    <span>Default auto-delete</span>
    <span>{#if editingRetentionConv && (editingRetentionConv.user_id || editingRetentionConv.id) === (conv.user_id || conv.id)}<select class="sel ret-sel" bind:value={editingRetentionValue}>{#each retentionOptions as opt}<option value={opt}>{opt}</option>{/each}</select><button class="btn btn-xs btn-primary" on:click={saveRet}>Save</button><button class="btn btn-xs" on:click={cancelRet}>Cancel</button>{:else}<span class="badge ret-badge">{conv.expires_in || 'Never'}</span><button class="btn btn-xs" on:click={() => editRet(conv)}>Change</button>{/if}</span>
  </div>
  <div class="row">
    <span>Auto show images</span>
    <label class="toggle">
      <input type="checkbox" checked={getShowImages(conv)} on:change={() => toggleImages(conv)} />
      <span class="toggle-slider"></span>
    </label>
  </div>
</div>{/each}{/if}
</section>

<section class="sec"><h3 class="st">About</h3><div class="card">
  <div class="row"><span>App version</span><span class="mono">{serverInfo?.version || '1.0.0'}</span></div>
  <div class="row"><span>Tailnet</span><span class="mono">{serverInfo?.tailnet_domain || '…'}</span></div>
  <div class="row"><span>Server</span><span class="mono">{serverInfo ? `${serverInfo.status} · ${serverInfo.uptime}` : '…'}</span></div>
</div></section>
{/if}
</div></div></div>
{/if}

{#if confirmLogout}<div class="dlg-bg" on:click={() => { confirmLogout = false; }}><div class="dlg" on:click|stopPropagation>
  <h3>Log out?</h3><p>You will need to re-authenticate via Tailscale and prove ownership of your keypair to log back in.</p>
  <div class="dlg-actions"><button class="btn" on:click={() => { confirmLogout = false; }}>Cancel</button><button class="btn btn-danger" on:click={doLogout}>Log out</button></div>
</div></div>{/if}

{#if confirmExportKey}<div class="dlg-bg" on:click={() => { confirmExportKey = false; }}><div class="dlg" on:click|stopPropagation>
  <h3>Export private key?</h3><p>Your private key will be saved as an encrypted file. Keep it safe.</p>
  <div class="dlg-actions"><button class="btn" on:click={() => { confirmExportKey = false; }}>Cancel</button><button class="btn btn-primary" on:click={doExport}>Export</button></div>
</div></div>{/if}

<style>
.overlay{position:fixed;inset:0;z-index:50;display:flex;background:var(--color-bg)}
.page{width:100%;max-width:640px;margin:0 auto;display:flex;flex-direction:column;background:var(--color-bg)}
.hdr{display:flex;align-items:center;padding:.75rem 1rem;border-bottom:1px solid var(--color-border);background:var(--color-bg-secondary);flex-shrink:0}
.back{display:inline-flex;align-items:center;gap:4px;padding:.375rem .5rem;border:none;border-radius:6px;background:none;color:var(--color-text);font-size:1rem;font-weight:600;cursor:pointer;font-family:inherit}
.back:hover{background:var(--color-bg-tertiary)}
.scroll{flex:1;overflow-y:auto;padding:1rem}
.sec{margin-bottom:1.25rem}
.sec-hdr{display:flex;align-items:center;gap:.5rem;margin-bottom:.5rem}
.st{font-size:.8125rem;font-weight:700;text-transform:uppercase;letter-spacing:.05em;color:var(--color-text-muted);margin-bottom:.5rem}
.card{background:var(--color-bg-secondary);border:1px solid var(--color-border);border-radius:10px;overflow:hidden}
.row{display:flex;align-items:center;justify-content:space-between;padding:.75rem 1rem;gap:.75rem;border-bottom:1px solid var(--color-border)}
.row:last-child{border-bottom:none}
.row.clickable{cursor:pointer}
.row.clickable:hover{background:var(--color-bg-tertiary)}
.row>span:first-child{font-size:.9375rem;color:var(--color-text);flex-shrink:0}
.mono{font-family:'SF Mono','Cascadia Code','JetBrains Mono',Consolas,monospace;font-size:.8125rem;color:var(--color-text-muted);text-align:right}
.cpy{display:inline-flex;align-items:center;gap:4px;padding:.25rem .5rem;border:none;border-radius:6px;background:none;color:var(--color-text-muted);cursor:pointer;font-family:inherit}
.cpy:hover{background:var(--color-bg-tertiary);color:var(--color-text)}
.flex-row{display:flex;align-items:center;gap:.5rem;color:var(--color-text-muted);font-size:.875rem}
.badge{display:inline-flex;align-items:center;padding:2px 8px;border-radius:4px;font-size:.75rem;font-weight:600;background:var(--color-bg-tertiary);color:var(--color-text-muted)}
.ret-badge{color:var(--color-accent);font-family:monospace;font-size:.6875rem}
.muted{padding:1rem;font-size:.875rem;color:var(--color-text-muted);text-align:center}
.seg{display:inline-flex;border:1px solid var(--color-border);border-radius:8px;overflow:hidden}
.seg-btn{padding:.375rem .75rem;border:none;background:none;color:var(--color-text-muted);font-size:.8125rem;font-weight:500;cursor:pointer;font-family:inherit}
.seg-btn:not(:last-child){border-right:1px solid var(--color-border)}
.seg-btn.active{background:var(--color-accent);color:#fff}
.seg-btn:hover:not(.active){background:var(--color-bg-tertiary)}
.sel{padding:.375rem .625rem;border:1px solid var(--color-border);border-radius:6px;background:var(--color-bg);color:var(--color-text);font-size:.8125rem;font-family:inherit;cursor:pointer;outline:none}
.sel:focus{border-color:var(--color-accent)}
.conv-card{margin-bottom:.75rem}
.conv-head{display:flex;align-items:center;gap:.5rem;padding:.625rem .875rem;border-bottom:1px solid var(--color-border)}
.conv-name{font-size:.875rem;font-weight:600;flex:1}
.ret-sel{min-width:80px}
.btn{display:inline-flex;align-items:center;justify-content:center;gap:4px;padding:.5rem .875rem;font-size:.8125rem;font-weight:600;border:none;border-radius:6px;cursor:pointer;font-family:inherit;white-space:nowrap;background:var(--color-bg-tertiary);color:var(--color-text)}
.btn-xs{padding:.25rem .5rem;font-size:.75rem}
.btn-primary{background:var(--color-accent);color:#fff}
.btn-danger{background:var(--color-danger);color:#fff}
.btn-danger-outline{background:transparent;color:var(--color-danger);border:1px solid var(--color-danger)}
.w-full{width:calc(100% - 2rem)}
.m-3{margin:.75rem 1rem}
.p-3{padding:.75rem 1rem;font-size:.9375rem;color:var(--color-text)}
.warn{display:flex;gap:.5rem;padding:.75rem 1rem;background:rgba(239,68,68,.08);border-top:1px solid rgba(239,68,68,.2);font-size:.8125rem;color:var(--color-text-muted);line-height:1.4;align-items:flex-start}
.warn svg{flex-shrink:0;margin-top:2px;color:var(--color-danger)}
.dlg-bg{position:fixed;inset:0;z-index:100;display:flex;align-items:center;justify-content:center;background:rgba(0,0,0,.6);backdrop-filter:blur(4px);padding:1rem}
.dlg{background:var(--color-bg-secondary);border:1px solid var(--color-border);border-radius:12px;width:100%;max-width:400px;padding:1.25rem}
.dlg h3{font-size:1.125rem;font-weight:700;margin-bottom:.5rem}
.dlg p{font-size:.875rem;color:var(--color-text-muted);line-height:1.5;margin-bottom:1.25rem}
.dlg-actions{display:flex;justify-content:flex-end;gap:.5rem}
@media(max-width:767px){.page{max-width:100%}.scroll{padding:.75rem}}
.toggle{position:relative;display:inline-block;width:44px;height:24px}
.toggle input{opacity:0;width:0;height:0}
.toggle-slider{position:absolute;cursor:pointer;inset:0;background:var(--color-bg-tertiary);border-radius:24px;transition:.2s;border:1px solid var(--color-border)}
.toggle-slider:before{content:'';position:absolute;height:18px;width:18px;left:2px;bottom:2px;background:var(--color-text-muted);border-radius:50%;transition:.2s}
.toggle input:checked+.toggle-slider{background:var(--color-accent);border-color:var(--color-accent)}
.toggle input:checked+.toggle-slider:before{transform:translateX(20px);background:#fff}
</style>
