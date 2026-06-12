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
| `01-handoff-marker.json` | dispatch coordinator loop | On **confirmed handoff** (`rule.spawned_task` present — post-publish-success, NOT bare mint) stamps `agent.run.handoff` on the run entity. Publish-failure-safe: no handoff marker → run stays `dispatched` → D3 fails it. |
| `02-dispatched-to-executing.json` | run entity | `agent.run.handoff` + `phase==dispatched` → `executing`. |
| `03-executing-to-completed.json` | run entity | `agent.run.outcome==success` + `phase==executing` → `completed`. |
| `04-executing-to-failed.json` | run entity | `agent.run.outcome==failed` + `phase==executing` → `failed`. The symmetric twin of `03`. (Phase 4a′.) |
| `05-coordinator-failed-run-anchor.json` | coordinator loop | A coordinator that fails (`outcome in [failed,truncated]`) **while executing** and carries `agent.run.entity_id` (lineage absent) stamps `agent.run.outcome=failed` on the run entity. The zombie D3 can't catch (D3 fires only while `dispatched`). (Phase 4a′.) |
| `06-coordinator-failed-lineage-anchor.json` | coordinator loop | Same, for coordinators carrying `lineage.run-loop-entity-id` (dev-via-test 02b/02d/05/07d + the 4b-1a threaded recovery coordinators autoresearch/10b, dev-via-test 02e/02f-replan/07b/07e) — fenced on `length_gt 0`, stamps via the lineage anchor. Mutually exclusive with `05`. (Phase 4a′ + 4b-1a.) |
| `07-ask-user-pause-run-anchor.json` | coordinator loop | An in-run coordinator emitting `decide(ask_user)` and carrying `agent.run.entity_id` (lineage absent) stamps `agent.run.clarification_pending=$entity.instance` on the run entity. The interactive-pause marker, anchor branch 1. (Phase 4b-2.) |
| `08-ask-user-pause-lineage-anchor.json` | coordinator loop | Same, for `decide(ask_user)` coordinators carrying `lineage.run-loop-entity-id` (`length_gt 0`) — stamps via the lineage anchor. Mutually exclusive with `07`. (Phase 4b-2.) |
| `09-executing-to-awaiting-on-clarification.json` | run entity | `agent.run.clarification_pending` + `phase==executing` + **`clarification_resumed` absent** → `awaiting_approval`. The run is honestly paused on a human reply. The `clarification_resumed length_eq 0` guard (added in PR-2) is the bounce-proof mutual-exclusion with the resume. (Phase 4b-2.) |
| `10-clarification-reply-resume-marker.json` | reply coordinator loop | A coordinator re-entering as a clarification REPLY (`agent.run.entity_id` + `lineage.clarification-reply` both present) stamps `agent.run.clarification_resumed=$entity.instance` on the run entity. Resume marker, half 1 — single rule (no anchor split: semstreams#256 threads the run anchor onto the reply). **Inert until #256.** (Phase 4b-2 PR-2.) |
| `11-resume-awaiting-to-executing.json` | run entity | `agent.run.clarification_resumed` + `phase==awaiting_approval` → `executing`, then `remove clarification_pending`, then `remove clarification_resumed` (in that order). Resume transition + marker-clear, half 2. **Inert until #256.** (Phase 4b-2 PR-2.) |

### Interactive pause + resume (4b-2)

`decide(ask_user)` from a coordinator that belongs to a run pauses the run
`executing→awaiting_approval` (rules `07`/`08` markers → `09` transition), rather
than misleadingly staying `executing` while blocked on a human. Same anchor-pair
shape as the coordinator-failed pair (`05`/`06`): the recovery coordinators that
`ask_user` carry EITHER `agent.run.entity_id` (inherit — research/06,
autoresearch/10 baseline, dev-via-test/02f first-pass → rule `07`) OR
`lineage.run-loop-entity-id` (threaded — autoresearch/10b, dev-via-test
02e/02f-replan/07b/07e → rule `08`).

The **front-door** `ask_user` (an ambiguous FIRST turn) does NOT pause — no run is
minted, so the run-anchor guard makes `07`/`08` a no-op. Only an in-flight run that
needs clarification pauses.

`awaiting_approval` is **shared with 4c**'s `approval_required` tool-gate;
distinguished by CAUSE, not phase — 4b-2 sets `agent.run.clarification_pending` +
the asking loop has `coordinator.user_question` and no `pending_approval`; 4c sets
`pending_approval`. The deferred UI follow-up disambiguates on those.

**Autonomous mode is inert by construction**: under
`restricted_decide_actions:["ask_user"]` the framework rejects `decide(ask_user)`
before the decision triple is stamped, so `07`/`08` (gated on that triple) never
fire. Pinned by `TestAgentRunPack_AskUserPauseAutonomousInert`.

#### Resume (PR-2 — rules `10`/`11`, inert until semstreams#256)

When the operator answers, the reply re-enters the coordinator loop. Rule `10`
fires on that reply loop and stamps `agent.run.clarification_resumed` on the run;
rule `11` fires on the run entity and transitions `awaiting_approval→executing`,
then removes both clarification markers. Mirror of the pause pair, but a **single**
marker rule (`10`) rather than a `07`/`08` anchor split — semstreams#256 threads
the run anchor onto the reply, so the reply loop always carries
`agent.run.entity_id`; the `lineage.clarification-reply` discriminator is what
separates a reply from any other run coordinator.

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

**Inert until semstreams#256.** The reply path currently drops both the run anchor
and `msg.Metadata` (`agentic-dispatch/component.go`), so a real reply loop carries
neither of rule `10`'s trigger fields — these rules cannot fire in production yet.
semstreams#256 ("make the HTTP reply path resumable") threads the run anchor +
reply identity onto the reply `TaskMessage`. Until then the slice is pinned
**structurally** by the `agent_run_pack` contract tests (resume marker + ordered
marker-clear + the rule-09 bounce guard). The **behavioral** `clarification-resume`
mock journey is DEFERRED to #256's landing — at which point the real reply path
produces the trigger triples, so the journey drives the resume for real (no
seeding; the e2e harness has no triple-write seam today). The
`lineage.clarification-reply` discriminator is the preferred
#256 Thread-2 shape (reuses the `related_loops → lineage.*` channel); if upstream
picks the typed `agent.loop.reply_to` alternative, rule `10`'s discriminator field
is a one-line swap.

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
- **anchor_inherit** → rule 05 (`agent.run.entity_id` via the loop-spawn inherit
  chain — the firing role carries a bare `agent.run`): `research/06`,
  `autoresearch/10` (baseline), `dev-via-test/02f` (first-pass Lisa).
- **anchor_threaded** → rule 06 (`lineage.run-loop-entity-id` threaded onto the
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
research uses `agent.run.entity_id`; autoresearch + dev-via-test descend
through run-entity-fired rules so carry only `lineage.run-loop-entity-id`.

The failure outcome (`agent.run.outcome=failed`, Phase 4a′) is stamped by
the **coordinator-failed** rules here (`05`/`06`) plus the per-pack
**loop-failed run-outcome** rules that live in each category pack:
`research/09`, `autoresearch/12`+`13`, `dev-via-test/09`+`10`. Each fires on
a non-budgeted role's involuntary loop-failure (`outcome in [failed,
truncated]`) and stamps `agent.run.outcome=failed` on the run entity with
the **same per-pack anchor** as the success path (the `run_scope=new`
first-pass children — `autoresearch-baseline`, first-pass Lisa — split onto
`agent.run.entity_id`; the run-entity-descended roles onto
`lineage.run-loop-entity-id`). The existing `*/08`/`*/11` `chain.paused.marker`
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

- **4b-2 resume is wired but INERT** (rules `10`/`11`) — it activates only once
  semstreams#256 threads the run anchor + reply identity onto the reply
  `TaskMessage`. Until then the pause (`07`/`08`/`09`) parks the run and there is
  no live resume path in production.
- `cancel` semantics — an abandoned clarification parks in `awaiting_approval`
  with no terminal-timeout; the cancel/timeout slice (likely an upstream D3
  widening to the `executing`/`awaiting_approval` phase) is deferred (4b).
- The real `approval_required` tool-gate → `awaiting_approval` (4c). The
  dev-via-test CBG gate is an automated reviewer and stays `executing`.
- Product `MilestoneHandler`s (Phase 5 re-platforms the chain stampers).
