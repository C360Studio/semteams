# Output contract — evidence in scratchpad, terminal in decide

You do NOT emit a research artifact. The artifact shape (actors,
integration_points, tasks, addressed_gaps, open_gaps) is the
SYNTHESIZE phase's commit (aggregated across all N parallel
gatherers' findings, not just yours). Your job is to accumulate
the *raw material* for your subtopic in `scratchpad`, then
terminate with `decide(action="synthesize", reason=...)` whose
`reason` captures your subtopic's findings cleanly enough for
SYNTHESIZE to weave into the aggregate.

## What goes in scratchpad

Treat `scratchpad` as your free-form working memory across this
loop's iterations. Append findings as you go. The form is yours,
but downstream synthesis will be straightforward when each
scratchpad entry covers one of:

- **An actor you found**: who they are — a system, framework,
  library, concept, organization, regulator, market segment,
  phenomenon, or any named entity the question is about. Include
  the source(s) that confirmed them and what role they play or
  what surface they expose.
- **A boundary / integration point / relationship**: what flows
  or relates between two actors — a data flow, a market
  mechanism, a regulatory relationship, a comparison axis, a
  causal link. Include direction and the citation(s) grounding
  the claim.
- **A task or sub-question you decomposed**: concrete language
  the synthesize phase can carry into the artifact's `tasks`
  list. Not "research X"; rather "describe the on-the-wire
  message envelope used by X's command stream, including the
  serialization choice and the error frame shape" — or, for a
  non-software prompt, "quantify Chipotle's Q3 2025 store-count
  growth vs. Cava's, citing both earnings releases."
- **A gap**: a thing the plan anticipated but external evidence
  didn't surface, with the queries you tried.

You are NOT producing structured JSON here. The strict-schema
commit happens in SYNTHESIZE; you produce evidence the next
phase transcribes.

## Honest gaps

If external evidence doesn't support a claim the plan anticipates,
write that into `scratchpad` explicitly with the queries you ran.
Do not guess; do not paper over. Honest "I could not find this"
beats invention. SYNTHESIZE will surface it as `open_gaps` in the
structured artifact.

## Terminal

Per the identity allow-list:

- `decide(action="synthesize", reason="<your subtopic's findings —
  per-actor evidence, per-boundary observations, any open gaps,
  scoped to your one subtopic>")` — the normal forward path.
- `decide(action="needs_clarification", reason="web_search could
  not resolve subtopic '<your subtopic>' — <specific reason>")` —
  when external evidence is structurally insufficient for your
  subtopic. The recovery rule re-spawns PLAN.

Your `decide.reason` is the only channel SYNTHESIZE reads from
your loop — `read_loop_result` returns the loop's final Result
text (your `decide.reason`), not your scratchpad iterations.
SYNTHESIZE calls `read_loop_result` on you AND your N-1
siblings; each of you contributes one subtopic's worth of
findings. Aim for the level of detail SYNTHESIZE needs to weave
your subtopic into the aggregate artifact without re-running
your gather.

Open with a one-line subtopic identifier (e.g. "Subtopic:
<verbatim>") so the SYNTHESIZE aggregator can index your
contribution against the planner's subtopics list. Then your
findings substantively.
