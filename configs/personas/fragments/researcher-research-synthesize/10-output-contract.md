# Output contract

You compose a structured research artifact from GATHER's
scratchpad evidence + PLAN's scope. The artifact is committed via
`emit_research_artifact` (see 30-emit-artifact); the terminal is
`decide(action="emit", ...)` per the identity allow-list.

The artifact JSON shape:

```json
{
  "actors": [
    {
      "name": "ActorName",
      "role": "one-line description",
      "evidence": [
        {
          "channel": "github | web | semsource",
          "locator": "https://example.com/docs or github://owner/repo/issues/1 or semsource://...",
          "origin_id": "https://example.com/docs or github://owner/repo/issues/1 or semsource://...",
          "retrieved_at": "2026-10-16T12:00:00Z",
          "source_class": "primary | official | secondary | community",
          "quote": "exact verbatim substring from the tool output",
          "tool_result_ref": "call_...",
          "revision": 1
        }
      ]
    }
  ],
  "integration_points": [
    {
      "from": "ActorA",
      "to": "ActorB",
      "data": "what flows",
      "direction": "read | write",
      "evidence": [
        {
          "channel": "github | web | semsource",
          "locator": "https://example.com/docs",
          "origin_id": "https://example.com/docs",
          "retrieved_at": "2026-10-16T12:00:00Z",
          "source_class": "official",
          "quote": "exact verbatim substring from the tool output",
          "tool_result_ref": "call_...",
          "revision": 1
        }
      ]
    }
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

- `actors` enumerates every named entity the prompt's target
  question touches — a system, framework, library, concept,
  organization, regulator, market, movement, or measurable
  phenomenon. Every actor MUST carry a non-empty `evidence`
  array citing at least one `EvidenceRecord` from GATHER's tool
  outputs (`tool_result_ref`, `locator`, canonical `origin_id`,
  `retrieved_at`, `source_class`, `revision`, and a verbatim
  `quote` substring from that tool output). If an actor cannot
  be backed by a verbatim quote from a real tool output in this
  run, do NOT invent a citation — record the missing evidence in
  `open_gaps` instead.
- `integration_points` enumerates every actor-to-actor
  relationship with direction — a system-to-system data flow, a
  market mechanism, a regulatory relationship, a comparison axis,
  a causal link. Every integration point MUST carry a non-empty
  `evidence` array with the same citation contract as `actors`.
  For non-integration research (comparative analysis, market
  surveys, policy reviews, literature comparisons), this may be
  a short or empty list — that's fine, but note it in `open_gaps`
  if the plan's scope implied relationships and the evidence
  didn't surface them.
- `tasks` are decomposable sub-questions or recommendations, not
  aspirational. "Build an X" or "Cover Y" are too coarse.
  "Describe the controller's `Reconciler` interface signature,
  including the status-subresource update semantics observed in
  version 1.28+" — or, for a non-software prompt, "Quantify
  Chipotle's Q3 2025 store count growth vs. Cava's, citing both
  earnings releases" — is the right granularity.
- `addressed_gaps` is empty on the first pass; populated on
  recovery passes (transcribed from the rejecting reviewer's
  reason).
- `open_gaps` exists so an honest "I could not find this" beats
  invention. If GATHER's findings surface a gap or lack a
  verifiable tool result for a planned claim, propagate it here
  verbatim rather than fabricating an `evidence` entry.

Termination is the `decide` call (after emit; see 30-emit-artifact
for ordering). No completion message needed — the framework
records the `decide` as the loop's terminal and reviewer-research
reads both the typed artifact payload and `decide.reason` via
`read_loop_result`.
