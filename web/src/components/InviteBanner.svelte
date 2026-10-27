<script>
  /**
   * Pending group invitations. Until you accept, the group is not in your list,
   * you hold no key for it and cannot read anything in it; declining deletes the
   * key that was addressed to you.
   */
  import { createEventDispatcher } from 'svelte';
  export let invites = [];
  const dispatch = createEventDispatcher();
</script>

{#if invites.length > 0}
  <div class="invites" role="region" aria-label="Group invitations">
    {#each invites as inv (inv.group_id)}
      <div class="invite">
        <div class="text"><strong>@{inv.handle}</strong> invited you to join a group</div>
        <div class="actions">
          <button class="accept" on:click={() => dispatch('accept', inv)}>Accept</button>
          <button class="decline" on:click={() => dispatch('decline', inv)}>Decline</button>
        </div>
      </div>
    {/each}
  </div>
{/if}

<style>
  .invites { position: fixed; top: 1rem; left: 1rem; z-index: 150; display: flex; flex-direction: column; gap: .5rem; max-width: 340px; }
  .invite { background: var(--color-bg-secondary); border: 1px solid var(--color-accent); border-radius: 10px; padding: .75rem; box-shadow: 0 4px 12px rgba(0,0,0,.35); }
  .text { font-size: .875rem; color: var(--color-text); margin-bottom: .5rem; }
  .actions { display: flex; gap: .5rem; }
  button { flex: 1; padding: .4rem .75rem; border-radius: 8px; font-size: .8125rem; font-weight: 600; cursor: pointer; border: 1px solid var(--color-border); }
  .accept { background: var(--color-accent); color: #fff; border-color: var(--color-accent); }
  .decline { background: transparent; color: var(--color-text); }
</style>
