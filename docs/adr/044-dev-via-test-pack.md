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

## Addendum 2026-06-03 — Slice 1 framework-alignment review

Per CLAUDE.md "Product-Shell-Tool Discipline" and
[[framework-alignment-review]]: Slice 1 adds one product-shell tool
(`emit_dev_via_test_plan`). The review:

**1. Upstream survey.** `~/go/pkg/mod/github.com/c360studio/semstreams@v1.0.0-beta.96`
ships these emit-shaped tools in `processor/agentic-tools/executors/`:

- `decide` — structured terminal output. Pattern.
- `emit_diagnosis` — typed stamp + render. Pattern.
- `read_loop_result`, `read_entity`, `query_entities`,
  `query_relationships` — graph reads. Pattern.
- `submit_work` — terminal commit. Pattern.

No existing emit-tool ships a Karpathy-schema validator +
plan-as-triples stamping pattern. The closest siblings
(`emit_research_artifact`, `emit_autoresearch_baseline`,
`emit_plan`) all live in product code at `cmd/semteams/tools/`;
none of them cover the per-task triple-stamping shape this tool
needs (eight predicates per task ID, all prefixed
`plan.task.<id>.*`).

**2. Upstream roadmap check.**
`semstreams/docs/adr/028-orchestration-architecture.md` §"What's not
built here" anticipates a generic `write_artifact` / `read_artifact`
/ `list_artifacts` suite. Not shipped in beta.96 (verified
2026-06-03). Same posture as `emit_research_artifact` (per ADR-031
§addendum 2026-04-30) and `emit_autoresearch_baseline` (per ADR-042
§addendum 2026-05-29).

**3. Case for product-shell-local.** The Karpathy schema enforcement
is the **load-bearing primitive** of this ADR. Per
[[encode-principles-structurally]]: the discipline (assumptions,
non_goals, target_files, test_command must be explicitly surfaced)
must live in code that REJECTS payloads missing those fields. A
generic `write_artifact` accepting freeform JSON could not enforce
this. The per-task triple-stamping pattern (`plan.task.<id>.*`) is
also Lisa-specific — it's what makes the coordinator-walker
(Slice 3) and Ralph's lineage-read (Slice 2) work without
re-parsing JSON blobs.

**4. Alternatives ruled out.**

- *Reuse `emit_plan` (from the retired dev-via-spec arc).* Rejected:
  `emit_plan` renders to markdown + stamps four pointer triples
  (revision, epic_count, generated_at, path). Ralph + the coordinator
  walker need *triples* (queryable, substitutable into prompts via
  `$entity.triple.X`), not a markdown blob behind a path pointer.
  The markdown rendering would just be ceremony.
- *Per-element triples for arrays* (`plan.task.<id>.target_files.0`,
  `plan.task.<id>.target_files.1`, ...). Rejected: triple-count
  explosion (15 target_files = 15 triples) without any benefit —
  the rule engine's `$entity.triple.X` substitution can't iterate
  over indexed predicates today, so the consumer has to JSON-parse
  anyway. JSON-encoded strings in single triples are the predictable
  shape.
- *Inline the schema validation in Lisa's persona prose.* Rejected
  hard per [[encode-principles-structurally]] and the semspec
  lesson: persona prose is hopeful, schema is load-bearing. The
  whole point of using a schema for the Karpathy guidelines is that
  the LLM CANNOT skip them.

**5. Migration target.** When upstream ships the generic
`write_artifact` suite (per ADR-028), evaluate migration of
`emit_dev_via_test_plan` alongside `emit_research_artifact`,
`emit_plan`, and all `emit_autoresearch_*`. Migration replaces the
concrete executor with a configured one; the
Karpathy-required-fields schema stays in product code regardless
(it's domain-specific to the dev-via-test category contract). If
upstream's generic primitive exposes a schema-validation hook, the
JSON-Schema fragment (`taskSchema` + the plan-level required arrays
in `ListTools()`) lifts cleanly into config. If not, the executor
shape stays as-is and the migration is limited to swapping the
triple-publishing backend.

**6. tools/README.md row + ADR row recorded.** The single new tool
appears in `cmd/semteams/tools/README.md`'s table with the migration
target named explicitly. This addendum is the ADR-side evidence
trail per CLAUDE.md.

## Addendum 2026-06-03 — Slice 2 (Ralph executor) framework-alignment review + v1 binary semantics

Slice 2 adds one product-shell tool (`emit_dev_via_test_measurement`)
and three rules (04a/04b/08). The framework-alignment review for
the new tool + a load-bearing scope decision (v1 binary semantics
vs the ADR's original autoresearch-pattern reuse).

### Framework-alignment review — `emit_dev_via_test_measurement`

**1. Upstream survey.** Same baseline as Slice 1
(`semstreams@v1.0.0-beta.96`). No upstream "per-iteration stamp"
primitive — the closest pattern is `emit_diagnosis` (ops agent)
which writes typed finding triples but does not condition on
agentic-loop iteration semantics.

**2. Closest sibling: `emit_autoresearch_measurement`.** That tool
implements full empirical-reviewer logic (read best.value → compare
→ stamp outcome → maybe update best). Initial ADR §Open Q 5
asked: reuse with target=1.0 / higher-is-better inversion, or new
tool? Decision: **new tool**, because:

- *Semantics genuinely diverge.* Autoresearch's lower-is-better
  metric optimization has no terminal — every iteration is one of
  N until the cap. Dev-via-test's `value==1.0` (or simply
  `pass=true`) IS the terminal — the loop stops, no further
  iteration. Modeling this as "lower-is-better with target = 0"
  via inversion is unnatural; the terminal-stop concept doesn't
  exist in autoresearch's contract.
- *v1 binary semantics need much less machinery.* No best.value
  tracking, no kept/reverted dance, no `compareOutcome` function.
  Just stamp pass + value + tails. ~150 LOC vs autoresearch's ~350.
- *Autoresearch's contract surface stays clean.* No callers need
  to opt-in to a direction param; no "is this for autoresearch or
  for dev-via-test?" branching.
- *V2 consolidation stays open.* If v2 dev-via-test introduces
  fractional convergence with kept/reverted, evaluate consolidating
  with `emit_autoresearch_measurement` at that point — both tools
  would then implement the same "read best, compare, stamp" shape.

**3. Migration target.** Same as Slice 1 + autoresearch: upstream's
planned generic `write_artifact` suite ([ADR-028 §What's not built
here]). If upstream lands it AND the dev-via-test semantics
generalize to "single-iteration stamp", migration is straightforward.

### V1 binary semantics — scope decision

ADR-044's "Reuse vs deltas" table (line 182) listed reuse of
autoresearch rule 04c (best.value upsert) with "invert / target 1.0"
adaptation. Slice 2 **does not** ship rule 04c. Rationale:

- **The dev-via-test test_command is binary in v1.** Either
  `go test ./...` exits 0 (all tests pass) or it doesn't. There is
  no "passed 0.7 of tests" until the persona is taught to parse
  `go test -json` and report fractional progress — which is
  meaningful work the MVP-1 sponsor scenario (@mavlink-decode)
  does not require.
- **No best.value tracking is needed when value is binary.** The
  value is always 0.0 or 1.0. Once you see 1.0, you're done. The
  kept/reverted machinery exists in autoresearch because each
  iteration's change might HURT the metric; for dev-via-test, every
  iteration is trying to fix tests — there's no harm in changes
  that "don't help" because we just don't terminate yet.
- **Forward-compatible wire.** The `emit_dev_via_test_measurement`
  tool accepts an optional `value` field (0.0..1.0); persona + rules
  don't read it yet. When v2 introduces fractional convergence, we
  add rule 04c, extend the persona's "decide between iterations"
  guidance, and the wire stays backward-compatible.
- **Defer-with-evidence posture.** If MVP-1 smoke shows Ralph
  thrashing (e.g., making changes that pass some tests but break
  others, with no improvement signal), v2 fractional + kept/reverted
  becomes the documented remediation. Until then, framework
  `max_iterations` + coordinator `ask_user` (per §Stuck-task
  recovery) are the safety nets.

### Cross-entity stamping pattern (rules 04a/04b)

Rules 04a/04b each stamp TWO triples — one on Ralph's own loop
entity (`dev_via_test.execute.outcome`), one on the run entity
(`dev_via_test.execute.{task_completed,task_failed}`) via
`$entity.triple.lineage.run-loop-entity-id` subject substitution.

The second triple is load-bearing for Slice 3: the coordinator
walker watches the run entity for these markers to know which
Ralph just finished and pick the next ready task.

We do NOT mutate `plan.task.<id>.status` from rules 04a/04b
directly because the rule engine substitutes triple OBJECTS but
not PREDICATE FRAGMENTS in beta.96. Predicate substitution
(`plan.task.${TASK_ID}.status`) would either require framework
support OR per-task-ID rule generation (cardinality explosion).
The walker handles per-task status mutation in coordinator code
via parameterized `update_triple` actions in Slice 3.

This is a deliberate scope split between Slice 2 (data-plane:
Ralph stamps its terminal state on lineage-threaded entities) and
Slice 3 (control-plane: walker translates terminal state into
plan.task.<id>.status mutations + next-task dispatch).

## Addendum 2026-06-03 — Slice 3 (plan walker) design + scope

Slice 3 wires the coordinator-as-walker control plane: rule 02
(Lisa→walker), rule 03 (walker→Ralph via for_each), rule 05
(Ralph→walker), plus `30-plan-walking.md` persona fragment + the
`dev_via_test` two-mode action shape on the decide tool. Two
substrate-level design choices worth recording:

### Two-mode `dev_via_test` token (subtopics presence as differentiator)

Slice 1's rule 01 fires on `decide(action="dev_via_test")` to
spawn Lisa. Slice 3's walker also emits `decide(action="dev_via_test", ...)`
— to dispatch Ralph at a specific task. Same token, different
spawn target.

Differentiation via `coordinator.decision.subtopics.length`:

- Rule 01: `length_eq 0` (initial dispatch, no specific target)
  → spawn Lisa (planner)
- Rule 03: `length_gt 0` (walker chose target) → spawn Ralph
  (executor) at `subtopics[0]` via `for_each`

Verified against beta.96:

- `decide` tool stamps `coordinator.decision.subtopics` ONLY when
  non-empty (`processor/agentic-tools/decide.go`). Empty/nil →
  no triple stamped.
- Array operators (`length_eq`, `length_gt`) deliberately treat
  missing predicate as empty array — `length_eq 0` matches both
  "absent" and "explicit []" (`processor/rule/expression/types.go`).
- Mutually exclusive on the same condition triple — no double-fire.

Alternatives considered + rejected:

- *New token (`dispatch_task`, `walk_next`, etc.).* Would have
  expanded the closed action taxonomy on the coordinator persona
  — more tokens to maintain, more rules, more documentation. The
  semantic IS "dev_via_test work" in both cases; subtopics
  presence cleanly carries the "which task" payload without a
  new token.
- *Walker emits via a new tool (`dispatch_next_task`).* Would have
  required a new product-shell tool just to write a marker triple
  the rule layer could match on. The decide tool already does
  exactly this (`coordinator.decision.next_action` stamp); reusing
  it with the subtopics overload is the framework-aligned shape.

### `for_each` over subtopics for Ralph dispatch (v1 N=1, v2 N>1)

Rule 03 uses `for_each: "$entity.triple.coordinator.decision.subtopics"`
+ `for_each_var: "subtopic"` — the same pattern as research pack's
rule 02 (verified `configs/rules/research/02-plan-to-gather.json`).
v1 walker emits single-element subtopics for serial dispatch;
`for_each` runs once per walker decision and spawns one Ralph.

v2 will support parallel dispatch via multi-element subtopics
(`subtopics=["t1","t2"]`) — `for_each` spawns N Ralphs concurrently
(same parallel-loop semantics as research pack's N-gatherer
fan-out). Gated by `plan.task.<id>.depends_on` topo-walking which
is deferred to v2 per §DAG awareness; v1 walker chooses one task
per decision and walks in plan-order.

The choice to ship the same `for_each` pattern even for N=1 means
the v2 parallelization is a persona-level change ("emit multi-
element subtopics") rather than a rule/architecture change. Cheap
upgrade path.

### Derivative status (no plan.task.<id>.status mutation)

Slice 2's rules 04a/04b stamp `dev_via_test.execute.task_completed`
or `task_failed` on the run entity (multi-valued — one triple per
Ralph). Slice 3's walker reads ALL these triples and computes
effective status **derivatively**:

- `done` if task ID appears in `task_completed` list
- `blocked` if task ID appears in `task_failed` list
- `ready` otherwise (matches Lisa's initial stamp; never mutated)

Plan.task.<id>.status stays "ready" on every task across the
chain's lifetime. Status is a pure function of execution markers.

This avoids the predicate-substitution gap (rule engine substitutes
triple OBJECTS but not predicate fragments — `plan.task.${TASK_ID}.status`
isn't a thing in beta.96), and eliminates a race-condition class
(partial-write of status update vs. concurrent walker read). Walker
state computation is monotonic — adding new markers can only
advance a task from ready to done/blocked, never the reverse.

The trade-off: walker must read ALL run-entity triples to compute
status (single `query_entity` call), and process the multi-valued
markers locally. For plans up to ~50 tasks this is negligible; for
much larger plans we'd add a "compaction" rule that collapses
markers into a summary triple. Not worth the design overhead for
v1.

### Slice 4 handoff

Rule 05's prompt step 4 currently routes "all done" → `respond_direct`
directly. Slice 4 will:

- Add a new walker action token (likely `dev_via_test_finalize` or
  reuse `dev_via_test` with `subtopics=["__cbg__"]` sentinel) →
  spawn CBG (`reviewer-dev-via-test`).
- Add rule 06 (walker decide finalize → CBG spawn) + rule 07 (CBG
  approved → coordinator wake-up for respond_direct).
- Update rule 08's role list to include `reviewer-dev-via-test`.
- Update walker persona to route "all done" through CBG instead of
  direct respond_direct.

Slice 3's walker prompt explicitly notes "Slice 4 will replace
this with a CBG dispatch route" so persona drift between Slice 3
and Slice 4 stays visible.

## Addendum 2026-06-03 — Slice 4 (CBG chain-end gate) design

Slice 4 closes the chain: rule 06 (walker→CBG), rule 07a (CBG
approved→final coordinator respond_direct), rule 07b (CBG
rejected→final coordinator ask_user), rule 08 extension to include
CBG, new `reviewer-dev-via-test` persona dir, new `dev_via_test_finalize`
action token in the coordinator's closed taxonomy.

### New action token: `dev_via_test_finalize`

Per Slice 3 reviewer R5 + the §Slice 4 handoff sketch above:
walker emits this token when all `plan.task.*` are done. Distinct
from `dev_via_test` (which routes to Lisa OR Ralph via rule 01/03's
subtopics differentiator) to avoid overloading a THIRD meaning on
the same token.

Alternatives considered + rejected:

- *Reuse `dev_via_test` with sentinel subtopics like `["__cbg__"]`.*
  Rejected: rule 03's `for_each` over subtopics would dispatch Ralph
  at task ID `"__cbg__"` (no such task in the plan, Ralph wedges).
  A new condition `subtopics[0] != "__cbg__"` on rule 03 would work
  but adds fragility (string-sentinel match).
- *Reuse `dev_via_test` with empty subtopics from walker.* Rejected
  per Slice 3 reviewer B1: walker has lineage, so rule 01 (which
  requires `lineage.run-loop-entity-id length_eq 0`) doesn't fire.
  The dispatch silently no-ops — chain wedges with no CBG.
- *Reuse `respond_direct` for "all done."* Rejected — walker would
  bypass CBG entirely (the gate that catches cross-task drift),
  defeating the chain-end review purpose. Plus the user-facing
  rollup the walker writes wouldn't have the integration-test
  result CBG provides.

The new token is the cleanest dispatch contract: one signal, one
target, no rule-condition gymnastics, no sentinel-string fragility.
Cost: one more closed-taxonomy entry the coordinator persona must
learn. Worth it for clarity.

### Approve/reject split (rules 07a vs 07b)

CBG's two outcomes route to different final-wake-up shapes:

- **Approved** (rule 07a): final coordinator scoped to
  `[respond_direct, ask_user]`. Delivers CBG's rollup to the user.
- **Rejected** (rule 07b): final coordinator scoped to
  `[ask_user, respond_direct]` (ask_user first — that's the expected
  path). Surfaces CBG's verdict and asks what to do.

Two rules instead of one rule that conditions on the outcome value
because: (1) rule 07a's prompt is "deliver the result," rule 07b's
prompt is "ask the user about the failure" — meaningfully different
guidance for the walker; (2) the `wakeup_mode` properties tag
differs (chain_terminal_dev_via_test_approved vs _rejected), and
operator-facing telemetry shouldn't have to grep through verdict
strings to distinguish.

### CBG's discipline — single-run gate, no recovery loop

Per ADR-044 §CBG's gate at chain-end: "Per-Ralph reviewer is
optional in v1 (test-pass is the deterministic signal). Don't slip
into per-Ralph reviewer ceremony — that's the BMAD pattern that
this ADR exists to avoid."

Slice 4's CBG persona (`reviewer-dev-via-test/10-review-contract.md`)
extends this discipline to CBG itself:

- ONE integration test run. Failure → reject. No retry, no
  "let me try with different args."
- Diff sanity-check is a skim, not a code review. CBG looks for
  file-scope drift + test-gaming + obvious bugs the integration
  test doesn't catch.
- No fixing. CBG rejects and routes to user; Ralph fixes (next
  chain after user amends plan), user picks the move.

CBG's tool set is deliberately narrow: `query_entity`,
`read_loop_result`, `bash`, `scratchpad`, `decide`. No
`emit_*` (CBG doesn't render artifacts — the artifact is the
diff + integration-test result, both already-on-disk in the
sandbox); no `request_sandbox` (already provisioned upstream); no
write tools other than via bash (which can edit files but persona
forbids it).

### Rule 08 extension (CBG)

CBG loop-failures (panic, framework max-iter without `decide`,
NATS error, anything where `outcome IN [failed, truncated, cancelled]`)
have no in-arc recovery path. Per Slice 3 reviewer's `R2/N8`
structural fence, CBG must appear in rule 08's role list to trigger
chain.paused on these classes. CBG `decide(rejected)` is NOT a
loop-failure (`outcome=success`); that routes through rule 07b
(ask_user).

### Upstream-criterion for `_finalize` per-pack tokens

Per Slice 4 reviewer R5: the `dev_via_test_finalize` token is the
right per-pack shape for v1, but the pattern will recur if more
"outer-loop + inner-arc with chain-end reviewer" packs ship
(`research_finalize`, `web_research_finalize`, etc.). Per
CLAUDE.md "Product-Shell-Tool Discipline" + the evidence-trail
discipline: name the **trigger condition** to escalate to a
framework primitive rather than letting tokens proliferate.

**Escalation trigger:** when a third dev-via-test-shaped pack
(coordinator-walked outer loop + per-task inner arc + chain-end
reviewer that gates on a deterministic signal) ships a `_finalize`
token, evaluate lifting to a framework-level `coordinator.chain.finalize`
action with pack-name as a parameter. Two packs is "coincidence";
three is "pattern" worth a framework primitive.

Until then: per-pack `_finalize` tokens carry the right per-pack
semantics (different reviewer roles, different artifact shapes,
different approval contracts) without the cost of a generic
machinery layer that would have to thread pack-specific state.

### Chain flow end-to-end (Slices 1-4)

See `configs/rules/dev-via-test/README.md` for the full ASCII flow.
Key invariants:

- Lineage threading: every dev-via-test entity carries
  `lineage.run-loop-entity-id` pointing at the original
  front-door coordinator's loop entity. Reads from the run
  entity always target that one ID.
- Two-mode `dev_via_test` distinguishes Lisa initial vs Ralph
  walker dispatch (`subtopics.length` differentiator + rule 01's
  lineage fence per Slice 3 reviewer B1).
- One-shot `dev_via_test_finalize` dispatches CBG; the chain
  cannot loop back to plan-editing automatically (no
  auto-recover per §Stuck-task recovery + §CBG's gate).
- Failure paths visible: rule 04b (Ralph fails → walker →
  ask_user), rule 07b (CBG rejects → final → ask_user), rule 08
  (Lisa/CBG loop-fails → chain.paused).

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
