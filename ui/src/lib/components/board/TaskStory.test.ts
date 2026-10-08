import { describe, it, expect, vi, beforeEach } from "vitest";
import { render, screen, waitFor, within } from "@testing-library/svelte";
import userEvent from "@testing-library/user-event";
import TaskStory from "./TaskStory.svelte";
import { agentApi } from "$lib/services/agentApi";
import { getTriples } from "$lib/services/runStatusApi";
import type { RawTriple } from "$lib/services/runStatusApi";
import type { AgentLoop, LoopTrajectory, TrajectoryFact } from "$lib/types/agent";
import type { TaskControl } from "$lib/types/control";

vi.mock("$lib/services/agentApi", () => ({
  agentApi: {
    getLoopTrajectory: vi.fn(),
    getTrajectory: vi.fn(),
  },
}));

// Controls are explained from two on-demand graph reads (design D3).
vi.mock("$lib/services/runStatusApi", () => ({
  getTriples: vi.fn(),
}));

function trajectory(
  facts: TrajectoryFact[],
  overrides: Partial<LoopTrajectory> = {},
): LoopTrajectory {
  return {
    schema_version: "v1",
    loop_id: "loop-1",
    coverage: "observed",
    terminal_observed: false,
    observed_totals: {
      facts: facts.length,
      tokens_in: 0,
      tokens_out: 0,
      elapsed_ms: 0,
      message_count: 0,
      tool_count: 0,
      url_count: 0,
      model_requests: 0,
      model_completions: 0,
      tool_requests: 0,
      tool_completions: 0,
      context_compactions: 0,
      terminal_observations: 0,
      requested_observations: 0,
      completed_observations: 0,
      failed_observations: 0,
      cancelled_observations: 0,
    },
    facts,
    ...overrides,
  };
}

function toolFact(overrides: Partial<TrajectoryFact> = {}): TrajectoryFact {
  return {
    kind: "tool.completed",
    causal_iteration: 1,
    causal_phase: "tool_result",
    causal_ordinal: 0,
    status: "completed",
    capability_preview: "coordinator",
    ...overrides,
  };
}

function modelFact(overrides: Partial<TrajectoryFact> = {}): TrajectoryFact {
  return {
    kind: "model.completed",
    causal_iteration: 1,
    causal_phase: "model_result",
    causal_ordinal: 0,
    status: "completed",
    capability_preview: "researcher",
    ...overrides,
  };
}

beforeEach(() => {
  vi.mocked(agentApi.getLoopTrajectory).mockReset();
  vi.mocked(getTriples).mockReset();
  vi.mocked(getTriples).mockResolvedValue([]);
});

describe("TaskStory — tool call narrative (no verdict content)", () => {
  it("renders a decide tool call generically — no action/reason content available", async () => {
    // beta.160: the trajectory fact log carries only tool_preview (the
    // tool name), never tool_arguments — so `decide` gets no special
    // verdict-chip treatment any more. This is a documented product
    // regression (see TaskStory.svelte's header comment).
    vi.mocked(agentApi.getLoopTrajectory).mockResolvedValue(
      trajectory([
        toolFact({ tool_preview: "decide", capability_preview: "coordinator" }),
      ]),
    );

    render(TaskStory, { props: { loopId: "loop-1" } });

    const step = await screen.findByTestId("story-step");
    expect(step).toHaveTextContent("Coordinator used decide");
    expect(screen.queryByTestId("story-verdict")).toBeNull();
  });

  it("renders a failed tool call with the failed headline and error preview", async () => {
    vi.mocked(agentApi.getLoopTrajectory).mockResolvedValue(
      trajectory([
        toolFact({
          tool_preview: "bash",
          status: "failed",
          error_category: "tool",
          capability_preview: "researcher",
        }),
      ]),
    );

    render(TaskStory, { props: { loopId: "loop-1" } });

    const step = await screen.findByTestId("story-step");
    expect(step).toHaveTextContent("Researcher tried bash — failed");
    expect(step).toHaveTextContent("error: tool");
  });

  it("expanding a step shows the fact's metadata as JSON, not fabricated content", async () => {
    vi.mocked(agentApi.getLoopTrajectory).mockResolvedValue(
      trajectory([
        toolFact({ tool_preview: "bash", attempt_id: "attempt-1" }),
      ]),
    );

    render(TaskStory, { props: { loopId: "loop-1" } });

    const step = await screen.findByTestId("story-step");
    await userEvent.click(within(step).getByRole("button"));

    const payload = await screen.findByTestId("story-step-payload");
    expect(payload.textContent).toContain('"tool_preview": "bash"');
    expect(payload.textContent).toContain('"attempt_id": "attempt-1"');
  });
});

describe("TaskStory — model call narrative", () => {
  it("renders 'replied' when the model call issued no tool calls", async () => {
    vi.mocked(agentApi.getLoopTrajectory).mockResolvedValue(
      trajectory([
        modelFact({
          tool_count: 0,
          model_preview: "gemini-2.5-flash",
          provider_preview: "google",
        }),
      ]),
    );

    render(TaskStory, { props: { loopId: "loop-1" } });

    const step = await screen.findByTestId("story-step");
    expect(step).toHaveTextContent("Researcher replied");
    expect(step).toHaveTextContent("gemini-2.5-flash · google");
  });

  it("renders 'reasoned' when the model call issued tool calls", async () => {
    vi.mocked(agentApi.getLoopTrajectory).mockResolvedValue(
      trajectory([modelFact({ tool_count: 2 })]),
    );

    render(TaskStory, { props: { loopId: "loop-1" } });

    const step = await screen.findByTestId("story-step");
    expect(step).toHaveTextContent("Researcher reasoned");
  });

  it("renders a failed model call distinctly", async () => {
    vi.mocked(agentApi.getLoopTrajectory).mockResolvedValue(
      trajectory([modelFact({ status: "failed", error_category: "model" })]),
    );

    render(TaskStory, { props: { loopId: "loop-1" } });

    const step = await screen.findByTestId("story-step");
    expect(step).toHaveTextContent("Researcher model call failed");
  });

  it("shows duration and token meta on a step", async () => {
    vi.mocked(agentApi.getLoopTrajectory).mockResolvedValue(
      trajectory([modelFact({ elapsed_ms: 842, tokens_in: 120, tokens_out: 40 })]),
    );

    render(TaskStory, { props: { loopId: "loop-1" } });

    const step = await screen.findByTestId("story-step");
    expect(step).toHaveTextContent("842ms");
    expect(step).toHaveTextContent("160 tokens");
  });
});

describe("TaskStory — context compaction", () => {
  it("renders a context-compacted marker", async () => {
    vi.mocked(agentApi.getLoopTrajectory).mockResolvedValue(
      trajectory([
        {
          kind: "context.compacted",
          causal_iteration: 2,
          causal_phase: "compaction",
          causal_ordinal: 0,
        },
      ]),
    );

    render(TaskStory, { props: { loopId: "loop-1" } });

    const step = await screen.findByTestId("story-step");
    expect(step).toHaveTextContent("Context compacted");
  });
});

describe("TaskStory — facts that don't get a narrative row", () => {
  it("does not render loop.started, *.requested, or loop.terminal as list rows", async () => {
    vi.mocked(agentApi.getLoopTrajectory).mockResolvedValue(
      trajectory(
        [
          {
            kind: "loop.started",
            causal_iteration: 0,
            causal_phase: "loop_start",
            causal_ordinal: 0,
            capability_preview: "coordinator",
          },
          {
            kind: "model.requested",
            causal_iteration: 1,
            causal_phase: "model_request",
            causal_ordinal: 0,
          },
          toolFact({ tool_preview: "bash" }),
          {
            kind: "loop.terminal",
            causal_iteration: 1,
            causal_phase: "terminal",
            causal_ordinal: 0,
            status: "completed",
            elapsed_ms: 900,
          },
        ],
        { terminal_observed: true },
      ),
    );

    render(TaskStory, { props: { loopId: "loop-1" } });

    await waitFor(() => {
      expect(screen.getAllByTestId("story-step")).toHaveLength(1);
    });
    expect(screen.getByTestId("story-step")).toHaveTextContent("used bash");
  });
});

describe("TaskStory — empty and error states", () => {
  it("shows 'No steps recorded yet' when there are no narrative facts", async () => {
    vi.mocked(agentApi.getLoopTrajectory).mockResolvedValue(trajectory([]));

    render(TaskStory, { props: { loopId: "loop-1" } });

    await waitFor(() =>
      expect(screen.getByText("No steps recorded yet.")).toBeInTheDocument(),
    );
  });

  it("shows an error banner when the fetch genuinely fails", async () => {
    vi.mocked(agentApi.getLoopTrajectory).mockRejectedValue(
      new Error("Network error"),
    );

    render(TaskStory, { props: { loopId: "loop-1" } });

    const error = await screen.findByTestId("story-error");
    expect(error).toHaveTextContent("Network error");
  });
});

describe("TaskStory — closing banner", () => {
  it("renders the outcome, duration, and total tokens once the loop.terminal fact is observed", async () => {
    vi.mocked(agentApi.getLoopTrajectory).mockResolvedValue(
      trajectory(
        [
          toolFact({ tool_preview: "bash" }),
          {
            kind: "loop.terminal",
            causal_iteration: 3,
            causal_phase: "terminal",
            causal_ordinal: 0,
            status: "completed",
            elapsed_ms: 5230,
          },
        ],
        {
          terminal_observed: true,
          observed_totals: {
            facts: 2,
            tokens_in: 300,
            tokens_out: 100,
            elapsed_ms: 5230,
            message_count: 0,
            tool_count: 1,
            url_count: 0,
            model_requests: 0,
            model_completions: 0,
            tool_requests: 1,
            tool_completions: 1,
            context_compactions: 0,
            terminal_observations: 1,
            requested_observations: 0,
            completed_observations: 2,
            failed_observations: 0,
            cancelled_observations: 0,
          },
        },
      ),
    );

    render(TaskStory, { props: { loopId: "loop-1" } });

    const done = await screen.findByTestId("story-line-done");
    expect(done).toHaveTextContent("Done");
    expect(done).toHaveTextContent("5.23s");
    expect(done).toHaveTextContent("400 tokens");
  });

  it("does not render the closing banner before the terminal fact is observed", async () => {
    vi.mocked(agentApi.getLoopTrajectory).mockResolvedValue(
      trajectory([toolFact({ tool_preview: "bash" })]),
    );

    render(TaskStory, { props: { loopId: "loop-1" } });

    await screen.findByTestId("story-step");
    expect(screen.queryByTestId("story-line-done")).toBeNull();
  });

  it("labels a failed terminal outcome", async () => {
    vi.mocked(agentApi.getLoopTrajectory).mockResolvedValue(
      trajectory(
        [
          {
            kind: "loop.terminal",
            causal_iteration: 1,
            causal_phase: "terminal",
            causal_ordinal: 0,
            status: "failed",
            elapsed_ms: 100,
          },
        ],
        { terminal_observed: true },
      ),
    );

    render(TaskStory, { props: { loopId: "loop-1" } });

    const done = await screen.findByTestId("story-line-done");
    expect(done).toHaveTextContent("Failed");
  });
});

describe("TaskStory — 'You asked' line", () => {
  it("renders the prompt when provided", async () => {
    vi.mocked(agentApi.getLoopTrajectory).mockResolvedValue(trajectory([]));

    render(TaskStory, { props: { loopId: "loop-1", prompt: "compare mqtt vs nats" } });

    const asked = await screen.findByTestId("story-line-asked");
    expect(asked).toHaveTextContent("compare mqtt vs nats");
  });

  it("does not render the line when prompt is absent", async () => {
    vi.mocked(agentApi.getLoopTrajectory).mockResolvedValue(trajectory([]));

    render(TaskStory, { props: { loopId: "loop-1" } });

    await waitFor(() =>
      expect(screen.getByTestId("task-story")).toBeInTheDocument(),
    );
    expect(screen.queryByTestId("story-line-asked")).toBeNull();
  });
});


// ---------------------------------------------------------------------------
// Controls group (design D3). Fixtures mirror a measured run: the ops observer
// spawned at ...893Z, after the run's `completed` transition at ...846Z.
// ---------------------------------------------------------------------------

const PREFIX = "c360.semteams-bootstrap-e2e-6c50f3";
const RUN = "b5a2d6ec-f67d-4cc7-ab5c-b40e8223a43e";
const OBSERVER = "8b9f1444-166f-4da3-a1b5-2718995c4b23";
const RUN_ENTITY = `${PREFIX}.chain.agent.execution.${RUN}`;
const OBSERVER_ENTITY = `${PREFIX}.agentic-loop.agent.execution.${OBSERVER}`;

function makeControl(
  state: AgentLoop["state"] = "complete",
  overrides: Partial<TaskControl> = {},
): TaskControl {
  return {
    loopId: OBSERVER,
    loopEntityId: OBSERVER_ENTITY,
    firingEntityId: RUN_ENTITY,
    runInstance: RUN,
    spawnedAt: new Date(1791480613893),
    spawnedAtNanos: 1791480613893413505n,
    loop: {
      loop_id: OBSERVER,
      task_id: "",
      state,
      role: "ops-chain-observer",
      iterations: 2,
      max_iterations: 5,
      user_id: "",
      channel_type: "",
      parent_loop_id: "",
      outcome: "",
      error: "",
    },
    ...overrides,
  };
}

function transition(at: string, from: string, to: string, source: string, note: string): RawTriple[] {
  const base = { subject: RUN_ENTITY, timestamp: at };
  return [
    { ...base, predicate: "lifecycle.transition.at", object: at },
    { ...base, predicate: "lifecycle.transition.from", object: from },
    { ...base, predicate: "lifecycle.transition.to", object: to },
    { ...base, predicate: "lifecycle.transition.source", object: source },
    { ...base, predicate: "lifecycle.transition.note", object: note },
  ];
}

const RUN_HISTORY: RawTriple[] = [
  ...transition("2026-10-08T17:30:13.516406629Z", "", "dispatched", "framework", "created"),
  ...transition("2026-10-08T17:30:13.629098755Z", "dispatched", "executing", "rule", "run is now executing"),
  ...transition(
    "2026-10-08T17:30:13.846566421Z",
    "executing",
    "completed",
    "rule",
    "chain reached its success terminal (reviewer/CBG approved) — run completed",
  ),
];

function loopTriple(predicate: string, object: string): RawTriple {
  return { subject: OBSERVER_ENTITY, predicate, object };
}

const FINISHED_OBSERVER: RawTriple[] = [
  loopTriple("agent.loop.description", "A run has reached a terminal phase. Diagnose the chain.\n\n**Run entity:** x"),
  loopTriple("coordinator.decision.next-action", "observed"),
  loopTriple("coordinator.decision.reason", "Emitted two findings at 0.72 confidence."),
  loopTriple("agent.loop.outcome", "success"),
];

function serveGraph(opts: { run?: RawTriple[]; loop?: RawTriple[] } = {}) {
  vi.mocked(getTriples).mockImplementation((params) => {
    if (params.subject === RUN_ENTITY) return Promise.resolve(opts.run ?? RUN_HISTORY);
    if (params.subject === OBSERVER_ENTITY) return Promise.resolve(opts.loop ?? FINISHED_OBSERVER);
    return Promise.resolve([]);
  });
}

describe("TaskStory — Controls group", () => {
  beforeEach(() => {
    vi.mocked(agentApi.getLoopTrajectory).mockResolvedValue(trajectory([]));
  });

  it("renders one explained row per control with identity, rule, fact and outcome", async () => {
    serveGraph();
    render(TaskStory, { props: { loopId: RUN, controls: [makeControl("complete")] } });

    const row = await screen.findByTestId("control-event");
    expect(row).toHaveAttribute("data-loop", OBSERVER);

    // The rule is unknown and the reason is on the page, not in a tooltip.
    const rule = within(row).getByTestId("control-rule");
    expect(rule).toHaveTextContent("rule: unknown");
    expect(rule).toHaveTextContent("rule identity is not recorded by the frozen runtime (#298)");
    expect(rule).toBeVisible();

    const identity = within(row).getByTestId("control-identity");
    expect(identity.tagName).toBe("BUTTON");
    expect(identity).toHaveTextContent("ops-chain-observer");
    expect(identity).toHaveTextContent(OBSERVER);

    expect(within(row).getByTestId("control-firing")).toHaveTextContent(RUN_ENTITY);

    await waitFor(() =>
      expect(within(row).getByTestId("control-fact")).toHaveTextContent("agent.run.phase → completed"),
    );
    const fact = within(row).getByTestId("control-fact");
    expect(fact).toHaveTextContent("source rule");
    expect(fact).toHaveTextContent("chain reached its success terminal");

    const outcome = within(row).getByTestId("control-outcome");
    expect(outcome).toHaveTextContent("applied");
    expect(outcome).toHaveTextContent("decide(observed)");
    expect(outcome).toHaveTextContent("Emitted two findings at 0.72 confidence.");
    // Only the first line of the prompt is shown as the control's description.
    expect(row).toHaveTextContent("A run has reached a terminal phase. Diagnose the chain.");
    expect(row).not.toHaveTextContent("**Run entity:**");
  });

  it("reads the firing run and the control loop, exactly once each", async () => {
    serveGraph();
    render(TaskStory, { props: { loopId: RUN, controls: [makeControl()] } });

    await waitFor(() =>
      expect(screen.getByTestId("control-fact")).toHaveTextContent("agent.run.phase"),
    );
    const subjects = vi.mocked(getTriples).mock.calls.map(([p]) => p.subject).sort();
    expect(subjects).toEqual([OBSERVER_ENTITY, RUN_ENTITY].sort());
  });

  it("labels a failed control loop rejected", async () => {
    serveGraph({ loop: [loopTriple("agent.loop.outcome", "failed")] });
    render(TaskStory, { props: { loopId: RUN, controls: [makeControl("failed")] } });

    expect(await screen.findByTestId("control-outcome")).toHaveTextContent("rejected");
  });

  it("labels a running control accepted and says its decide is not recorded yet", async () => {
    serveGraph({ loop: [] });
    render(TaskStory, { props: { loopId: RUN, controls: [makeControl("executing")] } });

    await waitFor(() =>
      expect(screen.getByTestId("control-outcome")).toHaveTextContent("decide: unknown (not recorded yet)"),
    );
    const outcome = screen.getByTestId("control-outcome");
    expect(outcome).toHaveTextContent("accepted");
    expect(outcome).toHaveTextContent("loop state: executing");
  });

  it("renders the fact as unknown when the spawn precedes every recorded transition", async () => {
    serveGraph({
      run: transition("2026-10-08T17:30:14.000000000Z", "", "dispatched", "framework", "created"),
    });
    render(TaskStory, { props: { loopId: RUN, controls: [makeControl()] } });

    await waitFor(() =>
      expect(screen.getByTestId("control-fact")).toHaveTextContent(
        "unknown (no lifecycle transition precedes the spawn)",
      ),
    );
  });

  it("renders fields from a truncated read as unknown: read truncated", async () => {
    const filler = Array.from({ length: 200 }, (_, i): RawTriple => ({
      subject: RUN_ENTITY,
      predicate: "lifecycle.transition.note",
      object: `n${i}`,
      timestamp: `2026-10-08T17:30:13.${String(100000000 + i)}Z`,
    }));
    serveGraph({ run: filler });
    render(TaskStory, { props: { loopId: RUN, controls: [makeControl()] } });

    await waitFor(() =>
      expect(screen.getByTestId("control-fact")).toHaveTextContent("unknown (read truncated)"),
    );
    // The loop read was complete, so its fields still render.
    expect(screen.getByTestId("control-outcome")).toHaveTextContent("decide(observed)");
  });

  it("says the control list may be incomplete when the control read was truncated", async () => {
    serveGraph();
    render(TaskStory, {
      props: { loopId: RUN, controls: [makeControl()], controlsTruncated: true },
    });

    expect(await screen.findByTestId("controls-truncated")).toHaveTextContent("may be incomplete");
  });

  it("does not claim the list is incomplete when it is not", async () => {
    serveGraph();
    render(TaskStory, { props: { loopId: RUN, controls: [makeControl()] } });

    await screen.findByTestId("control-event");
    expect(screen.queryByTestId("controls-truncated")).toBeNull();
  });

  it("degrades to unknown, not to a blank or a crash, when a graph read fails", async () => {
    vi.mocked(getTriples).mockRejectedValue(new Error("HTTP 503"));
    render(TaskStory, { props: { loopId: RUN, controls: [makeControl()] } });

    await waitFor(() =>
      expect(screen.getByTestId("control-fact")).toHaveTextContent("unknown (read failed: HTTP 503)"),
    );
    expect(screen.getByTestId("control-rule")).toHaveTextContent("rule: unknown");
  });

  it("focuses the control's own trajectory from the keyboard", async () => {
    serveGraph();
    const onFocusLoop = vi.fn();
    const user = userEvent.setup();
    render(TaskStory, { props: { loopId: RUN, controls: [makeControl()], onFocusLoop } });

    const identity = await screen.findByTestId("control-identity");
    expect(identity).toHaveAccessibleName(/ops-chain-observer/);
    await user.tab(); // first tab stop in the story is the identity button (no steps rendered)
    expect(identity).toHaveFocus();
    await user.keyboard("{Enter}");
    expect(onFocusLoop).toHaveBeenCalledWith(OBSERVER);
    await user.keyboard(" ");
    expect(onFocusLoop).toHaveBeenCalledTimes(2);
  });

  it("renders no Controls group when the task has no controls", async () => {
    render(TaskStory, { props: { loopId: RUN } });

    await waitFor(() => expect(screen.getByTestId("task-story")).toBeInTheDocument());
    expect(screen.queryByTestId("story-controls")).toBeNull();
    expect(getTriples).not.toHaveBeenCalled();
  });

  it("re-reads when a control's loop changes state, not when the array is merely re-created", async () => {
    serveGraph({ loop: [] });
    const { rerender } = render(TaskStory, {
      props: { loopId: RUN, controls: [makeControl("executing")] },
    });
    await waitFor(() =>
      expect(screen.getByTestId("control-outcome")).toHaveTextContent("not recorded yet"),
    );
    const afterFirstRead = vi.mocked(getTriples).mock.calls.length;
    expect(afterFirstRead).toBe(2);

    // taskStore re-derives on every loop event and hands over a new array.
    await rerender({ loopId: RUN, controls: [makeControl("executing")] });
    expect(vi.mocked(getTriples).mock.calls.length).toBe(afterFirstRead);

    // The terminal decide lands when the loop ends: that is the moment to re-read.
    serveGraph({ loop: FINISHED_OBSERVER });
    await rerender({ loopId: RUN, controls: [makeControl("complete")] });
    await waitFor(() =>
      expect(screen.getByTestId("control-outcome")).toHaveTextContent("decide(observed)"),
    );
    expect(vi.mocked(getTriples).mock.calls.length).toBe(afterFirstRead + 2);
  });
});
