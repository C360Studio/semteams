# Create-change category rule pack

**ADR-057 §D5 — the `create_change` journey.** Category-keyed rule
pack that drives the `create_change` task class: a prompt → author →
reviewer-gate arc that produces a **reviewed OpenSpec change** (a spec
delta — new/modified requirements as SHALL statements + Given/When/Then
scenarios, plus an implementation task breakdown). The reviewed change
is the standalone deliverable; it is renderable to OpenSpec markdown on
demand and independent of execution.

This pack runs through the substrate singletons configured by
`configs/flow-bootstrap.json` (substrate-plus-overlays, ADR-042) — **no
new components**.

## Naming convention

Per ADR-042 open question #2, role tokens follow
`<cognitive-role>-<category>-<phase?>`:

| Role token | Phase | Persona dir |
|---|---|---|
| `author-create-change` | author (single-phase) | `configs/personas/fragments/author-create-change/` |
| `reviewer-create-change` | review (single-phase) | `configs/personas/fragments/reviewer-create-change/` |

## Rules

| File | Trigger | Spawn / Action |
|---|---|---|
| `01-coordinator-create-change-spawn.json` | coordinator decide(create_change) | `author-create-change` (run_scope=new; pins `related_loops['create-change-run']` for emit_change) |
| `02-author-to-reviewer.json` | author decide(drafted) | `reviewer-create-change` (one `query_entity` reads the ask + the change) |
| `03-reviewer-approved-to-coordinator.json` | reviewer decide(approved) | stamp `agent.run.outcome=success` + coordinator wake-up (respond_direct) |
| `04-reviewer-rejected-to-coordinator.json` | reviewer decide(rejected) | coordinator re-dispatch (fixable gaps → fresh `create_change`; max_iterations=3) |
| `05-needs-clarification-to-coordinator.json` | any pack role decide(needs_clarification) | coordinator (ADR-039 recovery: ask_user / re-dispatch / respond; max_iterations=3) |
| `06-loop-failed-pause.json` | any pack role outcome=failed/truncated | `chain.paused.marker` (operator surface) |
| `07-loop-failed-run-outcome.json` | any pack role outcome=failed/truncated | `agent.run.outcome=failed` on the run entity (drives executing→failed) |

## Arc shape

```
prompt (front-door coordinator) → decide(action="create_change")
   ↓ rule 01 (run_scope=new — coordinator loop id IS the run anchor)
author-create-change: read ask → (ingest existing openspec/ context via bash)
   → emit_change(slug, proposal, deltas, tasks, acceptance_command)
   → decide(drafted)
   ↓ rule 02
reviewer-create-change: query_entity(run entity) reads ask + change
   → decide(approved | rejected | needs_clarification)
   ↓ rule 03 (approved)
coordinator wake-up → respond_direct (deliver the reviewed change)
```

Recovery (rules 04/05) routes to the **coordinator**, not back to the
author: the coordinator owns framing, and v1 `emit_change` is
single-emit with **no remover** (ADR-057 §Build addendum
deferred-upsert coupling), so re-running the same author would
double-stamp `change.<slug>.*`. A coordinator re-dispatch starts a
**fresh run** (new run entity, no collision) — supersession-via-new-spawn,
ADR-039 Shape A. The in-place reviewer→author recovery loop (with the
triple-remover upsert) is the deferred ADR-039 slice; when it lands,
that PR adds the remover (mirror `emitdevviatestplan/remover.go`).

## Run-anchor model

create-change is a pure **loop-spawn chain** (coordinator → author →
reviewer → coordinator wake-up). Every role carries `agent.run.entity_id`
because the framework re-stamps it at every spawn off a loop holding a
bare `agent.run` (the `run_scope=new` author seeds it). **No
run-entity-fired rule exists**, so there is no anchor split (unlike
dev-via-test) — rules 03/07 read `agent.run.entity_id` directly. Mirrors
the research pack.

The author's `emit_change` resolves the run entity from
`related_loops['create-change-run']` (= the coordinator loop id, pinned
by rule 01) via `TryChainExecutionEntityID`, which equals the
`run_scope=new` run entity — so the emitted `change.<slug>.*` and the
inherited `agent.run.entity_id` name the same entity.

## Relationship to dev-from-task (P4)

The per-task fields `emit_change` stamps (`change.<slug>.task.<n>.{goal,
target_files,test_command,assumptions,non_goals,expected_outcome}`) are a
superset of Lisa's `plan.task.*` (ADR-057 §D6). P4 `dev-from-task`
reprojects them onto `plan.task.*` + the chain-level
`acceptance_command → integration_test_command` and reuses Ralph + CBG
unchanged. This pack stops at the reviewed change; it does **not**
execute.
