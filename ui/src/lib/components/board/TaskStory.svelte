<script lang="ts">
  // Story view for a task — plain-language narrative of what the AI
  // did. Sources from the GraphQL `trajectory(loopId)` field (semstreams
  // beta.160), which returns an immutable, privacy-bounded fact log
  // (facts[]) — kind, causal position, status, timing, token counts, and
  // short previews (model/tool/provider name). It carries NO response
  // text, tool arguments, or tool results (see agentApi.ts and
  // types/agent.ts for the full contract). The story renders one line
  // per completed observation, in time order, in human-readable terms;
  // the expand affordance on each row shows the metadata fields we do
  // have rather than fabricated content.
  //
  // Product-regression note (beta.159 → beta.160 migration): the old
  // wire shape carried full model response prose and tool call
  // arguments/results, which powered a `decide(action, reason)` verdict
  // chip (with CBG final-gate detection) and structured ArtifactCard /
  // ProofReadinessCard rendering for emit_*/analyze_proof_readiness tool
  // calls. None of that content survives in the fact log — only a
  // StorageReference pointer to it, which this pass does not fetch. That
  // richer rendering is gone from this view until a follow-up pass wires
  // an evidence-store fetch; ArtifactCard/ProofReadinessCard/verdict.ts
  // are left in the tree (unused for now) for that follow-up to reuse.
  //
  // The wire-level message log lives behind a "Show raw activity"
  // toggle below — answers the few users who actually want it,
  // doesn't intrude on the rest.
  //
  // Controls (design D3): when the coordinator is in focus, rule-fired
  // control loops attached to its run render as one explained row each in a
  // closing "Controls" group. The rule's identity is not recorded by the
  // frozen runtime (#298), so every row says "rule: unknown" and why; the rest
  // of the row is read from the graph on demand (never polled) and any field
  // that cannot be read says so instead of guessing.

  import { untrack } from "svelte";
  import { agentApi } from "$lib/services/agentApi";
  import {
    SETTLE_REREAD_MS,
    awaitsRecording,
    loadControlEvidence,
  } from "$lib/services/controlEvidence";
  import type { LoopTrajectory, TrajectoryFact } from "$lib/types/agent";
  import {
    controlOutcome,
    type ControlEvidence,
    type Known,
    type TaskControl,
  } from "$lib/types/control";
  import { SvelteSet } from "svelte/reactivity";
  import TaskTrace from "./TaskTrace.svelte";

  // Facts worth a narrative row. loop.started/model.requested/
  // tool.requested are paired precursors to a completed observation (or,
  // for loop.started, redundant with the "You asked" line above) and
  // loop.terminal powers the closing banner instead.
  const NARRATIVE_KINDS: ReadonlySet<TrajectoryFact["kind"]> = new Set([
    "model.completed",
    "tool.completed",
    "context.compacted",
  ]);

  function isFailed(fact: TrajectoryFact): boolean {
    return !!fact.status && fact.status !== "completed";
  }

  interface Props {
    loopId: string;
    /** When set, used to provide the very first "You asked: …" line. */
    prompt?: string;
    /**
     * Rule-fired controls attached to this run, rendered as the closing
     * "Controls" group. Pass them only while the coordinator is in focus.
     */
    controls?: TaskControl[];
    /** A rule-fired loop on this run could not be classified, so `controls` may be incomplete. */
    controlsTruncated?: boolean;
    /** Focus a control's own trajectory (the panel's existing sub-loop focus). */
    onFocusLoop?: (loopId: string) => void;
  }

  let {
    loopId,
    prompt,
    controls = [],
    controlsTruncated = false,
    onFocusLoop,
  }: Props = $props();

  const POLL_INTERVAL_MS = 3000;

  let trajectory = $state<LoopTrajectory | null>(null);
  let lastError = $state<string | null>(null);
  let expanded = new SvelteSet<number>();
  let showRaw = $state(false);

  // Request-race guard (same pattern as RunEvidencePanel): TaskDetailPanel
  // switches focusedLoopId without remounting this component, so a slow
  // response for the previous loop must never overwrite the current loop's
  // story, and a loop switch must never be dropped behind an in-flight fetch.
  let requestSeq = 0;
  let displayedLoopId: string | null = null;

  async function refresh() {
    const requestedLoopId = loopId;
    const requestId = ++requestSeq;
    if (displayedLoopId !== requestedLoopId) {
      trajectory = null;
      lastError = null;
      expanded.clear();
      displayedLoopId = requestedLoopId;
    }
    try {
      const next = await agentApi.getLoopTrajectory(requestedLoopId);
      if (requestId !== requestSeq || requestedLoopId !== loopId) {
        return;
      }
      trajectory = next;
      lastError = null;
    } catch (err) {
      if (requestId !== requestSeq || requestedLoopId !== loopId) {
        return;
      }
      lastError = err instanceof Error ? err.message : String(err);
    }
  }

  $effect(() => {
    const id = loopId;
    if (!id) return;
    void refresh();
    const handle = setInterval(refresh, POLL_INTERVAL_MS);
    return () => clearInterval(handle);
  });

  // Control evidence is read once per change, not polled. The key names the
  // controls and each loop's state: the terminal decide lands when a control's
  // loop ends, so a state change is the moment worth re-reading. The effect
  // must NOT depend on the `controls` array itself — the task list re-derives
  // on every loop event and hands over a fresh array each time.
  let evidence = $state<Record<string, ControlEvidence>>({});
  let evidenceError = $state<string | null>(null);
  const controlsKey = $derived(
    controls.map((c) => `${c.loopId}:${c.loop.state}`).join("|"),
  );

  $effect(() => {
    const key = controlsKey;
    if (!key) {
      evidence = {};
      evidenceError = null;
      return;
    }
    const current = untrack(() => controls);
    const ctrl = new AbortController();
    let settleTimer: ReturnType<typeof setTimeout> | undefined;

    async function load(): Promise<Record<string, ControlEvidence> | null> {
      try {
        const next = await loadControlEvidence(current, ctrl.signal);
        if (ctrl.signal.aborted) return null;
        evidence = next;
        evidenceError = null;
        return next;
      } catch (err) {
        if (ctrl.signal.aborted) return null;
        evidenceError = err instanceof Error ? err.message : String(err);
        return null;
      }
    }

    void (async () => {
      const first = await load();
      if (!first || ctrl.signal.aborted) return;
      // A loop can be seen as finished a moment before its outcome and decide
      // are readable from the graph: read once more before settling on unknown.
      // The key changes if the loop's state does, which starts a fresh cycle.
      if (current.some((c) => awaitsRecording(c.loop.state, first[c.loopId]))) {
        settleTimer = setTimeout(() => void load(), SETTLE_REREAD_MS);
      }
    })();
    return () => {
      ctrl.abort();
      clearTimeout(settleTimer);
    };
  });

  /** A graph value rendered as text, or "unknown (reason)". */
  function shown(value: Known<string> | undefined): string {
    if (!value) return "loading…";
    return value.status === "known" ? value.value : `unknown (${value.reason})`;
  }

  /** The description is the first line of the control's prompt. */
  function firstLine(value: Known<string> | undefined): string {
    return value?.status === "known" ? (value.value.split("\n")[0] ?? "").trim() : "";
  }

  // narrativeFacts drives the main list; terminalFact powers the closing
  // banner; totalTokens sums the merged (possibly multi-page) totals
  // agentApi.ts already accumulated across next_cursor pages.
  const narrativeFacts = $derived(
    trajectory
      ? trajectory.facts.filter((f) => NARRATIVE_KINDS.has(f.kind))
      : [],
  );
  const terminalFact = $derived(
    trajectory?.facts.find((f) => f.kind === "loop.terminal"),
  );
  const totalTokens = $derived(
    trajectory
      ? trajectory.observed_totals.tokens_in +
          trajectory.observed_totals.tokens_out
      : 0,
  );

  function roleLabel(fact: TrajectoryFact): string {
    const cap = fact.capability_preview;
    if (!cap) return "Agent";
    return cap.charAt(0).toUpperCase() + cap.slice(1);
  }

  /**
   * One-line narrative summary for a fact. The detail behind the row's
   * expand affordance is the fact's own metadata fields (JSON) — no
   * response text, tool arguments, or tool results survive in the fact
   * log to show instead.
   */
  function lineFor(fact: TrajectoryFact): string {
    const role = roleLabel(fact);
    if (fact.kind === "model.completed") {
      if (isFailed(fact)) return `${role} model call failed`;
      // No response text is available — tool_count is the closest proxy
      // for "issued tool calls" (reasoned) vs "replied with text".
      return (fact.tool_count ?? 0) > 0 ? `${role} reasoned` : `${role} replied`;
    }
    if (fact.kind === "tool.completed") {
      const name = fact.tool_preview || "a tool";
      return isFailed(fact)
        ? `${role} tried ${name} — failed`
        : `${role} used ${name}`;
    }
    if (fact.kind === "context.compacted") {
      return "Context compacted";
    }
    return "Step";
  }

  function metaFor(fact: TrajectoryFact): string {
    const parts: string[] = [];
    if (typeof fact.elapsed_ms === "number") parts.push(`${fact.elapsed_ms}ms`);
    const tokens = (fact.tokens_in ?? 0) + (fact.tokens_out ?? 0);
    if (tokens > 0) parts.push(`${tokens} tokens`);
    return parts.join(" · ");
  }

  function previewFor(fact: TrajectoryFact): string {
    if (fact.kind === "model.completed") {
      return [fact.model_preview, fact.provider_preview]
        .filter((v): v is string => !!v)
        .join(" · ");
    }
    if (fact.kind === "tool.completed" && isFailed(fact) && fact.error_category) {
      return `error: ${fact.error_category}`;
    }
    return "";
  }

  // Facts never carry response text, tool arguments, or tool results —
  // dump the metadata fields we do have. When evidence was captured, the
  // storage pointer (key/content_type/size) is visible here too, ready
  // for a future evidence-fetch pass; this pass does not fetch it.
  function fullPayload(fact: TrajectoryFact): string {
    try {
      return JSON.stringify(fact, null, 2);
    } catch {
      return "(unable to serialise)";
    }
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

  // Outcome badge text for the closing line. TrajectoryStatus is a
  // closed, bounded-display vocabulary upstream (requested/completed/
  // failed/cancelled) — it does not carry the old dispatch-level
  // "truncated" outcome, so a max-length stop no longer gets its own
  // "Stopped (max length)" label here; it now reads as the closest
  // status the backend assigned (typically "failed").
  function outcomeLabel(status: string | undefined): string {
    if (!status) return "In progress";
    if (status === "completed") return "Done";
    if (status === "failed") return "Failed";
    if (status === "cancelled") return "Cancelled";
    return status;
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
  {:else if narrativeFacts.length === 0}
    <p class="story-empty">No steps recorded yet.</p>
  {:else}
    <ol class="story-list" data-testid="story-list">
      {#each narrativeFacts as fact, idx (fact.attempt_id ?? idx)}
        <li
          class="story-step"
          data-step-type={fact.kind}
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
              <span class="step-headline">{lineFor(fact)}</span>
              {#if previewFor(fact)}
                <span class="step-preview">{previewFor(fact)}</span>
              {/if}
              {#if metaFor(fact)}
                <span class="step-meta">{metaFor(fact)}</span>
              {/if}
            </span>
            <span class="line-chevron" aria-hidden="true">
              {expanded.has(idx) ? "▾" : "▸"}
            </span>
          </button>
          {#if expanded.has(idx)}
            <pre
              class="step-payload"
              data-testid="story-step-payload">{fullPayload(fact)}</pre>
          {/if}
        </li>
      {/each}
    </ol>
  {/if}

  {#if trajectory && trajectory.terminal_observed}
    <div class="story-line story-line-done" data-testid="story-line-done">
      <span class="line-icon" aria-hidden="true">✓</span>
      <span class="line-body">
        <span class="line-label">{outcomeLabel(terminalFact?.status)}</span>
        <span class="step-meta">
          {fmtDuration(terminalFact?.elapsed_ms)}
          {#if totalTokens > 0}
            · {totalTokens} tokens
          {/if}
        </span>
      </span>
    </div>
  {/if}

  {#if controls.length > 0}
    <section
      class="story-controls"
      data-testid="story-controls"
      aria-labelledby="story-controls-title"
    >
      <h3 class="controls-title" id="story-controls-title">Controls fired on this run</h3>
      {#if controlsTruncated}
        <p class="controls-note" data-testid="controls-truncated">
          Some rule-fired loops on this run could not be classified, so this list may be incomplete.
        </p>
      {/if}
      {#if evidenceError}
        <p class="story-error" role="alert" data-testid="controls-error">
          Couldn't load control details: {evidenceError}
        </p>
      {/if}
      <ul class="controls-list">
        {#each controls as control (control.loopId)}
          {@const ev = evidence[control.loopId]}
          {@const outcome = controlOutcome(control.loop.state)}
          {@const description = firstLine(ev?.description)}
          <li
            class="control-event"
            data-testid="control-event"
            data-loop={control.loopId}
          >
            <div class="control-identity-row">
              <button
                type="button"
                class="control-open"
                data-testid="control-identity"
                onclick={() => onFocusLoop?.(control.loopId)}
              >
                <span class="control-role">{control.loop.role || "control"}</span>
                <span class="control-loop-id">{control.loopId}</span>
                <span class="sr-only">— view this loop's trajectory</span>
              </button>
              {#if description}
                <span class="control-description">{description}</span>
              {/if}
            </div>

            <p class="control-line" data-testid="control-rule">
              <span class="control-key">rule: unknown</span>
              <span class="control-why"
                >rule identity is not recorded by the frozen runtime (#298)</span
              >
            </p>

            <p class="control-line" data-testid="control-firing">
              <span class="control-key">fired on</span>
              <code class="control-entity">{control.firingEntityId}</code>
            </p>

            <p class="control-line" data-testid="control-fact">
              <span class="control-key">run fact at spawn</span>
              {#if !ev}
                <span>loading…</span>
              {:else if ev.fact.status === "known"}
                <code>agent.run.phase → {ev.fact.value.to}</code>
                <span>· source {ev.fact.value.source || "unknown"}</span>
                {#if ev.fact.value.note}
                  <span class="control-note">{ev.fact.value.note}</span>
                {/if}
              {:else}
                <span>unknown ({ev.fact.reason})</span>
              {/if}
            </p>

            <p class="control-line" data-testid="control-outcome">
              <span class="control-key">outcome</span>
              <strong>{outcome}</strong>
              <span>(loop state: {control.loop.state.replace(/_/g, " ")})</span>
              <span>· recorded outcome: {shown(ev?.outcome)}</span>
              {#if ev?.nextAction.status === "known"}
                <span class="control-decide">decide({ev.nextAction.value})</span>
              {:else}
                <span class="control-decide">decide: {shown(ev?.nextAction)}</span>
              {/if}
              <span class="control-note">reason: {shown(ev?.reason)}</span>
            </p>
          </li>
        {/each}
      </ul>
    </section>
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

  .story-controls {
    margin-top: 0.25rem;
    border-top: 1px solid var(--ui-border-subtle, #e5e7eb);
    padding-top: 0.5rem;
  }

  .controls-title {
    margin: 0 0 0.375rem;
    font-size: 0.75rem;
    font-weight: 600;
    text-transform: uppercase;
    letter-spacing: 0.04em;
    color: var(--ui-text-secondary, #6b7280);
  }

  .controls-note {
    margin: 0 0 0.375rem;
    font-size: 0.75rem;
    color: var(--ui-text-secondary, #6b7280);
  }

  .controls-list {
    list-style: none;
    margin: 0;
    padding: 0;
    display: flex;
    flex-direction: column;
    gap: 0.5rem;
  }

  .control-event {
    display: flex;
    flex-direction: column;
    gap: 0.25rem;
    padding: 0.5rem 0.625rem;
    background: var(--ui-surface-primary, #fff);
    border: 1px solid var(--ui-border-subtle, #e5e7eb);
    border-left: 3px solid var(--ui-interactive-primary, #3b82f6);
    border-radius: 6px;
    font-size: 0.75rem;
    color: var(--ui-text-primary, #111827);
  }

  .control-identity-row {
    display: flex;
    flex-direction: column;
    gap: 0.125rem;
  }

  .control-open {
    all: unset;
    cursor: pointer;
    display: flex;
    flex-wrap: wrap;
    align-items: baseline;
    gap: 0.375rem;
    padding: 0.125rem 0.25rem;
    margin: 0 -0.25rem;
    border-radius: 4px;
  }

  .control-open:hover {
    background: var(--ui-surface-secondary, #f3f4f6);
  }

  .control-open:focus-visible {
    outline: 2px solid var(--ui-interactive-primary, #3b82f6);
    outline-offset: 1px;
  }

  .control-role {
    font-weight: 600;
    text-decoration: underline;
  }

  .control-loop-id {
    font-family: ui-monospace, SFMono-Regular, Menlo, monospace;
    font-size: 0.6875rem;
    color: var(--ui-text-secondary, #6b7280);
    overflow-wrap: anywhere;
  }

  .control-description {
    color: var(--ui-text-secondary, #6b7280);
    overflow-wrap: anywhere;
  }

  .control-line {
    margin: 0;
    display: flex;
    flex-wrap: wrap;
    gap: 0.25rem 0.375rem;
    align-items: baseline;
    overflow-wrap: anywhere;
  }

  .control-key {
    font-weight: 600;
  }

  .control-why,
  .control-note {
    color: var(--ui-text-secondary, #6b7280);
  }

  .control-entity {
    font-size: 0.6875rem;
    color: var(--ui-text-secondary, #6b7280);
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
