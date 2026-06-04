# Dev-via-test category rule pack

**ADR-044 — third live category pack on the substrate-plus-overlays
MVP, after research and autoresearch.** Ships
**decompose-and-dispatch software development**: one planner (Lisa)
emits a structured plan, the coordinator walks tasks sequentially,
each task converges via a Ralph-style inner loop in the per-tenant
sandbox, and one reviewer (CBG) gates the chain end.

The pack runs through the same substrate singletons as research +
autoresearch — `configs/flow-bootstrap.json` wires graph-ingest,
graph-query, rule-processor, agentic-loop, agentic-dispatch,
agentic-tools, agentic-model — and adds category-keyed rules +
role-scoped persona bundles.

## What this pack proves about the substrate

Autoresearch proved the substrate can run N-iteration inner loops
with empirical reviewers. This pack adds:

1. **Coordinator-driven outer loop.** Walking N tasks where each
   spawns its own Ralph inner loop, with state persisted on the
   chain entity (`plan.task.<id>.status`) for resumability. The
   coordinator owns the walk; rules are stateless dispatchers. Per
   [[coordinator-first-not-persona-patches]].
2. **Karpathy-shaped spec schema as discipline.** Lisa cannot emit
   a plan without surfacing assumptions, non_goals, target_files,
   and test commands. Structure is in the schema (`emit_dev_via_test_plan`
   executor), not persona prose. Per [[encode-principles-structurally]].
3. **Test-passing scalar as convergence signal.** Ralph's inner
   loop converges on `test_command` exit code 0 — a deterministic,
   non-LLM signal. Sidesteps the LLM-as-reviewer Goodhart vector
   for the inner loop (same posture as autoresearch's
   `emit_autoresearch_measurement`).

## Slices

This pack lands in 4 build slices + 1 real-LLM smoke gate. See
ADR-044 §"Sponsor scenarios" for the @mavlink-decode MVP-1 gate
and @mavlink-hard Accept-gate.

| Slice | Scope | Status |
|---|---|---|
| 1 | Lisa planner — `emit_dev_via_test_plan` + persona + rule 01 spawn | shipped |
| 2 | Ralph executor — `emit_dev_via_test_measurement` + persona + rules 04a/04b/08 | shipped |
| 3 | Plan walker — coordinator wake-up + plan-walking persona fragment + rules 02/03/05 | shipped |
| 4 | CBG reviewer — persona + rules 06/07a/07b; rule 08 extended; new `dev_via_test_finalize` action | shipped |

## Naming convention

Per ADR-042 open question #2 and autoresearch precedent, role tokens
follow `<cognitive-role>-<category>-<phase?>`:

| Role token | Phase | Persona dir | Friendly name |
|---|---|---|---|
| `dev-via-test-plan` | plan | `configs/personas/fragments/dev-via-test-plan/` | Lisa |
| `dev-via-test-execute` | execute | `configs/personas/fragments/dev-via-test-execute/` | Ralph |
| `reviewer-dev-via-test` | (single-phase) | `configs/personas/fragments/reviewer-dev-via-test/` | CBG |

The friendly names (Lisa / Ralph / CBG — Simpsons-themed, credit
Geoff Huntley's Ralph-loop framing) appear in operator-facing docs
and the persona front-matter. The role tokens are what rules match
on.

## Plan state as triples

Per ADR-044 §"Plan state as triples", the plan persists on the
chain entity (NOT workspace markdown). After Slice 1's emit:

```
plan.goal                       = "..."
plan.assumptions                = "[<JSON array>]"
plan.non_goals                  = "[<JSON array>]"
plan.integration_test_command   = "..."
plan.chain_start_git_tag        = "plan-start"
plan.task_count                 = N
plan.revision                   = 1
plan.generated_at               = RFC3339Nano

plan.task.<id>.goal             = "..."
plan.task.<id>.assumptions      = "[<JSON array>]"
plan.task.<id>.non_goals        = "[<JSON array>]"
plan.task.<id>.target_files     = "[<JSON array>]"
plan.task.<id>.depends_on       = "[<JSON array>]"
plan.task.<id>.test_command     = "..."
plan.task.<id>.expected_outcome = "..."  (optional)
plan.task.<id>.status           = "ready"  (initial; mutated by walker)
plan.task.<id>.position         = 0       (emit order; walker uses for linear walk)
```

Array fields are JSON-encoded strings so the rule engine's
`$entity.triple.X` substitution interpolates a predictable shape
into downstream prompts. Ralph parses the JSON string back into a
list at read time.

No `plan.task.<id>.retry_count` triple — per ADR-044
§"Stuck-task recovery", recovery is coordinator-driven via
`ask_user`, not auto-retry.

## Rules (current — will grow with each slice)

| File | Trigger | Spawn / Stamp |
|---|---|---|
| `01-coordinator-dev-via-test-spawn.json` | coordinator decide(dev_via_test) + subtopics.length=0 (INITIAL dispatch) | Lisa + stamp `dev_via_test.run.status=active` on coordinator (run) entity |
| `02-lisa-terminal-to-walker.json` | Lisa decide(planned) | Coordinator walker wake-up with run-entity lineage threaded; reads plan via query_entity, dispatches first task |
| `03-coordinator-dispatch-ralph.json` | coordinator decide(dev_via_test) + subtopics.length>0 (WALKER dispatch) | Ralph via `for_each` over subtopics — v1 N=1 (serial), v2 N>1 (parallel topo-walk) |
| `04a-execute-stamp-converged.json` | Ralph success + `dev_via_test.measurement.pass=true` | Stamp `dev_via_test.execute.outcome=converged` on Ralph entity + `dev_via_test.execute.task_completed=<ralph-loop-id>` on run entity (walker pickup) |
| `04b-execute-stamp-failed.json` | Ralph outcome IN [failed, truncated, cancelled] | Stamp `dev_via_test.execute.outcome=failed` on Ralph entity + `dev_via_test.execute.task_failed=<ralph-loop-id>` on run entity. No auto-retry per ADR §Stuck-task recovery. Walker routes to `ask_user`. |
| `05-ralph-terminal-to-walker.json` | Ralph outcome IN [success, failed, truncated, cancelled] (ANY terminal) | Coordinator walker wake-up; reads Ralph's terminal via read_loop_result + run-entity state via query_entity; decides next task / finalize (CBG) / ask_user / respond_direct |
| `06-coordinator-dispatch-cbg.json` | Walker decide(dev_via_test_finalize) | CBG (reviewer-dev-via-test) — chain-end gate. Runs `plan.integration_test_command`, diffs against `plan.chain_start_git_tag`, emits approved/rejected |
| `07a-cbg-approved-to-coordinator.json` | CBG decide(approved) | Final coordinator wake-up scoped to respond_direct — delivers chain result to user |
| `07b-cbg-rejected-to-coordinator.json` | CBG decide(rejected) | Final coordinator wake-up scoped to ask_user — user picks next move (amend plan, abandon, re-dispatch). No auto-recover per ADR §CBG's gate at chain-end. |
| `08-loop-failed-pause.json` | Non-execute dev-via-test role (Lisa, CBG) outcome IN [failed, truncated, cancelled] | Stamp `chain.paused.marker` + `chain.paused.role`. Chainpause subscriber propagates to chain entity (§D5). |

## V1 binary semantics — what's NOT here

The ADR's "Reuse vs deltas" table mentioned reusing rule 04c
(autoresearch best.value upsert, inverted for higher-is-better /
target 1.0). **Slice 2 does NOT ship a rule 04c.** Rationale:

- v1 uses **binary** pass/fail semantics — `dev_via_test.measurement.pass=true`
  is itself the terminal "converged" signal. No best.value tracking is
  needed because there's no kept/reverted choice to make.
- Fractional convergence (e.g., 7/10 tests passing → value=0.7) is
  **deferred to v2.** The `emit_dev_via_test_measurement` tool already
  accepts an optional `value` field (0.0..1.0) so the wire is forward-
  compatible; rules + persona just don't read it yet.
- If v2 introduces kept/reverted with `best.value` upsert, the rule
  shape will mirror autoresearch 04c (`update_triple` for true upsert);
  may consolidate the two pack's measurement tools at that point.

Per ADR-044 §addendum 2026-06-03 Slice 2.

## Cross-entity stamping pattern

Rules 04a + 04b each stamp TWO triples — one on Ralph's loop
entity (`dev_via_test.execute.outcome`), one on the run entity
(`dev_via_test.execute.{task_completed,task_failed}`) via
`$entity.triple.lineage.run-loop-entity-id` subject substitution.
The second triple is the **load-bearing** one for the walker —
the coordinator wake-up reads the run entity for `task_completed`
/ `task_failed` markers (multi-valued; one triple per Ralph) to
compute effective per-task status.

We do NOT mutate `plan.task.<id>.status` from rules 04a/04b — the
rule engine substitutes triple OBJECTS but not PREDICATE FRAGMENTS
(per beta.96). Predicate substitution (`plan.task.${TASK_ID}.status`)
would either require framework support OR per-task-ID rule
generation (cardinality explosion). Instead, the walker computes
status **derivatively** from the multi-valued execution markers:

- `done` if task ID appears in `task_completed` triples
- `blocked` if task ID appears in `task_failed` triples
- `ready` otherwise (initial plan state, no mutation)

This keeps the plan immutable across the chain's lifetime and
makes the walker's state computation a pure function of run-entity
contents — no race conditions on partial-write status updates.

## The `dev_via_test` two-mode action token

Slice 3 introduces a dual-mode shape on the same action token:

| Mode | When | Token shape | Routes to |
|---|---|---|---|
| Initial dispatch | First coordinator loop (front-door) | `decide(action="dev_via_test", reason=<user ask>)` — **no subtopics** | Rule 01 → Lisa |
| Walker dispatch | Coordinator woken after Lisa/Ralph terminal | `decide(action="dev_via_test", subtopics=["<task-id>"])` | Rule 03 → Ralph (via for_each) |

Rules 01 and 03 are mutually exclusive on `coordinator.decision.subtopics.length`
(`length_eq 0` vs `length_gt 0`). The `decide` tool stamps the subtopics
predicate ONLY when non-empty (verified upstream `processor/agentic-tools/decide.go`),
so absent + length_eq 0 are equivalent in array-operator semantics
(per `processor/rule/expression/types.go`).

## v1 walker — linear, single-Ralph-at-a-time

v1 ships sequential dispatch (`subtopics=[<one-id>]`, `for_each` N=1).
The architecture supports parallel dispatch via `subtopics=[<id1>,<id2>,...]`
(rule 03's `for_each` over subtopics fans out — same pattern as
research pack rule 02). Gated by `plan.task.<id>.depends_on`
topo-walking which is deferred to v2 per ADR-044 §DAG awareness.

## End-to-end chain flow (Slices 1-4)

```
User → front-door coordinator
  decide(action="dev_via_test", reason=<user ask>)
  ↓ rule 01 [subtopics.length=0 + lineage.length=0]
Lisa (dev-via-test-plan)
  bash git tag plan-start
  emit_dev_via_test_plan(...)
  decide(action="planned")
  ↓ rule 02
Walker A (coordinator, woken with run-loop-entity-id lineage)
  query_entity(<run-id>) → reads plan
  decide(action="dev_via_test", subtopics=["t1"])
  ↓ rule 03 [subtopics.length>0 + for_each]
Ralph 1 (dev-via-test-execute, task_id=t1)
  iterate: bash edit → bash test → emit_dev_via_test_measurement
  decide(action="measured")
  ↓ rule 04a (stamp task_completed on run) + rule 05
Walker B
  query_entity(<run-id>) → reads plan + execution markers
  picks next ready task → decide(action="dev_via_test", subtopics=["t2"])
  ↓ rule 03 → Ralph 2 → rule 05 → Walker C → ...
  ... when all tasks done ...
Walker N
  decide(action="dev_via_test_finalize", reason=<pre-CBG rollup>)
  ↓ rule 06
CBG (reviewer-dev-via-test)
  query_entity(<run-id>) → reads plan.integration_test_command
  bash <integration_test_command>
  bash git diff plan-start
  decide(action="approved" | "rejected")
  ↓ rule 07a (approved) OR rule 07b (rejected)
Final coordinator
  decide(action="respond_direct" | "ask_user")
  ↓ coordinator/03b-respond-direct.json OR coordinator/03-ask-user.json
User reply published
```

Failure paths (per ADR §Stuck-task recovery):
- Lisa or CBG loop-failed → rule 08 → chain.paused (no auto-recover)
- Ralph loop-failed → rule 04b → walker wakes → ask_user
- CBG rejected → rule 07b → ask_user (no auto-re-plan)
- User responds → fresh coordinator loop (new chain)

## Sandbox dependency

Same model as autoresearch — every role runs inside the per-tenant
devcontainer the coordinator provisioned via `request_sandbox`
(ADR-043). The chain-scoped `bash` tool routes commands there
automatically via `sandboxruntime.AttestationRunner`. The workspace
persists across all tasks in the chain's lifetime — Ralph on t2
sees the files Ralph on t1 wrote.

This is the **sharpest v1 risk** (per ADR-044 §"Cross-task
interference"). Lisa's `target_files` constraint is the primary
mitigation; per-task git tags (Slice 4) give CBG a diff to inspect.

## Migration posture

Net-new pack on the post-MVP-7 substrate. No legacy predecessor to
retire (ADR-035 dev-via-spec arc was already superseded by ADR-042).
The pack is config-only on the substrate plus one product-shell
tool (`emit_dev_via_test_plan`) per [[framework-alignment-review]]
— the migration target is the same as the other emit tools (upstream
write_artifact when it ships).
