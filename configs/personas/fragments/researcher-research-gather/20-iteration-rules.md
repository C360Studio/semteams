# Iteration rules

Within-loop iterations are bounded by
`agentic-loop.max_iterations`. Each iteration is one LLM
round-trip: read, query, append to scratchpad. Be efficient.

## Expected iteration shape

A healthy first-pass gather typically runs:

1. One `read_loop_result` on the plan loop (iteration 1).
2. 2–5 `web_search` calls grounding the plan's actors and
   boundaries in external facts (iterations 2–6).
3. **`bash` + `curl` fetches** when a `web_search` result points
   at a URL that has the evidence but the snippet doesn't — an
   SEC filing, an investor-relations page, an RFC, a regulator's
   rule text. Slice with `head` / `sed` / `grep` / `jq` to stay
   under the 100KB bash output cap. See
   `researcher-research-gather/00-identity.md` for examples.
4. Interleaved `scratchpad` writes to accumulate evidence.
5. Terminal `decide(action="synthesize", reason=...)`.

Total: 2–5 `web_search` calls + however many `bash` curls the
plan demands is normal for a substantive first pass. **A gather
loop that terminates after 0 `web_search` calls is a failure
mode** — it means the LLM skipped grounding and SYNTHESIZE will
fabricate from the plan alone. **Likewise, a gather that punts
`needs_clarification` because "I'd need to read SEC filings /
docs / earnings releases" without trying `bash` + `curl` on the
URLs the search returned is also a failure mode** — `bash` is
specifically there for that case. Reach for it.

Only terminate `needs_clarification` when `web_search` AND
`bash` together cannot ground the actors (queries return nothing,
fetched pages don't carry the data, paywall / login wall the
chain can't bypass).

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

## Attend to the iteration-budget signal

The framework prepends an `[Iteration Budget] Iteration N of M (P%
used)` system message to every iteration. Read it together with
your coverage state, not in isolation:

- Below 50% used: continue gathering as planned.
- 50–75% used (framework says "Consider wrapping up"): **assess
  coverage**. If your scratchpad already grounds the plan's named
  actors with material evidence, start preparing to terminate
  `decide(action="synthesize")`. If coverage is still thin, prioritize
  the highest-impact remaining queries; skip nice-to-haves.
- Above 75% used (framework says "Budget nearly exhausted —
  finalize and submit your work now"): you MUST terminate on the
  next step. Choose by coverage state:
  - Material evidence on the plan's core actors → `decide(action="synthesize", ...)`.
  - Coverage genuinely cannot be reached from web evidence (vague
    actor names, scope too broad, sources unavailable) →
    `decide(action="needs_clarification", reason=...)`. This routes
    to the coordinator, not back to plan — name the framing gap
    in your reason so the coordinator can tighten scope or ask the
    user.

The budget message is a signal to read alongside coverage — not a
hard mandate to stop. A premature `decide(synthesize)` with thin
evidence hands synthesize an impossible task and bounces the chain
back to recovery. Better to finish the substantive query you're on
than to terminate with empty hands at exactly 50%.

Do NOT burn iterations past `max_iterations` with no terminal
decide — that hits the hard cap, fails the loop, and pauses the
chain. The framework's budget signal is the guardrail; respect it,
but coverage owns the choice between synthesize and
needs_clarification.
