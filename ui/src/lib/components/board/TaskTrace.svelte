<script lang="ts">
  // Task-scoped runtime trace. Polls /message-logger/entries on a 3s
  // cadence, filters client-side for messages that mention the
  // task's loop_id (in subject or raw_data), classifies each entry,
  // and renders a time-ordered timeline. LLM request/response are
  // first-class (separate icon + color + prominent rendering of
  // prompt/result snippets).
  //
  // Pattern: $effect-driven setInterval with cleanup, rune state for
  // entries + UI toggles. No store wrapper — this view is per-loop
  // ephemeral and the polling lifecycle is tied to the component.

  import {
    messageLoggerApi,
    entryMentionsLoop,
    classifyEntry,
    type MessageLogEntry,
    type EntryKind,
  } from "$lib/services/messageLoggerApi";
  import { SvelteSet } from "svelte/reactivity";

  interface Props {
    loopId: string;
  }

  let { loopId }: Props = $props();

  const POLL_INTERVAL_MS = 3000;
  const FETCH_LIMIT = 500;

  let entries = $state<MessageLogEntry[]>([]);
  let lastError = $state<string | null>(null);
  let lastFetched = $state<Date | null>(null);
  let inFlight = false;
  let expanded = new SvelteSet<number>();

  // Filter the broad pull down to messages that mention this task.
  const scoped = $derived(
    entries.filter((e) => entryMentionsLoop(e, loopId)),
  );

  // Classify per entry once; multiple lookups during render reuse.
  const classified = $derived(
    scoped.map((e) => ({ entry: e, kind: classifyEntry(e) })),
  );

  // Coarse counts for the header.
  const counts = $derived.by(() => {
    const c: Record<EntryKind, number> = {
      "llm-request": 0,
      "llm-response": 0,
      "tool-execute": 0,
      "tool-result": 0,
      lifecycle: 0,
      graph: 0,
      other: 0,
    };
    for (const r of classified) c[r.kind]++;
    return c;
  });

  async function refresh() {
    if (inFlight) return;
    inFlight = true;
    try {
      const fetched = await messageLoggerApi.fetchEntries({
        limit: FETCH_LIMIT,
      });
      // The API returns newest-first; we want oldest-first for a
      // timeline read.
      entries = fetched.slice().reverse();
      lastError = null;
    } catch (err) {
      lastError = err instanceof Error ? err.message : String(err);
    } finally {
      lastFetched = new Date();
      inFlight = false;
    }
  }

  // Lifecycle: kick off immediately, poll on interval, cleanup on
  // teardown (component unmount or loopId change re-runs the effect).
  $effect(() => {
    // Tie the effect to loopId so a task switch resets state.
    const id = loopId;
    if (!id) return;
    void refresh();
    const handle = setInterval(refresh, POLL_INTERVAL_MS);
    return () => clearInterval(handle);
  });

  function toggleExpand(seq: number) {
    if (expanded.has(seq)) expanded.delete(seq);
    else expanded.add(seq);
  }

  function formatTime(iso: string): string {
    try {
      const d = new Date(iso);
      const hh = String(d.getHours()).padStart(2, "0");
      const mm = String(d.getMinutes()).padStart(2, "0");
      const ss = String(d.getSeconds()).padStart(2, "0");
      const ms = String(d.getMilliseconds()).padStart(3, "0");
      return `${hh}:${mm}:${ss}.${ms}`;
    } catch {
      return iso;
    }
  }

  function kindGlyph(kind: EntryKind): string {
    switch (kind) {
      case "llm-request":
        return "→ LLM";
      case "llm-response":
        return "← LLM";
      case "tool-execute":
        return "→ tool";
      case "tool-result":
        return "← tool";
      case "lifecycle":
        return "◇";
      case "graph":
        return "▣";
      case "other":
        return "·";
    }
  }

  function tokensSummary(raw: unknown): string | null {
    if (!raw || typeof raw !== "object") return null;
    const r = raw as Record<string, unknown>;
    const payload = (r.payload as Record<string, unknown>) ?? r;
    const tokensIn = payload?.tokens_in ?? payload?.input_tokens;
    const tokensOut = payload?.tokens_out ?? payload?.output_tokens;
    if (typeof tokensIn === "number" || typeof tokensOut === "number") {
      const inS = typeof tokensIn === "number" ? `${tokensIn} in` : "";
      const outS = typeof tokensOut === "number" ? `${tokensOut} out` : "";
      return [inS, outS].filter(Boolean).join(" / ");
    }
    return null;
  }

  function shortPreview(raw: unknown): string {
    if (!raw) return "";
    try {
      const s = JSON.stringify(raw);
      return s.length > 140 ? s.slice(0, 137) + "…" : s;
    } catch {
      return String(raw);
    }
  }

  function lastFetchedLabel(d: Date | null): string {
    if (!d) return "—";
    const ms = Date.now() - d.getTime();
    if (ms < 5000) return "just now";
    if (ms < 60000) return `${Math.round(ms / 1000)}s ago`;
    return `${Math.round(ms / 60000)}m ago`;
  }
</script>

<div class="task-trace" data-testid="task-trace">
  <div class="trace-header">
    <span class="trace-summary">
      <strong>{scoped.length}</strong> message{scoped.length === 1 ? "" : "s"}
      {#if counts["llm-request"] + counts["llm-response"] > 0}
        · <strong>{counts["llm-request"]}</strong> LLM request{counts["llm-request"] === 1 ? "" : "s"}
        · <strong>{counts["llm-response"]}</strong> response{counts["llm-response"] === 1 ? "" : "s"}
      {/if}
      {#if counts["tool-execute"] + counts["tool-result"] > 0}
        · <strong>{counts["tool-execute"]}</strong> tool call{counts["tool-execute"] === 1 ? "" : "s"}
      {/if}
    </span>
    <button
      type="button"
      class="trace-refresh"
      onclick={refresh}
      title="Refresh now"
      aria-label="Refresh trace"
    >↻</button>
  </div>

  {#if lastError}
    <p class="trace-error" role="alert" data-testid="trace-error">
      Couldn't fetch messages: {lastError}
    </p>
  {/if}

  {#if scoped.length === 0 && !lastError}
    <p class="trace-empty">
      No messages yet for this task. Updates every {POLL_INTERVAL_MS / 1000}s.
    </p>
  {:else}
    <ol class="trace-list" data-testid="trace-list">
      {#each classified as { entry, kind } (entry.sequence)}
        <li
          class="trace-row"
          data-kind={kind}
          data-testid="trace-row"
        >
          <button
            type="button"
            class="row-button"
            onclick={() => toggleExpand(entry.sequence)}
            aria-expanded={expanded.has(entry.sequence)}
          >
            <span class="row-time">{formatTime(entry.timestamp)}</span>
            <span class="row-kind">{kindGlyph(kind)}</span>
            <span class="row-subject">{entry.subject}</span>
            {#if tokensSummary(entry.raw_data)}
              <span class="row-tokens">{tokensSummary(entry.raw_data)}</span>
            {/if}
            <span class="row-disclosure" aria-hidden="true">
              {expanded.has(entry.sequence) ? "▾" : "▸"}
            </span>
          </button>
          {#if expanded.has(entry.sequence)}
            <pre
              class="row-expanded"
              data-testid="trace-row-expanded"
            >{shortPreview(entry.raw_data) === ""
                ? "(no payload)"
                : JSON.stringify(entry.raw_data, null, 2)}</pre>
          {/if}
        </li>
      {/each}
    </ol>
  {/if}

  <p class="trace-footer">
    Polling every {POLL_INTERVAL_MS / 1000}s · last fetched {lastFetchedLabel(lastFetched)}
  </p>
</div>

<style>
  .task-trace {
    display: flex;
    flex-direction: column;
    gap: 0.5rem;
    height: 100%;
  }

  .trace-header {
    display: flex;
    align-items: center;
    gap: 0.5rem;
    padding: 0.375rem 0.625rem;
    background: var(--ui-surface-secondary, #f3f4f6);
    border: 1px solid var(--ui-border-subtle, #e5e7eb);
    border-radius: 6px;
    font-size: 0.75rem;
    color: var(--ui-text-secondary, #6b7280);
  }

  .trace-summary {
    flex: 1;
  }

  .trace-summary strong {
    color: var(--ui-text-primary, #111827);
    font-variant-numeric: tabular-nums;
  }

  .trace-refresh {
    all: unset;
    cursor: pointer;
    width: 1.375rem;
    height: 1.375rem;
    line-height: 1;
    text-align: center;
    border-radius: 4px;
    color: var(--ui-text-secondary, #6b7280);
    font-size: 0.875rem;
  }

  .trace-refresh:hover {
    background: var(--ui-surface-tertiary, #e5e7eb);
    color: var(--ui-text-primary, #111827);
  }

  .trace-error {
    margin: 0;
    padding: 0.375rem 0.5rem;
    background: #fef2f2;
    border: 1px solid #fecaca;
    border-radius: 4px;
    font-size: 0.75rem;
    color: #991b1b;
  }

  .trace-empty {
    margin: 0.5rem 0;
    text-align: center;
    color: var(--ui-text-tertiary, #9ca3af);
    font-style: italic;
    font-size: 0.8125rem;
  }

  .trace-list {
    list-style: none;
    padding: 0;
    margin: 0;
    display: flex;
    flex-direction: column;
    gap: 0.125rem;
    flex: 1;
    overflow-y: auto;
  }

  .trace-row {
    display: flex;
    flex-direction: column;
    border-radius: 4px;
    border-left: 3px solid var(--ui-border-subtle, #e5e7eb);
    background: var(--ui-surface-primary, #fff);
  }

  /* Color the left edge to surface what kind of message this is. */
  .trace-row[data-kind="llm-request"] {
    border-left-color: var(--ui-interactive-primary, #3b82f6);
  }
  .trace-row[data-kind="llm-response"] {
    border-left-color: var(--ui-interactive-primary, #3b82f6);
    background: rgba(59, 130, 246, 0.04);
  }
  .trace-row[data-kind="tool-execute"],
  .trace-row[data-kind="tool-result"] {
    border-left-color: var(--col-executing, #14b8a6);
  }
  .trace-row[data-kind="lifecycle"] {
    border-left-color: var(--col-thinking, #3b82f6);
    opacity: 0.85;
  }
  .trace-row[data-kind="graph"],
  .trace-row[data-kind="other"] {
    opacity: 0.65;
  }

  .row-button {
    all: unset;
    cursor: pointer;
    display: grid;
    grid-template-columns: 6.5rem 5rem 1fr auto auto;
    gap: 0.5rem;
    align-items: baseline;
    padding: 0.25rem 0.5rem;
    font-size: 0.75rem;
    font-family: ui-monospace, SFMono-Regular, Menlo, monospace;
  }

  .row-button:hover {
    background: var(--ui-surface-secondary, #f3f4f6);
  }

  .row-button:focus-visible {
    outline: 2px solid var(--ui-interactive-primary, #3b82f6);
    outline-offset: 1px;
  }

  .row-time {
    color: var(--ui-text-tertiary, #9ca3af);
    font-variant-numeric: tabular-nums;
  }

  .row-kind {
    color: var(--ui-text-secondary, #6b7280);
    font-weight: 600;
  }

  .row-subject {
    color: var(--ui-text-primary, #111827);
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }

  .row-tokens {
    color: var(--ui-text-tertiary, #9ca3af);
    font-size: 0.6875rem;
  }

  .row-disclosure {
    color: var(--ui-text-tertiary, #9ca3af);
    font-size: 0.6875rem;
  }

  .row-expanded {
    margin: 0;
    padding: 0.5rem 0.625rem 0.625rem 1rem;
    background: var(--ui-surface-secondary, #f9fafb);
    border-top: 1px dashed var(--ui-border-subtle, #e5e7eb);
    color: var(--ui-text-primary, #111827);
    font-family: ui-monospace, SFMono-Regular, Menlo, monospace;
    font-size: 0.6875rem;
    line-height: 1.5;
    white-space: pre-wrap;
    word-break: break-word;
    max-height: 18rem;
    overflow-y: auto;
  }

  .trace-footer {
    margin: 0;
    text-align: center;
    color: var(--ui-text-tertiary, #9ca3af);
    font-size: 0.6875rem;
  }
</style>
