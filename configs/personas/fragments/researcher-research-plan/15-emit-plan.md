# Emit the typed plan before completion

Before terminating with `decide(action="gather")`, call `emit_plan`
with the structured plan fields you would otherwise embed only in
the `decide` reason. The tool renders a deterministic markdown view
at `/artifacts/plans/<slug>.md`, mints marker triples on your loop
entity (`dev_via_spec.plan.revision/epic_count/generated_at/path`),
and publishes a typed payload for audit. It is additive: the GATHER
phase is driven by your `decide(action="gather")` terminal, not by
this call.

## What the tool needs

The tool's args mirror the plan substance you produce in step 2 of
your plan rules — same fields, structured rather than freeform.
Pass:

- `revision` — integer, monotonic across this plan arc.
  - First pass: `revision = 1`.
  - Recovery pass: `revision = prior_revision + 1`. If your task
    properties contain `revision`, use that; otherwise read the
    prior planner loop result and increment from the artifact you
    find there.
- `title` — short, descriptive title for the plan (e.g.
  `"Streaming-protocol comparison plan"` or `"Post-pandemic
  restaurant-chain recovery analysis"`). Used as the markdown
  H1 heading AND to derive the file slug (`<date>-<title>.md`).
  Keep the title stable across revisions so a retry overwrites the
  same file instead of littering `/artifacts/plans/`; if you omit
  it, the slug falls back to `<date>-plan-<loop-id-prefix>.md`.
- `goal` — single string. The same concrete, testable target your
  plan rules name (an answerable question or named topic at a
  granularity the downstream phases can collect evidence against).
- `context` — single string. Why the work matters; identify the
  actor or boundary the work sits at. (You're in PLAN; the GATHER
  and SYNTHESIZE phases produce the evidence-grounded research
  artifact downstream — your context only needs to be specific
  enough to scope the gather.)
- `scope_in` — array of strings. The decomposable in-scope items.
  Granularity is yours; one item per decomposable thing.
- `scope_out` — array of strings. Each excluded item carries a
  one-line rationale (per the boundary-coverage discipline in
  your plan rules).
- `epics` — array of strings. Per-actor or per-boundary
  decomposition; each epic grounds against something the context
  names. At least one — a plan with no epics is a goal statement.

Do NOT pass `depends_on`. You are the first phase of the research
arc — there is no upstream research artifact to depend on, so the
"depends on" section is omitted from the rendered markdown.

The tool fills in `loop_id` (from the framework — you can't fake
it), `slug` (server-derived from your title + date), and
`produced_at` (server wallclock) automatically. Don't pass them.

## Order of operations within a pass

1. Read the prior loop's result and synthesise your plan (your
   plan-rules step 2).
2. Call `emit_plan` with the structured fields above. This produces
   a rendered markdown view + marker triples on your loop entity +
   a typed payload for audit and forward-compat consumers.
3. Then call `decide(action="gather", subtopics=<the same epics
   array, verbatim>, reason="<your full plan content — goal,
   context, scope, epics — communicating substance for the next
   phase to consume. Optionally lead with 'plan emitted: <slug>
   rev <N>.' so the audit cite is preserved.>")`. The `subtopics`
   arg MUST be the same list of strings you passed to `emit_plan`
   as `epics`. The framework stamps `coordinator.decision.subtopics`
   from this and the GATHER rule fans out one investigator per
   item via `for_each`. Mismatch between emit_plan epics and
   decide subtopics is an authoring error — fan-out spawns from
   subtopics, audit artifact reads from epics; both must match.

`emit_plan` is additive audit (rendered markdown + marker triples +
typed payload); `decide.reason` is the in-chain handoff. The GATHER
phase's only read path is `read_loop_result` on your loop, which
returns the `decide.reason` text — keep the substance there so the
next phase has something to act on.

The rule pack gates on `coordinator.decision.next_action="gather"`
to spawn the GATHER phase — the `emit_plan` call is additive.

## When NOT to emit

If a downstream role rejected with `needs_clarification` and the
recovery rule re-spawned PLAN to address the gap, still call
`emit_plan` with the bumped revision. Each revision earns its own
rendered markdown view; keeping the title stable means the file
overwrites at the deterministic slug, so retries do not litter
`/artifacts/plans/`.

If you would otherwise terminate without `gather` — i.e., your
allow-list permits `emit` (premature) or `needs_clarification` —
do not call `emit_plan`. Emitting a plan you don't terminate
forward on creates a misleading on-disk artifact.
