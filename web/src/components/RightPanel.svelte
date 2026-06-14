<script>
  /**
   * Right panel container — shows EmptyState or Conversation.
   *
   * Props:
   *   activeConversation  {object|null}  The currently selected conversation, or null
   *   activeId            {string}       Conversation id (used for mobile back button)
   *   messages            {Array}        Messages for the active conversation
   *   typingUser          {string|null}  Handle of user currently typing
   *
   * Events:
   *   on:back         — fired on mobile when the back arrow is clicked
   *   on:newChat      — fired from EmptyState "New Chat" button
   *   on:sendMessage  — fired from Conversation with { detail: text }
   *   on:attachFile   — fired from Conversation with { detail: File }
   *   on:typing       — fired when user types (for WS typing indicator)
   *   on:retention    — fired from Conversation with { detail: expiresIn }
   *   on:leaveGroup   — fired from Conversation
   *   on:openFiles    — fired from Conversation 3-dot menu
   *   on:delete       — fired from Conversation with { messageId } when a message is hidden
   *   on:batchDelete  — fired from Conversation with { messageIds } for batch hide
   */
  import EmptyState from './EmptyState.svelte';
  import Conversation from './Conversation.svelte';
  import { createEventDispatcher } from 'svelte';

  export let activeConversation = null;
  export let activeId = '';
  export let messages = [];
  export let typingUser = null;

  const dispatch = createEventDispatcher();

  function handleBack() {
    dispatch('back');
  }

  function handleNewChat() {
    dispatch('newChat');
  }

  function handleSendMessage(e) {
    dispatch('sendMessage', e.detail);
  }

  function handleAttachFile(e) {
    dispatch('attachFile', e.detail);
  }

  function handleTyping() {
    dispatch('typing');
  }

  function handleRetention(e) {
    dispatch('retention', e.detail);
  }

  function handleLeaveGroup(e) {
    dispatch('leaveGroup', e.detail);
  }

  function handleOpenFiles(e) {
    dispatch('openFiles', e.detail);
  }

  function handleDelete(e) {
    dispatch('delete', e.detail);
  }

  function handleBatchDelete(e) {
    dispatch('batchDelete', e.detail);
  }
</script>

<div class="right-panel">
  {#if activeConversation}
    <Conversation
      conversation={activeConversation}
      messages={messages}
      typingUser={typingUser}
      on:back={handleBack}
      on:sendMessage={handleSendMessage}
      on:attachFile={handleAttachFile}
      on:typing={handleTyping}
      on:retention={handleRetention}
      on:leaveGroup={handleLeaveGroup}
      on:openFiles={handleOpenFiles}
      on:delete={handleDelete}
      on:batchDelete={handleBatchDelete}
    />
  {:else}
    <EmptyState on:newChat={handleNewChat} />
  {/if}
</div>

<style>
  .right-panel {
    flex: 1;
    display: flex;
    flex-direction: column;
    min-width: 0;
    background-color: var(--color-bg);
    height: 100%;
  }

  @media (max-width: 767px) {
    .right-panel {
      position: absolute;
      inset: 0;
      z-index: 10;
      background-color: var(--color-bg);
    }
  }
</style>
