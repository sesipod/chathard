<script>
  /**
   * Safety number for a 1:1 chat. Both people see the same 60 digits. Compare
   * them in person or over a call you trust: if they match, nobody — including
   * the server — swapped either person's key.
   */
  import { createEventDispatcher } from 'svelte';
  import { getUser, markVerified } from '../lib/directory.js';
  import { getIdentity } from '../lib/keyring.js';
  import { fingerprint, safetyNumber } from '../lib/crypto/identity.js';

  export let show = false;
  export let userId = '';

  const dispatch = createEventDispatcher();
  let number = '';
  let theirFp = '';
  let handle = '';
  let verified = false;
  let error = '';

  $: if (show && userId) load();

  async function load() {
    error = '';
    try {
      const me = await getIdentity();
      const them = await getUser(userId);
      handle = them.handle;
      verified = !!them.verified;
      number = safetyNumber(me.edPub, them.edPub);
      theirFp = fingerprint(them.edPub);
    } catch (e) {
      error = e.message || 'Could not load security details';
    }
  }

  async function toggle() {
    verified = !verified;
    await markVerified(userId, verified);
  }
</script>

{#if show}
  <div class="bg" on:click={() => dispatch('close')}>
    <div class="dlg" on:click|stopPropagation>
      <h3>Verify security with @{handle}</h3>
      {#if error}
        <p class="err">{error}</p>
      {:else}
        <p>
          Messages are end-to-end encrypted. To be sure no one is intercepting, check that <strong>@{handle}</strong> sees
          exactly these numbers — in person or on a call you trust.
        </p>
        <div class="num">{number}</div>
        <p class="small">Their key fingerprint: <code>{theirFp}</code></p>
        <label class="check"><input type="checkbox" checked={verified} on:change={toggle} /> I compared the numbers and they match</label>
      {/if}
      <div class="actions"><button class="btn" on:click={() => dispatch('close')}>Done</button></div>
    </div>
  </div>
{/if}

<style>
  .bg { position: fixed; inset: 0; z-index: 100; display: flex; align-items: center; justify-content: center; background: rgba(0,0,0,.6); }
  .dlg { background: var(--color-bg-secondary); border: 1px solid var(--color-border); border-radius: 12px; padding: 1.25rem; max-width: 420px; width: calc(100% - 2rem); color: var(--color-text); }
  h3 { margin: 0 0 .5rem; font-size: 1rem; }
  p { font-size: .875rem; color: var(--color-text-muted); line-height: 1.5; }
  .num { font-family: ui-monospace, monospace; font-size: 1.1rem; letter-spacing: .08em; line-height: 1.9; text-align: center; padding: .75rem; border: 1px dashed var(--color-accent); border-radius: 8px; margin: .75rem 0; word-spacing: .4em; user-select: all; }
  .small { font-size: .75rem; }
  .err { color: #f87171; }
  .check { display: flex; gap: .5rem; align-items: center; font-size: .8125rem; margin-top: .75rem; }
  .actions { display: flex; justify-content: flex-end; margin-top: 1rem; }
  .btn { padding: .5rem 1rem; border-radius: 8px; border: 1px solid var(--color-border); background: transparent; color: var(--color-text); cursor: pointer; }
</style>
