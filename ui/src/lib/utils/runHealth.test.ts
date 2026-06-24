import { describe, expect, it } from "vitest";
import type { AgentLoop } from "$lib/types/agent";
import type { RawTriple } from "$lib/services/runStatusApi";
import {
  deriveMetricsEvidence,
  deriveGraphRunHealthFacts,
  deriveRunHealth,
  runIdFromEntity,
} from "./runHealth";

const RUN_ENTITY = "c360.semteams.agent.chain.execution.run-1";

function triple(predicate: string, object: string, subject = RUN_ENTITY): RawTriple {
  return { subject, predicate, object };
}

function loop(overrides: Partial<AgentLoop> = {}): AgentLoop {
  return {
    loop_id: "loop-1",
    task_id: "task-1",
    state: "executing",
    role: "coordinator",
    iterations: 1,
    max_iterations: 8,
    user_id: "user",
    channel_type: "web",
    parent_loop_id: "",
    outcome: "",
    error: "",
    ...overrides,
  };
}

describe("runHealth", () => {
  it("extracts bare run id from a full run entity id", () => {
    expect(runIdFromEntity(RUN_ENTITY)).toBe("run-1");
    expect(runIdFromEntity("c360.semteams.agent.agentic-loop.execution.loop-1")).toBe("");
  });

  it("derives graph facts from lifecycle, proof, CBG, and evidence triples", () => {
    const facts = deriveGraphRunHealthFacts(
      [
        triple("agent.run.phase", "executing"),
        triple("formal_claims.status", "failed"),
        triple("formal_claims.finding_count", "2"),
        triple("proof_readiness.route", "test_harness"),
        triple("proof_readiness.test_harness_required", "true"),
        triple("dev_via_test.cbg.retry.finding", "Use gomavlib."),
        triple("proof.readiness.smoke-1.status", "passed"),
        triple("proof.readiness.smoke-1.expires_at", "2026-06-25T00:00:00Z"),
      ],
      { now: new Date("2026-06-24T12:00:00Z") },
    );

    expect(facts.get("run-1")).toMatchObject({
      phase: "executing",
      formalClaimsStatus: "failed",
      formalClaimsFindingCount: 2,
      proofReadinessRoute: "test_harness",
      proofTestHarnessRequired: true,
      cbgRetryFinding: "Use gomavlib.",
      evidenceFreshness: "fresh",
      readinessRecordCount: 1,
    });
  });

  it("marks expired readiness evidence as stale", () => {
    const facts = deriveGraphRunHealthFacts(
      [
        triple("proof.readiness.smoke-1.status", "passed"),
        triple("proof.readiness.smoke-1.expires_at", "2026-06-23T00:00:00Z"),
      ],
      { now: new Date("2026-06-24T12:00:00Z") },
    );

    expect(facts.get("run-1")!.evidenceFreshness).toBe("stale");
    expect(facts.get("run-1")!.staleEvidenceCount).toBeGreaterThan(0);
  });

  it("prioritizes a human wait over active work", () => {
    const health = deriveRunHealth({
      runId: "run-1",
      loops: [loop({ state: "executing" })],
      pause: { cause: "clarification", askingLoopId: "loop-ask", question: "Which harness?" },
    });

    expect(health.state).toBe("waiting");
    expect(health.currentGate).toBe("Human clarification");
    expect(health.nextAction).toContain("Answer");
  });

  it("prioritizes lifecycle failure over active loops", () => {
    const graph = deriveGraphRunHealthFacts([
      triple("agent.run.phase", "failed"),
      triple("agent.run.outcome", "failed"),
    ]).get("run-1");

    const health = deriveRunHealth({
      runId: "run-1",
      loops: [loop({ state: "executing" })],
      graph,
    });

    expect(health.state).toBe("failing");
    expect(health.currentGate).toBe("Failure recovery");
  });

  it("blocks implementation when proof readiness routes to test harness", () => {
    const graph = deriveGraphRunHealthFacts([
      triple("formal_claims.status", "failed"),
      triple("formal_claims.finding_count", "1"),
      triple("proof_readiness.route", "test_harness"),
      triple("proof_readiness.test_harness_required", "true"),
    ]).get("run-1");

    const health = deriveRunHealth({ runId: "run-1", loops: [], graph });

    expect(health.state).toBe("blocked");
    expect(health.currentGate).toBe("Readiness Gate");
    expect(health.nextAction).toContain("harness");
  });

  it("waits for implementation request after proof readiness passes", () => {
    const graph = deriveGraphRunHealthFacts([
      triple("formal_claims.status", "passed"),
      triple("proof_readiness.route", "implementation"),
      triple("proof_readiness.implementation_ready", "true"),
    ]).get("run-1");

    const health = deriveRunHealth({ runId: "run-1", loops: [], graph });

    expect(health.state).toBe("waiting");
    expect(health.currentGate).toBe("Implement Task request");
  });

  it("reports working for active loops", () => {
    const health = deriveRunHealth({
      runId: "run-1",
      loops: [loop({ state: "reviewing", role: "reviewer-dev-via-test" })],
    });

    expect(health.state).toBe("working");
    expect(health.currentGate).toContain("reviewer-dev-via-test");
    expect(health.activeLoopCount).toBe(1);
  });

  it("reports complete only when no blocker is visible", () => {
    const graph = deriveGraphRunHealthFacts([
      triple("agent.run.phase", "completed"),
      triple("agent.run.outcome", "success"),
    ]).get("run-1");

    const health = deriveRunHealth({
      runId: "run-1",
      loops: [loop({ state: "complete", outcome: "success" })],
      graph,
    });

    expect(health.state).toBe("complete");
    expect(health.currentGate).toBe("Accepted");
  });

  it("derives Prometheus freshness and pressure signals without changing workflow state", () => {
    const metrics = deriveMetricsEvidence({
      connected: true,
      lastMetricsTimestamp: Date.parse("2026-06-24T12:00:00Z"),
      now: new Date("2026-06-24T12:00:10Z"),
      healthComponents: [
        { name: "teams-loop", status: "degraded", message: "slow handler" },
      ],
      metrics: [
        {
          component: "teams-dispatch",
          metricName: "semstreams_rule_queue_depth",
          rate: null,
          raw: {
            name: "semstreams_rule_queue_depth",
            type: "gauge",
            value: 7,
            labels: { component: "teams-dispatch" },
          },
        },
        {
          component: "agentic-loop",
          metricName: "semstreams_tool_latency_seconds",
          rate: null,
          raw: {
            name: "semstreams_tool_latency_seconds",
            type: "gauge",
            value: 1.2,
            labels: { component: "agentic-loop" },
          },
        },
        {
          component: "agentic-tools",
          metricName: "semstreams_tool_errors_total",
          rate: 0.5,
          raw: {
            name: "semstreams_tool_errors_total",
            type: "counter",
            value: 12,
            labels: { component: "agentic-tools" },
          },
        },
      ],
    });

    expect(metrics).toMatchObject({
      freshness: "fresh",
      scrapeAgeMs: 10_000,
      lastSampleAt: Date.parse("2026-06-24T12:00:00Z"),
      metricCount: 3,
      degradedComponentCount: 1,
      queueDepthMax: 7,
      latencyMsMax: 1200,
      errorRateMax: 0.5,
    });
    expect(metrics.signals.map((s) => s.kind)).toEqual([
      "component_pressure",
      "queue_depth",
      "latency",
      "error_rate",
    ]);

    const health = deriveRunHealth({
      runId: "run-1",
      loops: [loop({ state: "executing" })],
      metrics,
    });

    expect(health.state).toBe("working");
    expect(health.metrics?.freshness).toBe("fresh");
    expect(health.signals).toContainEqual(
      expect.objectContaining({ source: "metrics", label: "metrics fresh" }),
    );
  });

  it("marks missing or stale Prometheus samples as metric evidence gaps", () => {
    const missing = deriveMetricsEvidence({
      connected: false,
      metrics: [],
      lastMetricsTimestamp: null,
      now: new Date("2026-06-24T12:00:00Z"),
    });
    expect(missing.freshness).toBe("unavailable");
    expect(missing.signals[0]).toMatchObject({
      kind: "freshness",
      label: "Metrics unavailable",
    });

    const stale = deriveMetricsEvidence({
      connected: true,
      metrics: [
        {
          component: "teams-loop",
          metricName: "messages_processed_total",
          rate: 1,
          raw: { name: "messages_processed_total", type: "counter", value: 10, labels: {} },
        },
      ],
      lastMetricsTimestamp: Date.parse("2026-06-24T11:58:00Z"),
      now: new Date("2026-06-24T12:00:00Z"),
      staleAfterMs: 60_000,
    });
    expect(stale.freshness).toBe("stale");
    expect(stale.signals[0]).toMatchObject({
      kind: "freshness",
      label: "Metrics stale",
    });
  });

  it("surfaces missing metric correlation labels as an operational evidence gap", () => {
    const evidence = deriveMetricsEvidence({
      connected: true,
      lastMetricsTimestamp: Date.parse("2026-06-24T12:00:00Z"),
      now: new Date("2026-06-24T12:00:01Z"),
      metrics: [
        {
          component: "teams-loop",
          metricName: "messages_processed_total",
          rate: 1,
          raw: { name: "messages_processed_total", type: "counter", value: 3, labels: {} },
        },
      ],
    });

    expect(evidence.freshness).toBe("fresh");
    expect(evidence.signals).toContainEqual(
      expect.objectContaining({
        kind: "correlation",
        label: "Metric correlation unavailable",
      }),
    );
  });
});
