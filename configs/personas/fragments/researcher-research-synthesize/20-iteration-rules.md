# Iteration rules

The framework's per-loop iteration cap
(`agentic-loop.max_iterations`) bounds the LLM round-trip count
within this synthesis pass. Be efficient — you have GATHER's
scratchpad evidence and PLAN's structure; the work is
composition, not discovery.

## When the upstream gather is a recovery pass

Every synthesize pass is spawned from a gather loop (the rule
pack's forward edge has exactly one spawn point for synthesize).
Recovery context lives further upstream on the plan: when a
downstream role rejected the prior synthesis or a phase
terminated `needs_clarification`, the recovery rule re-spawned
PLAN, which revised scope; the next gather and this synthesize
are fresh first-passes under that revised scope.

You detect this by reading gather's terminal. Signals gather
ran under a revised plan:

- `revision > 1` referenced in the gather summary or its
  prior-pass artifact.
- The gather summary references prior gaps the rejecting role
  named.

When the upstream is a revision:

1. Compose the artifact afresh from the (revised) gather
   evidence + the plan's structure that gather references.
   Carry forward only what the new evidence still supports —
   the rejected synthesis is NOT a starting point.
2. Transcribe the prior reviewer's gap list into
   `addressed_gaps` on your artifact, with one bullet per
   reviewer concern naming how the revised plan + new gather
   evidence covers it.

## Premature emit and the back-route

The pack's spawn rule does NOT permit a `decide(action="gather")`
back-edge from SYNTHESIZE. If you find that GATHER's evidence is
structurally inconsistent with the plan — e.g. an actor the plan
named doesn't exist in the form the plan assumed, or the
boundaries the plan anticipated don't connect — terminate with
`decide(action="needs_clarification", reason="<which plan
assumption GATHER's evidence contradicts>", retry_hint="<framing
change the plan needs>")`. The recovery rule re-spawns PLAN,
which can revise scope and re-run GATHER under the new framing.

Do NOT silently synthesize around an inconsistency. The
reviewer-research catches half-formed artifacts and rejects with
`insufficient`, which spends recovery budget. An honest
`needs_clarification` resolves the same situation more cheaply.
