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

The success outcome (`agent.run.outcome=success`) is stamped by the three
reviewer/CBG-**approved** terminal rules (`research/07`, `autoresearch/08`,
`dev-via-test/07a`) — NOT by `respond_direct` (which also fires on the
"cannot be served" limitation path). The stamp subject is **per-pack**:
research uses `agent.run.entity_id`; autoresearch + dev-via-test descend
through run-entity-fired rules so carry only `lineage.run-loop-entity-id`.

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

## Not yet here

- `executing→failed` (Phase 4a′ — needs a `coordinator`-role failed rule +
  a per-pack `loop-failed-pause` role-coverage audit).
- `ask_user`/cancel semantics (4b); the real `approval_required` tool-gate
  → `awaiting_approval` (4c). The dev-via-test CBG gate is an automated
  reviewer and stays `executing`.
- Product `MilestoneHandler`s (Phase 5 re-platforms the chain stampers).
