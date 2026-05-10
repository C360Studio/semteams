# ADR-039: needs_clarification Recovery via Tiered Routing

## Status

**Proposed (2026-05-10).** Establishes that `needs_clarification` is
a recoverable terminal — not a wedge — and codifies the routing
model that gets a chain past it. Defines a three-tier recovery
hierarchy (rules → coordinator → human) with a structural
discriminator and explicit foreclosure on agent post-hoc mutation.

## Why this exists

Smoke #8 runs 9–12 wedged in four distinct ways. Three of the four
were `needs_clarification` terminals on different roles for
architecturally-distinct reasons. None had a recovery rule, so each
chain stopped before reaching qa-reviewer.

| Run | Wedge role | Cause class (full reason in `/tmp/smoke8-runN/trajectory-*.json`) |
|---|---|---|
| 9 | dev-via-spec-architect | Upstream-agent gap — researcher dropped `test_harness` field |
| 10 | dev-via-spec-builder | Operator/exogenous gap — catalog `meshtasticd:3.5.0` returned 404 from Docker Hub |
| 11 | (none — chain reached qa-reviewer cleanly) | Wire-format bug (rule subject mismatch); included for baseline completeness — not a needs_clarification case; closed by PR #117 |
| 12 | dev-via-spec-architect | Upstream-agent gap — researcher picked one `test_harness` but artifact has multiple integration boundaries (multi-boundary coverage gap) |

PR #115 closed run-9's specific shape via tool-layer Validate
("artifact must take a position on test_harness — pick one or flag
needs_test_harness"). That fix didn't generalise to run-12's
multi-boundary case. Each smoke revealed a deeper layer of the
verification-coverage discipline.

What every wedged run shares:

- A producer role observed a chain-coverage gap it cannot
  unilaterally fill
- It correctly emitted `decide(action="needs_clarification", ...)`
  (or `builder_decide(action="needs_clarification", ...)`) with a
  structured `reason` (sometimes also `blocking_question` or
  `retry_hint`, depending on producer)
- The framework had no rule listening for that terminal
- The chain stopped, reaching neither qa-reviewer nor the
  ops-chain-observer (PR #114) that would have surfaced the
  pattern as a diagnosis

`needs_clarification` is, by intent, the **recoverable terminal** in
the producer-role contract. It says "I have a verdict but it
requires information I don't have" — distinct from `failed`
(unrecoverable; chainpause owns it per ADR-037) and from `accept`
/ `reject` / `tests_passing` (verdicts that progress the chain).

The system-level gap: we built the producer-role contract for
needs_clarification (PR #113 architect, every dev-via-spec persona)
without building the consumer-side recovery primitive. Producer
emits the structured signal; nothing reads it.

## Decision

Commit to a **three-tier recovery hierarchy** with a structural
discriminator at the consumer end.

**Tier 1 covers KNOWN cheap shapes; Tier 2 (coordinator) absorbs
the long tail.** Adding a new Tier 1 rule for every novel shape is
an anti-pattern — novel shapes go to Tier 2 by default. The rule
set stays bounded; coordinator quality is what scales. If
coordinator can't handle a case, that's a coordinator-persona
improvement signal, not a new-rule signal.

### What rules fire on (the wire-format basis)

When a producer terminates with
`decide(action="needs_clarification", reason=R, retry_hint=H?, blocking_question=Q?)`,
the framework writes triples on the producer's loop entity:

- `coordinator.next_action = "needs_clarification"` (always)
- `coordinator.decision_reason = R` (always)
- `coordinator.retry_hint = H` (when the producer's tool schema
  includes the field — already shipped in dev-via-spec architect's
  decide and dev-via-spec-builder's builder_decide as of PR #113;
  qa-reviewer's decide already supports it as the
  blocking-question companion)
- `coordinator.blocking_question = Q` (when the producer is
  builder_decide; signals "operator-actionable" by shape)

Rules fire on the triples (entity-state pattern, same as every
working dev-via-spec rule), not on the tool call directly. The
existing `decide` and `builder_decide` schemas already carry the
fields ADR-039 needs; no upstream tool-schema changes required.

### Tier 1 — Rules (deterministic, default)

When the producer's terminal carries a `coordinator.retry_hint`
naming a specific upstream role and the chain has a known recovery
shape, a rule fires on the terminal and spawns the recovery loop.
No LLM in the loop; the routing is structural.

Discriminator: the producer-role × reason-pattern pair is in the
known set (see Phase 1 below for the initial three rules). The
architect's run-9 reason is the canonical fit — it names "the
researcher" as the role to retry and "test_harness selection" as
the gap. A rule can match that producer + pattern and spawn a new
researcher with the gap context.

### Tier 2 — Coordinator agent (configurable)

When the producer's terminal carries `coordinator.blocking_question`
(builder_decide's run-10 shape) OR Tier 1's rule set has no match
for the producer × reason pair, recovery routing requires
judgment: which role to re-spawn? Re-spawn at all? Escalate?
A coordinator agent reads the terminal, applies judgment, and
either spawns a recovery loop with custom context, proposes a
config change for human approval (per ADR-026's deploy surface),
or marks the chain `needs_human_attention` via Tier 3.

For smoke tests and read-only deployments, coordinator is the
default Tier 2 handler — keeps the loop self-contained and
demonstrable. Production deployments may default Tier 2 to human
escalation per operational policy.

### Tier 3 — Human escalation

When coordinator declines (out of scope, requires real-world
action) or is not configured, the chain stamps a new
`chain.needs_human.*` predicate cluster on the chain entity:

- `chain.needs_human.classification` — open-valued tag (e.g.
  `unrouted_needs_clarification`, `coordinator_declined`,
  `catalog_gap`)
- `chain.needs_human.producer_loop_id` — the loop that emitted
  the original needs_clarification
- `chain.needs_human.producer_role` — the role
- `chain.needs_human.reason` — the original `coordinator.decision_reason`
- `chain.needs_human.observed_at` — RFC3339 timestamp

This is **deliberately distinct from `chain.paused.*`** (ADR-037).
chain.paused is for FAILED loops with closed-enum classifications
(api_overloaded, max_iterations_exhausted, etc.) — it carries
failure semantics. needs_clarification is a recoverable verdict,
not a failure; conflating the two predicate clusters would
confuse downstream consumers (chainpause's existing handlers,
ops-chain-observer's diagnosis logic).

ops-chain-observer (PR #114) and any human-facing observability
surface read `chain.needs_human.*` triples and route to operator.

### The structural discriminator

```
producer emits decide(needs_clarification, reason=R, retry_hint=H?, blocking_question=Q?)

framework writes coordinator.next_action, coordinator.decision_reason,
                 coordinator.retry_hint?, coordinator.blocking_question?
                 on the producer's loop entity

if rule matches (producer_role, retry_hint or reason pattern):
    Tier 1 — rule fires, re-spawns named role with gap context
elif coordinator configured:
    Tier 2 — coordinator reads, decides
else:
    Tier 3 — chain.needs_human.* with classification=unrouted_needs_clarification
```

The producer doesn't choose the tier. Consumer-side configuration
does. This keeps the producer's contract stable as deployment
policy changes.

## Recovery is supersession, not mutation

**semstreams ADR-036** (proposed upstream — agent-private observable
state — see `/Users/coby/Code/c360/semstreams/docs/adr/036-agent-private-observable-state.md`,
not this repo's ADR-036 which covers test-harness lifecycle)
§Decision rule 3 says: *"authorised feedback flows via ADR-026's
deploy surface, never via direct mutation of live private state."*
The writer is the sole writer; readers don't write back into
agent-private state.

Three recovery shapes:

| Shape | Mechanism | Conformance with semstreams ADR-036 rule 3 |
|---|---|---|
| **A — supersede via new spawn** | Rule fires on needs_clarification; spawns new upstream role with gap context. New loop produces new artifact at new loop_id. Chain entity reference updates to new loop. Original loop entity remains immutable historical record. | ✅ No mutation; original loop's private state untouched. |
| **B — coordinator-decided supersession** | Coordinator agent reads terminal, decides which role to re-spawn (or escalate), spawns it. Same supersession mechanics as A. | ✅ Coordinator is a reader-then-spawner, not a mutator. |
| **C — agent edits prior work** | Coordinator (or some agent) rewrites the prior researcher's artifact triples to fill the gap. | ❌ **Forbidden.** Mutates agent-private state per semstreams ADR-036 rule 3. Recreates the writer-as-not-sole-writer Goodhart. |

This ADR codifies A and B; C is explicitly out of scope and any
future implementation that mutates a prior loop's triples must
re-litigate semstreams ADR-036's discipline.

The chain entity's `chain.<milestone>_loop` references update on
supersession (researcher_v1 → researcher_v2). Both loop entities
remain in the graph; only the chain's "current" pointer moves.
Same shape as how `chain.plan_loop` updates across rejected →
approved planner cycles in the existing dev-via-spec arc — the
pattern is established.

## What to build (slices)

### Phase 1 — Rules-only (Shape A) for known recovery shapes

Per-producer rules that fire on the
`coordinator.next_action="needs_clarification"` triple AND match a
known reason pattern. Each rule names the upstream role to
re-spawn and the gap context to forward via `related_loops` (the
canonical task-property channel established by D1+D2 work,
PRs #109 / #112).

Initial rules (in order of empirical priority from runs 9–12):

1. **architect-needs-clarification → researcher** — fires when
   `agent.loop.role = dev-via-spec-architect` AND
   `coordinator.next_action = needs_clarification`. Spawns a new
   researcher loop with the gap text in task properties. Handles
   run-9 + run-12 shapes.

2. **qa-reviewer-needs-clarification → architect** — fires when
   `agent.loop.role = dev-via-spec-qa-reviewer` AND
   `coordinator.next_action = needs_clarification`. Spawns a new
   architect loop with the qa-reviewer's `coordinator.decision_reason`
   as retry context. Handles the qa-side equivalent of run-9.

3. **builder-needs-clarification → Tier 3 (or Tier 2 if configured)** —
   fires when `agent.loop.role = dev-via-spec-builder` AND
   `coordinator.next_action = needs_clarification`. Builder's
   needs_clarification carries `coordinator.blocking_question` by
   shape — that signals "operator-actionable" (run-10's catalog
   gap is the canonical case). The rule writes
   `chain.needs_human.classification = catalog_gap` (or whatever
   the reason pattern matches) directly to the chain entity. If
   coordinator is configured (Phase 2), an additional rule on the
   `chain.needs_human.*` write spawns coordinator for triage; if
   not, the operator sees the triples directly.

   This rule is self-contained at Phase 1 even when Phase 2 doesn't
   exist: it writes a chain-level pause-shape triple cluster that
   ops-chain-observer / operator dashboards already know how to
   read. The Phase 2 coordinator is an additive consumer, not a
   prerequisite.

### Phase 2 — Coordinator agent (Shape B) for reasoning-required cases

Coordinator persona reads the needs_clarification terminal (or
the `chain.needs_human.*` triples Phase 1 rule #3 wrote), decides:
re-spawn upstream with custom context, propose a config change
(catalog edit) for human approval, or mark `needs_human_attention`
via Tier 3.

Configurable per deployment: smoke tests default to coordinator
(self-contained demos); production may bypass coordinator and go
directly to Tier 3.

The coordinator persona shape, tools allowlist, and decision
contract are deferred to a separate ADR or persona-fragment design
slice. Phase 2 here is the architectural commitment that the tier
exists; the contents are open.

### Phase 3 — Human escalation via chain.needs_human.*

Already covered structurally in Phase 1 rule #3 and the Tier 3
spec above. No additional engineering for the predicate cluster
beyond the rule writes; observability surfaces (ops-chain-observer,
operator dashboards) consume the triples by reading the graph.

### Phase 4 — Stamper supersession (independent prerequisite)

Independent of recovery routing, the
ResearchMilestoneStamper / PlanMilestoneStamper / etc. need to
re-fire on subsequent approvals so chain entity references stay
current across multi-pass arcs (today's stampers fire once per
chain on first approval). Two design directions:

- **Always-emit contract**: change researcher persona / tool to
  always call `emit_research_artifact` on completion, even if same
  content. Stabilization pass re-emits. Latest approved researcher
  always has the data. Persona change.
- **Walk-to-emitter**: stamper queries researcher's loop entity;
  if no `research.artifact.*` triples, walks `lineage.researcher`
  backward to find the loop that did emit. Stamper change.

Smoke #8 run-10 surfaced this: the
ResearchMilestoneStamper fired on a stabilization-pass researcher
that re-queried without re-emitting; chain reference pointed at a
loop with no artifact triples. Same root cause class as
needs_clarification recovery (multi-pass arcs need supersession-
aware stamping); pick one direction in a follow-up design slice.

This phase is independent of Phases 1–3; can land in any order.

## What this ADR does NOT decide

- **The multi-boundary test_harness coverage shape.** Run-12
  evidence shows `research.Artifact.TestHarness` (single string)
  doesn't cover the case where one artifact has N integration
  boundaries needing N test_harnesses. Two design paths exist
  (per-boundary harness mapping vs harness list); both are real
  changes to the artifact schema. The recovery design works
  regardless: when architect emits needs_clarification on this
  shape today, Tier 1 re-spawns the researcher with the gap
  context. A future schema change closes the gap upstream so the
  architect doesn't reach for needs_clarification at all.

- **The coordinator persona shape, tools allowlist, decision
  contract.** Phase 2 commits the architectural tier; the contents
  defer to a follow-up ADR or persona-fragment design slice. The
  read-only Phase 2 behavior (reads triples, spawns recovery loops,
  proposes ADR-026 deploy-surface changes for human approval) is
  the policy boundary; the persona prose is implementation.

- **Per-deployment configuration mechanics.** "Coordinator default
  for smoke; human default for production" is the policy; the
  config knob (rule-set selection, coordinator presence flag,
  per-tier escalation rules) is implementation detail.

- **Stamper supersession direction (always-emit vs walk-to-emitter).**
  Both are viable. Phase 4 picks one in a follow-up.

## Guarding against lazy needs_clarification

The risk: `needs_clarification` is a structurally valid terminal,
and a recovery primitive makes it a *cheap* terminal — an LLM under
pressure could reach for it whenever a task gets moderately hard,
turning the recoverable verdict into the easiest path to "I'm
done." Producer-side persona-prose enforcement ("only emit when
truly blocked") is exactly the discipline pattern that didn't
survive Goodhart pressure in earlier slices, so the same frame
won't survive here.

Three guards address the abuse vector. None are silver bullets;
together they discipline the producer's incentive over time.

### Coordinator as quality gate, not just router

When Tier 2 fires, coordinator can route in three directions
(re-spawn upstream, escalate to human, propose config change) —
AND a fourth: **re-spawn the SAME producer with "your prior
needs_clarification was insufficient — commit or be more
specific."** This makes coordinator a quality gate that pushes
back on lazy emissions instead of accepting them. Costs an extra
LLM hop per case but disciplines the producer's emission rate over
time. The decision is the coordinator's: "is this a substantive
gap or a give-up?" If the producer's reason doesn't name a
specific role to retry, doesn't name a specific field that's
missing, or doesn't justify why the gap blocks progress,
coordinator sends it back.

### Telemetry via ops-chain-observer

Per-role `needs_clarification` rate is a tracked metric. If a
producer hits >X% of terminals as `needs_clarification` (threshold
TBD with empirical evidence), ops-chain-observer emits an
`ops.diagnosis.persona_drift_suspected` finding for human review.
Catches abuse patterns across runs without blocking individual
chains. Same shape as the smoke #8 stabilization-pass observation
in run-10 — ops sees patterns that single-chain producers can't.

### Producer-side specificity validation (cautious; consider Phase 5+)

Tool-layer Validate could enforce minimum-specificity heuristics
for `coordinator.decision_reason` when `coordinator.next_action =
needs_clarification` (must reference a role, must reference a
field, length floor). This is the structural-over-LLM pattern
PR #113 + PR #115 leaned on. **Caution:** false positives make the
producer loop on validation errors instead of doing the work,
which is its own Goodhart trap. Likely better as a coordinator-side
heuristic than a hard tool gate; mark as Phase 5+ pending evidence
that Phase 1+2 isn't enough.

### Why coordinator quality is the load-bearing investment

If coordinator handles the long tail with judgment (substantive vs
give-up; route or send back), Tier 1 stays bounded and producers
get disciplined incrementally. If coordinator can't make those
distinctions, the system either wedges (lazy needs_clarification
goes nowhere useful) or gets noisy (everything escalates to human).
Either way the symptom looks like "needs_clarification doesn't
work"; the root cause is coordinator capability. Phase 2's coordinator
persona shape is therefore the single highest-leverage decision
in this ADR's implementation arc — more than the rule shapes,
more than the predicate clusters. Treat it as such.

## Consequences

**Positive:**

- Producer roles get a usable recovery primitive instead of a
  silent wedge. The needs_clarification contract pays its rent.
- The tiered structure keeps cheap (rule) routing for known shapes
  and pays the LLM cost (coordinator) only when judgment is
  required. Avoids the Stanford-Meta-Harness "everything goes
  through the LLM" trap.
- Supersession-not-mutation preserves semstreams ADR-036's
  discipline. The chain's history remains immutable; corrections
  produce new loops with new IDs, observable in the graph as "v1
  was superseded by v2" via chain entity reference movement.
- The ADR explicitly forecloses Shape C, so future agents/humans
  reading the codebase don't reach for "easier" mutation
  workarounds without re-litigating the discipline.
- Tier 3 introduces `chain.needs_human.*` as a distinct predicate
  cluster from chain.paused.* — keeps failure semantics
  (chain.paused, ADR-037 closed-enum classifications) distinct
  from recoverable-pending semantics (chain.needs_human, open-valued
  classifications).

**Negative:**

- Tier 1 rule maintenance grows per producer role × per reason
  pattern. Mitigation: most producers have 1–2 reason shapes worth
  handling; the rule set stays bounded. If it doesn't, that's a
  signal to graduate the case to Tier 2.
- Tier 2 coordinator design surface area is real. Phase 2 will
  need its own ADR (or addendum) once we have empirical signal
  from Phase 1 + Tier 3 deployments.
- Stamper supersession is a real change to the milestone-stamping
  contract; a future PR has to land it carefully (existing one-shot
  consumers depend on the current behavior).
- A new predicate cluster (`chain.needs_human.*`) means
  ops-chain-observer's persona fragment needs to know about it
  alongside `chain.paused.*` — a small persona update when Phase 1
  ships.
- **The system's quality on needs_clarification recovery is bounded
  by coordinator quality.** If coordinator can't distinguish
  substantive needs_clarification from lazy emissions, OR can't
  make smart routing decisions across the long tail of novel
  shapes, the whole tier structure degrades: lazy emissions go
  nowhere useful (silent wedge from a different cause) or
  everything escalates to human (noisy). This is the
  load-bearing investment for ADR-039 to deliver value — see
  §"Guarding against lazy needs_clarification" for the discipline
  story. Phase 2's coordinator persona is therefore the highest-
  leverage single decision in the implementation arc.

**Neutral:**

- Per-deployment configurability adds an operator-facing knob.
  Documented as part of the operator runbook for ops-chain-observer
  / coordinator deployment. Default policy stays explicit per
  deployment kind.

## Relationship to other ADRs

- **ADR-027 (ops agent meta-harness)** — Phase 2 coordinator role
  is a sibling pattern to ops-analyst (sampling) and
  ops-chain-observer (per-chain detail). The recovery routing is
  the consumer-side of needs_clarification that ADR-027's
  diagnosis stream complements (ops surfaces patterns; coordinator
  acts on individual instances).
- **ADR-033 (harness-anchored verification and coordinator
  authority)** — §3 establishes coordinator decision authority over
  the chain. ADR-039's Phase 2 coordinator agent implements that
  authority for the needs_clarification case specifically. The
  authority model is established; the application is the slice.
- **ADR-035 (dev-via-spec arc)** — codifies the producer-side
  needs_clarification contract per role (architect's
  30-commitment-contract.md, qa-reviewer's 10-evaluation-contract.md).
  ADR-039 is the consumer-side counterpart.
- **semstreams ADR-036 (proposed — agent-private observable state)** —
  Rule 3 is the load-bearing constraint on Shape C. This ADR makes
  the foreclosure explicit in the recovery design. (Note: NOT
  this repo's ADR-036, which covers test-harness lifecycle —
  unrelated concern.)
- **ADR-037 (chain-pause primitive)** — chain.paused.* triples are
  reserved for FAILED loops with closed-enum classifications.
  ADR-039 deliberately introduces a new chain.needs_human.* cluster
  rather than extending chain.paused, to keep failure semantics
  separate from recoverable-pending semantics.
- **ADR-038 (chain entity)** — chain entity reference updates on
  supersession depend on chain entity being the canonical
  cross-arc substrate. No conflict; this ADR just adds a write
  pattern (re-stamp on supersession).

## References

### Empirical baseline (smoke #8 runs 9–12)

| Run | Outcome | Evidence file |
|---|---|---|
| 9 | architect needs_clarification (researcher dropped test_harness) | `/tmp/smoke8-run9/triples.json`, `/tmp/smoke8-run9/trajectory-b85dfdec-*.json` |
| 10 | builder needs_clarification (catalog `:3.5.0` not on Docker Hub) | `/tmp/smoke8-run10/trajectory-62211137-*.json` |
| 11 | chain reached qa-reviewer; ops rule subject was wrong | `/tmp/smoke8-run11/run.log` (closed by PR #117) |
| 12 | architect needs_clarification (multi-boundary coverage gap) | `/tmp/smoke8-run12/trajectory-69f75f74-*.json` |

Three architecturally-distinct cause classes across runs 9, 10, 12;
one wire-format bug in run 11 (closed separately). Pattern stable
enough to design routing against.

### Cross-referenced ADRs

- ADR-027 — ops agent meta-harness (per-flow sampling vs per-chain detail)
- ADR-033 — harness-anchored verification and coordinator authority
- ADR-035 — dev-via-spec arc (producer-side needs_clarification contract)
- semstreams ADR-036 (proposed) — agent-private observable state (Rule 3 forecloses Shape C)
- ADR-037 — chain-pause primitive (chain.paused.* reserved for FAILED loops)
- ADR-038 — chain entity (cross-arc reference substrate)

### Related code surfaces

- `/Users/coby/Code/c360/semteams/configs/personas/fragments/dev-via-spec-architect/30-commitment-contract.md` — architect's needs_clarification producer contract
- `/Users/coby/Code/c360/semteams/configs/personas/fragments/dev-via-spec-builder/20-builder-decide-contract.md` — builder_decide producer contract
- `/Users/coby/Code/c360/semteams/cmd/semteams/chainpause/pauser.go` — ADR-037 chain.paused.classification closed enum (reference for why ADR-039 doesn't reuse it)
- `/Users/coby/Code/c360/semteams/cmd/semteams/chain/research.go` — ResearchMilestoneStamper (Phase 4 supersession target)
