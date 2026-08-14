import { describe, it, expect, vi, beforeEach } from "vitest";
import { render, screen, waitFor, within } from "@testing-library/svelte";
import userEvent from "@testing-library/user-event";
import TaskStory from "./TaskStory.svelte";
import { agentApi } from "$lib/services/agentApi";
import type { LoopTrajectory, TrajectoryFact } from "$lib/types/agent";

vi.mock("$lib/services/agentApi", () => ({
  agentApi: {
    getLoopTrajectory: vi.fn(),
    getTrajectory: vi.fn(),
  },
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
