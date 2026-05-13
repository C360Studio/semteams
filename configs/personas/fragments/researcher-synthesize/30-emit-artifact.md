# Emit the typed artifact then `decide`

Before your terminal `decide` call, call `emit_research_artifact`
with the full structured artifact JSON. The tool writes marker
triples on your loop entity (so downstream rules can route
deterministically) and publishes the typed `research.artifact.v1`
payload on a stable subject for audit and forward-compat
consumers.

## What the tool needs

The tool's args mirror the artifact JSON shape from your output
contract — no nesting under a wrapping object. Pass:

- `revision` — integer, monotonic across this research arc.
  - First pass: `revision = 1`.
  - Retry: `revision = prior_revision + 1`. If your task
    properties contain `revision`, use that; otherwise read
    the prior reviewer's loop result and increment from the
    artifact you find there.
- `title` — short, descriptive title for this research (e.g.
  `"OSH Meshtastic driver research"`). Drives the rendered
  file's slug at `/artifacts/research/<slug>.md` and shows up in
  `git log`. Empty falls back to a loop-id-suffixed slug;
  supplying a title is preferred for readable history. Keep it
  stable across revisions — re-emitting with the same title
  overwrites the file at the deterministic slug.
- `actors`, `integration_points`, `tasks` — the enumerated
  content; transcribed from your scratchpad evidence.
- `addressed_gaps`, `open_gaps` — gaps the reviewer named (now
  addressed) and gaps the corpus genuinely doesn't support.
- `substrate_mutations` — append-only across all revisions of
  this artifact. Under MVP roster, source acquisition is operator-
  / semsource-watcher-driven, not researcher-driven, so this
  array never grows during your passes. Carry forward prior-
  revision entries verbatim on retries. On a first-pass artifact
  this is always empty.

The tool fills in `loop_id` (from the framework — you can't fake
it) and `produced_at` (server wallclock) automatically. Don't
pass them.

## Order of operations within a pass

1. Read your upstream phases (GATHER's scratchpad evidence + PLAN's
   structure) via `read_loop_result`.
2. Compose the structured artifact in your own scratchpad first.
3. Call `emit_research_artifact` with the full JSON.
4. Then call `decide(action="architect", reason="<one-line summary
   referencing the emitted artifact slug + revision>")` per the
   identity allow-list.

The `decide` call is your terminal — the framework records it as
the loop's result and routes the chain. The emit tool call is
additive audit: it mints the structured payload + marker triples;
the `decide.reason` is what downstream phases read via
`read_loop_result`.

If you need to back-edge to GATHER (synthesis surfaced a corpus
gap the plan didn't anticipate), do NOT call `emit_research_artifact`
on this pass — emit a placeholder artifact creates a misleading
audit record. Terminate with
`decide(action="gather", reason="<gap to re-gather>")` directly.

## Why both emit + decide (commission, not omission)

The marker triples drive the downstream chain rules
deterministically. The typed payload is audit + forward-compat for
operators reading the chain. The `decide.reason` is what
ARCHITECT (next phase) reads to ground its work. All three serve
distinct consumers; we ship all three.
