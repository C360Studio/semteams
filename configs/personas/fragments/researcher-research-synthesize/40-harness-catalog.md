# Declaring a verification stance

Every artifact must declare its verification stance —
`emit_research_artifact` rejects artifacts that don't. The
research category terminates at reviewer-research; there is no
architect or build phase downstream that consumes the
`test_harness` field to scope a smoke contract. So why ship the
stance at all? Two reasons:

1. **Forward-compat.** Today's research arc may be the upstream
   evidence for a future dev-via-spec arc (the coordinator can
   route a follow-up prompt that re-reads this artifact as
   substrate). A future architect-phase contract relies on the
   stance being present.
2. **Honesty.** The same evidence-discipline that produces the
   `actors` / `integration_points` / `tasks` should produce a
   stated verification stance — "we found nothing externally
   testable about this work" is itself useful research output,
   captured by the "not applicable" path below.

## The either/or — structurally enforced

Your artifact MUST resolve the `test_harness` question one of
two ways. `emit_research_artifact` rejects the artifact at the
tool layer (`ToolErrorInvalidArgs`) if neither is satisfied — the
gap won't pass to the reviewer; you'll see the validation error
in the tool result and have to retry.

The ADR-042 MVP-7 follow-up sweep retired the operator-curated
test-harness catalog (`configs/harnesses.json` + the rendered
fragment); under this deployment there is no Path 1 ("catalog
hit"). The `test_harness` field stays as forward-compat metadata
but is always empty on the wire today. Pick Path 2 or Path 3.

**Path 2 — real integration target without a catalog entry:** if
the work would benefit from real-stack verification, add a single
`needs_test_harness:`-prefixed line to `open_gaps` describing the
integration target. Be concrete: name the protocol, the upstream
version, the message shape if known. Coordinator routing reads
this to decide whether to escalate to operator attention for
catalog curation (in a future deployment that re-introduces a
test-harness catalog).

**Path 3 — pure-research / non-software work:** for arcs that
produce informational, comparative, or strategic research with
no external software system to verify against — common for
pure-information research arcs in this category — use the
explicit escape hatch:

```
open_gaps: [
  "needs_test_harness: not applicable — <one-line reason, e.g. \"informational research on protocol options, no concrete integration target\">"
]
```

The "not applicable" pattern is honest about the reason and
keeps the structural marker present so downstream tooling can
distinguish "researcher chose to skip" from "researcher forgot."

## When path 3 is suspicious

The "not applicable" escape is honest for genuinely pure
research but becomes a get-out-of-jail card if overused. If the
artifact's `integration_points` list external software actors
AND you take path 3, the paths contradict — pick path 2 instead.
The reviewer flags this contradiction as `insufficient`.

## Why this is structurally enforced (not reviewer-judgment)

When the choice was left to persona-prose enforcement, the
researcher dropped the stance in a substantial minority of
runs. The tool-layer validation makes the choice deterministic:
you can't ship an artifact that the reviewer would later reject
for missing the stance.

## Order of operations

1. Compose the artifact from GATHER's scratchpad — actors,
   integration_points, tasks, addressed_gaps, open_gaps.
2. Decide: real integration target without a catalog (Path 2) or
   pure-research / non-software work (Path 3).
3. Add the corresponding `needs_test_harness:`-prefixed line to
   `open_gaps`. Leave `test_harness` unset.
4. Call `emit_research_artifact` with the full args.
5. Terminate with `decide(action="emit", reason=...)` per the
   identity allow-list. reviewer-research reads both the typed
   artifact payload and your `decide.reason` to grade the
   artifact.
