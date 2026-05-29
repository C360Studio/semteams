# Upstream issues filed during the cycle

Three issues filed against the [semstreams framework](https://github.com/c360studio/semstreams)
during smoke validation. Pattern: real-LLM stamping cadence surfacing
engine semantics that mock-driven tests don't exercise.

## [semstreams#158](https://github.com/C360Studio/semstreams/issues/158) — text-only LLM completions strand work

When an agentic loop's LLM returns prose content instead of a tool
call, the framework marks the loop `outcome=success` with `result=""`.
The 200+ tokens of prose go into the void as far as downstream consumers
are concerned — `read_loop_result` returns the empty Result field, not
the prose content (which lives in scratchpad triples with no cross-loop
read tool).

Observed in smoke #fan-out-validation-3: 2 of 3 parallel gemini-flash
investigators terminated text-only at 219+ tokens, all stranded.

Proposed: widen the framework's existing `#133` synthetic-decide
mechanism to fire on this path too, preserving the LLM's text in the
synthesized terminal's reason field; OR add a narrow `read_scratchpad`
cross-loop tool.

Workaround applied immediately: the framework's beta.80 `#132` shipped
`tool_choice={mode:"required"}` for exactly this failure class. Applied
to every flash-model spawn rule in our pack.

**Status:** open / engine-completion design discussion.

## [semstreams#159](https://github.com/C360Studio/semstreams/issues/159) — completion-state stamp race

When a loop completes, the framework stamps `agent.loop.outcome=success`
and `agent.loop.parent` in the same millisecond — but they arrive at the
rule engine as separate `EntityState UPDATED` events. A rule matching
on `outcome=success` and substituting `$entity.triple.agent.loop.parent`
in its action sees the entity snapshot mid-update: outcome is present,
parent is not. Substitution fails; framework's safety mechanism refuses
to write a triple with a garbled subject; downstream join wedges.

Observed across smoke runs 1, 2, 3 with concrete timestamps (race
window: 14 microseconds in run 3). Reproducible across three framework
versions.

Proposed: stamp completion-path triples in an atomic batch (one
`EntityState UPDATED` event with all completion triples present); OR
stamp `agent.loop.parent` at spawn time, when the parent ID is already
known and stable.

Workaround applied: rule 02's `related_loops` stamps a separate
spawn-time triple containing the parent's full entity ID. Rule 03a's
action substitutes against that spawn-time triple instead of the
race-prone completion-time one.

**Status:** open / design discussion.

## [semstreams#160](https://github.com/C360Studio/semstreams/issues/160) — template-variable substitution prefix collision

The rule engine's `$entity.triple.<predicate>` substitution iterates
the entity's triples and does `strings.ReplaceAll` per predicate.
Go map iteration order is randomized. If two predicates share a prefix
(`lineage.X` and `lineage.X-something`), depending on which is processed
first, the shorter one can match inside the longer reference and
substitute, leaving the suffix orphaned.

Observed in smoke #fan-out-validation-4: rule 03a referenced
`$entity.triple.lineage.researcher-plan-entity` but the entity also
carried `lineage.researcher-plan` (shorter prefix). All 4 counter
writes landed on a phantom subject ending in `-entity` rather than the
real plan entity.

Proposed: sort triples by predicate length descending before iteration
in the substitution loop. Negligible perf cost; deterministic behavior
regardless of consumer-chosen predicate names; no API surface change.

Workaround applied: renamed the lineage triple to
`lineage.plan-loop-entity-id` (no prefix overlap with
`lineage.researcher-plan`).

**Status:** open / proposed fix is small (~10 LoC) and additive.

## Pattern observed across all three filings

- Real-LLM stamping cadence surfaces engine semantics that mock-driven
  tests don't catch.
- Each issue isolates a single primitive's behavior — not "the chain
  doesn't work end-to-end" but "this substitution / this stamp ordering
  / this iteration order has a sharp edge."
- Each filing includes a proposed fix at the framework layer + a
  workaround SemTeams is applying immediately for unblock + an explicit
  marker that the workaround retires when the upstream fix lands.
- Two of the three include alternatives-considered sections so the
  framework team can weigh the proposed shape vs. alternatives.

This is the closure-pass discipline operationalized: file at the right
layer, ship workaround app-side only when explicitly time-bounded, and
keep the application-side surface intentionally thin.
