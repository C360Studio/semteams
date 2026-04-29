<script lang="ts">
  import type { ApprovalDecision, PendingApproval } from "$lib/types/agent";
  import { agentApi, AgentApiError } from "$lib/services/agentApi";

  interface Props {
    loopId: string;
    pendingApproval: PendingApproval;
  }

  let { loopId, pendingApproval }: Props = $props();

  // editingArgs flips the UI into the modify-path: the args block
  // becomes a JSON editor, Approve / Reject collapse, "Submit modified"
  // and "Cancel" appear.
  let editingArgs = $state(false);
  let argsDraft = $state("");
  let argsError = $state<string | null>(null);
  let reasonDraft = $state("");

  // submitting holds the decision that is in flight, or null. Used
  // to disable the row of buttons during the round-trip and show
  // which one is firing.
  let submitting = $state<ApprovalDecision | null>(null);
  let submitError = $state<string | null>(null);

  // Pretty-print the current arguments for the read-only view and
  // the editor seed. Falls back to "{}" so the editor is never
  // empty (helps the user understand the field is JSON).
  const formattedArgs = $derived(
    JSON.stringify(pendingApproval.arguments ?? {}, null, 2),
  );

  function startModify() {
    argsDraft = formattedArgs;
    argsError = null;
    editingArgs = true;
  }

  function cancelModify() {
    editingArgs = false;
    argsDraft = "";
    argsError = null;
  }

  async function submit(decision: ApprovalDecision) {
    submitError = null;
    let modifiedArguments: Record<string, unknown> | undefined;

    if (decision === "modify") {
      // Validate JSON before firing. Leaving the editor open on
      // error gives the user a chance to fix without losing input.
      try {
        const parsed: unknown = JSON.parse(argsDraft);
        if (typeof parsed !== "object" || parsed === null || Array.isArray(parsed)) {
          argsError = "Arguments must be a JSON object.";
          return;
        }
        modifiedArguments = parsed as Record<string, unknown>;
      } catch (err) {
        argsError = `Invalid JSON: ${err instanceof Error ? err.message : "parse failed"}`;
        return;
      }
    }

    submitting = decision;
    try {
      await agentApi.submitApproval(loopId, {
        decision,
        ...(modifiedArguments && { modified_arguments: modifiedArguments }),
        ...(reasonDraft.trim() !== "" && { reason: reasonDraft.trim() }),
      });
      // Clear local form state on success. The loop's state
      // transition arrives via the SSE stream and the parent
      // re-renders without this section once pending_approval
      // clears upstream.
      reasonDraft = "";
      editingArgs = false;
      argsDraft = "";
    } catch (err) {
      if (err instanceof AgentApiError) {
        // 409 means the loop already resolved (race with another
        // approver, dispatch restart, etc.). Surface the specific
        // case so the user knows to refresh rather than retry.
        submitError =
          err.statusCode === 409
            ? "This task is no longer awaiting approval — it may have been resolved already."
            : `Approval failed (${err.statusCode}): ${err.message}`;
      } else {
        submitError = err instanceof Error ? err.message : "Approval failed.";
      }
    } finally {
      submitting = null;
    }
  }
</script>

<section
  class="pending-approval"
  data-testid="pending-approval-section"
  aria-label="Tool call awaiting approval"
>
  <header class="approval-header">
    <h3 class="approval-title">
      Approval requested:
      <span class="tool-name" data-testid="approval-tool-name"
        >{pendingApproval.tool_name}</span
      >
    </h3>
    {#if pendingApproval.reason}
      <p class="approval-reason" data-testid="approval-reason">
        {pendingApproval.reason}
      </p>
    {/if}
  </header>

  <div class="approval-args">
    <div class="args-label-row">
      <span class="args-label">Arguments</span>
      {#if !editingArgs}
        <button
          type="button"
          class="args-edit-btn"
          data-testid="approval-edit-args"
          onclick={startModify}
          disabled={submitting !== null}
        >
          Edit
        </button>
      {/if}
    </div>
    {#if editingArgs}
      <textarea
        class="args-editor"
        data-testid="approval-args-editor"
        bind:value={argsDraft}
        rows="8"
        spellcheck="false"
        aria-label="Modified tool arguments (JSON)"
      ></textarea>
      {#if argsError}
        <p class="args-error" role="alert" data-testid="approval-args-error">
          {argsError}
        </p>
      {/if}
    {:else}
      <pre class="args-display" data-testid="approval-args-display">{formattedArgs}</pre>
    {/if}
  </div>

  <label class="reason-field">
    <span class="reason-label">Reason (optional)</span>
    <input
      class="reason-input"
      data-testid="approval-reason-input"
      type="text"
      bind:value={reasonDraft}
      placeholder="Why this decision?"
      disabled={submitting !== null}
    />
  </label>

  {#if submitError}
    <p class="submit-error" role="alert" data-testid="approval-submit-error">
      {submitError}
    </p>
  {/if}

  <div class="approval-buttons" role="group" aria-label="Approval decision">
    {#if editingArgs}
      <button
        type="button"
        class="approval-btn modify"
        data-testid="approval-submit-modify"
        aria-label="{submitting === 'modify'
          ? 'Submitting modified arguments for'
          : 'Submit modified arguments for'} {pendingApproval.tool_name}"
        onclick={() => submit("modify")}
        disabled={submitting !== null}
      >
        {submitting === "modify" ? "Submitting…" : "Submit modified"}
      </button>
      <button
        type="button"
        class="approval-btn cancel"
        data-testid="approval-cancel-edit"
        onclick={cancelModify}
        disabled={submitting !== null}
      >
        Cancel
      </button>
    {:else}
      <button
        type="button"
        class="approval-btn approve"
        data-testid="approval-approve"
        aria-label="{submitting === 'approve'
          ? 'Approving'
          : 'Approve'} {pendingApproval.tool_name}"
        onclick={() => submit("approve")}
        disabled={submitting !== null}
      >
        {submitting === "approve" ? "Approving…" : "Approve"}
      </button>
      <button
        type="button"
        class="approval-btn danger"
        data-testid="approval-reject"
        aria-label="{submitting === 'reject'
          ? 'Rejecting'
          : 'Reject'} {pendingApproval.tool_name}"
        onclick={() => submit("reject")}
        disabled={submitting !== null}
      >
        {submitting === "reject" ? "Rejecting…" : "Reject"}
      </button>
    {/if}
  </div>
</section>

<style>
  .pending-approval {
    margin: 0.5rem 0;
    padding: 0.625rem 0.75rem;
    border: 1px solid var(--ui-warning-border, #fbbf24);
    border-radius: 6px;
    background: var(--ui-warning-surface, #fef3c7);
    display: flex;
    flex-direction: column;
    gap: 0.5rem;
  }

  .approval-header {
    display: flex;
    flex-direction: column;
    gap: 0.25rem;
  }

  .approval-title {
    margin: 0;
    font-size: 0.8125rem;
    font-weight: 600;
    color: var(--ui-text-primary, #374151);
  }

  .tool-name {
    font-family: ui-monospace, SFMono-Regular, Menlo, monospace;
    color: var(--ui-warning-text, #92400e);
  }

  .approval-reason {
    margin: 0;
    font-size: 0.75rem;
    color: var(--ui-text-secondary, #6b7280);
    white-space: pre-wrap;
    word-break: break-word;
  }

  .approval-args {
    display: flex;
    flex-direction: column;
    gap: 0.25rem;
  }

  .args-label-row {
    display: flex;
    align-items: center;
    justify-content: space-between;
  }

  .args-label {
    font-size: 0.6875rem;
    font-weight: 600;
    color: var(--ui-text-secondary, #6b7280);
    text-transform: uppercase;
    letter-spacing: 0.05em;
  }

  .args-edit-btn {
    all: unset;
    cursor: pointer;
    font-size: 0.75rem;
    color: var(--ui-interactive-primary, #2563eb);
    padding: 0.125rem 0.375rem;
    border-radius: 3px;
  }

  .args-edit-btn:hover:not(:disabled) {
    background: var(--ui-surface-secondary, #f3f4f6);
  }

  .args-edit-btn:focus-visible {
    outline: 2px solid var(--ui-interactive-primary, #3b82f6);
    outline-offset: 1px;
  }

  .args-edit-btn:disabled {
    opacity: 0.5;
    cursor: not-allowed;
  }

  .args-display {
    margin: 0;
    padding: 0.5rem;
    border-radius: 4px;
    background: var(--ui-surface-primary, #ffffff);
    font-family: ui-monospace, SFMono-Regular, Menlo, monospace;
    font-size: 0.6875rem;
    color: var(--ui-text-primary, #374151);
    overflow-x: auto;
    max-height: 12rem;
    white-space: pre;
  }

  .args-editor {
    padding: 0.5rem;
    border: 1px solid var(--ui-border-subtle, #e5e7eb);
    border-radius: 4px;
    background: var(--ui-surface-primary, #ffffff);
    font-family: ui-monospace, SFMono-Regular, Menlo, monospace;
    font-size: 0.75rem;
    color: var(--ui-text-primary, #374151);
    resize: vertical;
    min-height: 6rem;
  }

  .args-editor:focus-visible {
    outline: 2px solid var(--ui-interactive-primary, #3b82f6);
    outline-offset: 1px;
  }

  .args-error,
  .submit-error {
    margin: 0;
    font-size: 0.75rem;
    color: var(--ui-danger-text, #b91c1c);
  }

  .reason-field {
    display: flex;
    flex-direction: column;
    gap: 0.1875rem;
  }

  .reason-label {
    font-size: 0.6875rem;
    font-weight: 600;
    color: var(--ui-text-secondary, #6b7280);
    text-transform: uppercase;
    letter-spacing: 0.05em;
  }

  .reason-input {
    padding: 0.375rem 0.5rem;
    border: 1px solid var(--ui-border-subtle, #e5e7eb);
    border-radius: 4px;
    background: var(--ui-surface-primary, #ffffff);
    font-size: 0.8125rem;
    color: var(--ui-text-primary, #374151);
  }

  .reason-input:focus-visible {
    outline: 2px solid var(--ui-interactive-primary, #3b82f6);
    outline-offset: 1px;
  }

  .reason-input:disabled {
    opacity: 0.6;
    cursor: not-allowed;
  }

  .approval-buttons {
    display: flex;
    gap: 0.375rem;
    flex-wrap: wrap;
  }

  .approval-btn {
    cursor: pointer;
    padding: 0.375rem 0.75rem;
    border: 1px solid transparent;
    border-radius: 4px;
    font-size: 0.8125rem;
    font-weight: 500;
    transition: opacity 100ms;
  }

  .approval-btn:disabled {
    opacity: 0.6;
    cursor: not-allowed;
  }

  .approval-btn:focus-visible {
    outline: 2px solid var(--ui-interactive-primary, #3b82f6);
    outline-offset: 2px;
  }

  .approval-btn.approve {
    background: var(--ui-success-surface, #16a34a);
    color: white;
  }

  .approval-btn.approve:hover:not(:disabled) {
    background: var(--ui-success-surface-hover, #15803d);
  }

  .approval-btn.danger {
    background: var(--ui-danger-surface, #dc2626);
    color: white;
  }

  .approval-btn.danger:hover:not(:disabled) {
    background: var(--ui-danger-surface-hover, #b91c1c);
  }

  .approval-btn.modify {
    background: var(--ui-interactive-primary, #2563eb);
    color: white;
  }

  .approval-btn.modify:hover:not(:disabled) {
    background: var(--ui-interactive-primary-hover, #1d4ed8);
  }

  .approval-btn.cancel {
    background: var(--ui-surface-primary, #ffffff);
    border-color: var(--ui-border-subtle, #d1d5db);
    color: var(--ui-text-secondary, #4b5563);
  }

  .approval-btn.cancel:hover:not(:disabled) {
    background: var(--ui-surface-secondary, #f3f4f6);
  }
</style>
