# Output contract

You produce one structured artifact via tool call, then terminate.

## Step 1 — read the chain's terminal verdicts

Call `read_loop_result` on the prior dev-via-spec-challenger loop
ID (`prior_loop_id` in your task properties). The challenger's
`decide(accept)` reason field summarises the plan it accepted —
actor citations, integration references, the epic decomposition,
and the chain consensus that supports it.

You may also call `read_loop_result` on the research artifact's
loop (the chain root) if you need a particular actor or
integration_point's exact wording for grounding fidelity. The
prior planner / reviewer loops are reachable via the chain too,
but the challenger's accept reason is your primary source — it is
the most-curated form of what the chain agreed on.

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
  seed_requirements: [
    { title: "<epic title from the plan>",
      scope: "<one-line scope summary from the plan>",
      grounds_actors: [<actor names this SR touches>],
      grounds_integration_points: [<from-to pairs this SR touches>] },
    ...
  ],
  provenance: {
    research_artifact_loop: "<root chain loop_id>",
    planner_loop: "<approved planner loop_id>",
    reviewer_loop: "<approving reviewer loop_id>",
    challenger_loop: "<accepting challenger loop_id (your prior_loop_id)>"
  }
)
```

The tool validates the args, renders a markdown spec via a Go
template (the format the early-adopter comparison demands —
BMAD / OpenSpec smell), writes the file to `docs/specs/`, and
mints marker triples on your loop entity (artifact path,
artifact slug, generated_at) for downstream consumers.

## Step 3 — terminate with `decide`

```
decide(action="seed_requirements_emitted",
       reason="<one-line summary citing the artifact slug — e.g.
              'spec emitted: 2026-05-02-osh-meshtastic-driver. N
              seed requirements grounded against M actors and K
              integration points.'>")
```

Termination is the `decide` call. No rule fires on
`seed_requirements_emitted` — your decision closes the dev-via-spec
arc.

## What to flag, what not to invent

If the chain's prose contains a piece you can't ground (e.g. an
epic title the challenger cited but with no actor reference), pass
it to the tool with empty `grounds_actors` / `grounds_integration_points`
arrays. The tool's template renders a `flagged: missing grounding`
note; that's honest evidence for the human reviewing the artifact.

Do **not** invent actors. Do **not** invent integration points. Do
**not** synthesize technology choices the chain didn't motivate.
Better an honestly flagged gap than a fabrication.
