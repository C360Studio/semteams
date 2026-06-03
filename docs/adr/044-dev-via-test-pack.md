# ADR-044: Dev-via-Test Pack — Lisa / Ralph / CBG over Sandbox + Attestation

## Status

**Proposed (2026-06-03).** Sketches the third live category pack
on the substrate-plus-overlays MVP after research and autoresearch.
The pack ships **decompose-and-dispatch software development**:
one planner emits a structured plan, the coordinator walks tasks
sequentially, each task converges via a Ralph-style inner loop in
the per-tenant sandbox, and one reviewer gates the chain end.

Supersedes the design intent of
[ADR-035 (dev-via-spec arc)](035-dev-via-spec-arc.md), which was
itself superseded by ADR-042 §Phase 2 redesign. This ADR is the
new attempt at the same goal under the substrate model:

- Inner loop reframed as **Ralph** (autoresearch-pattern
  convergence on a test-passing scalar) rather than BMAD's
  per-story architect/dev/QA pipeline.
- Planner role compressed to one chain-level fragment; reviewer
  role compressed to one chain-level fragment.
- Role names changed (Lisa / Ralph / CBG) — credit to Geoff
  Huntley's Ralph-loop framing; lightens the SDD tone.

Stays load-bearing on:

- [ADR-042](042-coordinator-instantiated-flows-via-templates.md)
  substrate-plus-overlays (rule packs + persona bundles; no new
  components, no runtime flow construction).
- [ADR-043](043-devcontainer-as-sandbox-spec.md) sandbox
  requirements contract + canonical-profile catalog + attestation.
- [ADR-029](029-product-shell-wiring.md) wiring discipline.
- [ADR-039](039-needs-clarification-recovery.md) recovery cycles.

Does **not** change those ADRs.

**MVP-1 not started.** ADR is the seed for the work; sponsor
scenarios (`@mavlink-decode` greenfield + `@mavlink-hard` OSH/MAVSDK
brownfield, pulled from semspec's real-LLM e2e suite) are named
in §Sponsor scenarios. Slice 1 (Lisa schema + spawn rule) is the
next implementation PR.

## Why this exists

Semspec has burned thousands of dollars over hundreds of hours
trying to wrap BMAD / OpenSpec with enough determinism to ship.
The honest read of that work: **the ceremony itself is the cost**.
Each phase (analyst → PM → architect → SM → dev → QA) has its own
artifact format and handoff expectations; recovery from a failed
phase requires reworking prior artifacts; spec quality has to be
high *before* dev starts because the pipeline assumes the spec is
done.

SemTeams already ships the structural primitives that let us
sidestep that pattern:

- **Autoresearch** ([[research-pack-redesign]] pattern) is a
  scalar-attestation iteration loop with coordinator-driven
  recovery at the boundaries. It is *already* SDD's "loop until
  verified" wrapped around a measurable scalar.
- **Sandbox + attestation** (ADR-043) gives a verified execution
  environment and a deterministic ground truth.
- **Coordinator wake-up + plan state on the chain entity** gives
  resumability and outer-loop dispatch without inventing a new
  orchestrator.

The distillation: **dev-via-test is autoresearch with a different
scalar (test-passing fraction), plus a one-shot planner at the
chain start and a one-shot reviewer at the chain end, with
coordinator walking a sequence of inner Ralph loops.**

That gives us **1 planner + N Ralphs + 1 reviewer = N+2** agent
participants per plan walk, instead of BMAD's 7×N.

## Chain shape

```
User: "do this plan"  (or "implement feature X with these tests")
   ↓
Lisa (one-shot at chain start) ── role: dev-via-test-plan
   ├─ Read the user ask + any provided context
   ├─ Decompose into N tasks; each task carries
   │  goal + assumptions + non_goals + target_files + test_command
   └─ Emit plan.task.* triples on the chain entity
   ↓
Coordinator (outer loop, woken between tasks)
   ├─ read_entity → see plan.task.* triples
   ├─ pick next ready task (deps satisfied; not yet done)
   └─ decide(action="dev_via_test", target=<task_id>)
   ↓
Ralph arc (inner — convergence loop)
   ├─ baseline: read spec from chain triples
   ├─ execute (iterate): edit code in sandbox via `bash` →
   │   run task.test_command → emit measurement
   │   (value = passing_fraction; pass = (value == 1.0))
   ├─ rule 04c upsert best.value (higher-is-better)
   └─ chain-terminal triple on inner-arc reviewer (or skip
       inner reviewer — see §Open Questions)
   ↓
Coordinator wake-up
   ├─ stamp plan.task.<id>.status = done | blocked
   ├─ if more ready tasks → loop back; dispatch next Ralph
   └─ if all done → dispatch CBG
   ↓
CBG (one-shot at chain end) ── role: reviewer-dev-via-test
   ├─ bash: re-run FULL acceptance suite (cross-task drift gate)
   ├─ bash: diff against chain-start git tag
   └─ decide(action="approved" | "rejected", reason=rollup)
   ↓
Coordinator final wake-up
   └─ decide(action="respond_direct", reason=<user-facing rollup>)
```

The **outer loop is coordinator**; the **inner loop is Ralph**.
Planner and reviewer are one-shot at chain level, not per-task.

## Spec schema (Karpathy-shaped)

Lisa's output is a structured spec, not prose. The schema
literally encodes Karpathy's four guidelines as required fields —
see [[encode-principles-structurally]] for the discipline.

```jsonc
{
  "type": "dev_via_test.plan.v1",
  "goal": "<chain-level goal in user's own words>",
  "assumptions": ["..."],          // Karpathy Rule 1 — required, ≥0
  "non_goals": ["..."],            // Karpathy Rule 2 — required, ≥0
  "tasks": [
    {
      "id": "t1",
      "goal": "<task-level goal>",
      "assumptions": ["..."],      // task-local; can be empty
      "non_goals": ["..."],        // task-local anti-scope
      "target_files": ["..."],     // Karpathy Rule 3 — required, ≥1
      "depends_on": ["..."],       // v2; v1 ignores (linear)
      "success_criteria": {
        "test_command": "...",     // Karpathy Rule 4 — required
        "expected_outcome": "..."  // human-readable "done looks like"
      }
    }
  ],
  "integration_test_command": "..." // CBG's full-suite gate
}
```

Validator rejects payloads missing any of the required fields.
Lisa **cannot** emit a spec without explicitly surfacing
assumptions, non-goals, target files, and a test command. The
discipline is in the schema, not the persona prose.

## Plan state as triples

Plan persistence is on the chain entity, not workspace markdown:

```
plan.task.t1.goal                = "..."
plan.task.t1.assumptions         = ["..."]
plan.task.t1.non_goals           = ["..."]
plan.task.t1.target_files        = ["..."]
plan.task.t1.test_command        = "..."
plan.task.t1.expected_outcome    = "..."
plan.task.t1.depends_on          = ["..."]
plan.task.t1.status              = "ready" | "in_progress" | "done" | "blocked"
plan.task.t1.last_failure_stderr = "..."  // populated on Ralph failure for coordinator to read
plan.integration_test_command    = "..."
plan.chain_start_git_tag         = "task-start"
```

No `retry_count` triple. Recovery is coordinator-driven via
`ask_user` (see §Stuck-task recovery); there is no auto-retry
counter for the persona or rule layer to read.

**Resumability falls out for free.** Kill the process mid-walk;
restart with the chain ID; coordinator reads triples and picks up.
The workspace persists in the per-tenant container
([ADR-043](043-devcontainer-as-sandbox-spec.md)). State machine
rigor per the semspec lesson is structural rather than aspirational.

## Reuse vs deltas

| Surface | Reused as-is | Adapted | New |
|---|---|---|---|
| Sandbox manager + canonical profiles (ADR-043) | ✓ | | |
| `request_sandbox` + `query_sandbox_attestation` | ✓ | | |
| Chain-scoped `bash` tool | ✓ | | |
| Autoresearch rule 04c (best.value upsert) | | invert / target 1.0 | |
| Autoresearch rule 05 (iteration-dispatch) | ✓ | | |
| Chain-terminal wake-up pattern | ✓ | | |
| `emit_autoresearch_measurement` tool | | persona reframes scalar | |
| Coordinator `decide` taxonomy | | extend | new action `dev_via_test` |
| Plan walking | | | coordinator persona fragment |
| Lisa persona dir | | | `dev-via-test-plan/` |
| Ralph persona dir | | | `dev-via-test-execute/` |
| CBG persona dir | | | `reviewer-dev-via-test/` |
| Rule pack | | | `configs/rules/dev-via-test/` (~7 rules) |
| Spec payload schema | | | `dev_via_test.plan.v1` + `dev_via_test.task.v1` |

**No new components. No framework changes.** Pack is config-only
on the existing substrate.

## Named risks and v1 dispositions

### Cross-task interference (sharpest risk)

Workspace persists across tasks; Ralph on t2 can edit files outside
its lane and contaminate t3.

v1 mitigations, in order of bang-for-buck:

1. **`target_files` constraint in Ralph's persona** — "only modify
   files listed in `target_files`; for broader scope,
   `decide(needs_clarification)` to amend the spec."
2. **Per-task git tag at task-end** — `bash git tag task-t<id>-end`
   so CBG can diff against any task boundary.
3. **Narrow per-task acceptance.** Per-task `test_command` runs only
   that task's tests. CBG's `integration_test_command` runs the
   full suite at chain end. Local proof per task; global proof at
   end.

v2 (deferred): branch-per-task with merge-at-end. Overhead not
worth it yet.

### Stuck-task recovery

Ralph terminates without `plan.task.<k>.test_command` passing —
either it hits the framework's `agentic-loop.max_iterations` ceiling
(50 in `flow-bootstrap.json`) without converging, or it
`decide(needs_clarification)` because it cannot see a path forward.

v1 policy: **coordinator owns recovery via `ask_user`. No
auto-retry, no persona-level iteration counter.**

- Ralph fails (loop-failed or needs_clarification) → rule 04b
  stamps `experiment.loop_failed` + `plan.task.<k>.last_failure_stderr`
- coordinator wakes to chain-failed state, reads stderr
- coordinator calls `decide(action="ask_user", reason="task <k>
  stuck after <N> iters; last error: <…>; continue with hint,
  abandon the task, or split it?")`
- user replies; their answer drives the next move (amend spec +
  re-dispatch, mark task `blocked` + skip, or kill chain)

Rationale — per [[coordinator-first-not-persona-patches]] and
[[for-each-n1-no-classifier]]: pushing the "give up?" decision UP
to the coordinator with `ask_user` is honest about cost (the user
sees what they've spent and chooses) and removes the magic
`retries=1` number that has no empirical basis. Framework
`max_iterations: 50` + per-loop `timeout: 300s` stay as the safety
floor — they bound blast radius without becoming a policy the
persona reasons about.

Don't try to classify failure modes (spec vs impl vs flake) in
MVP. Once we have data, [[r35-coordinator-meta-reviewer]] earns
its keep — that deferred ADR was waiting for a failure-mode
dataset, and this is the dataset.

The persona prompt for Ralph contains **no numeric caps** — no
"you have N tries", no "iteration X of Y". Ralph's job is "iterate
until `success_criteria.test_command` exits 0; if you can't see a
path forward, `decide(needs_clarification, reason=<what's
blocking>)`." Per [[encode-principles-structurally]] and
[[personas-describe-job-not-plumbing]]: budget enforcement is
framework machinery (max_iterations + timeout); persona prose
describes the job. The same posture extends to N (plan size) and
per-plan task count — Lisa picks N based on the work; if it's too
coarse/fine the coordinator can `ask_user` before walking begins.

**Honest trade-off:** unattended/CI runs lose the auto-abort.
Framework `max_iterations` (50) + per-loop `timeout` (300s) cap
worst-case Ralph blast radius to ~$0.20–0.50; chain pause on
loop-failed routes to the UI which a watching operator can
interrupt. For sponsor-demo + interactive use this is correct.
For true headless ops we'd add a chain-level $-budget rule later
— deferred until a real headless workload demands it.

### DAG awareness

v1: **linear only.** Planner outputs ordered task list; coordinator
walks in order; every task is ready when the prior is done. Most
plans have a natural sequence ("scaffold → A → B → wire → ship");
the constraint forces the planner to think about ordering.

v2: explicit `depends_on`; coordinator topo-walks. Parallelism via
the `for_each` fan-out pattern from
[[research-pack-redesign]] when planner emits parallel groups.
Architecture supports it; persona work deferred until v1 ships.

### CBG's gate at chain-end (sharpen the role)

Not "review the diff." Concretely:

1. **Re-run `plan.integration_test_command`** (FULL acceptance
   suite). This is the deterministic cross-task-drift catcher.
2. **Read the cumulative diff** against `plan.chain_start_git_tag`.
   Sanity check: did we ship what the plan asked for?
3. **Emit `decide(approved | rejected)`**. On reject: coordinator
   wakes, dispatches `ask_user` with CBG's reasoning. v1 does not
   auto-recover from CBG reject; user picks next move.

Per-Ralph reviewer is **optional** in v1 (test-pass is the
deterministic signal). Don't slip into per-Ralph reviewer
ceremony — that's the BMAD pattern that this ADR exists to avoid.

## Sponsor scenarios — semspec's burn list, head-to-head

The pitch only beats freeform `claude code <task>` when
**attestation, audit trail, or policy enforcement actually
matters** — regulated code, customer-facing API contracts,
codebases with strict coverage / lint / non-regression gates.

For "I want a function," freeform is cheaper. The convergence-loop
overhead is wasted ceremony in a different costume.

Sponsor scenarios are pulled from semspec's existing real-LLM
e2e suite (`~/Code/c360/semspec/ui/e2e/plan-lifecycle-llm-mavlink*.spec.ts`)
so each run is **a head-to-head against semspec's BMAD-shaped
pipeline on identical inputs.** This is the load-bearing sponsor
argument: same prompt, same fixture, same acceptance, different
harness — measure which converges, at what cost, with what
artifact.

### MVP-1 — `@mavlink-decode` (easy, greenfield)

**Fixture:** `semspec/test/e2e/fixtures/mavlink-heartbeat-go/` —
skeleton `main.go` (empty `func main()`) + empty `go.mod`,
resettable via `task reset-fixtures`.

**Prompt** (verbatim from semspec's spec):

> "Add a Go HTTP service that listens for MAVLink v2 HEARTBEAT
> frames over UDP on port 14540 and exposes the most recent
> heartbeat at GET /heartbeat as JSON containing 'system_id',
> 'component_id', 'autopilot_type', 'base_mode', and 'received_at'.
> Use a real Go MAVLink library (e.g., github.com/bluenviron/gomavlib)
> for frame parsing — do not hand-roll the MAVLink wire format.
> Include unit tests that decode captured MAVLink HEARTBEAT frames
> from testdata/ files and assert the parsed fields."

**CBG's `plan.integration_test_command`:** `go test ./... && go vet ./...`

### MVP-2 (Accept-gate) — `@mavlink-hard` (brownfield, OSH/MAVSDK)

**Fixture:** `semspec/test/e2e/fixtures/osh-driver-mavsdk/` —
Java/Gradle OSH driver skeleton; requires pre-cloned `osh-addons`
+ `MAVSDK-Java` + `osh-core` + `ogc-cs` at `/sources/` (epic overlay
pattern from semspec's existing harness).

**Goal:** extend the OSH MAVSDK addon with full Connected Systems
API coverage; plugin-coverage matrix; preserve OSH sensor module
patterns. This is the hard case semspec has been grinding on —
beating it is the demo.

### Acceptance

- **MVP-1 (mavlink-decode) passes** when ≥4 of 5 real-LLM runs
  converge with `go test ./...` green AND `go vet ./...` clean,
  inside the framework `max_iterations: 50` ceiling. Convergence
  iteration count is an **observation**, not a budget — record it,
  don't gate on it.
- **MVP-1 fails** (and this ADR moves to **Rejected**) if ≥3 of 5
  runs hit `max_iterations` or terminate via `ask_user` without
  user-resolvable hints. Spec-quality risk is too high to overcome
  with iteration; the BMAD ceremony semspec inherited may be the
  honest read of the problem.
- **MVP-2 (mavlink-hard) gates ADR Accept.** Either we converge on
  the OSH/MAVSDK epic where semspec has not, or this ADR ships as
  "useful for greenfield, deferred for brownfield" — useful but
  not the win we pitched.

**Budget to know (MVP-1):** ≤$10 across 5 runs on Gemini (Lisa +
CBG) + claude-haiku (Ralph). Per-run target: ≤$2, ≤5 minutes
wallclock. No iteration-cap enforcement — the framework
`max_iterations: 50` + per-loop `timeout: 300s` cap the worst case
naturally; coordinator `ask_user` is the human-in-the-loop budget
gate (see §Stuck-task recovery).

## Open questions for the acceptance gate

1. **Should the inner Ralph arc have its own reviewer?** Argument
   for: catch narrow-fit-to-tests at the per-task level before
   compounding. Argument against: BMAD-lite slip. v1 default:
   skip inner reviewer; CBG's plan-level gate is sufficient.
2. **Is `dev_via_test` a distinct action token or a variant of
   `autoresearch` with a different scalar?** v1 leans
   distinct-token for taxonomy clarity. Could collapse later.
3. **Plan source — does Lisa get invoked via `decide(action=
   "plan")` first, or does the user paste a structured plan and
   skip Lisa?** v1: support both. User-pasted plan stamps triples
   directly (no Lisa hop); prose ask spawns Lisa.
4. **Brownfield support — when does the planner read existing
   repo context?** MVP-1 (`@mavlink-decode`) is greenfield. MVP-2
   (`@mavlink-hard`, Accept-gate) is brownfield — Lisa must walk
   pre-cloned source trees at `/sources/` to ground the plan. The
   delta is `bash`-led repo exploration in Lisa's persona + a
   `repo_context_globs` field on the spec schema; both deferred
   until MVP-1 passes.
5. **Tool-naming convention — `emit_dev_via_test_measurement` or
   reuse `emit_autoresearch_measurement`?** Reuse if scalar
   semantics match (one numeric value, pass bool); new tool if
   we need richer test-result payloads. Decide in PR 1.

## Risks (per ADR convention)

1. **Spec quality from Lisa.** If specs are too vague even with
   iteration, the loop diverges. MVP-1 smoke is designed to falsify
   this cheaply.
2. **Gaming the tests.** Ralph passes by writing narrow code that
   fits the test rather than the goal. CBG's diff review catches
   some; long-term mitigation is broader test suites at planner
   time (Karpathy Rule 4 done well).
3. **Inter-task workspace contamination.** Mitigations above; full
   plan-level gate catches what slips through.
4. **Cost runaway.** No persona-level iteration caps (see
   §Stuck-task recovery for rationale). Framework
   `max_iterations: 50` + per-loop `timeout: 300s` bound worst-case
   Ralph blast radius to ~$0.20–0.50; coordinator `ask_user` on
   loop-failed is the human-in-the-loop budget gate. For
   unattended / true-headless workloads we'd add a chain-level
   $-budget rule later; deferred until a real headless workload
   demands it.

## Cross-links

- [ADR-035 dev-via-spec arc](035-dev-via-spec-arc.md) (superseded
  by ADR-042; this ADR is the next attempt under the new substrate)
- [ADR-042 substrate-plus-overlays](042-coordinator-instantiated-flows-via-templates.md)
- [ADR-043 sandbox requirements + attestation](043-devcontainer-as-sandbox-spec.md)
- [ADR-029 product-shell wiring](029-product-shell-wiring.md)
- [ADR-039 needs-clarification recovery](039-needs-clarification-recovery.md)
- [Karpathy guidelines skill](https://github.com/multica-ai/andrej-karpathy-skills/blob/main/skills/karpathy-guidelines/SKILL.md)
  — basis for the spec schema's required fields; structural
  encoding rationale in [[encode-principles-structurally]]
