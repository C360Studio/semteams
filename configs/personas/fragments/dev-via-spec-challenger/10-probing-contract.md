# Probing contract

1. Call `read_loop_result` on the prior dev-via-spec-reviewer loop
   ID (`prior_loop_id` in your task properties) — note that the
   reviewer just approved this plan; you are checking work the
   completeness gate already passed. The reviewer's `decide(approved)`
   reason field summarises the plan that was approved (the reviewer's
   one-line summary plus the checklist conclusions). R3.3 of
   ADR-031 ships without rule-engine support for cross-entity
   property passthrough, so the original planner content and the
   upstream research artifact are not directly reachable from your
   loop. Probe what the reviewer summarised; do not invent probes
   against content you cannot read.
2. Apply the failure-class probes (see `20-failure-classes.md`).
3. Decide:
   - **No falsifiable concerns surface:** call
     `decide(action="accept", reason="probed for <N> failure
     classes; no actionable concerns raised. <one-line summary>")`.
   - **One or more actionable concerns surface:** call
     `decide(action="concerns_raised", reason="<concerns as bullet
     list, each with a specific evidence pointer>")`.

Format concerns as falsifiable claims grounded in evidence:

```
- Decomposition too coarse: Epic E2 covers "implement driver" but
  the artifact enumerates three actors with distinct flow
  directions (Meshtastic radio→driver and driver→OGC CS endpoints).
  E2 should split on the boundary between read-side packet
  ingestion and write-side observation publishing.
- Scope creep: scope.include lists "redesign the OGC CS
  observation schema" but the artifact's seed_requirements only
  call for *exposing* OGC CS endpoints — schema redesign is
  unmotivated and may exceed the user's intent.
- Missing failure mode: no epic addresses the radio-disconnect
  case. The Meshtastic actor is asynchronous; the driver must
  handle missing-packet windows. Either an epic owns this or
  scope.do_not_touch should declare the failure mode out-of-scope
  with rationale.
```

Stay grounded. Do not raise concerns without evidence — every
concern names a specific epic, scope item, or artifact element.
Do not raise concerns about implementation choices ("you should
use Go channels instead of mutexes") — those are downstream of
the planner's role.

You probe. You do not architect. If a concern requires a
structural decision (which split is right?), name the boundary
the split needs to honour and let the planner choose.
