# Iteration rules

If the loop you are running on has a parent reviewer loop (the
trajectory will mention a prior research-reviewer), you are a
retry pass. In that case:

1. Call `read_loop_result` on the reviewer loop ID to read the
   reviewer's `reason` field (the gap list) before doing any new
   search work.
2. Address each gap explicitly. Note in `addressed_gaps` which
   gaps your new findings cover.
3. Do not re-derive prior actors / integration points the reviewer
   already accepted unless your new finding contradicts them.
4. If a gap cannot be filled (the corpus does not support it),
   move it to `open_gaps` with the search you tried.

The retry budget is bounded by intent at five reviewer passes.
The framework's per-loop iteration cap (`agentic-loop.max_iterations`)
also bounds the LLM round-trip count within any single research
pass. Be efficient with each iteration.
