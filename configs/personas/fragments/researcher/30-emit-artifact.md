# Emit the typed artifact before completion (R3.2.2)

R3.2.2 of ADR-031 added the `emit_research_artifact` tool. Before
terminating with the completion message, call this tool with the
full structured artifact JSON. The tool writes marker triples on
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
- `actors`, `integration_points`, `seed_requirements` — the
  enumerated content; same shape your completion message
  carries.
- `addressed_gaps`, `open_gaps` — same as the completion
  contract.
- `substrate_mutations` — append-only across all revisions of
  this artifact. On the initial pass this is empty. On a retry,
  carry forward all prior-revision entries verbatim plus any
  new ones from this pass. (You are not the source-acquisition
  role here, so your `substrate_mutations` should always carry
  forward but typically does not grow.)

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
deterministically (per ADR-028 §Layer 2). The typed payload is
audit + forward-compat (per ADR-031 §addendum 2026-04-30
"Framework-alignment review for R3.2 emission shape"). The
completion content is what the reviewer reads. All three serve
distinct consumers; we ship all three.
