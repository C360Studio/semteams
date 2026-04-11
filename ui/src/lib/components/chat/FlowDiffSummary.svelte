<script lang="ts">
  import type { FlowDiff } from "$lib/types/chat";

  interface Props {
    diff: FlowDiff;
  }

  let { diff }: Props = $props();

  let isEmpty = $derived(
    diff.nodesAdded.length === 0 &&
      diff.nodesRemoved.length === 0 &&
      diff.nodesModified.length === 0 &&
      diff.connectionsAdded === 0 &&
      diff.connectionsRemoved === 0,
  );
</script>

<div data-testid="flow-diff-summary">
  {#if isEmpty}
    <span>No changes</span>
  {:else}
    {#if diff.nodesAdded.length > 0}
      <div>
        <strong>Added:</strong>
        {diff.nodesAdded.join(", ")}
      </div>
    {/if}
    {#if diff.nodesRemoved.length > 0}
      <div>
        <strong>Removed:</strong>
        {diff.nodesRemoved.join(", ")}
      </div>
    {/if}
    {#if diff.nodesModified.length > 0}
      <div>
        <strong>Modified:</strong>
        {diff.nodesModified.join(", ")}
      </div>
    {/if}
    {#if diff.connectionsAdded > 0}
      <div>Connections added: {diff.connectionsAdded}</div>
    {/if}
    {#if diff.connectionsRemoved > 0}
      <div>Connections removed: {diff.connectionsRemoved}</div>
    {/if}
  {/if}
</div>
