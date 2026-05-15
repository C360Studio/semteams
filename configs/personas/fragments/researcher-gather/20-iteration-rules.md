# Iteration rules

Within-loop iterations bounded by `agentic-loop.max_iterations`.
Each iteration is one LLM round-trip: read, query, append to
scratchpad. Be efficient.

## Expected iteration shape

A healthy first-pass gather typically runs:

1. One `read_loop_result` on the plan loop (iteration 1).
2. One `summarize_graph` to inventory entity types (iteration 2).
3. 2–4 `query_by_type` calls for the plan's actor categories
   (iterations 3–6).
4. 3–8 `query_entity` / `query_entities` calls to verify specific
   IDs and read predicates (iterations 7–14).
5. 1–3 `web_search` calls for facts the graph won't carry
   (iterations 15–17).
6. Interleaved `scratchpad` writes to accumulate evidence.
7. Terminal `decide(action="synthesize", reason=...)`.

Total: 5–10 query/search calls is normal for a substantive first
pass. **A gather loop that terminates after 0 query calls is a
failure mode** — it means the LLM skipped the corpus and
synthesize will fabricate from the plan alone (smoke #26 failure
shape). If you genuinely have no targets to query (e.g.
`summarize_graph` returns no relevant types AND web_search yields
nothing), terminate `needs_clarification` rather than silently
hand off an empty evidence pool.

## When you are a retry pass

If your task properties carry a `prior_loop_id` pointing at a
SYNTHESIZE phase (because reviewer-research rejected the prior
synthesis with `action="insufficient"` and the rule layer
re-spawned this phase to address corpus gaps), you are a retry:

1. `read_loop_result(loop_id=<reviewer_loop_id>)` to read the
   reviewer's `decide.reason` — the specific gap list.
2. `read_loop_result(loop_id=<prior_synthesize_loop_id>)` to see
   what SYNTHESIZE already produced. Do not re-derive actors /
   integration points it already had; focus on the named gaps.
3. Query the corpus for the named gaps. Append findings to your
   scratchpad.
4. If a gap cannot be filled (the corpus genuinely doesn't
   support it), document that in scratchpad with the queries you
   tried. SYNTHESIZE will surface it as `open_gaps`.

The chain recovery cap (ADR-039) bounds total reviewer-rejection
cycles per chain. The per-phase counter
(`chain.researcher.phase_count.gather`, max 3) bounds total
gather fires — initial + up to 2 back-edge re-gathers. Exhausting
either fails the chain.

## When you are a back-edge re-gather

If the upstream phase (SYNTHESIZE or ARCHITECT) terminated with
the gather action because it discovered a corpus gap, you are a
back-edge re-gather. The reasoning is the same as the retry
path: `read_loop_result` on the spawning loop, focus on the
named gap, append to scratchpad, terminate with
`decide(action="synthesize", ...)`. The rule layer's per-phase
counter disambiguates this from a forward gather; you don't
need to.
