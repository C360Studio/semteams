<script lang="ts">
  import type { ApprovalAttachment } from "$lib/types/chat";

  interface Props {
    attachment: ApprovalAttachment;
    onApprove?: (loopId: string) => void;
    onReject?: (loopId: string, reason?: string) => void;
  }

  let { attachment, onApprove, onReject }: Props = $props();
  let rejectReason = $state("");
  let showRejectReason = $state(false);
</script>

<div data-testid="approval-prompt" class="approval-prompt">
  <div class="approval-header">
    <span class="approval-question">{attachment.question}</span>
  </div>

  {#if attachment.toolName}
    <div class="approval-tool">
      <span class="tool-label">Tool:</span>
      <span class="tool-name">{attachment.toolName}</span>
    </div>
  {/if}

  <div class="approval-actions">
    {#if attachment.resolved}
      <span
        class="resolution-badge {attachment.resolution}"
        data-resolution={attachment.resolution}
      >
        {attachment.resolution === "approved" ? "Approved" : "Rejected"}
      </span>
    {:else}
      <button
        data-testid="approve-button"
        type="button"
        class="approve-btn"
        onclick={() => onApprove?.(attachment.loopId)}
      >
        Approve
      </button>
      <button
        data-testid="reject-button"
        type="button"
        class="reject-btn"
        onclick={() => {
          if (showRejectReason && rejectReason) {
            onReject?.(attachment.loopId, rejectReason);
          } else if (showRejectReason) {
            onReject?.(attachment.loopId);
          } else {
            showRejectReason = true;
          }
        }}
      >
        Reject
      </button>
      {#if showRejectReason}
        <textarea
          data-testid="reject-reason"
          class="reject-reason-input"
          placeholder="Reason (optional)"
          bind:value={rejectReason}
        ></textarea>
      {/if}
    {/if}
  </div>
</div>

<style>
  .approval-prompt {
    padding: var(--pico-spacing, 0.75rem);
    border: 1px solid var(--pico-muted-border-color, #e0e0e0);
    border-radius: var(--pico-border-radius, 4px);
    background: var(--pico-card-background-color, #fff);
  }

  .approval-header {
    margin-bottom: 0.25rem;
  }

  .approval-question {
    font-weight: 600;
    font-size: 0.9375rem;
  }

  .approval-tool {
    display: flex;
    align-items: center;
    gap: 0.375rem;
    margin-bottom: 0.5rem;
    font-size: 0.8125rem;
  }

  .tool-label {
    color: var(--pico-muted-color, #6b7280);
  }

  .tool-name {
    font-family: monospace;
    background: var(--pico-code-background-color, #f3f4f6);
    padding: 0.0625rem 0.375rem;
    border-radius: 3px;
    font-size: 0.8125rem;
  }

  .approval-actions {
    display: flex;
    flex-wrap: wrap;
    align-items: center;
    gap: 0.5rem;
    margin-top: 0.5rem;
  }

  .approve-btn,
  .reject-btn {
    padding: 0.25rem 0.75rem;
    border: 1px solid;
    border-radius: var(--pico-border-radius, 4px);
    cursor: pointer;
    font-size: 0.8125rem;
    font-weight: 500;
    transition:
      background 0.15s,
      border-color 0.15s;
  }

  .approve-btn {
    background: #d1fae5;
    border-color: #6ee7b7;
    color: #065f46;
  }

  .approve-btn:hover {
    background: #a7f3d0;
  }

  .reject-btn {
    background: #fee2e2;
    border-color: #fca5a5;
    color: #991b1b;
  }

  .reject-btn:hover {
    background: #fecaca;
  }

  .resolution-badge {
    display: inline-block;
    padding: 0.125rem 0.5rem;
    border-radius: 9999px;
    font-size: 0.75rem;
    font-weight: 500;
  }

  .resolution-badge.approved {
    background: #d1fae5;
    color: #065f46;
  }

  .resolution-badge.rejected {
    background: #fee2e2;
    color: #991b1b;
  }

  .reject-reason-input {
    width: 100%;
    min-height: 2.5rem;
    padding: 0.375rem;
    border: 1px solid var(--pico-muted-border-color, #e0e0e0);
    border-radius: var(--pico-border-radius, 4px);
    font-size: 0.8125rem;
    resize: vertical;
  }
</style>
