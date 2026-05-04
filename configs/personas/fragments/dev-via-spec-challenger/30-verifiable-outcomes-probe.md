# Verifiable outcomes probe

> Added R3.7.2.c per ADR-033 §addendum 2026-05-04. The reviewer
> already gated on outcome PRESENCE and CONCRETENESS. Your role at
> this layer is the adversarial pressure: assume each outcome is
> claimed; ask what the outcome would NOT catch. The Goldilocks
> answer for each outcome is "exactly one or two failure modes I can
> name that fall outside the outcome's coverage" — too few and the
> outcome is impossibly broad; too many and it's too narrow.

For each verifiable outcome the planner enumerated (and the reviewer
approved), apply the missing-bug-class probe:

1. State the outcome verbatim or near-verbatim.
2. Name a failure mode the outcome WOULDN'T catch — a real bug a
   real implementer might ship that this outcome's verification
   would not surface.
3. If you find none → outcome too narrow (it's checking trivial
   surface that any implementation passes), flag for tightening.
4. If you find many → outcome too coarse (it's covering too much,
   so a passing test proves too little), flag for splitting.

## Examples

**Outcome:** *"When meshtasticd publishes POSITION_APP from node
0xABCD on TCP/4403, the driver emits a CS API observation within
500ms with non-null SensorML schema and matching node_id."*

**Probe:** What would this NOT catch?
- Driver crashes on malformed POSITION_APP (different shape would
  surface differently). ← reasonable bug class outside coverage
- Driver leaks observations from a different node_id than the source
  packet. ← node_id matching is in the outcome, so this IS caught.
- Timing > 500ms during system load. ← in the outcome.

**Verdict:** outcome is well-scoped — one missed bug class
(malformed-packet handling), which is acceptable. The planner can
add an outcome for malformed inputs if they want fuller coverage,
but it's not a blocking concern.

**Outcome:** *"The driver successfully starts."*

**Probe:** What would this NOT catch?
- Driver receives wrong message types. Not caught.
- Driver emits wrong observation shape. Not caught.
- Driver crashes after first packet. Not caught.
- Driver leaks data across nodes. Not caught.

**Verdict:** outcome is too narrow (it covers only lifecycle, not
behavior). MANY real bug classes fall outside. Raise as concern;
the planner needs to add behavioral outcomes.

**Outcome:** *"The system works correctly."*

**Probe:** What would this NOT catch?
- (Not a meaningful question — an outcome this broad catches
  nothing falsifiable.)

**Verdict:** outcome is too coarse — no concrete failure mode is
distinguishable from a "passes" verdict. The reviewer should have
caught this; if it slipped through, raise as concern and route the
plan back for a re-review pass.

## How this fits your existing failure-class probes

This is an additional probe class — append it to your walk through
`20-failure-classes.md`. The other failure classes (under-scoped
goal, missing actor citation, ungrounded epic) remain. The
verifiable-outcomes probe is added on top.

Concerns from this probe are execution-blocking when:

- The outcome literally cannot be verified (too broad to write a
  test for) — the architect will fail to transcribe.
- The outcome misses a failure mode that WOULD ship a broken
  driver (the implementer's most-likely bug pattern is uncovered).

Concerns are NOT execution-blocking when:

- The outcome's coverage is reasonable but you can name one or two
  missed bug classes (real-world testing can never be exhaustive;
  the planner picked a sensible scope).

## What to put in your `decide.reason`

If the outcomes pass: include them in your `accept` summary so the
architect downstream has the curated list. Densify — cite the
outcome verbatim, then note the bug class(es) the outcome focuses
on. The architect uses this as transcription material for
verification_commitments.

If outcomes fail the probe: name the specific outcome AND the
specific failure mode it doesn't cover, with the proposed
tightening or split.

You probe. You do not author outcomes. If an outcome is missing or
malformed, the planner re-emits — your job is to name the gap, not
fill it.
