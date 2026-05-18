# Iteration rules

Within-loop iterations are bounded by
`agentic-loop.max_iterations`. Each iteration is one LLM
round-trip: read, query, append to scratchpad. Be efficient.

## Expected iteration shape

A healthy first-pass gather typically runs:

1. One `read_loop_result` on the plan loop (iteration 1).
2. 2–5 `web_search` calls grounding the plan's actors and
   boundaries in external facts (iterations 2–6).
3. Interleaved `scratchpad` writes to accumulate evidence.
4. Terminal `decide(action="synthesize", reason=...)`.

Total: 2–5 `web_search` calls is normal for a substantive first
pass. **A gather loop that terminates after 0 `web_search` calls
is a failure mode** — it means the LLM skipped grounding and
SYNTHESIZE will fabricate from the plan alone. If `web_search`
cannot resolve the plan's actors (vague names, unavailable facts),
terminate `needs_clarification` rather than silently hand off an
empty evidence pool.

Chain agents do NOT have graph-query tools. Don't reach for
`query_entity` etc.; they aren't allowed.

## When the plan you read is a recovery revision

Every gather pass is spawned from a plan loop (the rule pack's
forward edge has exactly one spawn point for gather). Recovery
context lives upstream on the plan: when a downstream role
rejected `insufficient` or `needs_clarification`, the recovery
rule re-spawned PLAN, which revised scope and emitted a new
plan with a higher `revision` number. The gather pass that
follows is structurally a fresh first-pass under the revised
scope.

You detect this by reading the plan's terminal. Signals the
plan is a revision rather than a first-pass:

- `revision > 1` in the rendered plan artifact (`emit_plan`
  stamps this on each pass).
- The plan's `decide.reason` references prior gaps the
  rejecting role named — an `addressed_gaps`-style list, or
  prose tightening scope around specific actors/boundaries.

When the plan is a revision:

1. Focus `web_search` calls on the items the revised plan
   tightened or added. Don't re-derive actors the plan already
   carries from a prior pass unless the new scope contradicts
   them.
2. Append findings to scratchpad as on a first pass.
3. If a gap cannot be filled (external evidence genuinely
   doesn't support it), document that in scratchpad with the
   queries you tried. SYNTHESIZE will surface it as `open_gaps`.

The chain recovery cap (rule `max_iterations`, currently 3)
bounds total reviewer-rejection cycles across the chain.
Exhausting it routes to the failure handler. Within a single
gather pass the loop's own `max_iterations` bounds round-trips.
