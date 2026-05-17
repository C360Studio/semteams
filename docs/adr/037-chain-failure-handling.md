# ADR-037: Chain Failure Handling and Decision Authority

## Status

**Proposed (2026-05-06).** Establishes the chain-pause primitive smoke
#8 surfaced as missing. Composes with ADR-030 (operator approval
surface) and reserves seams for ADR-033 (coordinator authority) and
upstream ADR-027 (ops-agent pattern aggregation).

## Why this exists

Smoke #8 (2026-05-06) wedged when `dev-via-spec-reviewer`'s first LLM
call returned Anthropic "Overloaded". The loop short-circuited to
`state=failed, outcome=failed, iterations=0` and emitted no
`coordinator.next_action` triple. Every dev-via-spec rule triggers on
that triple, so no rule fired and the chain died silently. Operators
detected the wedge only by counting expected loops and noticing the
absence.

Smoke #7 run-2 (2026-05-05) hit the same shape from a 180s sonnet
timeout. Two consecutive smokes, same wedge, same recurring
vulnerability: a single upstream API hiccup ends the chain with no
observable recovery path.

This ADR ships the substrate that lets operators handle the case
deterministically. The richer policy questions (auto-retry budgets,
coordinator authority over retries, cross-run pattern detection) are
named with explicit "earns its slot when" gates.

## Decision

### D1. Pause-and-alert primitive on `outcome=failed`

When `agent.loop.outcome == "failed"` for any loop whose `role`
matches a configured arc prefix (v1: `dev-via-spec-*` and the
research-arc roles `researcher`, `researcher-with-source-acquisition`,
`research-reviewer`), the chain pauses. A single `chain.paused`
triple is written. The operator approval surface (ADR-030) carries
the resulting decision request.

The trigger is a single rule pattern (`configs/rules/chain-failure/
01-pause-on-failed.json`) with a starts-with match on the role
prefix. No per-arc duplication. New arcs add their role-prefix to the
rule's match list, not a new rule.

The pause does not retry, kill, or escalate by itself. It writes the
audit triple and surfaces the decision in the approval queue. v1's
substrate is "make the failure visible and decidable"; the policy
that consumes it is operator-driven.

### D2. Decision-verb namespace (closed in v1)

A `chain.decision` triple carries the operator's resolution. The verb
namespace is a CLOSED enum in v1; the validator rejects any value not
in the v1 set with a structured error.

| Verb | v1 status | Effect |
|---|---|---|
| `retry` | shipped | Re-publish the original spawn-rule's TaskMessage with the same `prior_loop_id` pointer. Graph state intact; new loop reads context via `read_loop_result`. |
| `kill` | shipped | Write `chain.killed` triple. Chain terminates; no further rules fire on the failed lineage. |
| `defer` | shipped | Write `chain.deferred`. State preserved. Operator may re-issue a `retry` decision later. |
| `apply_fix_and_retry` | RESERVED (v2) | Validator rejects in v1 with a structured `decision verb not enabled in v1` error. |

**v1 validator behaviour.** Decisions arriving with unknown verbs (or
the reserved `apply_fix_and_retry`) MUST be rejected at the HTTP
boundary. The error response cites the closed-enum set. No silent
fallback, no opaque dispatch.

### D3. Decision-authority namespace (closed in v1)

A flow-level config field `chain_failure_authority` declares who owns
the resolution. v1 wires only `operator`. The other values are
RESERVED and the v1 validator rejects them at config-load time so
deployments declaring forward-looking values fail loudly rather than
silently degrading.

| Authority | v1 status | Resolution path |
|---|---|---|
| `operator` | shipped | Decision request lands in ADR-030 approval queue; human submits via `POST /loops/{id}/chain-decision` (mirrors `/approval` shape). |
| `coordinator` | RESERVED (v2) | Validator rejects in v1. v2 lifts the rejection; ADR-033 coordinator-authority block consumes it. |
| `auto` | RESERVED (v3) | Validator rejects in v1. v3 lifts the rejection if pattern data justifies. |

### D4. Reserved `fix.kind` enum (namespace locked in v1, no implementation)

The v2 `apply_fix_and_retry` verb's payload shape is locked now even
though no v1 code consumes it. Locking the namespace prevents v2
implementers from inventing ad-hoc `fix.kind` values that conflict
with the patterns ops-agent expects to aggregate.

```json
{
  "verb": "apply_fix_and_retry",       // RESERVED v1
  "fix": {
    "kind": "<one-of-closed-set>",
    "payload": { ... per-kind shape ... }
  }
}
```

Closed `fix.kind` set:

| Kind | Intended payload shape | Use-case sketch |
|---|---|---|
| `persona_hint_inject` | `{role, fragment_id, content}` | Inject a persona hint fragment for the retry; remove on success. |
| `config_bump` | `{component, field, value}` | Adjust a single config field (e.g. `max_iterations`, `temperature`) for the retry. |
| `persona_variant_swap` | `{role, variant_id}` | Swap to a curated alternate persona variant for the retry. |
| `tool_args_amend` | `{tool, arg_path, value}` | Amend a single tool-call argument shape ahead of retry. |

v1 ships none of these. The schema reservation is the contract that
keeps v2 implementations consistent and ops-agent's pattern
aggregation (D7) tractable.

### D5. Audit-trail triple schema (full schema in v1)

The full audit-trail triples are written in v1 even though only the
operator path consumes most fields today. Forward-compat for v2
ops-agent aggregation and v3 cross-run pattern detection is
load-bearing — pattern detectors need consistent data going back.

The pause writes:

```
<chain_id> chain.paused.cause           "<short-token>"
<chain_id> chain.paused.classification  "<api_overloaded|api_timeout|...>"
<chain_id> chain.paused.role            "<failed-loop-role>"
<chain_id> chain.paused.error_shape     "<sanitised-error-shape>"
<chain_id> chain.paused.prior_attempts  "<int>"
<chain_id> chain.paused.failed_loop_id  "<uuid>"
<chain_id> chain.paused.spawn_loop_id   "<uuid-or-null>"
<chain_id> chain.paused.observed_at     "<rfc3339>"
```

`cause` is a short token (`api_overloaded`, `api_timeout`,
`config_load_failure`, `tool_executor_panic`, `unknown`).
`classification` is a finer-grained tag for ops-agent pattern
matching. `error_shape` is a sanitised representative string (no
PII, length-bounded, control-chars stripped per ADR-030 §sanitisation
discipline).

The decision writes:

```
<chain_id> chain.decision.verb          "retry|kill|defer"
<chain_id> chain.decision.authority     "operator"
<chain_id> chain.decision.actor         "<X-User-Id-value>"
<chain_id> chain.decision.reason        "<free-text-bounded>"
<chain_id> chain.decision.decided_at    "<rfc3339>"
```

The terminal state writes ONE of:

```
<chain_id> chain.resumed                "<new-loop-id>"          // retry path
<chain_id> chain.killed                 "<rfc3339>"              // kill path
<chain_id> chain.deferred               "<rfc3339>"              // defer path
```

Future `apply_fix_and_retry` writes additional `chain.decision.fix.{
kind, payload}` triples; the schema reservation in D4 means the
predicate names are pre-claimed.

### D6. Cross-flow applicability — one rule, all chains

The pause primitive is flow-agnostic. The single rule pattern fires
on any failed loop whose role matches the configured prefix list.
Adding research-arc, dev-via-spec arc, future harness-via-spec arc,
dev-research, ops-agent observation arc requires extending the
prefix list — not duplicating the rule.

Lineage is walked from the failed loop's spawn metadata
(`prior_loop_id` chain). The `chain_id` is derived from the spawn
lineage (the topmost loop ID in the prior_loop_id chain), not a
separate identifier. See open question 1 below.

The rule's `on_enter` payload captures: failed loop ID, role, the
spawn rule that originated the loop (so `retry` can re-publish the
exact same TaskMessage shape), the chain_id, and the upstream error
metadata.

### D7. Resume mechanics — re-publish, no replay

`retry` re-publishes the original spawn rule's TaskMessage with the
same `prior_loop_id` pointer the failed loop carried. The new loop
gets a fresh loop ID, reads the same upstream context via
`read_loop_result(prior_loop_id)`, and proceeds. No state replay; no
mid-loop checkpoint.

For the dev-via-spec **builder** specifically: workspace state is
reset to the bootstrap output on retry. The simpler-and-wasteful
approach (option a from prior planning) suits v1. Smoke evidence on
how often retry-from-bootstrap is invoked, and what the wallclock
cost is, will tell us whether smarter strategies earn their slot
(deferral 5 below).

For non-builder roles (planner, reviewer, challenger, architect,
researcher, research-reviewer, qa-reviewer): no workspace coupling.
Retry re-runs the role with the same upstream input. Graph triples
the failed loop emitted (if any) remain — `read_loop_result` reads
the new loop's output going forward; old failed-loop triples are
audit-trail.

`kill` writes `chain.killed` and stops. No cleanup of partial
artifacts; the audit trail keeps everything for ops-agent
post-mortem.

`defer` writes `chain.deferred` and leaves the chain alone. Operator
re-issues a `retry` decision via the same approval surface to resume.
Defer is the "I'll come back to this" verb — operationally useful
when API capacity is the cause and the operator wants to wait for
it to recover before deciding.

## Vocabulary

This ADR uses post-rename vocabulary per ADR-036 §Vocabulary:
`check`, `runtime`, `ref`, `tasks`, `test_harness`. Where this ADR
introduces new vocabulary, the words are chosen to sit within that
table without colliding.

| Term | Meaning here |
|---|---|
| chain | The lineage of loops sharing a `chain_id` derived from spawn lineage. |
| pause | A non-terminal state where the chain awaits a `chain.decision`. |
| decision-verb | The closed-enum action operator/coordinator/auto resolves a pause with. |
| decision-authority | The role/policy that resolves a pause for a given flow. |

## What this ADR explicitly does NOT decide

Each deferral names an explicit "earns its slot when X" gate. These
are not wishlist items.

1. **`apply_fix_and_retry` decision verb (v2).** The fix-injection
   meta-harness verb. Schema reserved in D4.
   **Earns its slot when:** ops-agent has aggregated ≥2 runs of
   pattern data showing a recurring fix-shape across smokes. Without
   evidence of recurrence, every fix is a one-off and a
   coordinator/operator decision is the right surface.

2. **`coordinator` decision authority (v2).** ADR-033 §3 el-jefe
   path. Coordinator owns the resolution within an
   operator-configured allowlist (cause classes, retry budgets).
   **Earns its slot when:** v1 operator-only path has been exercised
   on real smokes for ≥4 weeks AND ops-agent's
   failure-pattern-aggregation primitive exists (deferral 4).

3. **`auto` decision authority (v3).** Pure deterministic policy
   without coordinator-in-the-loop.
   **Earns its slot when:** v2 has shipped AND coordinator decisions
   converge to deterministic policy across the same fix-shape ≥3
   times. May never earn its slot if v2 covers the case.

4. **Cross-run pattern detection (v3 — ops-agent / polars
   territory).** Upstream ADR-027 (ops-agent meta-harness) and
   upstream polars eval-harness ADR already framed the cross-run
   primitive. This ADR's audit-trail schema (D5) feeds it.
   **Earns its slot when:** ops-agent's polar library has at least
   one regime annotation where the failure-pattern axis is non-trivial
   (i.e., distinguishing `api_overloaded` from `api_timeout` produces
   different recommended decisions).

5. **Sandbox / workspace replay strategies for builder.** v1 hardcodes
   "reset to bootstrap on retry" for the builder.
   **Earns its slot when:** smokes show retry-from-bootstrap is
   wasting >5min wallclock per occurrence on average. Until then,
   simpler-and-wasteful is correct.

6. **Auto-retry budget without operator decision.** v1 always pauses;
   it never auto-retries. A future "retry once silently before
   pausing" optimisation is plausible but defers to evidence.
   **Earns its slot when:** smoke evidence shows >50% of failures are
   transient API hiccups that succeed on first retry, AND operator
   surveys show the always-pause cost is friction. v1's bias is
   visibility over latency.

## Relationship to prior ADRs

- **ADR-027 (Ops Agent — upstream).** Audit-trail schema in D5 is
  designed for ops-agent consumption. The `cause` and `classification`
  predicates are the cross-run aggregation axis ops-agent will pivot
  on. v1 ships the schema; ops-agent consumption ships when ops-agent
  has the polars regime work to use it.
- **ADR-030 (Approval Flow + X-User-Id).** Operator decision surface
  composes with ADR-030's approval queue. The `POST /loops/{id}/
  chain-decision` endpoint mirrors `/approval` — same identity seam
  (`X-User-Id` header, body fallback), same sanitisation discipline,
  same audit-trail predicate pattern (`chain.decision.actor` parallels
  `approval.actor`).
- **ADR-033 (Coordinator Authority + Multi-Arc).** D3's
  `coordinator` reserved value composes with ADR-033 §3's
  `coordinator_authority` config block. When v2 ships, the
  coordinator's chain-failure decisions are bounded by an
  operator-configured policy (cause-class allowlist, retry budgets,
  escalation conditions) in the exact shape ADR-033 already
  established for other coordinator decisions.
- **ADR-035 (Dev-via-Spec Arc, R3.5).** R3.5
  coordinator-as-meta-reviewer is the target hook for the v2
  `coordinator` authority. ADR-035's `decide(action="needs_clarification",
  ...)` surface and this ADR's `chain_failure_authority: coordinator`
  are two sides of the same primitive — explicit role-emitted
  ambiguity vs implicit infrastructure failure. The v2 implementation
  unifies them.
- **ADR-036 (Test-Harness Lifecycle).** Builder workspace reset on
  retry (D7) compose with ADR-036 Phase 1 `bootstrap_workspace`. The
  retry path re-runs `bootstrap_workspace`; no new code, just a
  re-invocation of the existing seam.

## Phasing

Each phase has one explicit gate. No phase ships without its gate
satisfied.

### v1 — Pause primitive, operator authority (this slice)

**Ships:** D1–D7. One rule under
`configs/rules/chain-failure/01-pause-on-failed.json`. One HTTP
endpoint `POST /loops/{id}/chain-decision`. One UI component
extending `PendingApprovalSection` to render chain-failure decisions.
Validator rejects all reserved enum values.

**Gate to declare v1 complete:** Smoke re-run that intentionally
injects an `outcome=failed` (e.g., set a low-budget API key for one
role to force overload) verifies the pause primitive surfaces, the
operator submits `retry`, and the chain resumes successfully. Audit
trail in graph carries every D5 predicate.

### v2 — `apply_fix_and_retry` verb, `coordinator` authority

**Earns its slot when:** ops-agent has aggregated ≥2 runs of
fix-pattern data AND v1 has been live for ≥4 weeks of real smokes.
Both gates are AND.

### v3 — `auto` authority, cross-run pattern detection

**Earns its slot when:** v2 has shipped AND coordinator decisions
have converged to deterministic policy across the same fix-shape ≥3
times AND upstream polars regime work has at least one non-trivial
failure-pattern regime annotation. All three gates are AND.

## References

- `/tmp/smoke8-run1/findings.md` — the trigger event.
- `~/.claude/projects/-Users-coby-Code-c360-semteams/memory/feedback_failed_loops_wedge_chain.md`
  — failure pattern this ADR addresses.
- `~/.claude/projects/-Users-coby-Code-c360-semteams/memory/project_strategic_pivot_2026_05_06.md`
  — strategic pivot context.
- `/tmp/smoke7-run2/findings.md` — the prior occurrence (same shape).
- [ADR-027 upstream](../../../semstreams/docs/adr/027-ops-agent-meta-harness.md)
  — ops-agent pattern; audit-trail consumer.
- [ADR-030](030-approval-flow-ui-and-identity.md) — approval-flow
  surface this composes with.
- [ADR-033](033-harness-anchored-verification-and-coordinator-authority.md)
  — coordinator-as-decision-authority hook for v2.
- [ADR-035](035-dev-via-spec-arc.md) — R3.5 needs_clarification
  routing; unifies with v2 coordinator authority.
- [ADR-036](036-test-harness-lifecycle.md) — `bootstrap_workspace`
  reused for builder retry workspace reset.

## Open questions

These could not be resolved from reading the referenced material;
they're the explicit hand-off list.

1. **`chain_id` derivation.** Is `chain_id` a top-level identifier
   minted at the dispatch boundary and propagated through every loop
   in the lineage, or is it derived at query time from walking the
   `prior_loop_id` chain to its root? Derivation is simpler (no new
   field on loop wire) but every audit query pays the walk cost.
   Top-level is cheaper to query but requires a wire-format change
   on the loop entity. Recommend: derive in v1, promote to top-level
   if ops-agent's pattern queries make the walk cost a constraint.
2. **Where does `chain_failure_authority` live?** Top-level flow
   JSON config (one-line declaration per flow), per-rule field on
   spawn rules (more granular but bookkeeping-heavy), or a separate
   `coordinator_authority.chain_failure` block (composes with ADR-033
   shape but adds one more nested level)? Recommend top-level flow
   config in v1, migrate into `coordinator_authority` when v2 lands
   so the policy is co-located with the rest of the coordinator's
   bounded autonomy.
3. **Triple predicate naming.** The schema in D5 uses dot-separated
   names (`chain.paused.cause`, `chain.decision.verb`). This matches
   the `agent.loop.outcome` and `coordinator.next_action` shape
   already in use. Confirm the predicate naming convention is
   followed; rename pre-merge if it diverges.
4. **Should the rule trigger AND on `state=failed` AND
   `iterations==0`, or just on `outcome=failed`?** A loop that ran
   successfully for 5 iterations and then failed on the 6th may
   still need pause-and-decide, but the workspace state is
   different. v1 recommend: trigger on `outcome=failed` regardless;
   the operator's retry-vs-kill-vs-defer decision absorbs the
   nuance. Re-evaluate if smoke evidence shows the iteration-N
   failure mode wants different default policy.
5. **Multi-failure within one chain.** What if two roles fail
   simultaneously (parallel branches in a future arc shape)? v1
   has no parallel arc shape today (ADR-033 §5 is sequential), so
   this is moot for v1. Defer with the parallel-arc work.
