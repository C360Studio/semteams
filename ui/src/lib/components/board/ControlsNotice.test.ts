import { describe, it, expect, vi, beforeEach, afterEach } from "vitest";
import { render, screen } from "@testing-library/svelte";
import ControlsNotice from "./ControlsNotice.svelte";
import { controlsStore, RETRY_BASE_MS } from "$lib/stores/controlsStore.svelte";
import { agentStore } from "$lib/stores/agentStore.svelte";
import { getTriples } from "$lib/services/runStatusApi";
import type { AgentLoop } from "$lib/types/agent";

vi.mock("$lib/services/runStatusApi");
const mockGetTriples = vi.mocked(getTriples);

const PREFIX = "c360.semteams-bootstrap-e2e-6c50f3";

function candidate(loopId: string): AgentLoop {
  return {
    loop_id: loopId,
    task_id: `rule-${PREFIX}.chain.agent.execution.run-1-1791480613893413505`,
    state: "complete",
    role: "ops-chain-observer",
    iterations: 1,
    max_iterations: 5,
    user_id: "",
    channel_type: "",
    parent_loop_id: "",
    outcome: "",
    error: "",
  };
}

beforeEach(() => {
  controlsStore.stop();
  controlsStore.reset();
  agentStore.reset();
  mockGetTriples.mockReset();
  mockGetTriples.mockResolvedValue([]);
});

afterEach(() => {
  vi.useRealTimers();
});

describe("ControlsNotice", () => {
  it("says nothing when every rule-fired loop is classified", async () => {
    render(ControlsNotice);
    agentStore.updateLoop(candidate("observer-1"));
    mockGetTriples.mockResolvedValue([
      { subject: `${PREFIX}.agentic-loop.agent.execution.observer-1`, predicate: "agent.loop.task", object: "x" },
    ]);
    await controlsStore.pollOnce();

    expect(controlsStore.controlLoopIds.has("observer-1")).toBe(true);
    expect(screen.queryByTestId("controls-notice")).toBeNull();
  });

  it("tells the operator, on the board and not only in a task, when a membership read fails", async () => {
    render(ControlsNotice);
    agentStore.updateLoop(candidate("observer-1"));
    agentStore.updateLoop(candidate("observer-2"));
    mockGetTriples.mockRejectedValue(new Error("triples endpoint down"));
    await controlsStore.pollOnce();

    const notice = await screen.findByTestId("controls-notice");
    expect(notice).toHaveTextContent("2 rule-fired loops");
    expect(notice).toHaveTextContent("control lists may be incomplete");
    expect(notice).toHaveTextContent("triples endpoint down");
    // Announced politely, not as an alert, and the region outlives the message.
    expect(screen.getByRole("status")).toContainElement(notice);
  });

  it("uses the singular for a single loop, and goes away when the read recovers", async () => {
    vi.useFakeTimers({ toFake: ["Date"] });
    render(ControlsNotice);
    agentStore.updateLoop(candidate("observer-1"));
    mockGetTriples.mockRejectedValue(new Error("down"));
    await controlsStore.pollOnce();
    expect(await screen.findByTestId("controls-notice")).toHaveTextContent("1 rule-fired loop belongs");

    vi.setSystemTime(Date.now() + RETRY_BASE_MS + 1);
    mockGetTriples.mockResolvedValue([
      { subject: `${PREFIX}.agentic-loop.agent.execution.observer-1`, predicate: "agent.loop.task", object: "x" },
    ]);
    await controlsStore.pollOnce();

    await vi.waitFor(() => expect(screen.queryByTestId("controls-notice")).toBeNull());
    expect(screen.getByRole("status")).toBeInTheDocument();
  });

  it("stays quiet about an entity that is merely not readable yet", async () => {
    render(ControlsNotice);
    agentStore.updateLoop(candidate("observer-1"));
    mockGetTriples.mockResolvedValue([]); // graph lag
    await controlsStore.pollOnce();

    expect(screen.queryByTestId("controls-notice")).toBeNull();
  });
});
