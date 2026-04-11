<script lang="ts">
  import type { RuleDiffAttachment } from "$lib/types/chat";

  interface Props {
    attachment: RuleDiffAttachment;
  }

  let { attachment }: Props = $props();
</script>

<div data-testid="rule-diff-card" class="rule-diff-card">
  <div class="rule-header">
    <span class="rule-name">{attachment.ruleName}</span>
    <span
      class="operation-badge {attachment.operation}"
      data-operation={attachment.operation}
    >
      {attachment.operation}
    </span>
  </div>

  <span class="rule-id">{attachment.ruleId}</span>

  {#if attachment.operation === "update" && attachment.before && attachment.after}
    <div class="diff-view">
      <details class="diff-before">
        <summary>Before</summary>
        <pre>{JSON.stringify(attachment.before, null, 2)}</pre>
      </details>
      <details class="diff-after">
        <summary>After</summary>
        <pre>{JSON.stringify(attachment.after, null, 2)}</pre>
      </details>
    </div>
  {:else if attachment.operation === "create" && attachment.after}
    <details class="diff-after">
      <summary>Rule Definition</summary>
      <pre>{JSON.stringify(attachment.after, null, 2)}</pre>
    </details>
  {:else if attachment.operation === "delete" && attachment.before}
    <details class="diff-before">
      <summary>Deleted Rule</summary>
      <pre>{JSON.stringify(attachment.before, null, 2)}</pre>
    </details>
  {/if}
</div>

<style>
  .rule-diff-card {
    padding: var(--pico-spacing, 0.75rem);
    border: 1px solid var(--pico-muted-border-color, #e0e0e0);
    border-radius: var(--pico-border-radius, 4px);
    background: var(--pico-card-background-color, #fff);
  }

  .rule-header {
    display: flex;
    align-items: center;
    gap: 0.5rem;
    margin-bottom: 0.25rem;
  }

  .rule-name {
    font-weight: 600;
  }

  .operation-badge {
    padding: 0.125rem 0.5rem;
    border-radius: 9999px;
    font-size: 0.75rem;
    font-weight: 500;
    text-transform: capitalize;
  }

  /* create — green */
  .operation-badge.create {
    background: #d1fae5;
    color: #065f46;
  }

  /* update — blue */
  .operation-badge.update {
    background: #dbeafe;
    color: #1e40af;
  }

  /* delete — red */
  .operation-badge.delete {
    background: #fee2e2;
    color: #991b1b;
  }

  .rule-id {
    display: inline-block;
    margin-bottom: 0.375rem;
    font-size: 0.75rem;
    font-family: var(--pico-font-family-monospace, monospace);
    color: var(--pico-muted-color, #9ca3af);
  }

  .diff-view {
    display: flex;
    flex-direction: column;
    gap: 0.375rem;
  }

  .diff-before,
  .diff-after {
    margin-top: 0.375rem;
    font-size: 0.8125rem;
  }

  .diff-before summary,
  .diff-after summary {
    cursor: pointer;
    color: var(--pico-muted-color, #6b7280);
    font-size: 0.8125rem;
  }

  .diff-before pre,
  .diff-after pre {
    margin: 0.25rem 0 0;
    padding: 0.5rem;
    background: var(--pico-code-background-color, #f3f4f6);
    border-radius: var(--pico-border-radius, 4px);
    font-size: 0.75rem;
    overflow-x: auto;
    white-space: pre-wrap;
    word-break: break-word;
  }
</style>
