# Emit the typed artifact before completion (R3.2.2)

Same contract as the plain researcher's `30-emit-artifact.md`
fragment, with one addition specific to your role: you can
modify the substrate via `add_source_repo`, so your
`substrate_mutations` array is the load-bearing piece for the
reviewer's stabilisation gate.

## What the tool needs

Pass the artifact JSON as args (no wrapping object). The fields
that matter most for your role:

- `revision` — read the prior reviewer's loop result via
  `read_loop_result` (you do this anyway to read the gaps), locate
  the artifact JSON in its content, and use
  `prior_artifact.revision + 1`. The retry rule (02) does not
  pass an explicit revision — increment from what you read.
- `substrate_mutations` — **append-only across all revisions**.
  Carry forward every entry from the prior artifact verbatim,
  then append a new entry per `add_source_repo` call you made
  in this pass. Each new entry's `revision` field equals this
  artifact's `revision`.

  Mutation entry shape:

  ```json
  {
    "tool": "add_source_repo",
    "args": { "url": "...", "namespace": "...", "branch": "..." },
    "loop_id": "<your loop id>",
    "revision": <this revision number>,
    "approved_by": "<approver from the approval round-trip>",
    "status": "executed",
    "result": "<one-line result summary>",
    "timestamp": "<RFC3339>"
  }
  ```

The tool fills in `loop_id` on the artifact itself (server-supplied);
you fill in `loop_id` on each `Mutation` entry to attribute the
mutation to the loop that emitted it.

## Why this matters for stabilisation

The reviewer's stabilisation gate (ADR-031 §addendum 2026-04-30,
implemented in `research-reviewer/30-stabilisation-check.md`)
counts mutations whose `revision == artifact.revision`. If your
pass added a source via `add_source_repo`, that mutation has
`revision = current revision` and the reviewer will reject with
"awaiting stabilisation, re-iterate against augmented corpus" —
even if the rest of your artifact is complete. That is intended:
the next pass (with no new mutations, just queries against the
extended corpus) is what stabilises the arc.

## Order of operations within a pass

1. Read prior reviewer's `reason` via `read_loop_result`.
2. If a corpus gap, call `add_source_repo` (approval-gated).
3. Query the (now-augmented) corpus to fill the gaps.
4. Call `emit_research_artifact` with the full updated JSON
   (carry-forward prior `substrate_mutations` + this pass's new
   entries).
5. Emit the completion message with the same artifact JSON.

If your pass added a source, expect the next reviewer pass to
reject as `insufficient` per the stabilisation gate. The arc
stabilises on the *subsequent* researcher pass that queries
without modifying.
