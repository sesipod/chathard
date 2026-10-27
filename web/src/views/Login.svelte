<script>
  import { onMount } from 'svelte';
  import api from '../lib/api.js';
  import { auth } from '../lib/stores/auth.js';
  import { deriveIdentity, formatAccountKey, newAccountKey, parseAccountKey } from '../lib/crypto/identity.js';
  import { getIdentity, hasRememberedKey } from '../lib/keyring.js';
  import qrcode from 'qrcode-generator';

  /** @type {'login'|'register'} */
  let mode = 'login';

  let handle = localStorage.getItem('tailchat-last-handle') || '';
  let error = '';
  let loading = false;

  // ── Register state ──
  let handleAvailable = null;
  let checkingHandle = false;
  let handleDebounce;
  let pendingSeed = null; // set after the account exists, until the user confirms they saved the key
  let keyText = '';
  let qrSvg = '';
  let savedConfirmed = false;
  let copied = false;

  // ── Login state ──
  let keyInput = '';
  let remember = false;
  let rememberedAvailable = false;
  let canScan = typeof window !== 'undefined' && 'BarcodeDetector' in window;
  let scanning = false;
  let videoEl;
  let stopScan = () => {};
  let fileInput;

  onMount(async () => {
    rememberedAvailable = await hasRememberedKey();
    return () => stopScan();
  });

  // ── Handle availability (debounced) ──
  async function checkHandle() {
    if (handle.length < 3) {
      handleAvailable = null;
      return;
    }
    checkingHandle = true;
    try {
      handleAvailable = await api.handleAvailable(handle.trim());
    } catch {
      handleAvailable = null;
    } finally {
      checkingHandle = false;
    }
  }

  function onHandleInput() {
    error = '';
    clearTimeout(handleDebounce);
    if (mode === 'register' && handle.length >= 3) handleDebounce = setTimeout(checkHandle, 400);
  }

  function setMode(m) {
    mode = m;
    error = '';
    stopScan();
  }

  // ── Register: create the key in this browser, send only public keys ──
  async function handleRegister() {
    if (!handle || !handleAvailable) return;
    loading = true;
    error = '';
    try {
      const seed = newAccountKey();
      const id = deriveIdentity(seed);
      await api.register(handle.trim(), id.edPub, id.xPub, id.authPub);
      pendingSeed = seed;
      keyText = formatAccountKey(seed);
      const qr = qrcode(0, 'M');
      qr.addData(keyText);
      qr.make();
      qrSvg = qr.createSvgTag({ cellSize: 4, margin: 0, scalable: true });
    } catch (e) {
      error = e.message || 'Registration failed';
    } finally {
      loading = false;
    }
  }

  async function finishRegistration() {
    if (!savedConfirmed || !pendingSeed) return;
    loading = true;
    error = '';
    try {
      localStorage.setItem('tailchat-last-handle', handle.trim());
      await auth.login(handle.trim(), pendingSeed, remember);
      pendingSeed = null;
      keyText = '';
    } catch (e) {
      error = e.message || 'Login failed';
      loading = false;
    }
  }

  async function copyKey() {
    try {
      await navigator.clipboard.writeText(keyText);
      copied = true;
      setTimeout(() => (copied = false), 2000);
    } catch {
      error = 'Could not copy — select the key and copy it manually';
    }
  }

  function downloadKey() {
    const body = `TailChat account key for ${handle}\n\n${keyText}\n\nKeep this file private. Anyone with this key can read your messages.\nIf you lose it, your account and message history cannot be recovered.\n`;
    const url = URL.createObjectURL(new Blob([body], { type: 'text/plain' }));
    const a = document.createElement('a');
    a.href = url;
    a.download = `tailchat-key-${handle}.txt`;
    a.click();
    URL.revokeObjectURL(url);
  }

  // ── Login: any device, using the account key ──
  async function handleLogin() {
    if (!handle) return;
    loading = true;
    error = '';
    try {
      const seed = parseAccountKey(keyInput);
      localStorage.setItem('tailchat-last-handle', handle.trim());
      await auth.login(handle.trim(), seed, remember);
      keyInput = '';
    } catch (e) {
      error = e.message || 'Login failed';
    } finally {
      loading = false;
    }
  }

  async function continueOnThisDevice() {
    loading = true;
    error = '';
    try {
      const id = await getIdentity();
      if (!id) throw new Error('No saved key on this device');
      await auth.login(handle.trim(), id.seed, true);
    } catch (e) {
      error = e.message || 'Login failed';
    } finally {
      loading = false;
    }
  }

  async function onKeyFile(e) {
    const file = e.target.files?.[0];
    if (!file) return;
    const text = await file.text();
    const m = text.toUpperCase().match(/(?:[A-Z2-9]{5}-){10}[A-Z2-9]{5}/);
    if (m) keyInput = m[0];
    else error = 'No account key found in that file';
    e.target.value = '';
  }

  async function startScan() {
    error = '';
    try {
      const stream = await navigator.mediaDevices.getUserMedia({ video: { facingMode: 'environment' } });
      scanning = true;
      await Promise.resolve();
      videoEl.srcObject = stream;
      await videoEl.play();
      const detector = new BarcodeDetector({ formats: ['qr_code'] });
      let active = true;
      stopScan = () => {
        active = false;
        stream.getTracks().forEach((t) => t.stop());
        scanning = false;
        stopScan = () => {};
      };
      const tick = async () => {
        if (!active) return;
        try {
          const codes = await detector.detect(videoEl);
          const m = codes[0]?.rawValue?.toUpperCase().match(/(?:[A-Z2-9]{5}-){10}[A-Z2-9]{5}/);
          if (m) {
            keyInput = m[0];
            stopScan();
            return;
          }
        } catch {
          /* keep trying */
        }
        requestAnimationFrame(tick);
      };
      tick();
    } catch {
      scanning = false;
      error = 'Could not open the camera';
    }
  }
</script>

<div class="login-container">
  <div class="login-card">
    <div class="text-center mb-8">
      <h1 class="text-3xl font-bold tracking-tight" style="color: var(--color-accent);">TailChat</h1>
      <p class="mt-2" style="color: var(--color-text-muted); font-size: 0.875rem;">
        Zero-knowledge, E2E-encrypted messenger
      </p>
    </div>

    {#if pendingSeed}
      <!-- Show the account key once, before the user can continue -->
      <div class="codes-panel">
        <h2 class="text-lg font-semibold mb-2">Your account key</h2>
        <p class="warn">
          This key <strong>is</strong> your account. It decrypts your messages and signs you in on any device.
          <strong>We cannot recover it</strong> — not even the server admin. Save it in a password manager or print the QR code.
        </p>
        <div class="key-box">{keyText}</div>
        <div class="qr">{@html qrSvg}</div>
        <div class="row">
          <button type="button" class="btn btn-secondary btn-sm" on:click={copyKey}>{copied ? 'Copied ✓' : 'Copy'}</button>
          <button type="button" class="btn btn-secondary btn-sm" on:click={downloadKey}>Download file</button>
        </div>
        <label class="check">
          <input type="checkbox" bind:checked={savedConfirmed} />
          <span>I have saved my account key somewhere safe. I understand it cannot be recovered if lost.</span>
        </label>
        <label class="check">
          <input type="checkbox" bind:checked={remember} />
          <span>Remember this device (don't ask for the key again here). Only on devices you own.</span>
        </label>
        {#if error}<div class="error-banner">{error}</div>{/if}
        <button class="btn btn-primary w-full" disabled={!savedConfirmed || loading} on:click={finishRegistration}>
          {loading ? 'Signing in...' : 'Start Chatting'}
        </button>
      </div>
    {:else}
      <div class="tab-bar">
        <button class="tab" class:active={mode === 'login'} on:click={() => setMode('login')}>Sign In</button>
        <button class="tab" class:active={mode === 'register'} on:click={() => setMode('register')}>Create Account</button>
      </div>

      {#if error}<div class="error-banner">{error}</div>{/if}

      {#if mode === 'login'}
        <form on:submit|preventDefault={handleLogin} class="space-y-4">
          <div>
            <label for="login-handle" class="label">Handle</label>
            <input id="login-handle" type="text" class="input" placeholder="your handle" autocomplete="username" bind:value={handle} required />
          </div>

          {#if rememberedAvailable}
            <button type="button" class="btn btn-primary w-full" disabled={loading || !handle} on:click={continueOnThisDevice}>
              {loading ? 'Signing in...' : 'Continue on this device'}
            </button>
            <p class="warn" style="text-align:center;">or enter your account key to use a different one</p>
          {/if}

          <div>
            <label for="login-key" class="label">Account key</label>
            <textarea
              id="login-key" class="input" rows="3" spellcheck="false" autocomplete="off" autocapitalize="characters"
              placeholder="XXXXX-XXXXX-XXXXX-…" bind:value={keyInput}
            ></textarea>
            <div class="row mt-3">
              <button type="button" class="btn btn-secondary btn-sm" on:click={() => fileInput.click()}>Choose key file</button>
              {#if canScan}
                <button type="button" class="btn btn-secondary btn-sm" on:click={scanning ? stopScan : startScan}>
                  {scanning ? 'Stop camera' : 'Scan QR'}
                </button>
              {/if}
            </div>
            <input bind:this={fileInput} type="file" accept=".txt,.json,text/plain" hidden on:change={onKeyFile} />
            {#if scanning}<!-- svelte-ignore a11y-media-has-caption --><video class="scan-video" bind:this={videoEl} playsinline muted></video>{/if}
          </div>

          <label class="check">
            <input type="checkbox" bind:checked={remember} />
            <span>Remember this device — only on a device you own</span>
          </label>
          <button type="submit" class="btn btn-primary w-full" disabled={loading || !keyInput}>
            {loading ? 'Signing in...' : 'Sign In'}
          </button>
          <p class="warn">
            New phone, tablet or laptop? Just enter your handle and account key here — all your past messages will appear.
          </p>
        </form>
      {:else}
        <form on:submit|preventDefault={handleRegister} class="space-y-4">
          <div>
            <label for="reg-handle" class="label">Choose a Handle</label>
            <input
              id="reg-handle" type="text" class="input" placeholder="your handle" autocomplete="username"
              bind:value={handle} on:input={onHandleInput} required minlength="3" maxlength="32" pattern="[A-Za-z0-9_.\-]+"
            />
            {#if checkingHandle}
              <span class="field-status checking">Checking...</span>
            {:else if handleAvailable === true}
              <span class="field-status available">✓ Available</span>
            {:else if handleAvailable === false}
              <span class="field-status taken">✗ Taken</span>
            {/if}
          </div>
          <p class="warn">
            Your account key is created in this browser and shown once. There are no passwords and nothing for the server to reset.
          </p>
          <button type="submit" class="btn btn-primary w-full" disabled={loading || !handleAvailable}>
            {loading ? 'Creating account...' : 'Create Account'}
          </button>
        </form>
      {/if}
    {/if}
  </div>
</div>

<style>
  .login-container {
    display: flex;
    align-items: center;
    justify-content: center;
    min-height: 100vh;
    background-color: var(--color-bg);
    padding: 1rem;
  }

  .login-card {
    width: 100%;
    max-width: 420px;
    background-color: var(--color-bg-secondary);
    border: 1px solid var(--color-border);
    border-radius: 12px;
    padding: 2rem;
  }

  .tab-bar {
    display: flex;
    gap: 1px;
    background-color: var(--color-border);
    border-radius: 8px;
    overflow: hidden;
    margin-bottom: 1.5rem;
  }

  .tab {
    flex: 1;
    padding: 0.625rem 1rem;
    text-align: center;
    font-size: 0.875rem;
    font-weight: 500;
    border: none;
    background-color: var(--color-bg-tertiary);
    color: var(--color-text-muted);
    cursor: pointer;
    transition: background-color 0.15s, color 0.15s;
  }

  .tab.active {
    background-color: var(--color-bg-secondary);
    color: var(--color-text);
  }

  .tab:hover:not(.active) {
    background-color: var(--color-bg-tertiary);
    color: var(--color-text);
  }

  .label {
    display: block;
    font-size: 0.875rem;
    font-weight: 500;
    margin-bottom: 0.375rem;
    color: var(--color-text-muted);
  }

  .input {
    width: 100%;
    padding: 0.625rem 0.75rem;
    background-color: var(--color-bg);
    border: 1px solid var(--color-border);
    border-radius: 8px;
    color: var(--color-text);
    font-size: 0.9375rem;
    outline: none;
    transition: border-color 0.15s;
  }

  .input:focus {
    border-color: var(--color-accent);
  }

  .input::placeholder {
    color: var(--color-text-muted);
    opacity: 0.6;
  }

  .field-status {
    display: block;
    font-size: 0.75rem;
    margin-top: 0.25rem;
  }

  .field-status.checking {
    color: var(--color-text-muted);
  }

  .field-status.available {
    color: var(--color-success);
  }

  .field-status.taken {
    color: var(--color-danger);
  }

  .btn {
    display: inline-flex;
    align-items: center;
    justify-content: center;
    padding: 0.625rem 1.25rem;
    font-size: 0.9375rem;
    font-weight: 600;
    border: none;
    border-radius: 8px;
    cursor: pointer;
    transition: opacity 0.15s;
  }

  .btn:disabled {
    opacity: 0.4;
    cursor: not-allowed;
  }

  .btn-primary {
    background-color: var(--color-accent);
    color: #fff;
  }

  .btn-primary:hover:not(:disabled) {
    opacity: 0.9;
  }

  .error-banner {
    background-color: rgba(239, 68, 68, 0.1);
    border: 1px solid rgba(239, 68, 68, 0.3);
    color: var(--color-danger);
    padding: 0.625rem 0.75rem;
    border-radius: 8px;
    font-size: 0.8125rem;
    margin-bottom: 1rem;
  }

  .codes-panel {
    text-align: left;
  }

  .codes-list {
    display: grid;
    grid-template-columns: repeat(auto-fill, minmax(180px, 1fr));
    gap: 0.5rem;
    max-height: 240px;
    overflow-y: auto;
    padding-right: 0.25rem;
  }

  .code-item {
    display: flex;
    align-items: center;
    gap: 0.5rem;
    background-color: var(--color-bg);
    padding: 0.5rem 0.75rem;
    border-radius: 6px;
  }

  .code-index {
    font-size: 0.8125rem;
    color: var(--color-text-muted);
    min-width: 1.5rem;
  }

  .code-value {
    font-family: 'SF Mono', 'Fira Code', 'Consolas', monospace;
    font-size: 0.875rem;
    letter-spacing: 0.05em;
    color: var(--color-text);
  }

  .space-y-4 > * + * {
    margin-top: 1rem;
  }

  .mt-2 { margin-top: 0.5rem; }
  .mt-4 { margin-top: 1rem; }
  .mt-6 { margin-top: 1.5rem; }
  .mt-8 { margin-top: 2rem; }
  .mb-2 { margin-bottom: 0.5rem; }
  .mb-4 { margin-bottom: 1rem; }
  .mb-8 { margin-bottom: 2rem; }
  .w-full { width: 100%; }
  .text-center { text-align: center; }

  .key-box {
    font-family: ui-monospace, monospace;
    font-size: 0.95rem;
    line-height: 1.7;
    letter-spacing: 0.04em;
    word-break: break-all;
    padding: 0.75rem;
    border: 1px dashed var(--color-accent);
    border-radius: 8px;
    background: var(--color-bg);
    user-select: all;
  }
  .qr { display: flex; justify-content: center; margin: 1rem 0; }
  .qr :global(svg) { width: 180px; height: 180px; background: #fff; padding: 6px; border-radius: 8px; }
  .row { display: flex; gap: 0.5rem; flex-wrap: wrap; }
  .btn-secondary {
    background: transparent;
    color: var(--color-text);
    border: 1px solid var(--color-border);
  }
  .btn-secondary:hover:not(:disabled) { border-color: var(--color-accent); }
  .btn-sm { padding: 0.4rem 0.75rem; font-size: 0.8125rem; width: auto; }
  textarea.input { resize: none; font-family: ui-monospace, monospace; letter-spacing: 0.04em; }
  .check { display: flex; align-items: flex-start; gap: 0.5rem; font-size: 0.8125rem; color: var(--color-text-muted); margin: 0.75rem 0; }
  .warn { font-size: 0.8125rem; color: var(--color-text-muted); margin: 0.75rem 0; }
  .warn strong { color: var(--color-text); }
  .link { background: none; border: none; color: var(--color-accent); cursor: pointer; font-size: 0.8125rem; padding: 0; }
  .scan-video { width: 100%; border-radius: 8px; margin-top: 0.5rem; }
  .mt-3 { margin-top: 0.75rem; }
</style>
