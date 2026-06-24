import { describe, expect, it } from "vitest";
import { render, screen, within } from "@testing-library/svelte";
import RunHealthPanel from "./RunHealthPanel.svelte";
import type { RunHealth } from "$lib/utils/runHealth";

function makeHealth(overrides: Partial<RunHealth> = {}): RunHealth {
  return {
    state: "working",
    label: "Working",
    currentGate: "coordinator executing",
    nextAction: "Wait for the current loop or gate to emit evidence",
    detail: "The coordinator is executing.",
    evidenceFreshness: "fresh",
    activeLoopCount: 1,
    signals: [],
    ...overrides,
  };
}

describe("RunHealthPanel", () => {
  it("renders Prometheus metrics evidence in the full panel", () => {
    render(RunHealthPanel, {
      props: {
        health: makeHealth({
          metrics: {
            freshness: "fresh",
            scrapeAgeMs: 12_000,
            lastSampleAt: Date.parse("2026-06-24T12:00:00Z"),
            metricCount: 4,
            componentCount: 3,
            degradedComponentCount: 1,
            errorComponentCount: 0,
            queueDepthMax: 7,
            latencyMsMax: 1200,
            errorRateMax: 0.5,
            signals: [
              {
                kind: "queue_depth",
                severity: "warning",
                label: "teams-dispatch queue depth 7",
                detail: "semstreams_rule_queue_depth",
                component: "teams-dispatch",
                metricName: "semstreams_rule_queue_depth",
                value: 7,
              },
            ],
          },
        }),
      },
    });

    const panel = screen.getByTestId("run-health-panel");
    expect(panel).toHaveTextContent("Metrics");
    expect(panel).toHaveTextContent("fresh (4 samples)");

    const metrics = screen.getByTestId("run-metrics-evidence");
    expect(metrics).toHaveTextContent("Prometheus");
    expect(metrics).toHaveTextContent("12s");
    expect(metrics).toHaveTextContent("7");
    expect(metrics).toHaveTextContent("1,200ms");
    expect(metrics).toHaveTextContent("0.5/s");
    expect(within(metrics).getByText("teams-dispatch queue depth 7")).toBeInTheDocument();
  });

  it("keeps compact cards terse even when metrics evidence exists", () => {
    render(RunHealthPanel, {
      props: {
        compact: true,
        health: makeHealth({
          metrics: {
            freshness: "unavailable",
            scrapeAgeMs: null,
            lastSampleAt: null,
            metricCount: 0,
            componentCount: 0,
            degradedComponentCount: 0,
            errorComponentCount: 0,
            queueDepthMax: null,
            latencyMsMax: null,
            errorRateMax: null,
            signals: [
              {
                kind: "freshness",
                severity: "warning",
                label: "Metrics unavailable",
                detail: "Runtime stream is disconnected.",
              },
            ],
          },
        }),
      },
    });

    expect(screen.getByTestId("run-health-badge")).toHaveTextContent("Working");
    expect(screen.queryByTestId("run-metrics-evidence")).not.toBeInTheDocument();
  });
});
