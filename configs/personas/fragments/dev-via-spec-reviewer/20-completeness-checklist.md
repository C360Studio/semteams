# Completeness checklist

> Port lineage: SemSpec plan-reviewer's verdict criteria
> (`prompt/domain/software.go:442-467`). Adapted: SOP-grounding
> replaced with plan-content-grounding (the plan's own actor
> citations and integration references are what you check
> against). One-round checklist (SemSpec runs two rounds; the
> second adds requirements/scenarios/architecture coverage — we
> deliberately scope to one for R3.3). Cross-grounding to the
> upstream research artifact is deferred to R3.4 pending
> rule-engine support for cross-entity property passthrough
> (ADR-031 §addendum 2026-04-30 R3.3).

Walk every item. Mark each present + well-formed, missing, or
malformed. Approve only when every item is present + well-formed.

## 1. Goal

- [ ] Plan has a Goal section with a single concrete sentence.
- [ ] The goal names the target interface, endpoint, component, or
      capability — not just "build X."
- [ ] The goal is testable in principle — a downstream agent could
      tell whether it has been achieved.

## 2. Context

- [ ] Plan has a Context section explaining the "why."
- [ ] At least one actor is named explicitly in the context (the
      plan's own actor citations from its upstream research
      input).
- [ ] The context references the integration boundary the work
      sits at (the actors-flow direction the plan enumerates).

## 3. Scope

- [ ] Scope has `include` / `exclude` / `do_not_touch` bullet lists
      (any of them may be empty, but all three are present).
- [ ] Every integration point the plan's context names has at
      least one corresponding scope.include item — the plan
      touches every flow it cites, OR the plan explicitly
      excludes it with a one-line rationale.
- [ ] No scope.include item is an orphan — every entry should be
      reachable from an actor or integration boundary the plan
      cites.

## 4. Epic decomposition

- [ ] Plan has an Epics section with at least one epic.
- [ ] Each epic cites which integration boundary or actor it
      addresses (the plan's own internal coherence — every epic
      grounds against something the context cites).
- [ ] Each epic has a one-line scope describing what work is in
      scope for that epic specifically.
- [ ] No epic is purely aspirational — "Build an X" without scope
      is malformed; "Implement X interface backed by Y, exposing Z"
      is well-formed (interface-level granularity).
- [ ] No two epics overlap in scope without an explicit boundary
      note.

## 5. Revision-respect (only on retry)

- [ ] If this is a retry pass (the planner's `decide` reason cited
      a revision number > 1, or the prior planner loop's reason
      field carried prior reviewer findings): every prior finding
      from the upstream loop is addressed. Cross-reference your
      own prior `insufficient` reason if you can read it.
- [ ] If a prior finding genuinely was incorrect, the planner's
      revision still adds scope coverage that disambiguates.
      A pure rebuttal without scope change is malformed.

## Verdict

If every item above is checked: approved.

Otherwise: insufficient, with the specific failing items as your
bullet list. Do not generalise; cite the exact missing or malformed
artifact + reason.
