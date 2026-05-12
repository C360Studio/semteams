# Output contract

You produce one structured artifact via tool call, then terminate.

## Step 1 — read the chain's terminal verdicts

Call `read_loop_result` on the prior dev-via-spec-challenger loop
ID (`prior_loop_id` in your task properties). The challenger's
`decide(accept)` reason field summarises the plan it accepted —
actor citations, integration references, the epic decomposition,
and the chain consensus that supports it.

**Research artifact loop ID is in your prompt body.** Your spawn
rule substitutes the chain's `lineage.researcher` reference into
your prompt as a literal loop_id (the same UUID is also threaded
through `task.Metadata["agent.related_loops"]["researcher"]`, but
the prompt body is your reliable read path). Call
`read_loop_result` on that loop_id to read the research
artifact — its `harness` field names the catalog `test_harness`
reference you must cite in any check whose runtime is
`process-local-testcontainer`. Without the research artifact in
hand, you have no basis for selecting a harness; emit
`decide(needs_clarification, reason="research artifact loop_id
missing from prompt")` rather than inventing one.

The prior planner / reviewer / challenger loops are also reachable
via `read_loop_result`, but the challenger's accept reason
(your prior_loop_id) is your primary source — it is the
most-curated form of what the chain agreed on.

## Step 2 — call `emit_dev_via_spec_artifact`

Extract the chain's substance into the tool's structured args:

```
emit_dev_via_spec_artifact(
  title: "<short, descriptive title — e.g. 'OSH Driver — Meshtastic'>",
  goal: "<one-sentence target capability from the plan>",
  context: "<paragraph explaining why this work, with at least one
            actor named>",
  actors: [
    { name: "<from research artifact>", role: "<one-line role>" },
    ...
  ],
  integration_points: [
    { from: "<actor>", to: "<actor>",
      direction: "read" | "write",
      data: "<what flows>" },
    ...
  ],
  tasks: [
    { title: "<epic title from the plan>",
      scope: "<one-line scope summary from the plan>",
      grounds_actors: [<actor names this task touches>],
      grounds_integration_points: [<from-to pairs this task touches>] },
    ...
  ],
  checks: [
    { target: "<verifiable claim transcribed from the planner's
               accepted Verifiable Outcomes>",
      runtime: "in-process-unit" | "process-local-testcontainer"
             | "external-sidecar" | "browser-flow"
             | "static-analysis",
      test_harness: "<catalog name>",              // required for
                                                   // testcontainer /
                                                   // sidecar / browser-flow
      test_runtime: "<runtime id>",                // required when test_harness named
      ref: { type: "filepath" | "template_id",
             path: "<workspace-relative path>",    // when type=filepath
             id:   "<framework template id>" },    // when type=template_id
      evidence: [ ... ] },                         // optional in v1
    ...
  ],
  provenance: {
    // OPTIONAL — the server overrides every field below from the chain
    // entity (chain.research_artifact_loop, chain.plan_loop,
    // chain.plan_reviewer_loop, chain.consensus_loop) before the
    // markdown renders. Pass an empty object if you don't have the
    // values handy; the server still fills in the canonical IDs.
    // Smoke #8 run-5 showed personas guessing wrong (citing the
    // challenger as the planner_loop, etc.); the chain entity has
    // them right.
    research_artifact_loop: "<may be empty>",
    planner_loop: "<may be empty>",
    reviewer_loop: "<may be empty>",
    challenger_loop: "<may be empty>"
  }
)
```

`checks` is OPTIONAL at the wire but
the commitment contract defines when it MUST be populated:
any artifact whose `integration_points[]` names an external
actor requires at least one check. Populate it with the
shape above; see the commitment-transcription section for the
outcome-to-target translation and the commitment contract
for the runtime selection.

The tool validates the args, renders a markdown spec via a Go
template (the format the early-adopter comparison demands —
BMAD / OpenSpec smell), writes the file to `docs/specs/`, and
mints marker triples on your loop entity (artifact path,
artifact slug, generated_at) for downstream consumers.

## Step 3 — terminate with `decide`

```
decide(action="tasks_emitted",
       reason="<one-line summary citing the artifact slug — e.g.
              'spec emitted: 2026-05-02-osh-meshtastic-driver. N
              tasks grounded against M actors and K
              integration points.'>")
```

Termination is the `decide` call. No rule fires on
`tasks_emitted` — your decision closes the dev-via-spec arc.

## What to flag, what not to invent

If the chain's prose contains a piece you can't ground (e.g. an
epic title the challenger cited but with no actor reference), pass
it to the tool with empty `grounds_actors` / `grounds_integration_points`
arrays. The tool's template renders a `flagged: missing grounding`
note; that's honest evidence for the human reviewing the artifact.

Do **not** invent actors. Do **not** invent integration points. Do
**not** synthesize technology choices the chain didn't motivate.
Better an honestly flagged gap than a fabrication.
