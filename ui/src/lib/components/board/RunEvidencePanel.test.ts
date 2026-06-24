import { describe, expect, it, vi, beforeEach } from "vitest";
import { render, screen, within } from "@testing-library/svelte";
import RunEvidencePanel from "./RunEvidencePanel.svelte";
import { agentApi } from "$lib/services/agentApi";
import { getTriples } from "$lib/services/runStatusApi";
import {
  classifyEntry,
  entryMentionsLoop,
  messageLoggerApi,
} from "$lib/services/messageLoggerApi";

vi.mock("$lib/services/agentApi", () => ({
  agentApi: {
    getLoopTrajectory: vi.fn(),
  },
}));

vi.mock("$lib/services/runStatusApi", () => ({
  getTriples: vi.fn(),
}));

vi.mock("$lib/services/messageLoggerApi", () => ({
  messageLoggerApi: {
    fetchEntries: vi.fn(),
  },
  entryMentionsLoop: vi.fn((entry, loopId) =>
    entry.subject?.includes(loopId) || JSON.stringify(entry.raw_data ?? {}).includes(loopId),
  ),
  classifyEntry: vi.fn((entry) => {
    const subject = entry.subject ?? "";
    if (subject.startsWith("agent.request.")) return "llm-request";
    if (subject.startsWith("agent.response.")) return "llm-response";
    if (subject.startsWith("tool.execute.")) return "tool-execute";
    if (subject.startsWith("tool.result.")) return "tool-result";
    if (subject.startsWith("graph.mutation.")) return "graph";
    return "other";
  }),
}));

beforeEach(() => {
  vi.mocked(agentApi.getLoopTrajectory).mockReset();
  vi.mocked(getTriples).mockReset();
  vi.mocked(messageLoggerApi.fetchEntries).mockReset();
  vi.mocked(entryMentionsLoop).mockReset();
  vi.mocked(classifyEntry).mockReset();
  vi.mocked(agentApi.getLoopTrajectory).mockResolvedValue({
    loop_id: "loop-1",
    start_time: "2026-06-24T00:00:00Z",
    steps: [
      {
        step_type: "model_call",
        timestamp: "2026-06-24T00:00:01Z",
        request_id: "req-1",
        response: "Thinking",
      },
      {
        step_type: "tool_call",
        timestamp: "2026-06-24T00:00:02Z",
        tool_name: "decide",
        tool_arguments: { action: "approved" },
      },
    ],
    outcome: "success",
  });
  vi.mocked(getTriples).mockResolvedValue([
    {
      subject: "c360.semteams.agent.chain.execution.run-1",
      predicate: "proof_readiness.route",
      object: "implementation",
    },
    {
      subject: "c360.semteams.agent.chain.execution.run-1",
      predicate: "agent.run.phase",
      object: "executing",
    },
  ]);
  vi.mocked(messageLoggerApi.fetchEntries).mockResolvedValue([
    {
      sequence: 1,
      timestamp: "2026-06-24T00:00:01Z",
      subject: "agent.request.loop-1",
      message_type: "agent.request",
      raw_data: { loop_id: "loop-1" },
    },
  ]);
  vi.mocked(entryMentionsLoop).mockImplementation((entry, loopId) =>
    entry.subject?.includes(loopId) || JSON.stringify(entry.raw_data ?? {}).includes(loopId),
  );
  vi.mocked(classifyEntry).mockImplementation((entry) => {
    const subject = entry.subject ?? "";
    if (subject.startsWith("agent.request.")) return "llm-request";
    if (subject.startsWith("agent.response.")) return "llm-response";
    if (subject.startsWith("tool.execute.")) return "tool-execute";
    if (subject.startsWith("tool.result.")) return "tool-result";
    if (subject.startsWith("graph.mutation.")) return "graph";
    return "other";
  });
});

describe("RunEvidencePanel", () => {
  it("renders raw trajectory, message log, and graph triples", async () => {
    render(RunEvidencePanel, {
      props: {
        loopId: "loop-1",
        runEntityId: "c360.semteams.agent.chain.execution.run-1",
      },
    });

    expect(await screen.findByTestId("run-evidence-panel")).toBeInTheDocument();
    await vi.waitFor(() => {
      expect(agentApi.getLoopTrajectory).toHaveBeenCalledWith("loop-1");
      expect(getTriples).toHaveBeenCalledWith({
        subject: "c360.semteams.agent.chain.execution.run-1",
        limit: 250,
      });
    });

    const counts = screen.getByTestId("run-evidence-counts");
    expect(counts).toHaveTextContent("2 steps");
    expect(counts).toHaveTextContent("2");
    expect(screen.getByTestId("run-evidence-trajectory")).toHaveTextContent('"loop_id": "loop-1"');

    const triples = screen.getByTestId("run-evidence-triples");
    expect(within(triples).getByText("proof_readiness.route")).toBeInTheDocument();
    expect(within(triples).getByText("implementation")).toBeInTheDocument();
    expect(await screen.findByTestId("task-trace")).toBeInTheDocument();
  });

  it("shows an explicit graph-triples gap when run entity is unknown", async () => {
    render(RunEvidencePanel, { props: { loopId: "loop-1", runEntityId: null } });

    await vi.waitFor(() => {
      expect(agentApi.getLoopTrajectory).toHaveBeenCalledWith("loop-1");
    });
    expect(getTriples).not.toHaveBeenCalled();
    expect(screen.getByText("Run entity id is not available yet.")).toBeInTheDocument();
  });

  it("does not let a stale in-flight request overwrite a newer scope", async () => {
    let resolveOldTrajectory!: (value: Awaited<ReturnType<typeof agentApi.getLoopTrajectory>>) => void;
    let resolveOldTriples!: (value: Awaited<ReturnType<typeof getTriples>>) => void;
    const oldTrajectory = new Promise<Awaited<ReturnType<typeof agentApi.getLoopTrajectory>>>((resolve) => {
      resolveOldTrajectory = resolve;
    });
    const oldTriples = new Promise<Awaited<ReturnType<typeof getTriples>>>((resolve) => {
      resolveOldTriples = resolve;
    });

    vi.mocked(agentApi.getLoopTrajectory).mockImplementation((loopId: string) => {
      if (loopId === "loop-old") return oldTrajectory;
      return Promise.resolve({
        loop_id: loopId,
        start_time: "2026-06-24T00:01:00Z",
        steps: [],
        outcome: "success",
      });
    });
    vi.mocked(getTriples).mockImplementation((params) => {
      if (params.subject?.endsWith("run-old")) return oldTriples;
      return Promise.resolve([
        {
          subject: "c360.semteams.agent.chain.execution.run-new",
          predicate: "proof_readiness.route",
          object: "implementation",
        },
      ]);
    });

    const { rerender } = render(RunEvidencePanel, {
      props: {
        loopId: "loop-old",
        runEntityId: "c360.semteams.agent.chain.execution.run-old",
      },
    });

    await rerender({
      loopId: "loop-new",
      runEntityId: "c360.semteams.agent.chain.execution.run-new",
    });

    await vi.waitFor(() => {
      expect(screen.getByTestId("run-evidence-trajectory")).toHaveTextContent('"loop_id": "loop-new"');
      expect(screen.getByTestId("run-evidence-triples")).toHaveTextContent("implementation");
    });

    resolveOldTrajectory({
      loop_id: "loop-old",
      start_time: "2026-06-24T00:00:00Z",
      steps: [],
      outcome: "success",
    });
    resolveOldTriples([
      {
        subject: "c360.semteams.agent.chain.execution.run-old",
        predicate: "proof_readiness.route",
        object: "test_harness",
      },
    ]);

    await vi.waitFor(() => {
      expect(screen.getByTestId("run-evidence-trajectory")).toHaveTextContent('"loop_id": "loop-new"');
      expect(screen.getByTestId("run-evidence-triples")).toHaveTextContent("implementation");
    });
    expect(screen.queryByText("test_harness")).not.toBeInTheDocument();
  });
});
