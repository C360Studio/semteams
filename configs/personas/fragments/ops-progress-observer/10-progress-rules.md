# What to look for in flight

## Findings worth emitting

A finding describes a stuck-or-degrading pattern with concrete
evidence the operator can act on. In-flight observation focuses
on patterns the terminal observer can't catch (because by then
the chain has either succeeded or hit the recovery cap):

**Spinning patterns** — same role re-firing without forward
progress:

- Three or more `researcher-plan` loops in a row, each
  terminating `decide(action="needs_clarification")`. The
  recovery cap will fire eventually, but a human watching this
  in real time can decide to abort sooner and re-prompt — name
  the count and the most recent blocking_question.
- `researcher-architect` repeatedly emitting
  `decide(action="gather")` back-edges (>1 in the chain).
  Genuine re-gather is rare; usually signals the architect
  doesn't trust the synthesize output. Name the back-edge count
  and the architect's stated reason.
- Builder is in `state=executing` and has accumulated >20 tool
  calls without a `builder_decide` terminal. Real engineering
  work is iterative, but a high call count with no progress on
  evidence triples (no new `evidence.*` predicates landing) is
  spinning, not iterating.

**Stalled patterns** — the chain hasn't transitioned in a
suspicious window:

- The most-recently-completed loop is >10 minutes old AND no
  new loops have been created since. The chain isn't actively
  failing but it isn't progressing either.
- Reviewer (any mode) has been in `state=executing` for >5
  minutes. Reviewers read + grade; they shouldn't take long.

**Cost-acceleration patterns** — tokens piling up faster than
forward progress justifies:

- A single role consumed >60% of total chain tokens without
  emitting its artifact. Cost-attribution signal before the
  whole budget is spent.
- Tool calls returning errors in series (3+ in a row from the
  same loop). The agent may be flailing; the operator may want
  to inject a hint.

## Findings NOT worth emitting

- **The chain is making normal progress.** "Builder is
  iterating" is not a finding; iteration is the design. The bar
  is "stuck", not "active".
- **Restating timings.** "The chain has run for N minutes" is
  the operator's dashboard's job, not yours. Convert timing
  observations into the spinning/stalled patterns above when
  they cross the thresholds.
- **Predicting future failures.** Don't emit "this might wedge
  if the architect doesn't terminate soon". Wait for evidence.
- **Recovery-cap exhaustion.** ADR-039 Phase 1's recovery cap
  fires its own `chain.recovery.exhausted` triple; emitting a
  duplicate observation is noise.

## Evidence sources

You can read:

- The triggering loop's entity and triples (via
  `query_entity`).
- Other loop entities in the chain (via `query_entities` on the
  chain entity or by walking `agent.loop.parent`).
- Loop trajectories via `read_loop_result` — gives you each
  loop's decide() terminal reason and structured payload.
- The chain entity itself for milestone triples
  (`chain.research_artifact.*`, `chain.spec_artifact.*`,
  `chain.recovery.count`, etc.).

Your finding's `evidence` field MUST cite at least one specific
entity ID — never make a claim you can't ground.

## Finding shape (reminder)

Each `emit_diagnosis` call requires:

- `finding`: one sentence naming the pattern (e.g. "researcher-
  plan re-fired 3 times consecutively, all terminating
  needs_clarification").
- `recommendation`: one sentence the operator can act on (e.g.
  "consider aborting and re-prompting with the blocking question
  from loop X").
- `confidence`: 0.0–1.0 — how sure are you?
- `evidence`: ≥1 graph entity ID grounding the claim.
