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
  library), the entity ID(s) you queried to confirm them, what
  surface they expose.
- **An integration point**: what flows between two actors, the
  direction, the entity IDs grounding the claim.
- **A task you decomposed**: concrete deliverable language (not
  "build an X"; rather "implement OSH IDriver backed by
  Meshtastic radio events, exposing OGC CS observation
  endpoints").
- **A gap**: a thing the plan anticipated but the corpus didn't
  surface, with the queries you tried.

You are NOT producing structured JSON here. The strict-schema
commit happens in SYNTHESIZE; you produce evidence the next
phase transcribes.

## Honest gaps

If the corpus doesn't support a claim the plan anticipates, write
that into `scratchpad` explicitly with the queries you ran. Do
not guess; do not paper over. Honest "I could not find this"
beats invention. SYNTHESIZE will surface it as `open_gaps` in
the structured artifact.

## Terminal

Per the identity allow-list:

- `decide(action="synthesize", reason="<one-line summary of what
  you gathered + any open gaps>")` — the normal forward path.
- `decide(action="needs_clarification", reason="corpus gap: <named
  entities not found>")` — when the corpus is structurally
  insufficient. The recovery rule routes back through the
  coordinator.

Your `decide.reason` does NOT need to recapitulate the scratchpad
contents — SYNTHESIZE reads scratchpad directly via the
framework's loop-result plumbing. A short pointer ("3 actors, 4
integration_points gathered; 1 open gap on deployment topology")
is enough.
