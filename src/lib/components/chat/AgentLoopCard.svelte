<script lang="ts">
  import type { AgentLoopAttachment } from "$lib/types/chat";

  interface Props {
    attachment: AgentLoopAttachment;
  }

  let { attachment }: Props = $props();
</script>

<div data-testid="agent-loop-card" class="agent-loop-card">
  <div class="loop-header">
    <span class="loop-role">{attachment.role}</span>
    <span class="state-badge {attachment.state}" data-state={attachment.state}>
      {attachment.state.replace("_", " ")}
    </span>
  </div>

  {#if attachment.maxIterations > 0}
    <div class="loop-progress">
      <div class="progress-bar">
        <div
          class="progress-fill"
          style="width: {Math.min(100, (attachment.iterations / attachment.maxIterations) * 100)}%"
        ></div>
      </div>
      <span class="progress-text">{attachment.iterations}/{attachment.maxIterations}</span>
    </div>
  {/if}

  {#if attachment.loopId}
    <span class="loop-id">{attachment.loopId.slice(0, 12)}</span>
  {/if}
</div>

<style>
  .agent-loop-card {
    padding: var(--pico-spacing, 0.75rem);
    border: 1px solid var(--pico-muted-border-color, #e0e0e0);
    border-radius: var(--pico-border-radius, 4px);
    background: var(--pico-card-background-color, #fff);
  }

  .loop-header {
    display: flex;
    align-items: center;
    gap: 0.5rem;
    margin-bottom: 0.25rem;
  }

  .loop-role {
    font-weight: 600;
  }

  .state-badge {
    padding: 0.125rem 0.5rem;
    border-radius: 9999px;
    font-size: 0.75rem;
    font-weight: 500;
    text-transform: capitalize;
  }

  /* Active states — blue/teal */
  .state-badge.exploring,
  .state-badge.planning,
  .state-badge.architecting,
  .state-badge.executing,
  .state-badge.reviewing {
    background: #dbeafe;
    color: #1e40af;
  }

  /* Paused — amber */
  .state-badge.paused {
    background: #fef3c7;
    color: #92400e;
  }

  /* Awaiting approval — orange */
  .state-badge.awaiting_approval {
    background: #ffedd5;
    color: #9a3412;
  }

  /* Complete — green */
  .state-badge.complete {
    background: #d1fae5;
    color: #065f46;
  }

  /* Failed — red */
  .state-badge.failed {
    background: #fee2e2;
    color: #991b1b;
  }

  /* Cancelled — gray */
  .state-badge.cancelled {
    background: #f3f4f6;
    color: #6b7280;
  }

  .loop-progress {
    display: flex;
    align-items: center;
    gap: 0.5rem;
    margin-top: 0.375rem;
  }

  .progress-bar {
    flex: 1;
    height: 6px;
    background: var(--pico-muted-border-color, #e5e7eb);
    border-radius: 3px;
    overflow: hidden;
  }

  .progress-fill {
    height: 100%;
    background: var(--pico-primary-color, #3b82f6);
    border-radius: 3px;
    transition: width 0.2s ease;
  }

  .progress-text {
    font-size: 0.75rem;
    font-variant-numeric: tabular-nums;
    color: var(--pico-muted-color, #6b7280);
    white-space: nowrap;
  }

  .loop-id {
    display: inline-block;
    margin-top: 0.25rem;
    font-size: 0.6875rem;
    font-family: var(--pico-font-family-monospace, monospace);
    color: var(--pico-muted-color, #9ca3af);
  }
</style>
