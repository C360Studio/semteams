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
  import ArtifactCard from "./ArtifactCard.svelte";
  import ProofReadinessCard from "./ProofReadinessCard.svelte";
  import TaskTrace from "./TaskTrace.svelte";
  import { renderMarkdown } from "$lib/utils/markdown";
  import { classifyVerdict, verdictLabel } from "$lib/utils/verdict";

  // Tool-call steps whose `tool_name` matches this prefix render their
  // arguments through ArtifactCard (structured fields, markdown-aware
  // text) instead of as raw JSON. Convention across pack-emitters:
  // emit_plan, emit_research_artifact, emit_autoresearch_baseline,
  // emit_autoresearch_artifact, emit_autoresearch_measurement, …
  const EMIT_TOOL_PREFIX = "emit_";

  function isEmitTool(name: string | undefined): boolean {
    return typeof name === "string" && name.startsWith(EMIT_TOOL_PREFIX);
  }

  function isProofReadinessTool(name: string | undefined): boolean {
    return name === "analyze_proof_readiness";
  }

  // Every loop ends its turn with a `decide(action, reason)` call — the
  // routing verdict. We surface it as a prominent chip + always-visible
  // rationale (rendered as markdown) rather than burying it as a generic
  // tool-call row, because it's the single most scannable fact about a
  // loop and is identical across every category pack.
  function isDecideTool(name: string | undefined): boolean {
    return name === "decide";
  }

  function decideAction(step: ToolCallStep): string | undefined {
    const a = step.tool_arguments?.action;
    return typeof a === "string" ? a : undefined;
  }

  function decideReason(step: ToolCallStep): string {
    const r = step.tool_arguments?.reason;
    return typeof r === "string" ? r : "";
  }

  function decideSubtopics(step: ToolCallStep): string[] {
    const subtopics = step.tool_arguments?.subtopics;
    return Array.isArray(subtopics)
      ? subtopics.filter((item): item is string => typeof item === "string")
      : [];
  }

  function isCBGFinalGate(
    step: ToolCallStep,
    action: string | undefined,
  ): boolean {
    const role = step.capability ?? "";
    return (
      role === "reviewer-dev-via-test" &&
      (action === "approved" ||
        action === "rejected" ||
        action === "rejected_retry")
    );
  }

  function verdictRoleText(
    step: ToolCallStep,
    action: string | undefined,
  ): string {
    if (!isCBGFinalGate(step, action)) return `${roleLabel(step)} decided`;
    if (action === "approved") return "Final Review Gate passed";
    if (action === "rejected_retry") return "Final Review Gate requested retry";
    return "Final Review Gate rejected";
  }

  // A model_call carries free-form prose in `response`; render it as
  // markdown when expanded. Tool calls and tool-decision model calls (no
  // text response) keep the raw JSON payload.
  function hasProseResponse(step: TrajectoryStep): step is ModelCallStep {
    return (
      step.step_type === "model_call" &&
      typeof step.response === "string" &&
      step.response.trim() !== ""
    );
  }

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
        {#if step.step_type === "tool_call" && isDecideTool(step.tool_name)}
          {@const action = decideAction(step)}
          {@const tone = classifyVerdict(action)}
          {@const reason = decideReason(step)}
          {@const isCBGGate = isCBGFinalGate(step, action)}
          {@const targetTasks = decideSubtopics(step)}
          <li
            class="story-step story-verdict"
            data-step-type="verdict"
            data-testid="story-verdict"
            data-verdict-tone={tone}
            data-gate={isCBGGate ? "cbg-final" : undefined}
          >
            <span class="line-icon verdict-icon" aria-hidden="true">◆</span>
            <div class="verdict-body">
              <div class="verdict-head">
                {#if isCBGGate}
                  <span class="verdict-gate" data-testid="cbg-final-gate-label">
                    Final Review Gate
                  </span>
                {/if}
                <span
                  class="verdict-chip"
                  data-tone={tone}
                  data-testid="verdict-chip">{verdictLabel(action)}</span
                >
                <span class="verdict-role">{verdictRoleText(step, action)}</span
                >
                {#if isCBGGate && targetTasks.length > 0}
                  <span class="verdict-target" data-testid="cbg-target-task">
                    Target {targetTasks.join(", ")}
                  </span>
                {/if}
                {#if metaFor(step)}
                  <span class="step-meta verdict-meta">{metaFor(step)}</span>
                {/if}
              </div>
              {#if reason}
                <div class="verdict-reason" data-testid="verdict-reason">
                  <!-- reason is LLM-authored; renderMarkdown escapes first
                       and emits only a fixed tag whitelist (XSS-safe). -->
                  <!-- eslint-disable-next-line svelte/no-at-html-tags -->
                  {@html renderMarkdown(reason)}
                </div>
              {/if}
            </div>
          </li>
        {:else}
          <li
            class="story-step"
            data-step-type={step.step_type}
            data-testid="story-step"
          >
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
              {#if step.step_type === "tool_call" && isEmitTool(step.tool_name)}
                <div class="step-payload-card" data-testid="story-step-payload">
                  <ArtifactCard
                    toolName={step.tool_name}
                    args={(step as ToolCallStep).tool_arguments}
                  />
                </div>
              {:else if step.step_type === "tool_call" && isProofReadinessTool(step.tool_name)}
                <div class="step-payload-card" data-testid="story-step-payload">
                  <ProofReadinessCard
                    result={(step as ToolCallStep).tool_result}
                  />
                </div>
              {:else if hasProseResponse(step)}
                <div
                  class="step-payload step-markdown"
                  data-testid="story-step-payload"
                >
                  <!-- model prose is LLM-authored; renderMarkdown escapes
                       first and emits only a fixed tag whitelist (XSS-safe). -->
                  <!-- eslint-disable-next-line svelte/no-at-html-tags -->
                  {@html renderMarkdown(step.response ?? "")}
                </div>
              {:else}
                <pre
                  class="step-payload"
                  data-testid="story-step-payload">{fullPayload(step)}</pre>
              {/if}
            {/if}
          </li>
        {/if}
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
            · {(trajectory.total_tokens_in ?? 0) +
              (trajectory.total_tokens_out ?? 0)} tokens
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

  .step-payload-card {
    margin: 0.125rem 0 0.5rem 2.25rem;
    max-height: 28rem;
    overflow-y: auto;
  }

  /* Verdict row — the decide(action, reason) surfacing. Chip + always-on
     rationale, no expand affordance (the reason is the payload). */
  .story-verdict {
    display: grid;
    grid-template-columns: 1.5rem 1fr;
    gap: 0.5rem;
    align-items: baseline;
    padding: 0.4375rem 0.5rem;
    width: calc(100% - 1rem);
  }

  .verdict-icon {
    color: var(--ui-text-secondary, #6b7280);
    background: var(--ui-surface-primary, #fff);
    width: 1rem;
    height: 1rem;
    line-height: 1;
    text-align: center;
    font-size: 0.625rem;
    align-self: center;
    z-index: 1;
  }

  .verdict-body {
    display: flex;
    flex-direction: column;
    gap: 0.25rem;
    min-width: 0;
  }

  .verdict-head {
    display: flex;
    align-items: baseline;
    flex-wrap: wrap;
    gap: 0.375rem;
  }

  .verdict-chip {
    font-size: 0.6875rem;
    font-weight: 600;
    padding: 0.0625rem 0.375rem;
    border-radius: 9999px;
    text-transform: capitalize;
    border: 1px solid transparent;
    /* Default (route) tone. */
    background: #eff6ff;
    color: #1d4ed8;
    border-color: #bfdbfe;
  }

  .verdict-chip[data-tone="approve"] {
    background: #d1fae5;
    color: #065f46;
    border-color: #6ee7b7;
  }

  .verdict-chip[data-tone="reject"] {
    background: #fee2e2;
    color: #991b1b;
    border-color: #fca5a5;
  }

  .verdict-chip[data-tone="clarify"] {
    background: #ffedd5;
    color: #9a3412;
    border-color: #fed7aa;
  }

  .verdict-gate {
    font-size: 0.6875rem;
    font-weight: 700;
    color: var(--ui-text-primary, #111827);
    text-transform: uppercase;
    letter-spacing: 0;
  }

  .verdict-role {
    font-size: 0.75rem;
    color: var(--ui-text-secondary, #6b7280);
  }

  .verdict-target {
    font-size: 0.6875rem;
    color: var(--ui-text-secondary, #6b7280);
    background: var(--ui-surface-secondary, #f3f4f6);
    border: 1px solid var(--ui-border-subtle, #e5e7eb);
    border-radius: 9999px;
    padding: 0.0625rem 0.375rem;
  }

  .verdict-meta {
    margin-top: 0;
  }

  .verdict-reason {
    font-size: 0.8125rem;
    line-height: 1.5;
    color: var(--ui-text-primary, #111827);
    border-left: 2px solid var(--ui-border-subtle, #e5e7eb);
    padding-left: 0.625rem;
  }

  /* Tone-tinted left rule so the verdict reads at a glance. */
  .story-verdict[data-verdict-tone="approve"] .verdict-reason {
    border-left-color: #6ee7b7;
  }
  .story-verdict[data-verdict-tone="reject"] .verdict-reason {
    border-left-color: #fca5a5;
  }
  .story-verdict[data-verdict-tone="clarify"] .verdict-reason {
    border-left-color: #fed7aa;
  }
  .story-verdict[data-verdict-tone="route"] .verdict-reason {
    border-left-color: #bfdbfe;
  }

  /* Rendered-markdown payload (model prose). Shares the indent + framing
     of .step-payload but lays out block elements instead of <pre> text. */
  .step-markdown {
    font-family: inherit;
    white-space: normal;
  }

  /* Shared element styles for renderMarkdown() output, scoped via :global
     since the markup is injected with {@html}. */
  .verdict-reason :global(.md-p),
  .step-markdown :global(.md-p) {
    margin: 0 0 0.5rem;
  }
  .verdict-reason :global(.md-p:last-child),
  .step-markdown :global(.md-p:last-child) {
    margin-bottom: 0;
  }
  .verdict-reason :global(.md-h),
  .step-markdown :global(.md-h) {
    margin: 0.5rem 0 0.25rem;
    font-size: 0.875rem;
    font-weight: 600;
  }
  .verdict-reason :global(.md-list),
  .step-markdown :global(.md-list) {
    margin: 0 0 0.5rem;
    padding-left: 1.25rem;
  }
  .verdict-reason :global(.md-quote),
  .step-markdown :global(.md-quote) {
    margin: 0 0 0.5rem;
    padding-left: 0.625rem;
    border-left: 2px solid var(--ui-border-subtle, #e5e7eb);
    color: var(--ui-text-secondary, #6b7280);
  }
  .verdict-reason :global(code),
  .step-markdown :global(code) {
    font-family: ui-monospace, SFMono-Regular, Menlo, monospace;
    font-size: 0.8125em;
    background: var(--ui-surface-tertiary, #e5e7eb);
    padding: 0.0625rem 0.25rem;
    border-radius: 3px;
  }
  .verdict-reason :global(.md-code),
  .step-markdown :global(.md-code) {
    margin: 0 0 0.5rem;
    padding: 0.5rem 0.625rem;
    background: var(--ui-surface-secondary, #f9fafb);
    border: 1px solid var(--ui-border-subtle, #e5e7eb);
    border-radius: 4px;
    overflow-x: auto;
    font-size: 0.75rem;
    line-height: 1.5;
  }
  .verdict-reason :global(.md-code code),
  .step-markdown :global(.md-code code) {
    background: none;
    padding: 0;
  }
  .verdict-reason :global(a),
  .step-markdown :global(a) {
    color: var(--ui-interactive-primary, #2563eb);
    text-decoration: underline;
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
