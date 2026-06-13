<script>
  import { push } from 'svelte-spa-router';
  import {
    generateKeyPair,
    deriveKeys,
    generateRecoveryCodes,
    formatRecoveryCode,
  } from '../lib/crypto/keygen.js';
  import { encryptMasterKeyWithCode } from '../lib/crypto/recover.js';
  import { storeKeyPair, loadKeyPair } from '../lib/db.js';

  /** @type {'register'|'login'|'recover'} */
  let mode = 'login';

  // ── Shared state ──
  let handle = '';
  let error = '';
  let loading = false;

  // ── Register state ──
  let handleAvailable = null; // null = unchecked, true/false
  let checkingHandle = false;
  let recoveryCodesShown = false;
  let recoveryCodes = [];

  // ── Login state ──
  let challenge = '';
  let user_id = '';

  // ── Recover state ──
  let recoveryCode = '';

  // ── Handle availability check (debounced) ──
  let handleDebounce;

  async function checkHandle() {
    if (handle.length < 3) {
      handleAvailable = null;
      return;
    }
    checkingHandle = true;
    try {
      const res = await fetch(`/api/users/search?handle=${encodeURIComponent(handle)}`);
      const data = await res.json();
      // If search returns results with exact match, handle is taken
      const taken = Array.isArray(data) && data.some((u) => u.handle === handle);
      handleAvailable = !taken;
    } catch {
      handleAvailable = null;
    } finally {
      checkingHandle = false;
    }
  }

  function onHandleInput() {
    error = '';
    clearTimeout(handleDebounce);
    if (mode === 'register' && handle.length >= 3) {
      handleDebounce = setTimeout(checkHandle, 400);
    }
  }

  // ── Register ──
  async function handleRegister() {
    if (!handle || !handleAvailable) return;
    loading = true;
    error = '';

    try {
      // 1. Generate master Ed25519 keypair
      const masterKp = await generateKeyPair();

      // 2. Derive X25519 + auth keypair
      const derived = await deriveKeys(masterKp);

      // 3. Generate 10 recovery codes and encrypt master key
      const codeBytes = generateRecoveryCodes();
      const formattedCodes = codeBytes.map(bytes => formatRecoveryCode(bytes));
      const backups = [];
      for (let i = 0; i < formattedCodes.length; i++) {
        const code = formattedCodes[i];
        const salt = crypto.getRandomValues(new Uint8Array(32));
        const encryptedKey = await encryptMasterKeyWithCode(masterKp.privateKey, code, salt);
        const codeHashBytes = new Uint8Array(
          await crypto.subtle.digest('SHA-256', new TextEncoder().encode(code))
        );
        const codeHash = Array.from(codeHashBytes)
          .map((b) => b.toString(16).padStart(2, '0'))
          .join('');
        backups.push({
          recovery_code_hash: codeHash,
          encrypted_private_key: Array.from(encryptedKey),
          salt: Array.from(salt),
        });
      }
      // 4. Store keys locally
      await storeKeyPair({
        masterPub: masterKp.publicKey,
        masterPriv: masterKp.privateKey,
        x25519Pub: derived.x25519Pub,
        x25519Priv: derived.x25519Priv,
        authPub: derived.authPub,
        authPriv: derived.authPriv,
        authSalt: derived.authSalt,
      });

      // 5. Register with server
      const res = await fetch('/api/register', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({
          handle,
          public_key_ed25519: Array.from(masterKp.publicKey),
          public_key_x25519: Array.from(derived.x25519Pub),
          derived_public_key_ed25519: Array.from(derived.authPub),
          encrypted_key_backups: backups,
        }),
      });

      if (!res.ok) {
        const text = await res.text();
        throw new Error(text || 'Registration failed');
      }

      const data = await res.json();
      user_id = data.user_id;
      recoveryCodes = formattedCodes;
      recoveryCodesShown = true;
      // Login happens after user clicks "I've Saved These Codes"
    } catch (e) {
      error = e.message || 'Registration failed';
    } finally {
      loading = false;
    }
  }

  // ── Login ──
  async function handleLogin() {
    if (!handle) return;
    loading = true;
    error = '';

    try {
      // Check if keys exist in IndexedDB
      const keys = await loadKeyPair();
      if (!keys) {
        error = 'No keys found. You need to recover your account.';
        loading = false;
        return;
      }
      await performLogin();
    } catch (e) {
      error = e.message || 'Login failed';
    } finally {
      loading = false;
    }
  }

  async function performLogin() {
    // 1. Get challenge from server
    const chalRes = await fetch('/api/auth/challenge', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ handle }),
    });
    if (!chalRes.ok) throw new Error('Challenge failed');
    const chalData = await chalRes.json();
    challenge = chalData.challenge;

    // 2. Sign challenge with derived auth key
    const keys = await loadKeyPair();
    if (!keys) throw new Error('Keys not found');

    // Sign using Web Crypto API or noble fallback
    const challengeBytes = hexToBytes(challenge);
    let signature;
    try {
      const privKey = await crypto.subtle.importKey(
        'raw', keys.authPriv, { name: 'Ed25519' }, false, ['sign']
      );
      signature = await crypto.subtle.sign({ name: 'Ed25519' }, privKey, challengeBytes);
    } catch {
      // Fallback to @noble/curves
      const { ed25519 } = await import('@noble/curves/ed25519');
      signature = ed25519.sign(challengeBytes, keys.authPriv);
    }
    const sigHex = bytesToHex(new Uint8Array(signature));

    // 3. Verify with server
    const verifyRes = await fetch('/api/auth/verify', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ handle, challenge, signature: sigHex }),
    });
    if (!verifyRes.ok) throw new Error('Verification failed');
    const verifyData = await verifyRes.json();

    // 4. Store session token in sessionStorage
    sessionStorage.setItem('tailchat-token', verifyData.token);
    sessionStorage.setItem('tailchat-user-id', verifyData.user_id);

    // Navigate to chat
    push('/chat');
  }

  // ── Recover ──
  async function handleRecover() {
    if (!handle || !recoveryCode) return;
    loading = true;
    error = '';

    try {
      // 1. First, look up user by handle to get user_id
      const searchRes = await fetch(`/api/users/search?handle=${encodeURIComponent(handle)}`);
      const searchData = await searchRes.json();
      const user = Array.isArray(searchData)
        ? searchData.find((u) => u.handle === handle)
        : null;
      if (!user) throw new Error('User not found');

      const userId = user.id;

      // 2. Hash recovery code
      const codeHashBytes = new Uint8Array(
        await crypto.subtle.digest('SHA-256', new TextEncoder().encode(recoveryCode))
      );
      const codeHash = bytesToHex(codeHashBytes);

      // 3. Request encrypted key from server
      const recoverRes = await fetch('/api/recover', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ user_id: userId, recovery_code_hash: codeHash }),
      });
      if (!recoverRes.ok) throw new Error('Invalid recovery code');
      const recoverData = await recoverRes.json();

      // 4. Decrypt master key
      const encryptedKey = hexToBytes(recoverData.encrypted_private_key);
      const salt = hexToBytes(recoverData.salt);
      const { decryptMasterKeyWithCode } = await import('../lib/crypto/recover.js');
      const masterPriv = await decryptMasterKeyWithCode(encryptedKey, recoveryCode, salt);

      // 5. Re-derive all keys (need public key too — fetch from server or regenerate)
      // For now, we regenerate the Ed25519 public key from the private key
      const { ed25519 } = await import('@noble/curves/ed25519');
      const masterPub = ed25519.getPublicKey(masterPriv);
      const derived = await deriveKeys({ publicKey: masterPub, privateKey: masterPriv });

      // 6. Store restored keys
      await storeKeyPair({
        masterPub,
        masterPriv,
        x25519Pub: derived.x25519Pub,
        x25519Priv: derived.x25519Priv,
        authPub: derived.authPub,
        authPriv: derived.authPriv,
        authSalt: derived.authSalt,
      });

      // 7. Login with restored keys
      handle = user.handle;
      await performLogin();
    } catch (e) {
      error = e.message || 'Recovery failed';
    } finally {
      loading = false;
    }
  }

  // ── Dismiss recovery codes and navigate ──
  async function dismissCodes() {
    loading = true;
    try {
      await performLogin();
    } catch (e) {
      error = e.message || 'Login failed';
      loading = false;
      return;
    }
    recoveryCodesShown = false;
    push('/chat');
  }

  // ── Utility functions ──
  function bytesToHex(bytes) {
    return Array.from(bytes).map((b) => b.toString(16).padStart(2, '0')).join('');
  }

  function hexToBytes(hex) {
    const bytes = new Uint8Array(hex.length / 2);
    for (let i = 0; i < hex.length; i += 2) {
      bytes[i / 2] = parseInt(hex.substring(i, i + 2), 16);
    }
    return bytes;
  }
</script>

<div class="login-container">
  <div class="login-card">
    <!-- Logo / Title -->
    <div class="text-center mb-8">
      <h1 class="text-3xl font-bold tracking-tight" style="color: var(--color-accent);">
        TailChat
      </h1>
      <p class="mt-2" style="color: var(--color-text-muted); font-size: 0.875rem;">
        Zero-knowledge, E2E-encrypted messenger
      </p>
    </div>

    {#if recoveryCodesShown}
      <!-- Recovery codes display -->
      <div class="codes-panel">
        <h2 class="text-lg font-semibold mb-2">Recovery Codes</h2>
        <p class="mb-4" style="color: var(--color-text-muted); font-size: 0.875rem;">
          Save these codes somewhere safe. You'll need them to recover your account
          if you lose your device. Each code can only be used once.
        </p>
        <div class="codes-list">
          {#each recoveryCodes as code, i}
            <div class="code-item">
              <span class="code-index">{i + 1}.</span>
              <code class="code-value">{code}</code>
            </div>
          {/each}
        </div>
        <button
          class="btn btn-primary mt-6 w-full"
          on:click={dismissCodes}
        >
          I've Saved These Codes — Start Chatting
        </button>
      </div>
    {:else}
      <!-- Tab selector -->
      <div class="tab-bar">
        <button
          class="tab"
          class:active={mode === 'login'}
          on:click={() => { mode = 'login'; error = ''; }}
        >
          Login
        </button>
        <button
          class="tab"
          class:active={mode === 'register'}
          on:click={() => { mode = 'register'; error = ''; }}
        >
          Register
        </button>
        <button
          class="tab"
          class:active={mode === 'recover'}
          on:click={() => { mode = 'recover'; error = ''; }}
        >
          Recover
        </button>
      </div>

      <!-- Error message -->
      {#if error}
        <div class="error-banner">{error}</div>
      {/if}

      <!-- Login form -->
      {#if mode === 'login'}
        <form on:submit|preventDefault={handleLogin} class="space-y-4">
          <div>
            <label for="login-handle" class="label">Handle</label>
            <input
              id="login-handle"
              type="text"
              class="input"
              placeholder="your handle"
              bind:value={handle}
              required
            />
          </div>
          <button type="submit" class="btn btn-primary w-full" disabled={loading}>
            {loading ? 'Signing in...' : 'Sign In'}
          </button>
        </form>

      <!-- Register form -->
      {:else if mode === 'register'}
        <form on:submit|preventDefault={handleRegister} class="space-y-4">
          <div>
            <label for="reg-handle" class="label">Choose a Handle</label>
            <input
              id="reg-handle"
              type="text"
              class="input"
              placeholder="your handle"
              bind:value={handle}
              on:input={onHandleInput}
              required
              minlength="3"
            />
            {#if checkingHandle}
              <span class="field-status checking">Checking...</span>
            {:else if handleAvailable === true}
              <span class="field-status available">✓ Available</span>
            {:else if handleAvailable === false}
              <span class="field-status taken">✗ Taken</span>
            {/if}
          </div>
          <button
            type="submit"
            class="btn btn-primary w-full"
            disabled={loading || !handleAvailable}
          >
            {loading ? 'Creating account...' : 'Create Account'}
          </button>
        </form>

      <!-- Recover form -->
      {:else if mode === 'recover'}
        <form on:submit|preventDefault={handleRecover} class="space-y-4">
          <div>
            <label for="rec-handle" class="label">Handle</label>
            <input
              id="rec-handle"
              type="text"
              class="input"
              placeholder="your handle"
              bind:value={handle}
              required
            />
          </div>
          <div>
            <label for="rec-code" class="label">Recovery Code</label>
            <input
              id="rec-code"
              type="text"
              class="input"
              placeholder="X3KM-7FJ2-PQ9N-RT8B"
              bind:value={recoveryCode}
              required
            />
          </div>
          <button type="submit" class="btn btn-primary w-full" disabled={loading}>
            {loading ? 'Recovering...' : 'Recover Account'}
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
    display: flex;
    flex-direction: column;
    gap: 0.5rem;
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
</style>
