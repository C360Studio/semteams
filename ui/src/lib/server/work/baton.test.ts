// @vitest-environment node
import { describe, expect, it } from "vitest";
import {
  ASSIGNED_WITH_UNKNOWN_PRS_REASON,
  deriveBatonOverlays,
  deriveClaimOverlay,
  deriveGateOverlay,
  deriveReviewOverlay,
  NO_CLAIM_METADATA_REASON,
  NO_LINKED_PRS_REASON,
  NO_REVIEW_RECORDS_REASON,
} from "./baton";
import type { BatonDerivationInput } from "./baton";

function baseInput(overrides: Partial<BatonDerivationInput> = {}): BatonDerivationInput {
  return {
    labels: [],
    assignees: [],
    dependencies: [],
    delivery: { state: "none" },
    ...overrides,
  };
}

describe("deriveClaimOverlay (design D2.1)", () => {
  it("identifies an agent draft PR claim from branch prefix and preserves stop-point", () => {
    const claim = deriveClaimOverlay(
      baseInput({
        assignees: ["coby"],
        delivery: {
          state: "known",
          value: [
            {
              ref: "c360studio/semteams#318",
              state: "open",
              draft: true,
              branch: "claude/gh312-mqtt-nats-research",
              implemented_by: "claude",
              stop_point: "Awaiting benchmark gather synthesis",
            },
          ],
        },
      }),
    );

    expect(claim).toEqual({
      state: "known",
      value: {
        kind: "agent-pr",
        holder: "claude",
        agent_vendor: "claude",
        pull_request: "c360studio/semteams#318",
        branch: "claude/gh312-mqtt-nats-research",
        draft: true,
        stop_point: "Awaiting benchmark gather synthesis",
        implemented_by: "claude",
      },
    });
  });

  it("maps Sol implemented_by to codex vendor when branch prefix is not vendor-prefixed", () => {
    const claim = deriveClaimOverlay(
      baseInput({
        delivery: {
          state: "known",
          value: [
            {
              ref: "c360studio/semteams#319",
              state: "open",
              branch: "feat/baton-board",
              implemented_by: "Sol",
            },
          ],
        },
      }),
    );

    expect(claim).toEqual({
      state: "known",
      value: {
        kind: "agent-pr",
        holder: "Sol",
        agent_vendor: "codex",
        pull_request: "c360studio/semteams#319",
        branch: "feat/baton-board",
        implemented_by: "Sol",
      },
    });
  });

  it("identifies a human PR when branch has no agent vendor prefix", () => {
    const claim = deriveClaimOverlay(
      baseInput({
        delivery: {
          state: "known",
          value: [
            {
              ref: "c360studio/semteams#322",
              state: "open",
              branch: "coby/fix-typo",
              implemented_by: "coby",
            },
          ],
        },
      }),
    );

    expect(claim).toEqual({
      state: "known",
      value: {
        kind: "human-pr",
        holder: "coby",
        pull_request: "c360studio/semteams#322",
        branch: "coby/fix-typo",
        implemented_by: "coby",
      },
    });
  });

  it("reports unknown when an open PR exists without branch or claim metadata", () => {
    const claim = deriveClaimOverlay(
      baseInput({
        delivery: {
          state: "known",
          value: [{ ref: "c360studio/semteams#321", state: "open" }],
        },
      }),
    );

    expect(claim).toEqual({
      state: "unknown",
      reason: NO_CLAIM_METADATA_REASON,
    });
  });

  it("falls back to assignee with a reason when linked PRs are unknown", () => {
    const claim = deriveClaimOverlay(
      baseInput({
        assignees: ["codex"],
        delivery: { state: "unknown", reason: NO_LINKED_PRS_REASON },
      }),
    );

    expect(claim).toEqual({
      state: "known",
      value: { kind: "assignee", holder: "codex" },
      reason: ASSIGNED_WITH_UNKNOWN_PRS_REASON,
    });
  });

  it("is unknown when linked PRs are unknown and no assignee is set", () => {
    const claim = deriveClaimOverlay(
      baseInput({
        delivery: { state: "unknown", reason: NO_LINKED_PRS_REASON },
      }),
    );

    expect(claim).toEqual({
      state: "unknown",
      reason: NO_LINKED_PRS_REASON,
    });
  });

  it("is none only when delivery was inspected and there is no active PR or assignee", () => {
    expect(deriveClaimOverlay(baseInput({ delivery: { state: "none" } }))).toEqual({ state: "none" });
    expect(
      deriveClaimOverlay(
        baseInput({
          delivery: { state: "known", value: [{ ref: "o/r#1", state: "merged" }] },
        }),
      ),
    ).toEqual({ state: "none" });
  });
});

describe("deriveReviewOverlay (design D2.2)", () => {
  it("is unknown when delivery is unknown or open PR lacks head_sha", () => {
    expect(
      deriveReviewOverlay(baseInput({ delivery: { state: "unknown", reason: NO_LINKED_PRS_REASON } })),
    ).toEqual({ state: "unknown", reason: NO_LINKED_PRS_REASON });

    expect(
      deriveReviewOverlay(
        baseInput({
          delivery: { state: "known", value: [{ ref: "o/r#1", state: "open", branch: "claude/x" }] },
        }),
      ),
    ).toEqual({ state: "unknown", reason: NO_REVIEW_RECORDS_REASON });
  });

  it("is none when delivery has no open PRs", () => {
    expect(deriveReviewOverlay(baseInput({ delivery: { state: "none" } }))).toEqual({ state: "none" });
    expect(
      deriveReviewOverlay(
        baseInput({
          delivery: { state: "known", value: [{ ref: "o/r#1", state: "merged", head_sha: "abc" }] },
        }),
      ),
    ).toEqual({ state: "none" });
  });

  it("distinguishes not_requested, requested, current, stale, and changes_requested by commit SHA", () => {
    const forPr = (pr: Partial< Extract<BatonDerivationInput["delivery"], { state: "known" }>["value"][number]>) =>
      deriveReviewOverlay(
        baseInput({
          delivery: {
            state: "known",
            value: [{ ref: "c360studio/semteams#320", state: "open", head_sha: "a1b2c3d", ...pr }],
          },
        }),
      );

    expect(forPr({})).toEqual({
      state: "known",
      value: {
        pull_request: "c360studio/semteams#320",
        status: "not_requested",
        head_sha: "a1b2c3d",
      },
    });

    expect(
      forPr({
        review_request: { sha: "a1b2c3d", reviewer_lane: "go-reviewer" },
      }),
    ).toEqual({
      state: "known",
      value: {
        pull_request: "c360studio/semteams#320",
        status: "requested",
        head_sha: "a1b2c3d",
        request_sha: "a1b2c3d",
        reviewer_lane: "go-reviewer",
      },
    });

    expect(
      forPr({
        implemented_by: "claude",
        review_request: { sha: "a1b2c3d", reviewer_lane: "svelte-reviewer" },
        review_record: {
          sha: "a1b2c3d",
          verdict: "pass",
          reviewer_lane: "svelte-reviewer",
          reviewed_by: "codex (svelte-reviewer)",
        },
      }),
    ).toEqual({
      state: "known",
      value: {
        pull_request: "c360studio/semteams#320",
        status: "current",
        head_sha: "a1b2c3d",
        request_sha: "a1b2c3d",
        record_sha: "a1b2c3d",
        reviewer_lane: "svelte-reviewer",
        implemented_by: "claude",
        reviewed_by: "codex (svelte-reviewer)",
      },
    });

    // Commits landed after the review record without a new review request at HEAD -> stale
    expect(
      forPr({
        review_request: { sha: "9988776", reviewer_lane: "go-reviewer" },
        review_record: {
          sha: "9988776",
          verdict: "pass",
          reviewer_lane: "go-reviewer",
          reviewed_by: "claude (go-reviewer)",
        },
      }),
    ).toMatchObject({
      state: "known",
      value: {
        status: "stale",
        head_sha: "a1b2c3d",
        record_sha: "9988776",
      },
    });

    // A new review request at the new HEAD moves stale back to requested
    expect(
      forPr({
        review_request: { sha: "a1b2c3d", reviewer_lane: "go-reviewer" },
        review_record: { sha: "9988776", verdict: "pass", reviewer_lane: "go-reviewer" },
      }),
    ).toMatchObject({
      state: "known",
      value: {
        status: "requested",
        head_sha: "a1b2c3d",
        request_sha: "a1b2c3d",
        record_sha: "9988776",
      },
    });

    // Blocking verdict at HEAD -> changes_requested
    expect(
      forPr({
        review_record: { sha: "a1b2c3d", verdict: "blocking", reviewer_lane: "go-reviewer" },
      }),
    ).toMatchObject({
      state: "known",
      value: {
        status: "changes_requested",
        head_sha: "a1b2c3d",
        record_sha: "a1b2c3d",
      },
    });
  });
});

describe("deriveGateOverlay (design D2.3)", () => {
  it("aggregates needs-decision, blocked dependencies, flake holds, and OpenSpec holds", () => {
    const gate = deriveGateOverlay(
      baseInput({
        labels: ["status:needs-decision", "status:blocked"],
        dependencies: [
          { ref: "c360studio/semteams#314", source: "blocked-by" },
          { ref: "c360studio/semteams#315", source: "task-list" },
        ],
        delivery: {
          state: "known",
          value: [{ ref: "c360studio/semteams#318", state: "open", branch: "claude/x" }],
        },
        flake_holds: ["c360studio/semteams#309"],
        openspec_change: {
          id: "mqtt-nats-edge-study",
          holds: [{ line: 14, class: "BLOCKED", text: "Hold: waiting on ARM edge fixture traces" }],
        },
      }),
    );

    expect(gate).toEqual({
      state: "known",
      value: [
        { kind: "needs-decision", summary: "status:needs-decision (owner docket)" },
        { kind: "blocked", summary: "blocked by c360studio/semteams#314", ref: "c360studio/semteams#314" },
        {
          kind: "flake-hold",
          summary: "required-job flake hold (c360studio/semteams#309)",
          ref: "c360studio/semteams#309",
        },
        {
          kind: "openspec-hold",
          summary: "OpenSpec mqtt-nats-edge-study L14: Hold: waiting on ARM edge fixture traces",
          ref: "mqtt-nats-edge-study",
          caveat_class: "BLOCKED",
        },
      ],
    });
  });

  it("reports unknown when no gate is found and a linked OpenSpec change is uninspected", () => {
    const gate = deriveGateOverlay(
      baseInput({
        openspec_change: { id: "uninspected-change", inspected: false },
      }),
    );

    expect(gate).toEqual({
      state: "unknown",
      reason: "OpenSpec change uninspected-change tasks.md caveats not inspected",
    });
  });

  it("keeps a positive gate known even when an OpenSpec change is uninspected, carrying the reason", () => {
    const gate = deriveGateOverlay(
      baseInput({
        labels: ["status:needs-decision"],
        openspec_change: { id: "uninspected-change", inspected: false },
      }),
    );

    expect(gate).toEqual({
      state: "known",
      value: [{ kind: "needs-decision", summary: "status:needs-decision (owner docket)" }],
      reason: "OpenSpec change uninspected-change tasks.md caveats not inspected",
    });
  });

  it("is none when all sources are inspected and no gate applies", () => {
    expect(deriveBatonOverlays(baseInput()).gate).toEqual({ state: "none" });
  });
});
