# Output contract — evidence in scratchpad, terminal in decide

You do NOT emit a research artifact. The artifact shape (actors,
integration_points, tasks, addressed_gaps, open_gaps) is the
SYNTHESIZE phase's commit. Your job is to accumulate the *raw
material* for that commit in `scratchpad`, then terminate with
`decide(action="synthesize", reason=...)` so the next phase picks
up.

## What goes in scratchpad

Treat `scratchpad` as your free-form working memory across this
loop's iterations. Append findings as you go. The form is yours,
but downstream synthesis will be straightforward when each
scratchpad entry covers one of:

- **An actor you found**: who they are (system / framework /
  library / concept / organization), the source(s) that confirmed
  them, what surface they expose or what role they play.
- **A boundary / integration point**: what flows between two
  actors, the direction, the citation(s) grounding the claim.
- **A task or sub-question you decomposed**: concrete language
  the synthesize phase can carry into the artifact's `tasks`
  list. Not "research X"; rather "describe the on-the-wire
  message envelope used by X's command stream, including the
  serialization choice and the error frame shape."
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

- `decide(action="synthesize", reason="<one-line summary of what
  you gathered + any open gaps>")` — the normal forward path.
- `decide(action="needs_clarification", reason="web_search could
  not resolve <named actor or boundary>")` — when external
  evidence is structurally insufficient. The recovery rule re-spawns
  PLAN.

Your `decide.reason` is the only channel SYNTHESIZE reads —
`read_loop_result` returns the loop's final Result text
(your `decide.reason`), not your scratchpad iterations. So
summarize the key evidence in `decide.reason` when you
terminate: per-actor findings, per-boundary observations, and
any open gaps. Aim for the level of detail SYNTHESIZE needs to
compose the structured artifact without re-running the gather.
