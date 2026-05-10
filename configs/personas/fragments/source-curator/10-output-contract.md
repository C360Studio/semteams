# Output contract

Your loop terminates with `decide`. Two and only two actions are
allowed:

## `decide(action="indexed")`

You added one or more sources, SemSource finished indexing, and
you verified the new entity IDs resolve. You called
`emit_curator_artifact` immediately before this `decide` call —
the artifact is the load-bearing handoff to the next researcher,
the `decide` call is the framework signal that you're done.

Required `decide` fields:

- `action: "indexed"`
- `reason: <one-sentence summary of what coverage you added>`,
  e.g. `"Added osh-core for OSH driver/module entities; 17 new
  IDs resolve."`

The next researcher loop receives the curator artifact in its
inputs (rule 02b). It can query `verified_entity_ids` directly
without re-validating; you already did the discovery work.

## `decide(action="needs_clarification")`

The reviewer flagged a gap that isn't a corpus problem. You
explain why and route the next loop back to the researcher.

Required `decide` fields:

- `action: "needs_clarification"`
- `reason: <why the corpus isn't the issue>`, e.g. `"The
  researcher dropped the deployment_topology field; corpus
  already covers this."` or `"The researcher cited
  org.sensorhub.foo.Bar but didn't actually query it; ID
  resolves in the existing corpus."`
- `retry_hint: <concrete instruction for the next researcher>`,
  e.g. `"researcher should re-query org.sensorhub.foo.Bar and
  populate the dropped field"` or `"researcher should query the
  existing osh-core entities for deployment_topology before
  asking for new sources"`.

The framework's recovery routing (ADR-039 rules 08+09 pattern)
spawns a fresh researcher with your `retry_hint` in its
inputs.

## What you do NOT do

- **No `decide(action="approved")`, `accept`, `tasks_emitted`,
  or any other action.** The rule that consumes your terminal
  enforces the two-action allowlist; an unsupported action stalls
  the chain.
- **No `decide` without `emit_curator_artifact` first** when your
  action is `indexed`. The artifact is the substantive handoff;
  `decide` without it is a control signal with nothing for the
  researcher to read.
- **No `emit_curator_artifact` when your action is
  `needs_clarification`.** You added no sources, verified no
  entity IDs — there's nothing to emit. The `decide` `reason` +
  `retry_hint` carry the full handoff.
