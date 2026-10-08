<script lang="ts">
  // Board-level notice that some rule-fired loops could not be classified as
  // run controls or run members (OpenSpec work-board-read-only, S1). Such a loop
  // stays on the board as an ordinary card and the control list of the run it
  // fired on may be missing it, so the operator is told even when no task with
  // controls is open. The status region stays mounted so a change is announced.
  import { controlsStore } from "$lib/stores/controlsStore.svelte";

  const count = $derived(controlsStore.unclassifiedCount);
  const reason = $derived(controlsStore.lastError);
</script>

<div role="status" data-testid="controls-notice-region">
  {#if reason}
    <p class="controls-notice" data-testid="controls-notice">
      Couldn't tell whether {count === 1 ? "1 rule-fired loop belongs" : `${count} rule-fired loops belong`}
      to the run that fired {count === 1 ? "it" : "them"}, so run control lists may be incomplete. Shown
      as ordinary cards for now ({reason}).
    </p>
  {/if}
</div>

<style>
  .controls-notice {
    margin: 0;
    padding: 0.375rem 1rem;
    font-size: 0.75rem;
    line-height: 1.4;
    color: var(--ui-text-on-warning, #161616);
    background: var(--status-warning, #f1c21b);
  }
</style>
