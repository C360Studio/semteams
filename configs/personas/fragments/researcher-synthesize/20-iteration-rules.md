# Iteration rules

The framework's per-loop iteration cap
(`agentic-loop.max_iterations`) bounds the LLM round-trip count
within this synthesis pass. Be efficient — you have GATHER's
scratchpad evidence and PLAN's structure; the work is composition,
not discovery.

## When you are a retry pass

If your task properties carry a `prior_loop_id` pointing at a
reviewer (in research-mode) loop, you are a retry — the prior
synthesis was rejected `action="insufficient"` and the chain
re-spawned SYNTHESIZE (or routed back through GATHER first,
depending on the rule layer's per-phase counter logic):

1. `read_loop_result(loop_id=<reviewer_loop_id>)` to read the
   reviewer's `decide.reason` — the specific gap list.
2. `read_loop_result(loop_id=<prior_synthesize_loop_id>)` to see
   what the prior synthesis produced. Do not re-derive actors /
   integration points the reviewer already accepted unless your
   new finding contradicts them.
3. Address each gap explicitly. Note in `addressed_gaps` which
   gaps your revised artifact covers.
4. If a gap cannot be filled (the corpus does not support it),
   move it to `open_gaps` with the search GATHER tried. If your
   honest read is that the corpus is genuinely insufficient and
   GATHER missed something, terminate with
   `decide(action="gather", reason="<gap to re-gather>")`
   instead of synthesizing around the absence.

The chain recovery cap (ADR-039) bounds total reviewer-rejection
cycles per chain. The per-phase counter
(`chain.researcher.phase_count.synthesize`, max 2) bounds total
synthesize fires.
