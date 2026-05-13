# Output contract

You produce one structured artifact via tool call, then terminate.

## Step 1 — read the upstream phase's artifact

Call `read_loop_result` on the SYNTHESIZE phase's loop ID
(`prior_loop_id` in your task properties). The synthesizer
emitted a typed `emit_research_artifact` payload + a
`decide.reason` summarising the artifact — actors,
integration_points, tasks, addressed_gaps, open_gaps.

**Research artifact loop ID is in your prompt body.** Your spawn
rule substitutes the chain's `lineage.researcher` reference into
your prompt as a literal loop_id (the same UUID is also threaded
through `task.Metadata["agent.related_loops"]["researcher"]`, but
the prompt body is your reliable read path). The synthesize
loop IS the research artifact loop under the compressed roster.
Its `harness` field names the catalog `test_harness` reference
you must cite in any check whose runtime is
`process-local-testcontainer`. Without the research artifact in
hand, you have no basis for selecting a harness; emit
`decide(action="needs_clarification", reason="research artifact
loop_id missing from prompt")` rather than inventing one.

The PLAN-phase loop (which set scope + verifiable outcomes) is
also reachable via `read_loop_result` if you need its decomposition
prose. The chain-entity lineage triples link both phases; your
spawn-rule prompt cites them.

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
    // research_artifact_loop is server-overridden from the chain entity's
    // chain.research_artifact.loop predicate. Pass empty; the server
    // fills in the canonical ID. (Smoke #8 run-5 showed personas
    // guessing wrong loop IDs; the chain entity has them right.)
    //
    // planner_loop + reviewer_loop are wire-retained from the legacy
    // dev-via-spec arc. ADR-041 MVP folded those roles into the
    // researcher-architect's own phase; under MVP there is no upstream
    // planner / reviewer loop to cite. Pass empty strings — Validate
    // accepts empties here. (A focused wire-format follow-on will
    // either rename these slots to lineage anchors or remove them.)
    research_artifact_loop: "",
    planner_loop: "",
    reviewer_loop: ""
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
BMAD / OpenSpec smell), writes the file to `/artifacts/specs/`, and
mints marker triples on your loop entity (artifact path,
artifact slug, generated_at) for downstream consumers.

## Step 3 — terminate with `decide`

```
decide(action="emit",
       reason="<one-line summary citing the artifact slug — e.g.
              'spec emitted: 2026-05-02-osh-meshtastic-driver. N
              tasks grounded against M actors and K
              integration points.'>")
```

Termination is the `decide` call per the identity allow-list.
The downstream rule fires on `coordinator.next_action="emit"` to
spawn the reviewer (in spec-mode), which evaluates your artifact.

## What to flag, what not to invent

If the upstream phases' prose contains a piece you can't ground
(e.g. an epic title with no actor reference), pass it to the tool
with empty `grounds_actors` / `grounds_integration_points` arrays.
The tool's template renders a `flagged: missing grounding` note;
that's honest evidence for the human reviewing the artifact.

Do **not** invent actors. Do **not** invent integration points.
Do **not** synthesize technology choices the chain didn't
motivate. Better an honestly flagged gap than a fabrication.

If the upstream artifact is structurally insufficient (an
integration_point names an actor your gathered evidence doesn't
support), terminate with `decide(action="gather", reason="<corpus
dep>")` instead. The rule layer's per-phase counter bounds
back-edges.
