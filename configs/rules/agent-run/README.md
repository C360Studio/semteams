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
| `06-coordinator-failed-lineage-anchor.json` | coordinator loop | Same, for coordinators carrying `lineage.run-loop-entity-id` (dev-via-test 02b/02d/05/07d) — fenced on `length_gt 0`, stamps via the lineage anchor. Mutually exclusive with `05`. (Phase 4a′.) |

### Coordinator-failure coverage boundary (4a′ / 4b)

Not every coordinator that *could* fail needs an executing→failed rule. The
disposition of every coordinator-spawn rule is pinned by
`test/contract/agent_run_pack_test.go`
(`TestAgentRunPack_CoordinatorSpawnCoverage`) — an unclassified new
coordinator-spawn rule fails that test, so the boundary can't drift silently:

- **post-approval wake-ups** (`research/07`, `autoresearch/08`,
  `dev-via-test/07a`) — stamp `agent.run.outcome=success`, so rule 03 completes
  the run *before* the woken coordinator runs; a later failure is post-terminal.
  No coverage needed.
- **anchor-covered** — rules 05 (`agent.run.entity_id` via inherit:
  `research/06` + autoresearch/10's baseline branch) and 06
  (`lineage.run-loop-entity-id` via threading: dev-via-test `02b`/`02d`/`05`/`07d`).
- **deferred to 4b** — the run-entity-descended `ask_user`/`needs_clarification`
  recovery coordinators (`autoresearch/10` non-baseline; dev-via-test
  `02e`/`02f`/`07b`/`07e` re-plan branches) carry no readable anchor. Their
  `action_allowlist ⊆ {respond_direct, ask_user}` — pure human-in-the-loop
  delivery whose run-phase semantics (and a failed delivery's terminal) ADR-053
  assigns to Phase 4b. 4b threads their anchor as part of that work.

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

- `ask_user`/cancel semantics (4b); the real `approval_required` tool-gate
  → `awaiting_approval` (4c). The dev-via-test CBG gate is an automated
  reviewer and stays `executing`.
- Product `MilestoneHandler`s (Phase 5 re-platforms the chain stampers).
