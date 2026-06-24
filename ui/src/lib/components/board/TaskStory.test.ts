import { describe, it, expect, vi, beforeEach } from "vitest";
import { render, screen, waitFor, within } from "@testing-library/svelte";
import userEvent from "@testing-library/user-event";
import TaskStory from "./TaskStory.svelte";
import { agentApi } from "$lib/services/agentApi";
import type { LoopTrajectory } from "$lib/types/agent";

vi.mock("$lib/services/agentApi", () => ({
  agentApi: {
    getLoopTrajectory: vi.fn(),
    getTrajectory: vi.fn().mockResolvedValue([]),
  },
}));

function trajectory(steps: LoopTrajectory["steps"]): LoopTrajectory {
  return {
    loop_id: "loop-1",
    start_time: "2026-06-06T00:00:00Z",
    steps,
  };
}

beforeEach(() => {
  vi.mocked(agentApi.getLoopTrajectory).mockReset();
});

describe("TaskStory — decide verdict surfacing", () => {
  it("renders a decide step as a verdict chip + visible reason", async () => {
    vi.mocked(agentApi.getLoopTrajectory).mockResolvedValue(
      trajectory([
        {
          step_type: "tool_call",
          timestamp: "2026-06-06T00:00:01Z",
          tool_name: "decide",
          tool_arguments: {
            action: "propose",
            reason: "Baseline captured at 1.20s. Beginning the iteration loop.",
          },
          capability: "coordinator",
        },
      ]),
    );

    render(TaskStory, { props: { loopId: "loop-1" } });

    const verdict = await screen.findByTestId("story-verdict");
    expect(verdict).toHaveAttribute("data-verdict-tone", "route");
    expect(within(verdict).getByTestId("verdict-chip").textContent).toBe(
      "Propose",
    );
    expect(within(verdict).getByTestId("verdict-reason").textContent).toContain(
      "Baseline captured at 1.20s",
    );
    // The reason is always visible — not behind an expand toggle.
    expect(within(verdict).queryByRole("button")).toBeNull();
  });

  it("tones an approval verdict green (approve)", async () => {
    vi.mocked(agentApi.getLoopTrajectory).mockResolvedValue(
      trajectory([
        {
          step_type: "tool_call",
          timestamp: "2026-06-06T00:00:01Z",
          tool_name: "decide",
          tool_arguments: { action: "approved", reason: "Checklist met." },
          capability: "reviewer",
        },
      ]),
    );

    render(TaskStory, { props: { loopId: "loop-1" } });

    const verdict = await screen.findByTestId("story-verdict");
    expect(verdict).toHaveAttribute("data-verdict-tone", "approve");
    expect(within(verdict).getByTestId("verdict-chip").textContent).toBe(
      "Approved",
    );
  });

  it("tones a rejection verdict red (reject)", async () => {
    vi.mocked(agentApi.getLoopTrajectory).mockResolvedValue(
      trajectory([
        {
          step_type: "tool_call",
          timestamp: "2026-06-06T00:00:01Z",
          tool_name: "decide",
          tool_arguments: { action: "rejected", reason: "Missing evidence." },
          capability: "reviewer",
        },
      ]),
    );

    render(TaskStory, { props: { loopId: "loop-1" } });

    const verdict = await screen.findByTestId("story-verdict");
    expect(verdict).toHaveAttribute("data-verdict-tone", "reject");
  });

  it("surfaces CBG approved as the final gate", async () => {
    vi.mocked(agentApi.getLoopTrajectory).mockResolvedValue(
      trajectory([
        {
          step_type: "tool_call",
          timestamp: "2026-06-06T00:00:01Z",
          tool_name: "decide",
          tool_arguments: {
            action: "approved",
            reason:
              "Integration gate `task test` passed and diff stayed in scope.",
          },
          capability: "reviewer-dev-via-test",
        },
      ]),
    );

    render(TaskStory, { props: { loopId: "loop-1" } });

    const verdict = await screen.findByTestId("story-verdict");
    expect(verdict).toHaveAttribute("data-gate", "cbg-final");
    expect(verdict).toHaveAttribute("data-verdict-tone", "approve");
    expect(
      within(verdict).getByTestId("cbg-final-gate-label"),
    ).toHaveTextContent("Final Review Gate");
    expect(
      within(verdict).getByText("Final Review Gate passed"),
    ).toBeInTheDocument();
    expect(within(verdict).getByTestId("verdict-reason")).toHaveTextContent(
      "Integration gate",
    );
  });

  it("surfaces CBG rejected retry with target task and visible evidence", async () => {
    vi.mocked(agentApi.getLoopTrajectory).mockResolvedValue(
      trajectory([
        {
          step_type: "tool_call",
          timestamp: "2026-06-06T00:00:01Z",
          tool_name: "decide",
          tool_arguments: {
            action: "rejected_retry",
            subtopics: ["task-2"],
            reason:
              "Diff violates target scope; move the parser fix back under `mavlink/`.",
          },
          capability: "reviewer-dev-via-test",
        },
      ]),
    );

    render(TaskStory, { props: { loopId: "loop-1" } });

    const verdict = await screen.findByTestId("story-verdict");
    expect(verdict).toHaveAttribute("data-gate", "cbg-final");
    expect(verdict).toHaveAttribute("data-verdict-tone", "reject");
    expect(within(verdict).getByTestId("verdict-chip")).toHaveTextContent(
      "Rejected retry",
    );
    expect(
      within(verdict).getByText("Final Review Gate requested retry"),
    ).toBeInTheDocument();
    expect(within(verdict).getByTestId("cbg-target-task")).toHaveTextContent(
      "Target task-2",
    );
    expect(within(verdict).getByTestId("verdict-reason")).toHaveTextContent(
      "Diff violates target scope",
    );
  });

  it("renders markdown inside a verdict reason", async () => {
    vi.mocked(agentApi.getLoopTrajectory).mockResolvedValue(
      trajectory([
        {
          step_type: "tool_call",
          timestamp: "2026-06-06T00:00:01Z",
          tool_name: "decide",
          tool_arguments: {
            action: "respond_direct",
            reason: "Optimized `go test` by **29%**.",
          },
          capability: "coordinator",
        },
      ]),
    );

    render(TaskStory, { props: { loopId: "loop-1" } });

    const reason = await screen.findByTestId("verdict-reason");
    expect(reason.querySelector("code")?.textContent).toBe("go test");
    expect(reason.querySelector("strong")?.textContent).toBe("29%");
  });

  it("does not render an empty reason block when reason is absent", async () => {
    vi.mocked(agentApi.getLoopTrajectory).mockResolvedValue(
      trajectory([
        {
          step_type: "tool_call",
          timestamp: "2026-06-06T00:00:01Z",
          tool_name: "decide",
          tool_arguments: { action: "gather" },
          capability: "coordinator",
        },
      ]),
    );

    render(TaskStory, { props: { loopId: "loop-1" } });

    await screen.findByTestId("story-verdict");
    expect(screen.queryByTestId("verdict-reason")).toBeNull();
  });
});

describe("TaskStory — model prose markdown", () => {
  it("renders an expanded model response as markdown", async () => {
    vi.mocked(agentApi.getLoopTrajectory).mockResolvedValue(
      trajectory([
        {
          step_type: "model_call",
          timestamp: "2026-06-06T00:00:01Z",
          request_id: "req-1",
          response: "## Summary\n\nDone with `bash` and a **kept** change.",
          capability: "researcher",
        },
      ]),
    );

    render(TaskStory, { props: { loopId: "loop-1" } });

    // Expand the step to reveal its payload.
    const step = await screen.findByTestId("story-step");
    await userEvent.click(within(step).getByRole("button"));

    const payload = await screen.findByTestId("story-step-payload");
    expect(payload.querySelector("h4.md-h")?.textContent).toBe("Summary");
    expect(payload.querySelector("code")?.textContent).toBe("bash");
    expect(payload.querySelector("strong")?.textContent).toBe("kept");
  });

  it("escapes HTML in a model response (no raw markup)", async () => {
    vi.mocked(agentApi.getLoopTrajectory).mockResolvedValue(
      trajectory([
        {
          step_type: "model_call",
          timestamp: "2026-06-06T00:00:01Z",
          request_id: "req-1",
          response: "<script>alert(1)</script>",
          capability: "researcher",
        },
      ]),
    );

    render(TaskStory, { props: { loopId: "loop-1" } });

    const step = await screen.findByTestId("story-step");
    await userEvent.click(within(step).getByRole("button"));

    const payload = await screen.findByTestId("story-step-payload");
    expect(payload.querySelector("script")).toBeNull();
    expect(payload.textContent).toContain("<script>alert(1)</script>");
  });
});

describe("TaskStory — non-decide tool calls keep generic rendering", () => {
  it("renders a non-decide tool call as an expandable step, not a verdict", async () => {
    vi.mocked(agentApi.getLoopTrajectory).mockResolvedValue(
      trajectory([
        {
          step_type: "tool_call",
          timestamp: "2026-06-06T00:00:01Z",
          tool_name: "bash",
          tool_arguments: { command: "go test ./..." },
          tool_status: "success",
          capability: "researcher",
        },
      ]),
    );

    render(TaskStory, { props: { loopId: "loop-1" } });

    await waitFor(() =>
      expect(screen.getByTestId("story-step")).toBeInTheDocument(),
    );
    expect(screen.queryByTestId("story-verdict")).toBeNull();
  });
});

describe("TaskStory — proof-readiness artifacts", () => {
  it("renders analyze_proof_readiness as structured proof cards", async () => {
    vi.mocked(agentApi.getLoopTrajectory).mockResolvedValue(
      trajectory([
        {
          step_type: "tool_call",
          timestamp: "2026-06-06T00:00:01Z",
          tool_name: "analyze_proof_readiness",
          tool_arguments: {},
          tool_result: JSON.stringify({
            status: "failed",
            finding_count: 1,
            findings: [
              {
                kind: "missing_proof_dependency",
                route: "test_harness",
                dependency: "px4_sitl.boots",
                reason: "required proof dependency is not ready",
              },
            ],
            proof_facts: {
              dependencies: [
                {
                  id: "px4_sitl.boots",
                  status: "missing",
                  description: "PX4 SITL boots headlessly",
                },
              ],
              readiness: [],
              evidence: [],
              waivers: [],
            },
          }),
          tool_status: "success",
          capability: "coordinator",
        },
      ]),
    );

    render(TaskStory, { props: { loopId: "loop-1" } });

    const step = await screen.findByTestId("story-step");
    await userEvent.click(within(step).getByRole("button"));

    expect(
      await screen.findByTestId("proof-readiness-card"),
    ).toBeInTheDocument();
    expect(screen.getByTestId("proof-dependencies-card")).toHaveTextContent(
      "px4_sitl.boots",
    );
    expect(screen.getByTestId("proof-dependencies-card")).toHaveTextContent(
      "PX4 SITL boots headlessly",
    );
  });
});
