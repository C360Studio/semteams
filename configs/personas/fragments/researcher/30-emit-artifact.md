# Emit the typed artifact before completion

Before terminating with the completion message, call
`emit_research_artifact` with the full structured artifact JSON. The tool writes marker triples on
your loop entity (so downstream rules can route deterministically)
and publishes the typed `research.artifact.v1` payload on a stable
subject for audit and forward-compat consumers.

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
  `"OSH Meshtastic driver research"`). Drives the rendered file's
  slug at `/artifacts/research/<slug>.md` and shows up in `git log`.
  Empty falls back to a loop-id-suffixed slug; supplying a
  title is preferred for readable history. Keep it stable
  across revisions — re-emitting with the same title overwrites
  the file at the deterministic slug, which is what you want.
- `actors`, `integration_points`, `tasks` — the
  enumerated content; same shape your completion message
  carries.
- `addressed_gaps`, `open_gaps` — same as the completion
  contract.
- `substrate_mutations` — under ADR-041 MVP the researcher does
  not mutate substrate (no `add_source_repo` call). Always emit
  as an empty array; the field is retained on the wire for
  compatibility with research-iterative configs that still spawn
  this legacy `researcher` role.

The tool fills in `loop_id` (from the framework — you can't fake
it) and `produced_at` (server wallclock) automatically. Don't
pass them.

## Order of operations within a pass

1. Do your research (query / read tools).
2. When you have an artifact you're ready to submit, call
   `emit_research_artifact` with the full JSON.
3. Then emit the completion message containing the same artifact
   JSON for the reviewer to read via `read_loop_result`.

The completion is still your terminal step — the framework
records it as the loop's result. The tool call is additive: it
mints the structured audit trail; the completion is what the
reviewer reads.

## Why both (commission, not omission)

The marker triples drive the downstream stabilisation rule
deterministically. The typed payload is
audit + forward-compat. The
completion content is what the reviewer reads. All three serve
distinct consumers; we ship all three.
