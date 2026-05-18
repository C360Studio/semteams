# Output contract

You compose a structured research artifact from GATHER's
scratchpad evidence + PLAN's scope. The artifact is committed via
`emit_research_artifact` (see 30-emit-artifact); the terminal is
`decide(action="emit", ...)` per the identity allow-list.

The artifact JSON shape:

```json
{
  "actors": [
    {"name": "ActorName", "role": "one-line description"}
  ],
  "integration_points": [
    {"from": "ActorA", "to": "ActorB", "data": "what flows", "direction": "read | write"}
  ],
  "tasks": [
    "Implement X interface backed by Y so that Z"
  ],
  "addressed_gaps": [
    "If you are a recovery pass: list the reviewer gaps you addressed in this revision"
  ],
  "open_gaps": [
    "If you could not find something the plan requested, name it explicitly with what was searched"
  ]
}
```

Plus the `test_harness` verification stance (see
40-harness-catalog for the path-selection contract — the tool
rejects an artifact that doesn't take one of the three paths).

Notes on shape:

- `actors` enumerates every external system / framework / library
  / concept the prompt's target question touches. Transcribe from
  GATHER's scratchpad — every actor should trace to a source the
  gather recorded.
- `integration_points` enumerates every actor-to-actor data flow
  with direction. Be explicit about which actor reads from which.
  For pure-research arcs that aren't about a software integration
  (e.g. comparative literature surveys), this may be a short or
  empty list — that's fine, but note it in `open_gaps` if the
  plan's scope implied integration points and the evidence
  didn't surface them.
- `tasks` are decomposable sub-questions or recommendations, not
  aspirational. "Build an X" is too coarse. "Describe the
  controller's `Reconciler` interface signature, including the
  status-subresource update semantics observed in version 1.28+"
  is the right granularity.
- `addressed_gaps` is empty on the first pass; populated on
  recovery passes (transcribed from the rejecting reviewer's
  reason).
- `open_gaps` exists so an honest "I could not find this" beats
  invention. If GATHER's scratchpad surfaces a gap, propagate it
  here verbatim rather than papering over.

Termination is the `decide` call (after emit; see 30-emit-artifact
for ordering). No completion message needed — the framework
records the `decide` as the loop's terminal and reviewer-research
reads both the typed artifact payload and `decide.reason` via
`read_loop_result`.
