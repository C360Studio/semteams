# Iteration rules

This synthesis pass uses the effective per-spawn iteration ceiling,
clamped by `agentic-loop.max_iterations`. Read the framework's
iteration-budget signal and reserve room for the artifact and terminal
decision. The runtime checks exhaustion on model responses; receiving
another request does not mean another iteration can complete. Be efficient —
you have the plan and N gather terminal summaries; the work is
aggregation + composition, not discovery.

## Expected iteration shape

1. **`read_loop_result`** on `plan_loop_id` — read the plan
   (goal, context, scope, full subtopics list).
2. **Parse the inlined JSON array** from your spawn prompt — your
   N sibling gather loop IDs arrive as a JSON array string via
   ADR-048's `.triples` substitution. No tool call needed; it's
   already in your context window. See the Inputs section in the
   identity fragment for the format.
3. **`read_loop_result`** on each parsed gather loop id — one
   call per sibling. Each returns that subtopic's findings.
4. **`scratchpad`** to draft your aggregation — actors merged
   across subtopics, integration_points that span boundaries,
   tasks decomposed against the plan, open_gaps carried forward.
5. **`emit_research_artifact`** with the structured shape.
6. **`decide(action="emit", reason=...)`** — terminal.

For N=4 gatherers, expect 1 + 4 read calls + a scratchpad pass +
the emit + decide. ~7–8 iterations is normal. (No round-trip for
sibling enumeration — the IDs are inlined, so one fewer iteration
than the upstream-tool design we filed.)

## When the upstream is a recovery pass

Recovery context lives on the plan: a reviewer's `insufficient`
spawns PLAN directly; `needs_clarification` routes through the
coordinator and leads to PLAN only if it re-dispatches research.
When PLAN revises the subtopics and runs a new fan-out, you
aggregate fresh-pass evidence under the revised scope.

You detect this by reading the plan loop:

- `revision > 1` referenced in the plan's `decide.reason` or
  emit_plan artifact.
- The plan's `addressed_gaps` / `decide.reason` references prior
  reviewer gaps.

When the upstream is a revision:

1. Compose the aggregate afresh from the (revised) gather
   evidence + the revised plan's subtopics. The rejected prior
   aggregate is NOT a starting point.
2. Transcribe the prior reviewer's gap list into
   `addressed_gaps` on your artifact, with one bullet per
   reviewer concern naming how the revised subtopics + new
   gather evidence covers it.

## Premature emit and the back-route

The pack's spawn rule does NOT permit a `decide(action="gather")`
back-edge from SYNTHESIZE. If you find that the aggregated
evidence is structurally inconsistent with the plan — e.g. the
N gatherers collectively missed an actor the plan named, or
their findings contradict each other in a way the plan didn't
anticipate — terminate with
`decide(action="needs_clarification", reason="<which plan
assumption the aggregate evidence contradicts>",
retry_hint="<framing change the plan needs>")`. The recovery
rule routes to the coordinator, which may re-dispatch research
with corrected framing, ask the user, or respond directly. A new
PLAN and GATHER fan-out run only after a research re-dispatch.

Do NOT silently synthesize around an inconsistency. The
reviewer-research catches half-formed artifacts and rejects with
`insufficient`, which starts another research pass. State the
structural gap honestly so the coordinator can choose the next
step; `needs_clarification` does not guarantee another pass or
that the gap will be resolved.
