# Emit the typed artifact then `decide`

Before your terminal `decide` call, call `emit_research_artifact`
with the full structured artifact JSON. The tool writes marker
triples on your loop entity (so the downstream rule can route to
reviewer-research deterministically) and publishes the typed
`research.artifact.v1` payload on a stable subject for audit and
forward-compat consumers.

## What the tool needs

The tool's args mirror the artifact JSON shape from your output
contract — no nesting under a wrapping object. Pass:

- `revision` — integer, monotonic across this research arc.
  - First pass: `revision = 1`.
  - Recovery pass: `revision = prior_revision + 1`. If your task
    properties contain `revision`, use that; otherwise read the
    prior reviewer's loop result and increment from the artifact
    you find there.
- `title` — short, descriptive title for this research (e.g.
  `"Streaming-protocol comparison"` or `"Post-pandemic
  restaurant-chain recovery analysis"`). Drives the rendered
  file's slug at `/artifacts/research/<slug>.md` and shows up
  in `git log`. Empty falls back to a loop-id-suffixed slug;
  supplying a title is preferred for readable history. Keep it
  stable across revisions — re-emitting with the same title
  overwrites the file at the deterministic slug.
- `actors`, `integration_points`, `tasks` — the enumerated
  content; transcribed from your scratchpad evidence.
- `addressed_gaps`, `open_gaps` — gaps the reviewer named (now
  addressed) and gaps the corpus genuinely doesn't support.
- `test_harness` — see 40-harness-catalog for the path-selection
  contract. The tool rejects an artifact that doesn't take one
  of the three paths.
- `substrate_mutations` — append-only across all revisions of
  this artifact. Under the research category, source acquisition
  is operator-driven, not researcher-driven, so this array never
  grows during your passes. Carry forward prior-revision entries
  verbatim on recovery passes. On a first-pass artifact this is
  always empty.

The tool fills in `loop_id` (from the framework — you can't fake
it) and `produced_at` (server wallclock) automatically. Don't
pass them.

## Order of operations within a pass

1. Read your upstream phases (GATHER's scratchpad evidence +
   PLAN's structure) via `read_loop_result`.
2. Compose the structured artifact in your own scratchpad first
   (decide on the `test_harness` stance per 40-harness-catalog
   while you're there).
3. Call `emit_research_artifact` with the full JSON.
4. Then call `decide(action="emit", reason="<one-line summary
   referencing the emitted artifact slug + revision>")` per the
   identity allow-list.

The `decide` call is your terminal — the framework records it as
the loop's result and the pack's
`04-synthesize-to-reviewer.json` rule fires on
`coordinator.decision.next_action="emit"` to spawn
reviewer-research. The emit-tool call is additive audit: it
mints the structured payload + marker triples; the
`decide.reason` is what reviewer-research reads via
`read_loop_result`.

If you need to route back through the planner (synthesis surfaced
an inconsistency GATHER's evidence and the plan's scope can't be
reconciled), do NOT call `emit_research_artifact` on this pass —
emitting a placeholder artifact creates a misleading audit
record. Terminate with `decide(action="needs_clarification",
reason="...", retry_hint="...")` directly per the iteration
rules.

## Why both emit + decide (commission, not omission)

The marker triples drive the
`04-synthesize-to-reviewer.json` rule deterministically. The
typed payload is audit + forward-compat for operators reading
the chain. The `decide.reason` is what reviewer-research reads
to ground its evaluation. All three serve distinct consumers; we
ship all three.
