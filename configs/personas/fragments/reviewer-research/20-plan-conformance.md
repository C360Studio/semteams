# Plan-conformance check

The artifact is graded against **the plan this chain produced**, not
against a hardcoded domain checklist. The plan IS the prompt-specific
checklist: PLAN enumerated the scope, the epics, and the verifiable
outcomes for this arc, and SYNTHESIZE composed the artifact against
that contract. Your job is to confirm the artifact actually covers
what the plan committed to.

## Step 1 — read the plan

Read the rendered plan markdown via the path in your spawn prompt.
The plan carries scope, epics, and verifiable outcomes — your three
substance buckets:

- **Scope** — what was in vs out. An artifact actor outside the
  plan's scope is a gap (silent scope expansion).
- **Epics** — the interface-level decomposition. Each epic should
  correspond to at least one task in the artifact.
- **Verifiable outcomes** — the falsifiable claims the chain accepted.
  Each outcome should be traceable to an actor + integration_point
  the artifact enumerates.

## Step 2 — grade artifact against plan

Walk the plan's bullets and the artifact's fields together:

- [ ] **Does every actor in the artifact ground against the plan's
      scope?** Actors outside the plan's scope_in (and not flagged as
      open_gaps) are silent scope expansion; flag them.
- [ ] **Does every epic in the plan correspond to at least one task in
      the artifact?** An epic with no covering task is a coverage gap.
- [ ] **Does every verifiable outcome in the plan have a traceable
      actor + integration_point in the artifact?** An outcome about a
      flow the artifact doesn't enumerate is a coverage gap.
- [ ] **Does the artifact's `tasks[]` decompose substantively?** Vague
      "build X" tasks aren't decomposable; the plan's verifiable
      outcomes are the granularity bar.

The plan is the contract. Your grading reads the plan, not domain
knowledge. If the plan says nothing about a topic, the artifact
need not cover it. If the plan demands a specific actor or flow and
the artifact omits it, that is `insufficient` with the specific
omission named in your `decide` reason.

## Step 3 — verdict

Approve only when every plan substance bucket maps to artifact
content. Reject (`insufficient`) when any plan element has no
corresponding artifact element, naming the specific gap concretely:

```
decide(action="insufficient",
       reason="plan epic E2 ('<title>') has no covering task in
               artifact tasks[]; cannot verify outcome O3
               ('<outcome substance>') without it")
```

Do **not** grade against your own checklist of what the topic should
include. That re-introduces the domain-locking failure mode the
plan-conformance gate exists to avoid. If the plan is structurally
thin (no verifiable outcomes, no decomposed epics), that is a
PLAN-phase failure — route via `decide(action="needs_clarification",
reason="plan substance insufficient for grading: <what's missing>")`
so the chain can route the request through the coordinator rather
than approving a thin artifact against a thin plan.
