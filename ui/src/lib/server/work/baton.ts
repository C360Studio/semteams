// Deterministic protocol baton, review-parity, and decision/hold gate derivation
// (OpenSpec work-baton-overlays, design D2).
//
// Every overlay carries an explicit `known | none | unknown` state. `none` is
// emitted only when the source inspected the underlying records and confirmed
// absence; uninspected sources emit `unknown` with a reason.

import type {
  AgentVendor,
  OpenSpecCaveatClass,
  Overlay,
  PullRequestRef,
  WorkClaim,
  WorkDependency,
  WorkGate,
  WorkReviewBaton,
  WorkReviewStatus,
} from "$lib/types/work";

export interface OpenSpecHoldInput {
  line: number;
  class: OpenSpecCaveatClass;
  text: string;
}

export interface OpenSpecChangeInput {
  id: string;
  /** False when the change is referenced but its tasks.md could not be read. */
  inspected?: boolean;
  holds?: OpenSpecHoldInput[];
}

export interface BatonDerivationInput {
  labels: string[];
  assignees: string[];
  dependencies: WorkDependency[];
  delivery: Overlay<PullRequestRef[]>;
  /** Repository-level open `class:flake` issue refs (`owner/repo#n`). */
  flake_holds?: string[];
  /** Linked OpenSpec change and any unticked caveat lines from `tasks.md`. */
  openspec_change?: OpenSpecChangeInput | null;
}

export interface BatonOverlays {
  claim: Overlay<WorkClaim>;
  review: Overlay<WorkReviewBaton>;
  gate: Overlay<WorkGate[]>;
}

export const NO_LINKED_PRS_REASON = "source does not supply linked pull requests";
export const NO_CLAIM_METADATA_REASON = "open pull request has no branch or claim metadata";
export const ASSIGNED_WITH_UNKNOWN_PRS_REASON = "assigned on issue; linked pull requests unknown";
export const NO_REVIEW_RECORDS_REASON = "source does not supply PR review records";
export const UNINSPECTED_REVIEW_COMMENTS_REASON = "PR review comments not inspected";

const AGENT_VENDORS: readonly AgentVendor[] = ["claude", "codex", "gemini"];

export function shaMatchesHead(candidateSha: string, headSha: string): boolean {
  const candidate = candidateSha.trim().toLowerCase();
  const head = headSha.trim().toLowerCase();
  if (!candidate || !head) return false;
  if (candidate === head) return true;
  return candidate.length >= 7 && head.startsWith(candidate);
}

function parseAgentVendor(branch?: string, implementedBy?: string): AgentVendor | undefined {
  if (branch) {
    const prefix = branch.split("/")[0]?.trim().toLowerCase();
    if (prefix && (AGENT_VENDORS as readonly string[]).includes(prefix)) {
      return prefix as AgentVendor;
    }
  }
  if (implementedBy) {
    const token = implementedBy.trim().toLowerCase().split(/[\s(]+/)[0] ?? "";
    if ((AGENT_VENDORS as readonly string[]).includes(token)) {
      return token as AgentVendor;
    }
    if (token === "sol") return "codex";
  }
  return undefined;
}

function activePullRequests(delivery: Overlay<PullRequestRef[]>): PullRequestRef[] {
  if (delivery.state !== "known") return [];
  return delivery.value.filter((pr) => pr.state === "open");
}

export function deriveClaimOverlay(input: BatonDerivationInput): Overlay<WorkClaim> {
  const openPrs = activePullRequests(input.delivery);
  if (openPrs.length > 0) {
    // Prefer an open PR that carries branch or implemented_by metadata.
    const claimedPr = openPrs.find((pr) => Boolean(pr.branch || pr.implemented_by)) ?? openPrs[0];
    if (!claimedPr.branch && !claimedPr.implemented_by) {
      return { state: "unknown", reason: NO_CLAIM_METADATA_REASON };
    }
    const vendor = parseAgentVendor(claimedPr.branch, claimedPr.implemented_by);
    const holder = claimedPr.implemented_by ?? vendor ?? claimedPr.branch!;
    return {
      state: "known",
      value: {
        kind: vendor ? "agent-pr" : "human-pr",
        holder,
        ...(vendor && { agent_vendor: vendor }),
        pull_request: claimedPr.ref,
        ...(claimedPr.branch && { branch: claimedPr.branch }),
        ...(claimedPr.draft !== undefined && { draft: claimedPr.draft }),
        ...(claimedPr.stop_point && { stop_point: claimedPr.stop_point }),
        ...(claimedPr.implemented_by && { implemented_by: claimedPr.implemented_by }),
      },
    };
  }

  if (input.delivery.state === "unknown") {
    if (input.assignees.length > 0) {
      return {
        state: "known",
        value: {
          kind: "assignee",
          holder: input.assignees.join(", "),
        },
        reason: ASSIGNED_WITH_UNKNOWN_PRS_REASON,
      };
    }
    return { state: "unknown", reason: input.delivery.reason ?? NO_LINKED_PRS_REASON };
  }

  if (input.assignees.length > 0) {
    return {
      state: "known",
      value: {
        kind: "assignee",
        holder: input.assignees.join(", "),
      },
    };
  }

  return { state: "none" };
}

export function deriveReviewOverlay(input: BatonDerivationInput): Overlay<WorkReviewBaton> {
  if (input.delivery.state === "unknown") {
    return { state: "unknown", reason: input.delivery.reason ?? NO_LINKED_PRS_REASON };
  }
  if (input.delivery.state === "none") {
    return { state: "none" };
  }

  const openPrs = activePullRequests(input.delivery);
  if (openPrs.length === 0) {
    return { state: "none" };
  }

  const pr = openPrs.find((candidate) => Boolean(candidate.head_sha)) ?? openPrs[0];
  if (!pr.head_sha) {
    return { state: "unknown", reason: NO_REVIEW_RECORDS_REASON };
  }
  if (pr.review_inspected === false) {
    return { state: "unknown", reason: UNINSPECTED_REVIEW_COMMENTS_REASON };
  }

  const req = pr.review_request;
  const rec = pr.review_record;
  let status: WorkReviewStatus;
  if (rec) {
    if (shaMatchesHead(rec.sha, pr.head_sha)) {
      status = rec.verdict === "pass" ? "current" : "changes_requested";
    } else if (req && shaMatchesHead(req.sha, pr.head_sha)) {
      status = "requested";
    } else {
      status = "stale";
    }
  } else if (req) {
    status = "requested";
  } else {
    status = "not_requested";
  }

  const reviewerLane = rec?.reviewer_lane ?? req?.reviewer_lane;
  const reviewedBy = rec?.reviewed_by ?? pr.reviewed_by;

  return {
    state: "known",
    value: {
      pull_request: pr.ref,
      status,
      head_sha: pr.head_sha,
      ...(req?.sha && { request_sha: req.sha }),
      ...(rec?.sha && { record_sha: rec.sha }),
      ...(reviewerLane && { reviewer_lane: reviewerLane }),
      ...(pr.implemented_by && { implemented_by: pr.implemented_by }),
      ...(reviewedBy && { reviewed_by: reviewedBy }),
    },
  };
}

export function deriveGateOverlay(input: BatonDerivationInput): Overlay<WorkGate[]> {
  const gates: WorkGate[] = [];

  if (input.labels.includes("status:needs-decision")) {
    gates.push({
      kind: "needs-decision",
      summary: "status:needs-decision (owner docket)",
    });
  }

  const blockedDeps = input.dependencies.filter((dep) => dep.source === "blocked-by");
  for (const dep of blockedDeps) {
    gates.push({
      kind: "blocked",
      summary: `blocked by ${dep.ref}`,
      ref: dep.ref,
    });
  }
  if (input.labels.includes("status:blocked") && blockedDeps.length === 0) {
    gates.push({
      kind: "blocked",
      summary: "status:blocked",
    });
  }

  const openPrs = activePullRequests(input.delivery);
  if (openPrs.length > 0 && input.flake_holds && input.flake_holds.length > 0) {
    for (const flakeRef of input.flake_holds) {
      gates.push({
        kind: "flake-hold",
        summary: `required-job flake hold (${flakeRef})`,
        ref: flakeRef,
      });
    }
  }

  const change = input.openspec_change;
  if (change && change.inspected !== false) {
    for (const hold of change.holds ?? []) {
      gates.push({
        kind: "openspec-hold",
        summary: `OpenSpec ${change.id} L${hold.line}: ${hold.text}`,
        ref: change.id,
        caveat_class: hold.class,
      });
    }
  }

  const uninspectedReason =
    change && change.inspected === false
      ? `OpenSpec change ${change.id} tasks.md caveats not inspected`
      : undefined;

  if (gates.length > 0) {
    return {
      state: "known",
      value: gates,
      ...(uninspectedReason && { reason: uninspectedReason }),
    };
  }

  if (uninspectedReason) {
    return { state: "unknown", reason: uninspectedReason };
  }

  return { state: "none" };
}

export function deriveBatonOverlays(input: BatonDerivationInput): BatonOverlays {
  return {
    claim: deriveClaimOverlay(input),
    review: deriveReviewOverlay(input),
    gate: deriveGateOverlay(input),
  };
}
