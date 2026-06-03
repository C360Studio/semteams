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
| 1 | Lisa planner — `emit_dev_via_test_plan` + persona + rule 01 spawn | in progress |
| 2 | Ralph executor — measurement tool + persona + rules 03/04a/04b/04c | pending |
| 3 | Plan walker — coordinator wake-up rule 05 + plan-walking persona fragment | pending |
| 4 | CBG reviewer — persona + rules 06/07/08 | pending |

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
plan.generated_at               = RFC3339

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
| `01-coordinator-dev-via-test-spawn.json` | coordinator decide(dev_via_test) | Lisa + stamp `dev_via_test.run.status=active` on coordinator (run) entity |

(Rules 03+ land in Slices 2-4.)

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
