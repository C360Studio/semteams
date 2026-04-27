<script lang="ts">
  // Story view for a task — plain-language narrative of what the AI
  // did. Sources from /teams-loop/trajectories/<loop_id> which gives
  // structured steps (model_call / tool_call) with role, capability,
  // duration, and tokens. The story renders one line per step, in
  // time order, in human-readable terms.
  //
  // The wire-level message log lives behind a "Show raw activity"
  // toggle below — answers the few users who actually want it,
  // doesn't intrude on the rest.

  import { agentApi } from "$lib/services/agentApi";
  import type {
    LoopTrajectory,
    TrajectoryStep,
    ModelCallStep,
    ToolCallStep,
  } from "$lib/types/agent";
  import { SvelteSet } from "svelte/reactivity";
  import TaskTrace from "./TaskTrace.svelte";

  interface Props {
    loopId: string;
    /** When set, used to provide the very first "You asked: …" line. */
    prompt?: string;
  }

  let { loopId, prompt }: Props = $props();

  const POLL_INTERVAL_MS = 3000;

  let trajectory = $state<LoopTrajectory | null>(null);
  let lastError = $state<string | null>(null);
  let inFlight = false;
  let expanded = new SvelteSet<number>();
  let showRaw = $state(false);

  async function refresh() {
    if (inFlight) return;
    inFlight = true;
    try {
      trajectory = await agentApi.getLoopTrajectory(loopId);
      lastError = null;
    } catch (err) {
      lastError = err instanceof Error ? err.message : String(err);
    } finally {
      inFlight = false;
    }
  }

  $effect(() => {
    const id = loopId;
    if (!id) return;
    void refresh();
    const handle = setInterval(refresh, POLL_INTERVAL_MS);
    return () => clearInterval(handle);
  });

  function roleLabel(step: TrajectoryStep): string {
    const cap = step.capability;
    if (!cap) return "Agent";
    return cap.charAt(0).toUpperCase() + cap.slice(1);
  }

  /**
   * One-line narrative summary for a step. The detail (full response,
   * tool args, raw payload) is behind the row's expand affordance.
   */
  function lineFor(step: TrajectoryStep): string {
    if (step.step_type === "model_call") {
      const role = roleLabel(step);
      // No text response = model returned tool_calls instead of
      // free-form text. "Reasoned" reads better than "thinking…"
      // when the call is already complete and tokens were consumed.
      return step.response && step.response.trim() !== ""
        ? `${role} replied`
        : `${role} reasoned`;
    }
    if (step.step_type === "tool_call") {
      const role = roleLabel(step);
      const fail = step.tool_status && step.tool_status !== "success";
      return fail
        ? `${role} tried ${step.tool_name} — failed`
        : `${role} used ${step.tool_name}`;
    }
    return "Step";
  }

  function metaFor(step: TrajectoryStep): string {
    const parts: string[] = [];
    if (typeof step.duration === "number") parts.push(`${step.duration}ms`);
    if (step.step_type === "model_call") {
      const m = step as ModelCallStep;
      if (typeof m.tokens_in === "number" || typeof m.tokens_out === "number") {
        const tin = m.tokens_in ?? 0;
        const tout = m.tokens_out ?? 0;
        parts.push(`${tin + tout} tokens`);
      }
    }
    return parts.join(" · ");
  }

  function previewFor(step: TrajectoryStep): string {
    if (step.step_type === "model_call") {
      const m = step as ModelCallStep;
      if (!m.response) return "";
      // Trim leading/trailing whitespace; collapse blank lines for
      // the snippet preview only — full content shows on expand.
      const trimmed = m.response.trim().replace(/\n\s*\n/g, "\n");
      return trimmed.length > 200 ? trimmed.slice(0, 197) + "…" : trimmed;
    }
    if (step.step_type === "tool_call") {
      const t = step as ToolCallStep;
      if (t.tool_arguments && Object.keys(t.tool_arguments).length > 0) {
        try {
          const s = JSON.stringify(t.tool_arguments);
          return s.length > 140 ? s.slice(0, 137) + "…" : s;
        } catch {
          return "";
        }
      }
    }
    return "";
  }

  function fullPayload(step: TrajectoryStep): string {
    if (step.step_type === "model_call") {
      const m = step as ModelCallStep;
      // When there's a text response, show it. Otherwise the step
      // was a tool-decision call (model returned tool_calls, not
      // text) — show the full step record so the user sees the
      // model/capability/tokens at minimum.
      if (m.response && m.response.trim() !== "") return m.response;
      try {
        return JSON.stringify(step, null, 2);
      } catch {
        return "(unable to serialise)";
      }
    }
    if (step.step_type === "tool_call") {
      const t = step as ToolCallStep;
      const out = {
        arguments: t.tool_arguments,
        result: t.tool_result,
        status: t.tool_status,
      };
      try {
        return JSON.stringify(out, null, 2);
      } catch {
        return "(unable to serialise)";
      }
    }
    return "";
  }

  function toggleExpand(idx: number) {
    if (expanded.has(idx)) expanded.delete(idx);
    else expanded.add(idx);
  }

  function fmtDuration(ms: number | undefined): string {
    if (typeof ms !== "number") return "";
    if (ms < 1000) return `${ms}ms`;
    return `${(ms / 1000).toFixed(2)}s`;
  }

  // Outcome badge text for the closing line.
  function outcomeLabel(outcome: string | undefined): string {
    if (!outcome) return "In progress";
    if (outcome === "complete" || outcome === "success") return "Done";
    if (outcome === "failed" || outcome === "error") return "Failed";
    if (outcome === "cancelled") return "Cancelled";
    if (outcome === "truncated") return "Stopped (max length)";
    return outcome;
  }
</script>

<div class="task-story" data-testid="task-story">
  {#if prompt}
    <div class="story-line story-line-asked" data-testid="story-line-asked">
      <span class="line-icon" aria-hidden="true">▸</span>
      <span class="line-body">
        <span class="line-label">You asked</span>
        <span class="line-text">{prompt}</span>
      </span>
    </div>
  {/if}

  {#if lastError && !trajectory}
    <p class="story-error" role="alert" data-testid="story-error">
      Couldn't load the trajectory: {lastError}
    </p>
  {:else if !trajectory}
    <p class="story-empty">Loading…</p>
  {:else if trajectory.steps.length === 0}
    <p class="story-empty">No steps recorded yet.</p>
  {:else}
    <ol class="story-list" data-testid="story-list">
      {#each trajectory.steps as step, idx (idx + step.timestamp)}
        <li class="story-step" data-step-type={step.step_type} data-testid="story-step">
          <button
            type="button"
            class="step-button"
            onclick={() => toggleExpand(idx)}
            aria-expanded={expanded.has(idx)}
          >
            <span class="line-icon" aria-hidden="true">●</span>
            <span class="line-body">
              <span class="step-headline">{lineFor(step)}</span>
              {#if previewFor(step)}
                <span class="step-preview">{previewFor(step)}</span>
              {/if}
              {#if metaFor(step)}
                <span class="step-meta">{metaFor(step)}</span>
              {/if}
            </span>
            <span class="line-chevron" aria-hidden="true">
              {expanded.has(idx) ? "▾" : "▸"}
            </span>
          </button>
          {#if expanded.has(idx)}
            <pre
              class="step-payload"
              data-testid="story-step-payload"
            >{fullPayload(step)}</pre>
          {/if}
        </li>
      {/each}
    </ol>
  {/if}

  {#if trajectory && (trajectory.outcome || trajectory.duration !== undefined)}
    <div class="story-line story-line-done" data-testid="story-line-done">
      <span class="line-icon" aria-hidden="true">✓</span>
      <span class="line-body">
        <span class="line-label">{outcomeLabel(trajectory.outcome)}</span>
        <span class="step-meta">
          {fmtDuration(trajectory.duration)}
          {#if trajectory.total_tokens_in !== undefined || trajectory.total_tokens_out !== undefined}
            · {(trajectory.total_tokens_in ?? 0) + (trajectory.total_tokens_out ?? 0)} tokens
          {/if}
        </span>
      </span>
    </div>
  {/if}

  <details class="raw-toggle" bind:open={showRaw}>
    <summary class="raw-summary" data-testid="raw-toggle">
      Show raw activity
    </summary>
    {#if showRaw}
      <div class="raw-body">
        <TaskTrace {loopId} />
      </div>
    {/if}
  </details>
</div>

<style>
  .task-story {
    display: flex;
    flex-direction: column;
    gap: 0.5rem;
  }

  .story-line {
    display: flex;
    align-items: baseline;
    gap: 0.5rem;
    padding: 0.375rem 0.5rem;
    font-size: 0.8125rem;
  }

  .line-icon {
    color: var(--ui-text-tertiary, #9ca3af);
    width: 1rem;
    text-align: center;
    flex-shrink: 0;
  }

  .line-body {
    display: flex;
    flex-direction: column;
    gap: 0.125rem;
    flex: 1;
    min-width: 0;
  }

  .line-label {
    font-weight: 600;
    color: var(--ui-text-primary, #111827);
  }

  .line-text {
    color: var(--ui-text-secondary, #4b5563);
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }

  .story-line-asked {
    background: var(--ui-surface-secondary, #f3f4f6);
    border-radius: 6px;
    padding: 0.5rem 0.625rem;
  }

  .story-line-done {
    background: var(--ui-surface-secondary, #f3f4f6);
    border-radius: 6px;
    padding: 0.5rem 0.625rem;
    margin-top: 0.25rem;
  }

  .story-line-done .line-icon {
    color: var(--status-success, #22c55e);
  }

  .story-list {
    list-style: none;
    margin: 0;
    padding: 0;
    display: flex;
    flex-direction: column;
    gap: 0.125rem;
    /* Vertical connector — a faint line down the icon column. */
    position: relative;
  }

  .story-list::before {
    content: "";
    position: absolute;
    left: 1rem;
    top: 0.875rem;
    bottom: 0.875rem;
    width: 1px;
    background: var(--ui-border-subtle, #e5e7eb);
  }

  .story-step {
    position: relative;
  }

  .step-button {
    all: unset;
    cursor: pointer;
    display: grid;
    grid-template-columns: 1.5rem 1fr auto;
    gap: 0.5rem;
    align-items: baseline;
    padding: 0.4375rem 0.5rem;
    border-radius: 6px;
    width: calc(100% - 1rem);
  }

  .step-button:hover {
    background: var(--ui-surface-secondary, #f3f4f6);
  }

  .step-button:focus-visible {
    outline: 2px solid var(--ui-interactive-primary, #3b82f6);
    outline-offset: 1px;
  }

  .step-button .line-icon {
    color: var(--ui-interactive-primary, #3b82f6);
    background: var(--ui-surface-primary, #fff);
    width: 1rem;
    height: 1rem;
    line-height: 1;
    border-radius: 50%;
    text-align: center;
    font-size: 0.5625rem;
    align-self: center;
    z-index: 1;
  }

  .step-headline {
    font-weight: 500;
    color: var(--ui-text-primary, #111827);
  }

  .step-preview {
    display: block;
    font-size: 0.75rem;
    color: var(--ui-text-secondary, #6b7280);
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
    margin-top: 0.125rem;
  }

  .step-meta {
    font-size: 0.6875rem;
    color: var(--ui-text-tertiary, #9ca3af);
    font-variant-numeric: tabular-nums;
    margin-top: 0.125rem;
    display: block;
  }

  .line-chevron {
    color: var(--ui-text-tertiary, #9ca3af);
    font-size: 0.6875rem;
    align-self: center;
  }

  .step-payload {
    margin: 0.125rem 0 0.5rem 2.25rem;
    padding: 0.625rem 0.75rem;
    background: var(--ui-surface-secondary, #f9fafb);
    border-left: 2px solid var(--ui-border-subtle, #e5e7eb);
    border-radius: 0 4px 4px 0;
    font-family: ui-monospace, SFMono-Regular, Menlo, monospace;
    font-size: 0.75rem;
    line-height: 1.5;
    white-space: pre-wrap;
    word-break: break-word;
    max-height: 24rem;
    overflow-y: auto;
    color: var(--ui-text-primary, #111827);
  }

  .story-empty {
    margin: 0.5rem 0;
    text-align: center;
    color: var(--ui-text-tertiary, #9ca3af);
    font-style: italic;
    font-size: 0.8125rem;
  }

  .story-error {
    margin: 0;
    padding: 0.375rem 0.5rem;
    background: #fef2f2;
    border: 1px solid #fecaca;
    border-radius: 4px;
    font-size: 0.75rem;
    color: #991b1b;
  }

  .raw-toggle {
    margin-top: 0.5rem;
    border-top: 1px solid var(--ui-border-subtle, #e5e7eb);
    padding-top: 0.5rem;
  }

  .raw-summary {
    cursor: pointer;
    font-size: 0.75rem;
    color: var(--ui-text-secondary, #6b7280);
    padding: 0.25rem 0.5rem;
    border-radius: 4px;
    list-style: none;
    user-select: none;
  }

  .raw-summary::-webkit-details-marker {
    display: none;
  }

  .raw-summary::before {
    content: "▸ ";
    color: var(--ui-text-tertiary, #9ca3af);
    transition: transform 0.15s;
    display: inline-block;
  }

  .raw-toggle[open] .raw-summary::before {
    transform: rotate(90deg);
  }

  .raw-summary:hover {
    color: var(--ui-text-primary, #111827);
    background: var(--ui-surface-secondary, #f3f4f6);
  }

  .raw-body {
    margin-top: 0.5rem;
  }
</style>
