<script>
  /**
   * New group creation modal — multi-step: name → members → review.
   *
   * Props:
   *   show  {boolean}  Visibility toggle
   *
   * Events:
   *   on:close      — fired when the modal is dismissed
   *   on:create     — fired with { detail: { name, memberIds, encryptedName, members } }
   *                   when the group is created (parent handles encryption + API call)
   */
  import Modal from './common/Modal.svelte';
  import SearchInput from './common/SearchInput.svelte';
  import Avatar from './common/Avatar.svelte';
  import { createEventDispatcher } from 'svelte';
  import api from '../lib/api.js';

  export let show = false;

  const dispatch = createEventDispatcher();

  // ── Wizard steps ──
  let step = 1;
  const STEP_NAME = 1;
  const STEP_MEMBERS = 2;
  const STEP_REVIEW = 3;

  // ── State ──
  let groupName = '';
  let searchQuery = '';
  let searchResults = [];
  let searching = false;
  let searchError = '';
  let selectedMembers = []; // Array of { handle, id, uuid, public_key_x25519, key_fingerprint }
  let creating = false;
  let createError = '';

  // ── Validation ──
  $: validName = groupName.trim().length > 0;
  $: hasMinMembers = selectedMembers.length >= 1;
  $: canReview = validName && hasMinMembers;

  function reset() {
    step = 1;
    groupName = '';
    searchQuery = '';
    searchResults = [];
    selectedMembers = [];
    creating = false;
    createError = '';
  }

  function handleClose() {
    reset();
    dispatch('close');
  }

  function goToMembers() {
    if (!validName) return;
    step = STEP_MEMBERS;
  }

  function goToReview() {
    if (!canReview) return;
    step = STEP_REVIEW;
  }

  function goBack() {
    if (step === STEP_MEMBERS) { step = STEP_NAME; return; }
    if (step === STEP_REVIEW) { step = STEP_MEMBERS; return; }
  }

  // ── Member search ──
  async function handleSearch(e) {
    searchQuery = e.detail;
    if (searchQuery.length < 3) {
      searchResults = [];
      return;
    }

    searching = true;
    searchError = '';
    try {
      const data = await api.searchUsers(searchQuery);
      searchResults = (data.users || data || []).filter(
        (u) => !selectedMembers.some((m) => (m.id || m.uuid) === (u.id || u.uuid))
      );
    } catch (err) {
      searchError = err.message || 'Search failed';
      searchResults = [];
    } finally {
      searching = false;
    }
  }

  function addMember(user) {
    if (selectedMembers.some((m) => (m.id || m.uuid) === (user.id || user.uuid))) return;
    selectedMembers = [...selectedMembers, user];
    searchResults = [];
    searchQuery = '';
  }

  function removeMember(userId) {
    selectedMembers = selectedMembers.filter((m) => (m.id || m.uuid) !== userId);
  }

  // ── Create ──
  async function handleCreate() {
    if (!canReview || creating) return;
    creating = true;
    createError = '';

    try {
      const memberIds = selectedMembers.map((m) => m.id || m.uuid);
      const members = selectedMembers;
      dispatch('create', {
        name: groupName.trim(),
        memberIds,
        members,
      });
      reset();
    } catch (err) {
      createError = err.message || 'Failed to create group';
    } finally {
      creating = false;
    }
  }

  /** Determine modal title based on current step. */
  $: modalTitle = step === STEP_NAME
    ? 'New Group'
    : step === STEP_MEMBERS
      ? 'Add Members'
      : 'Review Group';
</script>

<Modal title={modalTitle} {show} on:close={handleClose}>
  <!-- Step 1: Group Name -->
  {#if step === STEP_NAME}
    <div class="step-content">
      <label class="field-label" for="group-name">Group Name</label>
      <input
        id="group-name"
        type="text"
        class="text-input"
        bind:value={groupName}
        placeholder="Enter group name..."
        maxlength="64"
        on:keydown={(e) => { if (e.key === 'Enter' && validName) goToMembers(); }}
        aria-label="Group name"
      />
      <p class="field-hint">This name will be encrypted end-to-end.</p>
    </div>
  {/if}

  <!-- Step 2: Add Members -->
  {#if step === STEP_MEMBERS}
    <div class="step-content">
      <SearchInput
        placeholder="Search by handle (min 3 chars)..."
        value={searchQuery}
        on:search={handleSearch}
        on:clear={() => { searchQuery = ''; searchResults = []; }}
      />

      {#if searchError}
        <div class="error-message">{searchError}</div>
      {/if}

      {#if searching}
        <div class="status-text">Searching…</div>
      {/if}

      <!-- Search results -->
      <div class="search-results">
        {#each searchResults as user (user.id || user.uuid)}
          <button
            class="result-item"
            on:click={() => addMember(user)}
            aria-label="Add {user.handle}"
          >
            <Avatar name={user.handle || '?'} size={36} />
            <div class="result-info">
              <span class="result-handle">{user.handle}</span>
              {#if user.key_fingerprint}
                <span class="result-fp">{user.key_fingerprint.slice(0, 16)}…</span>
              {/if}
            </div>
            <svg class="add-icon" width="18" height="18" viewBox="0 0 18 18" fill="none" stroke="currentColor" stroke-width="2">
              <line x1="9" y1="3" x2="9" y2="15" />
              <line x1="3" y1="9" x2="15" y2="9" />
            </svg>
          </button>
        {/each}
      </div>

      <!-- Selected members (chips) -->
      {#if selectedMembers.length > 0}
        <div class="selected-section">
          <p class="selected-label">{selectedMembers.length} member{selectedMembers.length !== 1 ? 's' : ''}</p>
          <div class="chips">
            {#each selectedMembers as member (member.id || member.uuid)}
              <div class="chip">
                <span class="chip-name">{member.handle}</span>
                <button
                  class="chip-remove"
                  on:click={() => removeMember(member.id || member.uuid)}
                  aria-label="Remove {member.handle}"
                >
                  <svg width="14" height="14" viewBox="0 0 14 14" fill="none" stroke="currentColor" stroke-width="2">
                    <line x1="3" y1="3" x2="11" y2="11" />
                    <line x1="11" y1="3" x2="3" y2="11" />
                  </svg>
                </button>
              </div>
            {/each}
          </div>
        </div>
      {:else}
        <div class="status-text">Add at least 1 person to start a group.</div>
      {/if}
    </div>
  {/if}

  <!-- Step 3: Review -->
  {#if step === STEP_REVIEW}
    <div class="step-content">
      <div class="review-section">
        <div class="review-row">
          <span class="review-label">Group Name</span>
          <span class="review-value">{groupName}</span>
        </div>
        <div class="review-row">
          <span class="review-label">Members</span>
          <span class="review-value">{selectedMembers.length}</span>
        </div>
      </div>

      <div class="member-list">
        {#each selectedMembers as member (member.id || member.uuid)}
          <div class="member-row">
            <Avatar name={member.handle || '?'} size={32} />
            <span class="member-handle">{member.handle}</span>
          </div>
        {/each}
      </div>

      {#if createError}
        <div class="error-message">{createError}</div>
      {/if}
    </div>
  {/if}

  <!-- Footer (must be direct child of Modal for named slots in Svelte 5) -->
  <div slot="footer">
    {#if step === STEP_NAME}
      <button class="btn btn-cancel" on:click={handleClose}>Cancel</button>
      <button class="btn btn-primary" disabled={!validName} on:click={goToMembers}>Next</button>
    {:else if step === STEP_MEMBERS}
      <button class="btn btn-cancel" on:click={goBack}>Back</button>
      <button class="btn btn-primary" disabled={!hasMinMembers} on:click={goToReview}>Next ({selectedMembers.length})</button>
    {:else if step === STEP_REVIEW}
      <button class="btn btn-cancel" on:click={goBack}>Back</button>
      <button class="btn btn-primary" disabled={creating} on:click={handleCreate}>
        {creating ? 'Creating…' : 'Create Group'}
      </button>
    {/if}
  </div>
</Modal>

<style>
  .step-content { display:flex; flex-direction:column; gap:.75rem; }
  .field-label { font-size:.8125rem; font-weight:600; color:var(--color-text-muted); text-transform:uppercase; letter-spacing:.05em; }
  .text-input { width:100%; padding:.625rem .75rem; border:1px solid var(--color-border); border-radius:8px; background:var(--color-bg); color:var(--color-text); font-size:.9375rem; font-family:inherit; outline:none; transition:border-color .15s; }
  .text-input:focus { border-color:var(--color-accent); }
  .text-input::placeholder { color:var(--color-text-muted); opacity:.6; }
  .field-hint { font-size:.75rem; color:var(--color-text-muted); opacity:.7; }
  .error-message { color:var(--color-danger); font-size:.8125rem; }
  .status-text { text-align:center; color:var(--color-text-muted); font-size:.8125rem; padding:.5rem 0; }
  .search-results { display:flex; flex-direction:column; gap:2px; max-height:160px; overflow-y:auto; }
  .result-item { display:flex; align-items:center; gap:.625rem; width:100%; padding:.5rem; border:none; border-radius:8px; background:none; color:var(--color-text); cursor:pointer; text-align:left; font-family:inherit; font-size:inherit; transition:background .12s; }
  .result-item:hover { background:var(--color-bg-tertiary); }
  .result-info { display:flex; flex-direction:column; gap:1px; min-width:0; flex:1; }
  .result-handle { font-size:.875rem; font-weight:500; }
  .result-fp { font-size:.6875rem; color:var(--color-text-muted); font-family:monospace; }
  .add-icon { color:var(--color-accent); flex-shrink:0; }
  .selected-section { border-top:1px solid var(--color-border); padding-top:.75rem; }
  .selected-label { font-size:.75rem; font-weight:600; color:var(--color-text-muted); margin-bottom:.5rem; }
  .chips { display:flex; flex-wrap:wrap; gap:.375rem; }
  .chip { display:inline-flex; align-items:center; gap:.25rem; padding:.25rem .5rem; border-radius:16px; background:var(--color-bg-tertiary); font-size:.8125rem; color:var(--color-text); }
  .chip-name { max-width:100px; overflow:hidden; text-overflow:ellipsis; white-space:nowrap; }
  .chip-remove { display:flex; align-items:center; justify-content:center; width:18px; height:18px; border:none; border-radius:50%; background:none; color:var(--color-text-muted); cursor:pointer; padding:0; transition:background .12s,color .12s; flex-shrink:0; }
  .chip-remove:hover { background:var(--color-border); color:var(--color-text); }
  .review-section { display:flex; flex-direction:column; gap:.5rem; padding-bottom:.75rem; border-bottom:1px solid var(--color-border); }
  .review-row { display:flex; align-items:center; justify-content:space-between; }
  .review-label { font-size:.8125rem; color:var(--color-text-muted); }
  .review-value { font-size:.875rem; font-weight:600; color:var(--color-text); }
  .member-list { display:flex; flex-direction:column; gap:.375rem; max-height:200px; overflow-y:auto; }
  .member-row { display:flex; align-items:center; gap:.625rem; padding:.25rem 0; }
  .member-handle { font-size:.875rem; color:var(--color-text); }
  .btn { display:inline-flex; align-items:center; justify-content:center; gap:.5rem; padding:.5rem 1rem; font-size:.875rem; font-weight:600; border:none; border-radius:8px; cursor:pointer; font-family:inherit; transition:opacity .15s; }
  .btn:disabled { opacity:.4; cursor:default; }
  .btn-primary { background:var(--color-accent); color:#fff; }
  .btn-primary:not(:disabled):hover { opacity:.9; }
  .btn-cancel { background:var(--color-bg-tertiary); color:var(--color-text); }
  .btn-cancel:hover { opacity:.85; }
</style>
