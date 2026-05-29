# Autoresearch category rule pack

**ADR-042 §Phase 2 redesign — second category pack.** Substrate-test pack
that runs a Karpathy/Shopify autoresearch loop: a metric is named at
classification time, the substrate measures a baseline, then iterates
propose → execute → empirical-decide until a stop condition fires, then
synthesizes a final artifact and routes to a reviewer.

The pack runs through the same substrate singletons as the research
pack — `configs/flow-bootstrap.json` wires graph-ingest, graph-query,
rule-processor, agentic-loop, agentic-dispatch, agentic-tools,
agentic-model — and adds category-keyed rules + role-scoped persona
bundles.

## What this pack proves about the substrate

The research pack runs ONE arc with an LLM reviewer judging substance.
This pack runs N arcs (one per iteration), shares state across them via
run-entity triples, and the per-iteration "did we keep this change?"
decision is **empirical** (the tool executor compares numbers; the rule
engine routes on the outcome) rather than LLM judgment.

If this pack runs, the substrate-plus-overlays claim from ADR-042 is
backed by two structurally-different category contracts, not one. The
specific substrate claims under test:

1. **Cross-iteration state model.** The COORDINATOR loop entity serves
   as the "run entity"; each iteration's experiment loops accumulate
   triples on it via `subject` override (beta.83). Iteration count is
   the `length` of the `autoresearch.experiment.completed` predicate;
   stop conditions match on `length_eq cap`.
2. **Empirical reviewer.** The per-iteration keep/revert decision is
   computed by `emit_autoresearch_measurement` (the tool executor reads
   prior best.value from the run entity, compares to this iteration's
   measured value, stamps `autoresearch.measurement.outcome` on the
   execute loop). The rule engine routes on outcome only — no LLM in
   the inner loop.
3. **Sandbox in the hot path.** Each execute loop runs the measurement
   command in the sandbox container (`services:sandbox:up`, ADR-032 +
   PR #179 always-warm). The research pack does not use sandbox; this
   pack consumes the primitive that PR #179 made always-on.

## Naming convention

Per ADR-042 open question #2, role tokens follow
`<cognitive-role>-<category>-<phase?>`:

| Role token | Phase | Persona dir |
|---|---|---|
| `autoresearch-baseline` | baseline | `configs/personas/fragments/autoresearch-baseline/` |
| `autoresearch-propose` | propose | `configs/personas/fragments/autoresearch-propose/` |
| `autoresearch-execute` | execute | `configs/personas/fragments/autoresearch-execute/` |
| `autoresearch-synthesize` | synthesize | `configs/personas/fragments/autoresearch-synthesize/` |
| `reviewer-autoresearch` | (single-phase) | `configs/personas/fragments/reviewer-autoresearch/` |

The `autoresearch-` prefix lets `ls configs/personas/fragments/ | grep
^autoresearch-` enumerate the pack's persona-producing roles in phase
order. `reviewer-autoresearch` follows the `reviewer-<category>`
convention (mirroring `reviewer-research`); it is a separate persona
from `reviewer-research` because the artifact shape it grades is
different (baseline + experiments + best, not actors/integration_points).

## Rules

| File | Trigger | Spawn / Stamp |
|---|---|---|
| `01-coordinator-autoresearch-spawn.json` | coordinator decide(autoresearch) | autoresearch-baseline + stamp `autoresearch.cap`, `.surface`, `.command`, `.run.status=active` on coordinator (run) entity |
| `02-baseline-to-propose.json` | autoresearch-baseline decide(propose) | autoresearch-propose (iteration 1) |
| `03-propose-to-execute.json` | autoresearch-propose decide(measure) | autoresearch-execute |
| `04a-execute-stamp-completion.json` | autoresearch-execute decide(measured) + outcome=success | stamp `autoresearch.experiment.completed` on run entity (Object = execute loop id) |
| `04b-execute-stamp-failed.json` | autoresearch-execute outcome=failed (no decide; loop crashed) | stamp `autoresearch.experiment.completed` + `autoresearch.experiment.loop_failed` on run entity |
| `05-experiment-continue.json` | run entity's `experiment.completed length_lt cap` | autoresearch-propose (next iteration) |
| `06-experiment-stop-cap.json` | run entity's `experiment.completed length_eq cap` | autoresearch-synthesize + stamp `autoresearch.stop.reason=cap` |
| `07-synthesize-to-reviewer.json` | autoresearch-synthesize decide(emit) | reviewer-autoresearch |
| `08-reviewer-approved-to-coordinator.json` | reviewer-autoresearch decide(approved) | coordinator (wake-up for respond_direct) |
| `09-reviewer-rejected-resynthesize.json` | reviewer-autoresearch decide(insufficient) | autoresearch-synthesize (max_iterations=2 — budget already spent on iteration loops, so this re-rolls the rollup but does NOT re-iterate experiments) |
| `10-needs-clarification-replan.json` | any pack role decide(needs_clarification) | coordinator (max_iterations=3) |
| `11-loop-failed-pause.json` | any pack role EXCEPT autoresearch-execute terminates with outcome=failed | stamp `chain.paused.marker` (autoresearch-execute loop-failures route through 04b instead — counted toward cap, chain continues) |

### Iteration counter accounting (per reviewer C1 fix 2026-05-29)

The cap budget measures **execute attempts**, not "clean executes." Three classes of execute terminal:

- **Clean (outcome=success + decide=measured):** rule 04a stamps `experiment.completed` and `measurement.outcome=kept|reverted|crashed` (executor-computed). Counts toward cap. Normal path.
- **Loop-failed (outcome=failed, no decide):** rule 04b stamps `experiment.completed` AND `experiment.loop_failed`. Counts toward cap; chain continues to next propose. SYNTHESIZE's rollup marks these "iterations that consumed budget without producing measurement data."
- **Needs clarification (outcome=success + decide=needs_clarification):** rule 10 routes to coordinator. Does NOT count toward cap — structural failure surfaces to user; the run can be re-issued with tightened framing.

Pre-execute failures (propose persona can't form a hypothesis, propose's revert step crashes before spawning execute) similarly bypass the cap counter and route through rule 10. The semantic boundary: the cap protects the user against runaway execute spend; failures BEFORE/OUTSIDE execute are user-visible and re-issuable, not silent budget waste.

### Iteration shape (rules 02-06)

The coordinator parses the user's autoresearch ask into:

- `command` — what to run for the measurement (e.g. `task test:integration`)
- `surface` — file globs the agent may edit (e.g. `test/, Taskfile.yml`)
- `cap` — iteration cap (integer; v1 stop condition)
- `metric_parser` — how to extract the metric from command stdout (e.g. "last wallclock seconds")

Rule 01 stamps those four onto the coordinator's loop entity (THE RUN
ENTITY) and spawns `autoresearch-baseline`. Baseline runs the command
once, stamps `autoresearch.baseline.value` + `.baseline.pass` on the
run entity, and terminates `decide(propose)`.

Rule 02 spawns the first `autoresearch-propose` loop. Propose reads run
state (baseline value, surface, prior experiment summaries via the
beta.86 `.triples` substitution) from its spawn prompt, generates a
hypothesis via scratchpad, applies a diff via `bash` within the surface,
and terminates `decide(measure)`.

Rule 03 spawns `autoresearch-execute`. Execute runs the command in
sandbox, captures stdout + exit code, calls
`emit_autoresearch_measurement(value, pass)`. The tool executor reads
the run entity's `autoresearch.best.value`, compares numerically (lower
== better in v1), stamps `autoresearch.measurement.outcome=kept` if
improved-and-pass else `=reverted` (or `=crashed` if pass=false), and
on `kept` also updates `autoresearch.best.value` + `.best.experiment_id`
on the run entity via subject override. Execute then terminates
`decide(measured)`.

Rule 04 stamps `autoresearch.experiment.completed` on the run entity
(Object = execute loop id) so the experiment counter accumulates. This
is the **iteration JOIN** — every completed iteration adds one triple.

Rules 05 and 06 are mutually exclusive on the run entity:

- **05 (continue)**: `experiment.completed length_lt cap` → spawn the
  next `autoresearch-propose`. Fires every time a new completion lands,
  up to `cap-1` total fires.
- **06 (stop)**: `experiment.completed length_eq cap` → spawn
  `autoresearch-synthesize` + stamp `autoresearch.stop.reason=cap`.
  Fires once when the Nth completion lands.

The `cap` value is `$entity.triple.autoresearch.cap` (substituted from
rule 01's stamp), so the iteration ceiling is data-driven rather than
hard-coded in the rule.

### V1 stop conditions (cap only)

This v1 implements the iteration-cap stop only. Plateau detection
(consecutive `outcome=reverted` for N iterations), time-budget, and
`$`-budget stops are **deferred** because each requires substrate
machinery not yet present:

- **Plateau** needs "last-N triples" semantics or ordered iteration —
  the rule engine has length comparison but not "consecutive in
  insertion order."
- **Time budget** needs wallclock subtraction at rule-fire time.
- **$ budget** needs cost telemetry threaded through the run entity.

The propose persona MAY recommend stop early via `decide(action="stop",
reason=...)` if it observes a clear plateau; that's a follow-up if v1
substrate validation surfaces the need.

### Empirical reviewer lives in the emit tool

The per-iteration "did we keep this change?" decision is computed by
`emit_autoresearch_measurement` in the product-shell tool executor —
NOT by the LLM, NOT by the rule engine. This is the **load-bearing
architectural test** for the autoresearch category contract:

1. The execute loop's LLM observes `measurement.value` from running the
   command but does NOT decide outcome.
2. The tool executor reads `autoresearch.best.value` from the run entity
   (graph KV read), compares numerically, stamps outcome.
3. The rule engine routes on outcome.

This sidesteps the `tool_choice=required` + text-only-completion class
that bit the research pack (semstreams#158) because the inner loop is
not LLM-decided. It also sidesteps the LLM-as-reviewer Goodhart vector
for the inner loop — the LLM cannot fabricate "improved" because it
doesn't make the call. The final SYNTHESIZE-phase rollup IS still
LLM-composed, but its reviewer (`reviewer-autoresearch`) grades the
artifact's structural completeness, not the inner empirical decisions.

The tool executor for `emit_autoresearch_measurement` is **product-shell
work** the pack files do not include. Per CLAUDE.md
"Product-Shell-Tool Discipline" + "framework-alignment review":

- Upstream survey: no existing or planned "compare-and-stamp" primitive
  in semstreams' executor catalog. The closest pattern is the typed
  emit-tool family (emit_plan, emit_research_artifact, emit_diagnosis)
  but those are pure stamp-and-render, no read-compare-route logic.
- Case for product-shell-local: the empirical comparison is the
  load-bearing primitive of the autoresearch category contract.
  Routing the comparison through the LLM defeats the empirical-reviewer
  property; routing through a rule needs conditional numeric updates
  the rule engine does not support today.
- Migration target: if upstream lands an "executor-side read-and-stamp"
  pattern, port to it; otherwise this stays product-shell.
- ADR addendum recording the survey + alternatives ruled out lives at
  `docs/adr/042-coordinator-instantiated-flows-via-templates.md`
  §addendum (to be added when the tool executor lands).

### Sandbox dependency — per-phase posture (per review M4 2026-05-29)

The pack uses bash in different sandboxing postures per phase. ADR-042
§addendum 2026-05-29 §A established the dual-mode sandbox model
(always-warm sandbox + per-tenant containers); this pack consumes both.

| Role | Sandbox posture | Why |
|---|---|---|
| `autoresearch-baseline` | **always-warm sandbox** | Parameter parsing + one initial measurement. The measurement command is the same one used in execute, but baseline runs it once via the always-warm sandbox's bash + `docker exec <tenant>` for cases where the autoresearch is chained from a sandbox-bootstrap arc and the tenant container is already prepared. For standalone autoresearch (no preceding bootstrap), baseline uses always-warm bash directly. |
| `autoresearch-propose` | **always-warm sandbox** | Diff authoring + `git diff --stat` verification. No measurement; no need for tenant. |
| `autoresearch-execute` | **tenant container** (preferred) OR always-warm (fallback) | The measurement command runs against the prepared environment. When this autoresearch arc was chained from a sandbox-bootstrap arc, $entity.triple.lineage.autoresearch-run carries a tenant_container_name and execute runs `docker exec <tenant> <command>`. For self-contained measurements (shell-script profiling, no deps), execute can fall back to always-warm. |
| `autoresearch-synthesize` | **always-warm sandbox** | Artifact composition + `git log` / `git show` reading. No measurement. |
| `reviewer-autoresearch` | **always-warm sandbox** | Reads the rendered artifact via `bash cat`. No measurement. |

Tenant container reference threading: the sandbox-bootstrap pack's
rule 07 chained wake-up stamps `chain.arc.bootstrap.tenant_signature`
+ passes `tenant_container_name` in coordinator wake-up properties.
The wake-up coordinator's `decide(autoresearch, reason="<intent +
tenant_ref>")` threads the tenant references into autoresearch's
spawn rule. Baseline reads them; subsequent execute spawns inherit
via related_loops.

When this autoresearch arc is NOT chained from a bootstrap arc
(user directly asks for autoresearch on a self-contained target),
`tenant_container_name` is empty and execute uses always-warm bash.
If sandbox is down (always-warm), execute loops fail fast and rule
04b stamps experiment.completed + experiment.loop_failed (per the
C1 fix; budget-consumed-but-evidence-empty iteration).

## Open substrate questions (filed as follow-ups, not blockers for ship)

1. **Plateau-N stop condition.** Requires ordered-last-N semantics in
   the rule engine. Workaround: propose persona observes its own
   prior-experiment summaries via `.triples` substitution and may
   recommend stop. Real fix: framework primitive for "last-N triples
   in insertion order."
2. **`emit_autoresearch_measurement` tool executor.** Product-shell tool
   the rules reference but the pack does not implement. Lands as a
   companion PR; until then, this pack does not boot end-to-end against
   real measurements. (Mock-LLM journey can stub the outcome via fixture
   triples.)
3. **UI rendering of N=10 iteration chains.** The kanban view was
   tuned for 4-6-loop research chains. A 10-iteration autoresearch
   chain may look like spaghetti. Likely a UI follow-up after the v1
   real-LLM smoke confirms the pack runs at all; not a substrate-side
   blocker.
4. **Recovery cap interaction.** The chain recovery cap (Slice 2
   chainstall machinery) was tuned for short chains. An autoresearch
   chain that hits multiple `reverted` outcomes is not "stalling" — but
   the chainstall subscriber may misclassify the long chain. Smoke
   validation will surface this.

## No chain.mode / phasevalidator / chainstall

Same posture as the research pack: this pack deliberately omits the
gating triples used by the legacy chain.mode machinery (retired in MVP-7).
The pack uses direct role+decision matches + length-equality on
accumulator triples, with no chain-entity mode or phase-validator
sentinels.

## Migration posture

This is a net-new pack on the post-MVP-7 substrate. No legacy
predecessor to retire. Future revisions may split the empirical-reviewer
logic out of the tool executor into a framework-side rule primitive (if
the substrate gains "executor-side read-and-stamp" capability), but the
pack file layout stays stable.
