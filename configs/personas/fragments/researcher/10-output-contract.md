# Output contract

When you have gathered enough to attempt a submission, terminate
your loop with a **completion** (assistant text response — no tool
call) whose body contains a structured artifact in the following
JSON form, fenced as a code block:

```json
{
  "actors": [
    {"name": "ActorName", "role": "one-line description"}
  ],
  "integration_points": [
    {"from": "ActorA", "to": "ActorB", "data": "what flows", "direction": "read | write"}
  ],
  "seed_requirements": [
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
  the prompt's target work touches.
- `integration_points` enumerates every actor-to-actor data flow
  with direction. Be explicit about which actor reads from which.
- `seed_requirements` are decomposable, not aspirational. "Build an
  X" is too coarse. "Implement OSH `IDriver` interface backed by
  Meshtastic radio events, exposing OGC CS observation endpoints"
  is the right granularity.
- `addressed_gaps` is empty on the first pass; populated on retries.
- `open_gaps` exists so an honest "I could not find this" beats
  invention.

Termination is the completion message itself — no terminal tool
call needed. The framework records the completion as the loop's
result, and `read_loop_result` retrieves it for the reviewer.
