<script lang="ts">
  import type { RunPause } from "$lib/types/task";
  import { agentApi, AgentApiError } from "$lib/services/agentApi";

  interface Props {
    runId: string;
    pause: RunPause;
  }

  let { runId, pause }: Props = $props();

  // ---------------------------------------------------------------------------
  // Clarification path (4b-2)
  // ---------------------------------------------------------------------------

  let replyDraft = $state("");
  let replySending = $state(false);
  let replyError = $state<string | null>(null);
  // The poll that drops the pause marker (and unmounts this section) can lag the
  // reply by up to one interval (~2.5s). Show an inline confirmation in that gap
  // so the operator knows the answer registered, rather than staring at a
  // cleared form wondering if it sent.
  let replySubmitted = $state(false);

  async function sendReply() {
    if (pause.cause !== "clarification") return;
    const trimmed = replyDraft.trim();
    if (!trimmed) return;

    replyError = null;
    replySubmitted = false;
    replySending = true;
    try {
      await agentApi.sendMessage(trimmed, {
        runId,
        inReplyTo: pause.askingLoopId,
      });
      // Clear on success. The run resumes and the next poll drops the pause
      // marker, causing this section to unmount.
      replyDraft = "";
      replySubmitted = true;
    } catch (err) {
      replyError = err instanceof Error ? err.message : "Failed to send reply.";
    } finally {
      replySending = false;
    }
  }

  function handleReplyKey(e: KeyboardEvent) {
    if (e.key === "Enter" && !e.shiftKey) {
      e.preventDefault();
      void sendReply();
    }
  }

  // ---------------------------------------------------------------------------
  // Tool-gate path (4c)
  // ---------------------------------------------------------------------------

  let reasonDraft = $state("");
  let approveSending = $state(false);
  let approveError = $state<string | null>(null);
  let gateSubmitted = $state<"approve" | "reject" | null>(null);

  async function submitGateDecision(decision: "approve" | "reject") {
    if (pause.cause !== "tool_gate") return;

    approveError = null;
    gateSubmitted = null;
    approveSending = true;
    try {
      await agentApi.submitApproval(pause.gatedLoopId, {
        decision,
        ...(reasonDraft.trim() ? { reason: reasonDraft.trim() } : {}),
      });
      reasonDraft = "";
      gateSubmitted = decision;
    } catch (err) {
      if (err instanceof AgentApiError && err.statusCode === 409) {
        // Race: the loop resolved already — inform the operator to refresh.
        approveError =
          "This approval is no longer pending — it may have already been resolved.";
      } else {
        approveError =
          err instanceof Error ? err.message : "Failed to submit decision.";
      }
    } finally {
      approveSending = false;
    }
  }
</script>

<section
  class="run-waiting"
  data-testid="run-waiting-section"
  aria-label="Run waiting for operator input"
>
  {#if pause.cause === "clarification"}
    <!-- 4b-2: The recovery coordinator asked a question. -->
    <header class="waiting-header">
      <h3 class="waiting-title">Agent needs your input to continue</h3>
    </header>

    <p class="waiting-question" data-testid="run-question">
      {pause.question || "The agent needs your input to continue."}
    </p>

    <label class="reply-label" for="run-reply-input">Your answer</label>
    <textarea
      id="run-reply-input"
      class="reply-input"
      data-testid="run-reply-input"
      bind:value={replyDraft}
      onkeydown={handleReplyKey}
      oninput={() => (replySubmitted = false)}
      placeholder="Type your answer… (Enter to send)"
      rows="3"
      disabled={replySending}
    ></textarea>

    {#if replyError}
      <p class="waiting-error" role="alert" data-testid="run-reply-error">
        {replyError}
      </p>
    {/if}

    {#if replySubmitted}
      <p class="waiting-confirm" role="status" data-testid="run-reply-confirm">
        Reply sent — waiting for the agent to resume…
      </p>
    {/if}

    <button
      type="button"
      class="waiting-btn primary"
      data-testid="run-reply-send"
      onclick={() => void sendReply()}
      disabled={replySending || !replyDraft.trim()}
      aria-label={replySending ? "Sending reply…" : "Send reply to agent"}
    >
      {replySending ? "Sending…" : "Send"}
    </button>

  {:else}
    <!-- 4c: A gated tool call needs approval. -->
    <header class="waiting-header">
      <h3 class="waiting-title">A tool call needs your approval</h3>
    </header>
    <!-- TODO: Display tool name and args when the gated loop's pending_approval
         is accessible. The gated loop is rule-spawned and typically 404s on
         /teams-dispatch/loops/{id}, so we present a minimal approve/reject
         affordance for v1. Modify path deferred to a follow-up slice. -->

    <div class="approval-section" data-testid="run-approval-section">
      <label class="reason-label" for="run-approval-reason">Reason (optional)</label>
      <input
        id="run-approval-reason"
        class="reason-input"
        type="text"
        bind:value={reasonDraft}
        oninput={() => (gateSubmitted = null)}
        placeholder="Why this decision?"
        disabled={approveSending}
      />

      {#if approveError}
        <p class="waiting-error" role="alert" data-testid="run-approval-error">
          {approveError}
        </p>
      {/if}

      {#if gateSubmitted}
        <p class="waiting-confirm" role="status" data-testid="run-approval-confirm">
          {gateSubmitted === "approve" ? "Approval" : "Rejection"} submitted — waiting
          for the agent to resume…
        </p>
      {/if}

      <div class="approval-buttons" role="group" aria-label="Tool-gate approval decision">
        <button
          type="button"
          class="waiting-btn approve"
          data-testid="run-approve"
          onclick={() => void submitGateDecision("approve")}
          disabled={approveSending}
          aria-label={approveSending ? "Approving…" : "Approve tool call"}
        >
          {approveSending ? "Approving…" : "Approve"}
        </button>
        <button
          type="button"
          class="waiting-btn danger"
          data-testid="run-reject"
          onclick={() => void submitGateDecision("reject")}
          disabled={approveSending}
          aria-label={approveSending ? "Rejecting…" : "Reject tool call"}
        >
          {approveSending ? "Rejecting…" : "Reject"}
        </button>
      </div>
    </div>
  {/if}
</section>

<style>
  .run-waiting {
    margin: 0.5rem 0;
    padding: 0.625rem 0.75rem;
    border: 1px solid var(--ui-warning-border, #fbbf24);
    border-radius: 6px;
    background: var(--ui-warning-surface, #fef3c7);
    display: flex;
    flex-direction: column;
    gap: 0.5rem;
  }

  .waiting-header {
    display: flex;
    flex-direction: column;
    gap: 0.25rem;
  }

  .waiting-title {
    margin: 0;
    font-size: 0.8125rem;
    font-weight: 600;
    color: var(--ui-text-primary, #374151);
  }

  .waiting-question {
    margin: 0;
    font-size: 0.8125rem;
    color: var(--ui-text-primary, #374151);
    line-height: 1.5;
    white-space: pre-wrap;
    word-break: break-word;
  }

  .reply-label,
  .reason-label {
    font-size: 0.6875rem;
    font-weight: 600;
    color: var(--ui-text-secondary, #6b7280);
    text-transform: uppercase;
    letter-spacing: 0.05em;
  }

  .reply-input {
    padding: 0.375rem 0.5rem;
    border: 1px solid var(--ui-border-subtle, #e5e7eb);
    border-radius: 4px;
    background: var(--ui-surface-primary, #ffffff);
    font-size: 0.8125rem;
    color: var(--ui-text-primary, #374151);
    font-family: inherit;
    resize: vertical;
    min-height: 4rem;
  }

  .reply-input:focus-visible {
    outline: 2px solid var(--ui-interactive-primary, #3b82f6);
    outline-offset: 1px;
  }

  .reply-input:disabled {
    opacity: 0.6;
    cursor: not-allowed;
  }

  .waiting-error {
    margin: 0;
    font-size: 0.75rem;
    color: var(--ui-danger-text, #b91c1c);
  }

  .waiting-confirm {
    margin: 0;
    font-size: 0.75rem;
    color: var(--ui-text-secondary, #6b7280);
    font-style: italic;
  }

  .approval-section {
    display: flex;
    flex-direction: column;
    gap: 0.375rem;
  }

  .reason-input {
    padding: 0.375rem 0.5rem;
    border: 1px solid var(--ui-border-subtle, #e5e7eb);
    border-radius: 4px;
    background: var(--ui-surface-primary, #ffffff);
    font-size: 0.8125rem;
    color: var(--ui-text-primary, #374151);
    font-family: inherit;
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

  .waiting-btn {
    cursor: pointer;
    padding: 0.375rem 0.75rem;
    border: 1px solid transparent;
    border-radius: 4px;
    font-size: 0.8125rem;
    font-weight: 500;
    transition: opacity 100ms;
    font-family: inherit;
  }

  .waiting-btn:disabled {
    opacity: 0.6;
    cursor: not-allowed;
  }

  .waiting-btn:focus-visible {
    outline: 2px solid var(--ui-interactive-primary, #3b82f6);
    outline-offset: 2px;
  }

  .waiting-btn.primary {
    background: var(--ui-interactive-primary, #2563eb);
    color: white;
    align-self: flex-start;
  }

  .waiting-btn.primary:hover:not(:disabled) {
    background: var(--ui-interactive-primary-hover, #1d4ed8);
  }

  .waiting-btn.approve {
    background: var(--ui-success-surface, #16a34a);
    color: white;
  }

  .waiting-btn.approve:hover:not(:disabled) {
    background: var(--ui-success-surface-hover, #15803d);
  }

  .waiting-btn.danger {
    background: var(--ui-danger-surface, #dc2626);
    color: white;
  }

  .waiting-btn.danger:hover:not(:disabled) {
    background: var(--ui-danger-surface-hover, #b91c1c);
  }
</style>
