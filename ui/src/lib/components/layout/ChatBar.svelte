<script lang="ts">
  import { agentApi } from "$lib/services/agentApi";
  import { chatHandoff } from "$lib/stores/chatHandoff.svelte";
  import { taskStore } from "$lib/stores/taskStore.svelte";
  import type { ControlSignal } from "$lib/types/agent";

  let input = $state("");
  let sending = $state(false);
  let error = $state<string | null>(null);
  let inputEl = $state<HTMLInputElement>();

  /** Slash commands that operate on the selected task. */
  const TASK_COMMANDS: Record<string, ControlSignal> = {
    "/approve": "approve",
    "/reject": "reject",
    "/pause": "pause",
    "/resume": "resume",
    "/cancel": "cancel",
  };
  // The spec (/create-change, /spec), build (/dev-via-test), and
  // /implement-spec commands are parked with their category packs pending
  // the canonical-predicate migration (ADR-058); restore them when the
  // packs are re-wired.
  const TEAM_MESSAGE_COMMANDS = new Set(["/research", "/optimize"]);
  const RUN_MESSAGE_COMMANDS = new Set<string>([]);
  const TASK_SLASH_COMMANDS = Object.keys(TASK_COMMANDS);
  const ALL_SLASH_COMMANDS = [
    ...TEAM_MESSAGE_COMMANDS,
    ...RUN_MESSAGE_COMMANDS,
    ...TASK_SLASH_COMMANDS,
  ];

  /**
   * Public team shortcuts. They are coordinator-routed hints, not
   * direct role overrides; the coordinator still validates the prompt.
   */
  const TEAM_CHIPS = [
    { id: "research", label: "Research", prefix: "/research " },
    { id: "optimize", label: "Optimize", prefix: "/optimize " },
  ] as const;

  // Placeholder copy is human-shaped, not slash-command-shaped. Slash
  // commands still work — we just don't shout them in the placeholder.
  let placeholder = $derived(
    taskStore.selectedTask
      ? "Reply to this task…"
      : "What should the team work on?",
  );

  // Surface the slash-command hints only when the user is actively
  // typing a slash command — "/" followed by no whitespace yet. Once a
  // space lands ("/research survey…") the user is composing a hinted
  // message: the hints dropdown closes and the team chips return, so
  // clicking another chip can REPLACE the prefix (the applyTeamHint
  // flow the action-chips journey exercises). Default state stays clean.
  let showingSlash = $derived(/^\/\S*$/.test(input.trimStart()));
  let visibleSlashCommands = $derived(
    taskStore.selectedTask ? ALL_SLASH_COMMANDS : [...TEAM_MESSAGE_COMMANDS],
  );

  function applyTeamHint(prefix: string) {
    // If the user already started typing a different team prefix,
    // replace it; otherwise prepend to whatever's there.
    const stripped = input.replace(
      /^(@\w+|\/(?:research|optimize|autoresearch))\s*/i,
      "",
    );
    input = prefix + stripped;
    inputEl?.focus();
    // Move the caret to the end so the user starts typing after the
    // team prefix, not in the middle of it.
    queueMicrotask(() => {
      if (inputEl) inputEl.setSelectionRange(input.length, input.length);
    });
  }

  function approveNext() {
    // Find the most-urgent waiting task (first in the needs_you column)
    // and select it. The right-rail drilldown opens with Approve/Reject
    // buttons already in scope — user clicks once more to actually
    // approve. We don't fire the signal directly so the user sees what
    // they're approving before it goes.
    const waiting = taskStore.byColumn.needs_you;
    const next = waiting[0];
    if (next) taskStore.selectTask(next.id);
  }

  $effect(() => {
    const draft = chatHandoff.draft;
    if (!draft) return;
    input = draft.content;
    error = null;
    chatHandoff.consumeDraft(draft.id);
    inputEl?.focus();
    queueMicrotask(() => {
      if (inputEl) inputEl.setSelectionRange(input.length, input.length);
    });
  });

  function outboundMessage(text: string): string {
    const artifact = chatHandoff.artifactContext;
    if (!artifact) return text;
    return [
      text,
      "",
      "---",
      `Artifact context: ${artifact.title}`,
      `Artifact tool: ${artifact.toolName}`,
      "",
      artifact.content,
    ].join("\n");
  }

  async function handleSubmit() {
    const text = input.trim();
    if (!text || sending) return;
    const message = outboundMessage(text);

    error = null;

    // Check for slash commands before entering the async sending state.
    // The guard must run before `sending = true` to avoid getting stuck
    // in the disabled state when the guard early-returns.
    const firstWord = text.split(/\s+/)[0].toLowerCase();
    if (firstWord in TASK_COMMANDS || RUN_MESSAGE_COMMANDS.has(firstWord)) {
      if (!taskStore.selectedTask) {
        error = "Select a task first to use slash commands.";
        return;
      }
    }

    sending = true;

    try {
      if (firstWord in TASK_COMMANDS) {
        // Slash command — route to signal API on the selected task.
        const signal = TASK_COMMANDS[firstWord];
        const reason = text.slice(firstWord.length).trim() || undefined;
        await agentApi.sendSignal(taskStore.selectedTask!.id, signal, reason);
        // Unreachable while RUN_MESSAGE_COMMANDS is empty (dev packs parked,
      // ADR-058) — kept so repopulating the Set re-activates run routing.
    } else if (RUN_MESSAGE_COMMANDS.has(firstWord)) {
        // Slash command — dispatch as a run-attached message so the backend
        // command sees the selected task as UserMessage.RunID.
        await agentApi.sendMessage(message, {
          runId: taskStore.selectedTask!.id,
        });
      } else if (TEAM_MESSAGE_COMMANDS.has(firstWord)) {
        // Team shortcuts are front-door messages. The coordinator receives
        // the hint and validates the prompt before routing.
        await agentApi.sendMessage(message);
      } else {
        // Regular message — dispatch to create a new agent loop.
        await agentApi.sendMessage(message);
      }
      input = "";
      chatHandoff.clearArtifact();
    } catch (err) {
      error = err instanceof Error ? err.message : "Failed to send message";
    } finally {
      sending = false;
    }
  }

  function handleKeydown(e: KeyboardEvent) {
    if (e.key === "Enter" && !e.shiftKey) {
      e.preventDefault();
      handleSubmit();
    }
  }
</script>

<div class="chat-bar" data-testid="chat-bar">
  {#if error}
    <div class="chat-error" role="alert" data-testid="chat-error">
      {error}
      <button
        class="error-dismiss"
        type="button"
        onclick={() => (error = null)}
        aria-label="Dismiss error">×</button
      >
    </div>
  {/if}

  {#if taskStore.selectedTask}
    <!-- Just the task as a chip. No "Task:" label (the chip-shape says
         it), no state badge (the user just clicked the card and saw
         the badge there), no slash-hint chrome. The #N ref makes the
         chip a stable handle even if the title gets edited later. -->
    <div class="chat-context" data-testid="chat-context">
      <span class="context-chip" title={taskStore.selectedTask.title}>
        {#if taskStore.selectedTask.shortRef !== null}
          <span class="context-chip-ref"
            >#{taskStore.selectedTask.shortRef}</span
          >
        {/if}
        <span class="context-chip-title">{taskStore.selectedTask.title}</span>
        <button
          class="context-clear"
          type="button"
          onclick={() => taskStore.deselectTask()}
          aria-label="Clear task selection">×</button
        >
      </span>
    </div>
  {/if}

  {#if chatHandoff.artifactContext}
    <div class="artifact-context" data-testid="artifact-context-chip">
      <span class="artifact-context-copy">
        Using artifact:
        <strong>{chatHandoff.artifactContext.title}</strong>
      </span>
      <button
        class="artifact-context-clear"
        type="button"
        onclick={() => chatHandoff.clearArtifact()}
        aria-label="Remove artifact context">×</button
      >
    </div>
  {/if}

  <div class="input-row">
    <input
      class="chat-input"
      type="text"
      bind:value={input}
      bind:this={inputEl}
      {placeholder}
      onkeydown={handleKeydown}
      disabled={sending}
      data-testid="chat-input"
      aria-label="Chat input"
    />
    <button
      class="send-button"
      type="button"
      onclick={handleSubmit}
      disabled={sending || !input.trim()}
      data-testid="send-button"
      aria-label="Send message"
    >
      {sending ? "..." : "Send"}
    </button>
  </div>

  {#if !taskStore.selectedTask && !showingSlash}
    <!-- Team action chips. Only on empty state — when a task is
         selected, those affordances live in the right-rail drilldown.
         The "Approve next" chip is colored and surfaces only when
         tasks are actually waiting on the user. -->
    <div class="action-chips" data-testid="action-chips">
      {#each TEAM_CHIPS as chip (chip.id)}
        <button
          type="button"
          class="action-chip"
          data-testid="action-chip-{chip.id}"
          onclick={() => applyTeamHint(chip.prefix)}
        >
          {chip.label}
        </button>
      {/each}
      {#if taskStore.needsAttentionCount > 0}
        <button
          type="button"
          class="action-chip approve-next"
          data-testid="action-chip-approve-next"
          onclick={approveNext}
          title="Open the first task waiting on you"
        >
          Approve next
          <span class="chip-count">{taskStore.needsAttentionCount}</span>
        </button>
      {/if}
    </div>
  {/if}

  {#if showingSlash}
    <!-- Only show command hints when the user has actually typed "/".
         Task-scoped commands appear only when a task/run is selected. -->
    <div class="slash-hints" aria-hidden="true">
      {#each visibleSlashCommands as cmd (cmd)}
        <span class="hint-chip">{cmd}</span>
      {/each}
    </div>
  {/if}
</div>

<style>
  .chat-bar {
    /* Anchored at the top of the work area (below TopNav). The "ask"
       is the primary verb of this product — pride of place.
       Surface-secondary lifts the bar off the page so it reads as a
       distinct ask zone rather than blending into the workboard. The
       input within stays on surface-primary so it feels recessed. */
    border-bottom: 1px solid var(--ui-border-subtle, #e5e7eb);
    background: var(--ui-surface-secondary, #f7f7f7);
    padding: 0.75rem 1rem 0.875rem;
    flex-shrink: 0;
  }

  .chat-error {
    display: flex;
    align-items: center;
    justify-content: space-between;
    padding: 0.375rem 0.5rem;
    margin-bottom: 0.375rem;
    background: #fef2f2;
    border: 1px solid #fecaca;
    border-radius: 4px;
    font-size: 0.8125rem;
    color: #991b1b;
  }

  .error-dismiss {
    all: unset;
    cursor: pointer;
    padding: 0 0.25rem;
    font-size: 1rem;
    line-height: 1;
    color: #991b1b;
    opacity: 0.6;
  }

  .error-dismiss:hover {
    opacity: 1;
  }

  .chat-context {
    display: flex;
    align-items: center;
    gap: 0.375rem;
    margin-bottom: 0.4rem;
  }

  .artifact-context {
    display: flex;
    align-items: center;
    justify-content: space-between;
    gap: 0.5rem;
    margin-bottom: 0.4rem;
    padding: 0.375rem 0.5rem;
    border: 1px solid var(--ui-border-subtle, #d1d5db);
    border-radius: 4px;
    background: var(--ui-surface-primary, #fff);
    font-size: 0.75rem;
    color: var(--ui-text-secondary, #4b5563);
  }

  .artifact-context-copy {
    min-width: 0;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }

  .artifact-context-copy strong {
    color: var(--ui-text-primary, #111827);
    font-weight: 700;
  }

  .artifact-context-clear {
    all: unset;
    cursor: pointer;
    flex-shrink: 0;
    padding: 0 0.25rem;
    color: var(--ui-text-tertiary, #9ca3af);
    font-size: 1rem;
    line-height: 1;
  }

  .artifact-context-clear:hover {
    color: var(--ui-text-primary, #374151);
  }

  .context-chip {
    display: inline-flex;
    align-items: center;
    gap: 0.4rem;
    max-width: 100%;
    padding: 0.25rem 0.25rem 0.25rem 0.625rem;
    border: 1px solid var(--ui-border-subtle, #d1d5db);
    background: var(--ui-surface-secondary, #f3f4f6);
    border-radius: 9999px;
    font-size: 0.75rem;
    color: var(--ui-text-primary, #111827);
  }

  .context-chip-ref {
    font-weight: 600;
    color: var(--ui-text-tertiary, #6b7280);
    font-variant-numeric: tabular-nums;
    letter-spacing: 0.01em;
  }

  .context-chip-title {
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
    max-width: 320px;
  }

  .context-clear {
    all: unset;
    cursor: pointer;
    width: 1.125rem;
    height: 1.125rem;
    line-height: 1;
    text-align: center;
    border-radius: 9999px;
    color: var(--ui-text-secondary, #9ca3af);
    font-size: 0.8125rem;
  }

  .context-clear:hover {
    background: var(--ui-surface-primary, #e5e7eb);
    color: var(--ui-text-primary, #374151);
  }

  .input-row {
    display: flex;
    gap: 0.5rem;
  }

  .chat-input {
    flex: 1;
    padding: 0.5rem 0.75rem;
    border: 1px solid var(--ui-border-subtle, #d1d5db);
    border-radius: 6px;
    font-size: 0.875rem;
    color: var(--ui-text-primary, #111827);
    background: var(--ui-surface-primary, #fff);
    outline: none;
    transition: border-color 0.15s;
  }

  .chat-input:focus {
    border-color: var(--ui-interactive-primary, #3b82f6);
    box-shadow: 0 0 0 1px var(--ui-interactive-primary, #3b82f6);
  }

  .chat-input::placeholder {
    color: var(--ui-text-secondary, #9ca3af);
  }

  .chat-input:disabled {
    opacity: 0.6;
  }

  .send-button {
    padding: 0.5rem 1rem;
    border: none;
    border-radius: 6px;
    background: var(--ui-interactive-primary, #3b82f6);
    color: white;
    font-size: 0.8125rem;
    font-weight: 600;
    cursor: pointer;
    transition: background 0.15s;
    white-space: nowrap;
  }

  .send-button:hover:not(:disabled) {
    background: var(--ui-interactive-primary-hover, #2563eb);
  }

  .send-button:disabled {
    opacity: 0.5;
    cursor: not-allowed;
  }

  .slash-hints {
    display: flex;
    gap: 0.25rem;
    padding-top: 0.375rem;
    flex-wrap: wrap;
  }

  .hint-chip {
    font-size: 0.6875rem;
    color: var(--ui-text-secondary, #9ca3af);
    padding: 0.0625rem 0.375rem;
    background: var(--ui-surface-secondary, #f9fafb);
    border-radius: 3px;
  }

  .action-chips {
    display: flex;
    gap: 0.4375rem;
    padding-top: 0.5rem;
    flex-wrap: wrap;
  }

  .action-chip {
    all: unset;
    cursor: pointer;
    display: inline-flex;
    align-items: center;
    gap: 0.375rem;
    padding: 0.25rem 0.625rem;
    border: 1px solid var(--ui-border-subtle, #d1d5db);
    background: var(--ui-surface-primary, #fff);
    border-radius: 9999px;
    font-size: 0.75rem;
    font-weight: 500;
    color: var(--ui-text-secondary, #6b7280);
    transition:
      border-color 0.15s,
      color 0.15s,
      background 0.15s;
  }

  .action-chip:hover {
    border-color: var(--ui-interactive-primary, #3b82f6);
    color: var(--ui-text-primary, #111827);
  }

  .action-chip:focus-visible {
    outline: 2px solid var(--ui-interactive-primary, #3b82f6);
    outline-offset: 2px;
  }

  /* Approve-next pops in the same orange family as the needs-you
     pill in TopNav so the eye recognizes them as the same concept. */
  .action-chip.approve-next {
    border-color: var(--col-needs-you, #f97316);
    color: var(--col-needs-you, #f97316);
    font-weight: 600;
  }

  .action-chip.approve-next:hover {
    background: var(--col-needs-you, #f97316);
    color: white;
  }

  .chip-count {
    font-variant-numeric: tabular-nums;
    background: var(--col-needs-you, #f97316);
    color: white;
    padding: 0 0.4rem;
    border-radius: 9999px;
    font-size: 0.6875rem;
    font-weight: 700;
    min-width: 1rem;
    text-align: center;
  }

  .action-chip.approve-next:hover .chip-count {
    background: white;
    color: var(--col-needs-you, #f97316);
  }
</style>
