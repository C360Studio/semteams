/**
 * Verdict classification for `decide(action, reason)` tool calls.
 *
 * Every agentic loop — coordinator, researcher, reviewer, and every
 * category pack — ends its turn with a `decide` tool call whose `action`
 * is the routing verdict and `reason` is the rationale (see the research /
 * autoresearch / dev-via-test journey fixtures). This is the single most
 * important fact about a loop, so the story view surfaces it as a chip.
 *
 * The action vocabulary is open: each pack introduces its own routing
 * tokens (research, gather, synthesize, propose, measure, planned,
 * dev_via_test_finalize, …). Rather than enumerate them all, we classify
 * by a small set of universal *outcome* tokens and treat everything else
 * as a neutral routing step. The classifier is intentionally
 * coarse-grained — tone, not taxonomy.
 */

export type VerdictTone = "approve" | "reject" | "clarify" | "route";

/** Terminal-positive outcomes shared across packs. */
const APPROVE = new Set([
  "approved",
  "approve",
  "accepted",
  "accept",
  "plan_approved",
  "respond_direct",
  // Forward-compat: no current pack emits decide(action="complete"), but
  // treat it as a terminal positive if one ever does.
  "complete",
  "completed",
]);

/** Negative / blocking outcomes. */
const REJECT = new Set([
  "rejected",
  "rejected_retry",
  "reject",
  "denied",
  "deny",
  "blocked",
  "failed",
  "fail",
  "abort",
  "aborted",
  "plan_rejected",
  "plan_rejected_retry",
  "insufficient",
]);

/** Outcomes that hand back to a human / need more input. */
const CLARIFY = new Set([
  "needs_clarification",
  "needs_input",
  "clarify",
  "clarification",
  "escalate",
  "escalated",
]);

/**
 * Classify a decide action into a presentation tone. Unknown / pack-specific
 * routing tokens fall through to "route".
 */
export function classifyVerdict(action: string | undefined): VerdictTone {
  if (!action) return "route";
  const key = action.trim().toLowerCase();
  if (APPROVE.has(key)) return "approve";
  if (REJECT.has(key)) return "reject";
  if (CLARIFY.has(key)) return "clarify";
  return "route";
}

/**
 * Human-facing label for a decide action: snake_case → "Title case".
 * `respond_direct` → "Respond direct", `dev_via_test` → "Dev via test".
 */
export function verdictLabel(action: string | undefined): string {
  if (!action || action.trim() === "") return "decided";
  const spaced = action.trim().replace(/_/g, " ");
  return spaced.charAt(0).toUpperCase() + spaced.slice(1);
}
