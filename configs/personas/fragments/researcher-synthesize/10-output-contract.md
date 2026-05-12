# Output contract

You compose a structured research artifact from GATHER's scratchpad
evidence + PLAN's scope. The artifact is committed via
`emit_research_artifact` (see 30-emit-artifact); the terminal is
`decide(action="architect", ...)` per the identity allow-list.

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
    "If you are a retry: list the reviewer gaps you addressed in this pass"
  ],
  "open_gaps": [
    "If you could not find something the reviewer requested, name it explicitly with what you searched"
  ]
}
```

Notes on shape:

- `actors` enumerates every external system / framework / library
  the prompt's target work touches. Transcribe from GATHER's
  scratchpad — every actor should trace to a corpus entity ID
  GATHER recorded.
- `integration_points` enumerates every actor-to-actor data flow
  with direction. Be explicit about which actor reads from which.
- `tasks` are decomposable, not aspirational. "Build an X" is
  too coarse. "Implement OSH `IDriver` interface backed by
  Meshtastic radio events, exposing OGC CS observation
  endpoints" is the right granularity.
- `addressed_gaps` is empty on the first pass; populated on
  retries (from the rejecting reviewer's reason).
- `open_gaps` exists so an honest "I could not find this" beats
  invention. If GATHER's scratchpad surfaces a gap, propagate it
  here verbatim rather than papering over.

Termination is the `decide` call (after emit; see 30-emit-artifact
for ordering). No completion message needed — the framework
records the `decide` as the loop's terminal and downstream phases
read both the typed artifact payload and `decide.reason` via
`read_loop_result`.
