# agent-run substrate rule pack (ADR-053 Phase 4a)

Substrate-level rule pack that advances the **run** (`AgentRun`, entity
`agent.chain.execution.<runID>`) through its lifecycle phase machine.
Loaded by the bootstrap alongside `coordinator`/`ops` — NOT a category
pack (it is pack-agnostic; one set of rules serves research,
autoresearch, and dev-via-test).

See [docs/adr/053-adoption-plan.md](../../../docs/adr/053-adoption-plan.md)
§"Phase 4 design spike" for the full design + the architect/Coby review
trail.

## The firing-entity constraint

`lifecycle_transition` acts on the **firing entity**, so a run-phase
transition rule must fire **on the run entity**. The coordinator/loop
terminal rules fire on *loop* entities, so each transition is a **2-step
marker**: a loop-firing rule stamps a marker on the run entity (via the
`$entity.triple.<anchor>` subject override), and a run-entity-firing rule
matches the marker and transitions.

## Rules

| File | Fires on | Does |
|---|---|---|
| `01-handoff-marker.json` | dispatch coordinator loop | On **confirmed handoff** (`rule.task.spawned` present — post-publish-success, NOT bare mint) stamps `agent.run.handoff` on the run entity. Publish-failure-safe: no handoff marker → run stays `dispatched` → rules 05/06 stamp failed + rule 04b fails it (beta.160: the upstream D3 subscriber guard is deleted; the subscriber is observe-only). |
| `02-dispatched-to-executing.json` | run entity | `agent.run.handoff` + `phase==dispatched` → `executing`. |
| `03-executing-to-completed.json` | run entity | `agent.run.outcome==success` + `phase==executing` → `completed`. |
| `04-executing-to-failed.json` | run entity | `agent.run.outcome==failed` + `phase==executing` → `failed`. The symmetric twin of `03`. (Phase 4a′.) |
| `05-coordinator-failed-run-anchor.json` | coordinator loop | A coordinator that fails (`outcome in [failed,truncated]`) **while executing** and carries `agent.run.entity-id` (lineage absent) stamps `agent.run.outcome=failed` on the run entity. The executing-phase zombie class (rule 04 consumes the stamp; dispatched-phase goes to rule 04b). (Phase 4a′.) |
| `06-coordinator-failed-lineage-anchor.json` | coordinator loop | Same, for coordinators carrying `agent.lineage.run-loop-entity-id` (dev-via-test 02b/02d/05/07d + the 4b-1a threaded recovery coordinators autoresearch/10b, dev-via-test 02e/02f-replan/07b/07e) — fenced on `length_gt 0`, stamps via the lineage anchor. Mutually exclusive with `05`. (Phase 4a′ + 4b-1a.) |
| `07-ask-user-pause-run-anchor.json` | coordinator loop | An in-run coordinator emitting `decide(ask_user)` and carrying `agent.run.entity-id` (lineage absent) stamps `agent.run.clarification-pending=$entity.instance` on the run entity. The interactive-pause marker, anchor branch 1. (Phase 4b-2.) |
| `08-ask-user-pause-lineage-anchor.json` | coordinator loop | Same, for `decide(ask_user)` coordinators carrying `agent.lineage.run-loop-entity-id` (`length_gt 0`) — stamps via the lineage anchor. Mutually exclusive with `07`. (Phase 4b-2.) |
| `09-executing-to-awaiting-on-clarification.json` | run entity | `agent.run.clarification-pending` + `phase==executing` + **`clarification_resumed` absent** → `awaiting_approval`. The run is honestly paused on a human reply. The `clarification_resumed length_eq 0` guard (added in PR-2) is the bounce-proof mutual-exclusion with the resume. (Phase 4b-2.) |
| `10-clarification-reply-resume-marker.json` | reply coordinator loop | A coordinator re-entering as a clarification REPLY (`agent.run.entity-id` + `agent.loop.reply-to` both present) stamps `agent.run.clarification-resumed=$entity.instance` on the run entity. Resume marker, half 1 — single rule (no anchor split: semstreams#256 threads the run anchor onto the reply). **Active on beta.106.** (Phase 4b-2 PR-2.) |
| `11-resume-awaiting-to-executing.json` | run entity | `agent.run.clarification-resumed` + `phase==awaiting_approval` → `executing`, then `remove clarification_pending`, then `remove clarification_resumed` (in that order). Resume transition + marker-clear, half 2. **Active on beta.106.** (Phase 4b-2 PR-2.) |

### Interactive pause + resume (4b-2)

`decide(ask_user)` from a coordinator that belongs to a run pauses the run
`executing→awaiting_approval` (rules `07`/`08` markers → `09` transition), rather
than misleadingly staying `executing` while blocked on a human. Same anchor-pair
shape as the coordinator-failed pair (`05`/`06`): the recovery coordinators that
`ask_user` carry EITHER `agent.run.entity-id` (inherit — research/06,
autoresearch/10 baseline, dev-via-test/02f first-pass → rule `07`) OR
`agent.lineage.run-loop-entity-id` (threaded — autoresearch/10b, dev-via-test
02e/02f-replan/07b/07e → rule `08`).

The **front-door** `ask_user` (an ambiguous FIRST turn) does NOT pause — no run is
minted, so the run-anchor guard makes `07`/`08` a no-op. Only an in-flight run that
needs clarification pauses.

`awaiting_approval` is **shared with 4c**'s `approval_required` tool-gate;
distinguished by CAUSE, not phase — 4b-2 sets `agent.run.clarification-pending` +
the asking loop has `coordinator.clarification.question` and no `pending_approval`; 4c sets
`pending_approval`. The deferred UI follow-up disambiguates on those.

**Autonomous mode is inert by construction**: under
`restricted_decide_actions:["ask_user"]` the framework rejects `decide(ask_user)`
before the decision triple is stamped, so `07`/`08` (gated on that triple) never
fire. Pinned by `TestAgentRunPack_AskUserPauseAutonomousInert`.

#### Resume (PR-2 — rules `10`/`11`, ACTIVE on semstreams beta.106)

When the operator answers, the reply re-enters as a fresh coordinator loop. Rule
`10` fires on that reply loop and stamps `agent.run.clarification-resumed` on the
run; rule `11` fires on the run entity and transitions `awaiting_approval→executing`,
then removes both clarification markers. Mirror of the pause pair, but a **single**
marker rule (`10`) rather than a `07`/`08` anchor split — semstreams#256 threads
the run anchor onto the reply, so the reply loop always carries
`agent.run.entity-id`; the `agent.loop.reply-to` discriminator is what separates a
reply from any other run coordinator.

**Bounce-proof without an atomic-write primitive.** The naïve risk is that rule
`11`'s transition back to `executing` re-trips rule `09` (`clarification_pending` is
still set for an instant) → infinite pause↔resume bounce. The fix is *structural
mutual-exclusion*, not timing: rule `09` carries a `clarification_resumed length_eq
0` guard, and rule `11` removes `clarification_pending` **before**
`clarification_resumed`. At every intermediate KV revision rule `09` is blocked by
*either* `clarification_resumed` still being set *or* `clarification_pending`
already gone — so no bounce is possible regardless of the engine's re-eval
granularity. Pinned by `TestAgentRunPack_ClarificationResume{Marker,Transition}` +
`TestAgentRunPack_PauseResumeBounceGuard`.

**Active on semstreams beta.106 (PR #261).** The reply path now threads both the
run anchor (`HTTPMessageRequest.run_id → TaskMessage.RunID → agent.run.entity-id`)
and the reply identity (`in_reply_to → TaskMessage.InReplyTo → agent.loop.reply-to`,
a 6-part loop entity ref via `agvocab.LoopReplyTo`). `buildSpawnIdentityTriples`
stamps both on the reply loop **at spawn** — before any LLM call — so rules `10`/`11`
fire on loop creation. The maintainer chose the **typed** `agent.loop.reply-to`
predicate over a third `lineage.*` overload, so rule `10`'s discriminator keys on it
directly (no lineage-namespace reuse). Pinned **structurally** by the
`agent_run_pack` contract tests (resume marker + ordered marker-clear + the rule-09
bounce guard) **and behaviorally** by the `clarification-resume` mock journey
(`ui/e2e/agentic/clarification-resume.spec.ts` + `test/fixtures/journeys/
clarification-resume.yaml`), which drives the real dispatch `/message` reply path
with `run_id` + `in_reply_to` (no seeding) and asserts
`awaiting_approval→executing` + both markers cleared + no re-pause (the run stays
`executing` — a plain coordinator `respond_direct` stamps no `agent.run.outcome`). A
production human-facing reply affordance (surface `coordinator.clarification.question`,
capture the free-text answer, POST the anchors) is a separate UI follow-up slice.

### Tool-gate pause + resume (4c — rules `12`/`13` + the `approvalpause` subscriber)

The run-phase reflection of the existing **loop-level** `approval_required`
tool-gate (ADR-030). When a loop calls a tool listed in
`agentic-tools.approval_required`, upstream rejects the call with the
`approval_required:` prefix and the loop transitions to
`LoopStateAwaitingApproval`, emitting `agent.approval_pending.<loopID>`. The loop
pauses today, but the RUN stayed misleadingly `executing` — the same honesty gap
4b-2 closed for clarification.

Unlike 4b-2's clarification (which had a persona-decision triple for rules
`07`/`08` to fire on), the tool-gate pause is **in-memory loop state with no graph
triple**. So the product-shell subscriber `cmd/semteams/approvalpause` consumes the
`agent.approval_pending` wire event, graph-reads the gated loop's run anchor
(`agent.run.entity-id`, falling back to `agent.lineage.run-loop-entity-id` — the 4b-1a
inherit/threaded split, **absorbed in Go** so there is no `07`/`08`-analog rule),
and stamps `agent.run.approval-pending` on the run entity. Rule `12`
(`approval_pending` + `phase==executing` top-level guard + the bounce-proof
`approval_resumed length_eq 0` guard) is the marker→transition half — a direct
mirror of rule `09`. A run-less gated loop (front-door coordinator) has no anchor
and is a no-op, exactly mirroring the front-door `ask_user`.

**Disjoint from 4b-2.** Both pauses land in `awaiting_approval`; they never
cross-resume because their cause-markers are distinct predicates
(`approval_pending`/`approval_resumed` vs `clarification_pending`/
`clarification_resumed`). Pinned by the `agent_run_pack` disambiguation tests.
Prod (`flow-bootstrap.json`) sets **no** `approval_required` (autonomous), so the
subscriber + rules are inert there; the `e2e-flow-bootstrap.json` gates `create_rule`
to drive the `approval-pause` + `approval-resume` mock journeys.

**Resume (4c PR-2 — rule `13`).** When the human approves/rejects/modifies via
`POST /teams-dispatch/loops/{id}/approval`, dispatch publishes
`agent.approval_response.<loopID>`. TWO consumers react: (1) the teams-loop
`agent.approval_response` **input port** (declared for PR-2 — without it the loop
never subscribes and can NEVER resume) routes to `ResolveApprovalIfPending`, which
un-parks the loop; (2) the `approvalpause` subscriber stamps
`agent.run.approval-resumed` on the run entity. Rule `13` (`approval_resumed` +
`phase==awaiting_approval` top-level guard) does, IN ORDER: transition
`awaiting_approval→executing`, remove `approval_pending`, remove `approval_resumed`.
The marker is **decision-independent** — approve, reject, AND modify all un-park the
loop. **Bounce-proof** the same way as 4b-2: rule `12`'s `approval_resumed length_eq
0` guard + rule `13`'s ordered removes make rule `12` dead at every intermediate KV
revision. Pinned by `TestAgentRunPack_ApprovalResumeTransition` +
`ApprovalMarkerSubscriberParity` + the `approval-resume` journey (POSTs the real
approval, asserts `awaiting_approval→executing` + both markers cleared + no bounce).

### Coordinator-failure coverage boundary (4a′ / 4b-1a)

Not every coordinator that *could* fail needs an executing→failed rule. The
disposition of every coordinator-spawn rule is pinned by
`test/contract/agent_run_pack_test.go`
(`TestAgentRunPack_CoordinatorSpawnCoverage`) — an unclassified new
coordinator-spawn rule fails that test, so the boundary can't drift silently:

- **post-approval wake-ups** (`research/07`, `autoresearch/08`,
  `dev-via-test/07a`) — stamp `agent.run.outcome=success`, so rule 03 completes
  the run *before* the woken coordinator runs; a later failure is post-terminal.
  No coverage needed.
- **anchor_inherit** → rule 05 (`agent.run.entity-id` via the loop-spawn inherit
  chain — the firing role carries a bare `agent.run`): `research/06`,
  `autoresearch/10` (baseline), `dev-via-test/02f` (first-pass Lisa).
- **anchor_threaded** → rule 06 (`agent.lineage.run-loop-entity-id` threaded onto the
  woken coordinator — the firing role descends through a run-entity-fired rule
  and carries no bare `agent.run`): the intermediate/escalate coordinators
  dev-via-test `02b`/`02d`/`05`/`07d`, plus the **4b-1a** recovery coordinators
  `autoresearch/10b` and dev-via-test `02e`/`02f-replan`/`07b`/`07e`.
- **deferred_4b (now empty)** — ADR-053 Phase 4b-1a threaded an anchor onto every
  run-entity-descended `ask_user`/`needs_clarification` recovery coordinator, so
  the zombie hole is closed: no coordinator-spawn path lacks executing→failed
  coverage. The disposition class is retained in the test only for a future pack
  that might introduce a new deferred path before wiring its anchor.

The success outcome (`agent.run.outcome=success`) is stamped by the three
reviewer/CBG-**approved** terminal rules (`research/07`, `autoresearch/08`,
`dev-via-test/07a`) — NOT by `respond_direct` (which also fires on the
"cannot be served" limitation path). The stamp subject is **per-pack**:
research uses `agent.run.entity-id`; autoresearch + dev-via-test descend
through run-entity-fired rules so carry only `agent.lineage.run-loop-entity-id`.

The failure outcome (`agent.run.outcome=failed`, Phase 4a′) is stamped by
the **coordinator-failed** rules here (`05`/`06`) plus the per-pack
**loop-failed run-outcome** rules that live in each category pack:
`research/09`, `autoresearch/12`+`13`, `dev-via-test/09`+`10`. Each fires on
a non-budgeted role's involuntary loop-failure (`outcome in [failed,
truncated]`) and stamps `agent.run.outcome=failed` on the run entity with
the **same per-pack anchor** as the success path (the `run_scope=new`
first-pass children — `autoresearch-baseline`, first-pass Lisa — split onto
`agent.run.entity-id`; the run-entity-descended roles onto
`agent.lineage.run-loop-entity-id`). The existing `*/08`/`*/11` `chain.paused.marker`
rules (operator surface) are left untouched — these run-outcome rules are
their run-lifecycle companion. **Budgeted** loop-failures
(`autoresearch-execute` via `04b`; `dev-via-test-execute`/Ralph via `04b`+`05`)
are deliberately absent from every failed-stamp role list — a budgeted
failure keeps the run `executing` (counter increments / coordinator→ask_user
recovery). `cancelled` is out of 4a′ scope (→ `cancelled`, Phase 4b).

## Load-bearing invariants (pinned by `test/contract/agent_run_pack_test.go`)

- The phase guard is a **top-level rule condition**, never an action
  `when`. A `when`-buried guard would let the rule enter on the marker
  alone, skip the action in the wrong phase, and never re-enter (lost
  transition). With a top-level condition the rule is simply not-matching
  until the edge is legal; the engine re-evaluates every rule per KV
  revision (`message_handler.go:245`), so a durable marker re-drives the
  transition exactly once when the phase advances.
- `dispatched→executing` triggers on **confirmed handoff**, not bare mint.
- `executing→completed` is **outcome-driven**, decoupled from
  `respond_direct`.
- `executing→failed` (Phase 4a′) is **outcome-driven** too, scoped to the
  involuntary-failure class (`outcome in [failed,truncated]`); budgeted
  failures (`autoresearch-execute`, `dev-via-test`/Ralph) are excluded and a
  `cancelled` loop is left to Phase 4b. Pinned by
  `test/contract/agent_run_pack_test.go`
  (`FailedOutcomeStamps`/`FailedStampAnchorGuarded`/`BudgetedRolesExcludedFromFailedStamp`).

## Not yet here

- **A production human-facing reply affordance** — 4b-2 resume (rules `10`/`11`) is
  ACTIVE on beta.106 and exercised by the `clarification-resume` mock journey via the
  direct dispatch `/message` API, but no UI surface yet renders a
  `coordinator.clarification.question` with a free-text answer box that POSTs `run_id` +
  `in_reply_to`. That is a separate UI follow-up slice (it pairs with surfacing
  `awaiting_approval`).
- `cancel` semantics — an abandoned clarification parks in `awaiting_approval`
  with no terminal-timeout; the cancel/timeout slice (likely an upstream D3
  widening to the `executing`/`awaiting_approval` phase) is deferred (4b).
- **A production human-facing approval affordance for the run-phase pause** — the
  loop-level approval UI (PendingApprovalSection + `POST /loops/{id}/approval`)
  exists (ADR-030); the 4c run-phase pause/resume (rules `12`/`13`) is backend-only.
  Surfacing `awaiting_approval` at the RUN level pairs with the deferred 4b-2 reply
  affordance (a UI slice). The dev-via-test CBG gate is an automated reviewer and
  stays `executing` (not an `approval_required` tool-gate).
- Product `MilestoneHandler`s (Phase 5 re-platforms the chain stampers).
