import { describe, it, expect } from "vitest";
import { render, screen, within } from "@testing-library/svelte";
import ProofReadinessCard from "./ProofReadinessCard.svelte";

function proofResult() {
  return JSON.stringify({
    run_entity_id: "c360.ops.agent.chain.execution.run-123",
    status: "failed",
    version: "go-native-v1",
    finding_count: 2,
    findings: [
      {
        id: "f001",
        kind: "missing_proof_dependency",
        severity: "blocker",
        route: "test_harness",
        reason: "required proof dependency is not ready",
        claim: "mavlink.mission_upload",
        dependency: "px4_sitl.boots",
      },
      {
        id: "f002",
        kind: "stale_readiness_record",
        severity: "blocker",
        route: "test_harness",
        reason: "readiness record is stale or expired",
        readiness: "smoke-001",
      },
    ],
    proof_facts: {
      dependencies: [
        {
          id: "px4_sitl.boots",
          kind: "service",
          description: "PX4 SITL boots headlessly",
          status: "missing",
          profile_ref: "mavlink.px4-sitl@v1",
          next_route: "test_harness",
        },
      ],
      readiness: [
        {
          id: "smoke-001",
          profile_ref: "mavlink.px4-sitl@v1",
          status: "stale",
          smoke_status: "failed",
          expires_at: "2026-06-23T12:00:00Z",
          attestation_ref: "sandbox.attestation.signature:abc123",
        },
      ],
      evidence: [
        {
          id: "smoke-log",
          kind: "log",
          producer: "test-harness",
          created_at: "2026-06-24T12:00:00Z",
          exit_code: "1",
          uri: "object://proof/smoke-log",
          covers: ["mavlink.mission_upload"],
        },
      ],
      waivers: [
        {
          id: "operator-001",
          status: "active",
          reason: "PX4 unavailable in this environment",
          expires_at: "2026-06-25T12:00:00Z",
          approved_by: "coby",
          residual_risk: "Mission upload remains unproved against PX4 SITL",
        },
      ],
    },
  });
}

describe("ProofReadinessCard", () => {
  it("renders the proof-readiness status and run summary", () => {
    render(ProofReadinessCard, { props: { result: proofResult() } });

    expect(screen.getByTestId("proof-readiness-card")).toBeInTheDocument();
    expect(screen.getByText("Readiness Gate")).toBeInTheDocument();
    expect(screen.getByText("Claim Analysis")).toBeInTheDocument();
    expect(screen.getByTestId("proof-status")).toHaveTextContent("failed");
    expect(screen.getByText("c360.ops.agent.chain.execution.run-123")).toBeInTheDocument();
    expect(screen.getByText("go-native-v1")).toBeInTheDocument();
  });

  it("renders proof dependency, readiness, evidence, and waiver sections", () => {
    render(ProofReadinessCard, { props: { result: proofResult() } });

    const dependencies = screen.getByTestId("proof-dependencies-card");
    expect(within(dependencies).getByText("px4_sitl.boots")).toBeInTheDocument();
    expect(within(dependencies).getByText("PX4 SITL boots headlessly")).toBeInTheDocument();
    expect(within(dependencies).getByText("mavlink.px4-sitl@v1")).toBeInTheDocument();

    const readiness = screen.getByTestId("proof-readiness-records-card");
    expect(within(readiness).getByText("smoke-001")).toBeInTheDocument();
    expect(within(readiness).getAllByText("stale").length).toBeGreaterThan(0);
    expect(
      within(readiness).getByText("sandbox.attestation.signature:abc123"),
    ).toBeInTheDocument();

    const evidence = screen.getByTestId("proof-evidence-card");
    expect(within(evidence).getByText("smoke-log")).toBeInTheDocument();
    expect(within(evidence).getByText("test-harness")).toBeInTheDocument();
    expect(within(evidence).getByText(/mavlink\.mission_upload/)).toBeInTheDocument();

    const waivers = screen.getByTestId("proof-waivers-card");
    expect(within(waivers).getByText("operator-001")).toBeInTheDocument();
    expect(
      within(waivers).getByText("PX4 unavailable in this environment"),
    ).toBeInTheDocument();
    expect(within(waivers).getByText("coby")).toBeInTheDocument();
  });

  it("falls back to finding references when fact summaries are absent", () => {
    render(ProofReadinessCard, {
      props: {
        result: JSON.stringify({
          status: "failed",
          finding_count: 1,
          findings: [
            {
              kind: "expired_waiver",
              severity: "blocker",
              route: "coordinator",
              dependency: "vehicle_health.ready_detectable",
              waiver: "operator-002",
              reason: "matching waiver exists but is not active",
            },
          ],
        }),
      },
    });

    expect(
      within(screen.getByTestId("proof-dependencies-card")).getByText(
        "vehicle_health.ready_detectable",
      ),
    ).toBeInTheDocument();
    expect(
      within(screen.getByTestId("proof-waivers-card")).getByText("operator-002"),
    ).toBeInTheDocument();
  });
});
