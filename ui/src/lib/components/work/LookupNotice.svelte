<script lang="ts">
  import type { LookupStatus } from "$lib/types/work";

  interface Props {
    lookup: LookupStatus;
    reason?: string;
    /** What was looked up, e.g. "Items" or "Linked runs". */
    subject: string;
    testid?: string;
  }

  let { lookup, reason, subject, testid }: Props = $props();

  const OUTCOME: Record<LookupStatus, string> = {
    complete: "complete",
    partial: "partial result",
    failed: "lookup failed",
    unsupported: "not supported",
  };
</script>

<!-- A complete lookup says nothing. Anything else says what is missing and why. -->
{#if lookup !== "complete"}
  <p class="lookup-notice" data-lookup={lookup} data-testid={testid}>
    <span class="lookup-glyph" aria-hidden="true">!</span>
    <strong>{subject}: {OUTCOME[lookup]}</strong>
    {#if reason}
      <span class="lookup-reason">&mdash; {reason}</span>
    {/if}
  </p>
{/if}

<style>
  .lookup-notice {
    display: flex;
    flex-wrap: wrap;
    align-items: baseline;
    gap: 0.375rem;
    margin: 0;
    padding: 0.375rem 0.625rem;
    border: 1px dashed var(--ui-border-strong, #9ca3af);
    border-radius: var(--radius-md, 6px);
    background: var(--ui-surface-secondary, #f3f4f6);
    color: var(--ui-text-primary, #111827);
    font-size: 0.8125rem;
  }

  .lookup-glyph {
    font-weight: 700;
  }

  .lookup-notice[data-lookup="failed"] {
    border-style: solid;
  }
</style>
