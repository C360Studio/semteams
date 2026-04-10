<script lang="ts">
  import type { ToolCallAttachment } from "$lib/types/chat";

  interface Props {
    attachment: ToolCallAttachment;
  }

  let { attachment }: Props = $props();
</script>

<div data-testid="tool-call-card" class="tool-call-card">
  <div class="tool-header">
    <span class="tool-name">{attachment.toolName}</span>
    <span
      class="tool-status {attachment.status}"
      data-status={attachment.status}
    >
      {attachment.status}
    </span>
  </div>

  {#if Object.keys(attachment.args).length > 0}
    <details class="tool-args">
      <summary>Arguments</summary>
      <pre>{JSON.stringify(attachment.args, null, 2)}</pre>
    </details>
  {/if}

  {#if attachment.result}
    <details class="tool-result">
      <summary>Result</summary>
      <pre>{attachment.result}</pre>
    </details>
  {/if}

  {#if attachment.error}
    <div class="tool-error" data-testid="tool-error">
      <span>{attachment.error}</span>
    </div>
  {/if}

  {#if attachment.durationMs != null}
    <span class="tool-duration">{attachment.durationMs}ms</span>
  {/if}
</div>

<style>
  .tool-call-card {
    padding: var(--pico-spacing, 0.75rem);
    border: 1px solid var(--pico-muted-border-color, #e0e0e0);
    border-radius: var(--pico-border-radius, 4px);
    background: var(--pico-card-background-color, #fff);
  }

  .tool-header {
    display: flex;
    align-items: center;
    gap: 0.5rem;
    margin-bottom: 0.25rem;
  }

  .tool-name {
    font-weight: 600;
    font-family: var(--pico-font-family-monospace, monospace);
    font-size: 0.875rem;
  }

  .tool-status {
    padding: 0.125rem 0.5rem;
    border-radius: 9999px;
    font-size: 0.75rem;
    font-weight: 500;
    text-transform: capitalize;
  }

  /* Pending — gray */
  .tool-status.pending {
    background: #f3f4f6;
    color: #6b7280;
  }

  /* Running — blue with pulse animation */
  .tool-status.running {
    background: #dbeafe;
    color: #1e40af;
    animation: pulse 1.5s ease-in-out infinite;
  }

  @keyframes pulse {
    0%,
    100% {
      opacity: 1;
    }
    50% {
      opacity: 0.6;
    }
  }

  /* Complete — green */
  .tool-status.complete {
    background: #d1fae5;
    color: #065f46;
  }

  /* Error — red */
  .tool-status.error {
    background: #fee2e2;
    color: #991b1b;
  }

  .tool-args,
  .tool-result {
    margin-top: 0.375rem;
    font-size: 0.8125rem;
  }

  .tool-args summary,
  .tool-result summary {
    cursor: pointer;
    color: var(--pico-muted-color, #6b7280);
    font-size: 0.8125rem;
  }

  .tool-args pre,
  .tool-result pre {
    margin: 0.25rem 0 0;
    padding: 0.5rem;
    background: var(--pico-code-background-color, #f3f4f6);
    border-radius: var(--pico-border-radius, 4px);
    font-size: 0.75rem;
    overflow-x: auto;
    white-space: pre-wrap;
    word-break: break-word;
  }

  .tool-error {
    margin-top: 0.375rem;
    padding: 0.375rem 0.5rem;
    background: #fef2f2;
    border: 1px solid #fecaca;
    border-radius: var(--pico-border-radius, 4px);
    color: #991b1b;
    font-size: 0.8125rem;
  }

  .tool-duration {
    display: inline-block;
    margin-top: 0.25rem;
    font-size: 0.75rem;
    font-variant-numeric: tabular-nums;
    color: var(--pico-muted-color, #9ca3af);
  }
</style>
