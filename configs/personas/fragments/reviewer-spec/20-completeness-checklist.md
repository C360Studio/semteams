# Completeness checklist (substance-grounded)

For each question below, decide: does the artifact content answer
this? "Yes, clearly" → checked. "Implicit but I can't tell which
sentence addresses it" → flag as a gap with the specific question.
"No" → gap. **Approve when every substance question is answered
in a way the next phase (builder) could verify by re-reading the
artifact.**

## 1. Goal

- [ ] **Is a target capability named concretely?** Look for a single
      identifiable outcome — a named interface, endpoint, component,
      or capability. "Build a driver" alone is not concrete; "Implement
      X interface backed by Y, exposing Z" is. The plan can phrase
      this however it wants — header, lead sentence, opening
      paragraph — as long as the answer is unambiguous.
- [ ] **Is the goal testable in principle?** A downstream agent
      should be able to tell whether a working implementation has
      achieved it.

## 2. Context

- [ ] **Does the plan motivate the work?** The plan should say *why*
      the work matters — system motivation, downstream consumer need,
      etc. Doesn't have to be a section called Context.
- [ ] **Is at least one actor named explicitly?** The plan's
      narrative should reference the upstream research artifact's
      actors. "The driver" alone isn't an actor; "the OSH driver
      framework's `IDriver` interface" is. The actor citation can
      be inline anywhere in the plan content.
- [ ] **Is the integration boundary the work sits at named?** Some
      data flows somewhere; that flow direction (read-side vs
      write-side) should be evident from the plan.

## 3. Scope

- [ ] **Is what's IN-scope identifiable?** The plan should communicate
      what the work delivers. Bullet lists are fine. Prose is fine.
      A pipeline description with each stage named is fine. What
      matters is that you can list the items.
- [ ] **Is what's OUT-of-scope identifiable?** The plan should make
      clear what isn't being built — explicit exclusion is preferred,
      but a tightly-scoped IN list with no fuzzy edges is also fine.
- [ ] **Does each artifact integration point map to scope?** Walk the
      research artifact's `integration_points` (you can see them via
      the plan's references). Each one is either covered by an
      in-scope item OR explicitly excluded with a one-line rationale.
      Missing integration points without rationale is a gap.

## 4. Epic decomposition

- [ ] **Does the plan decompose into epics?** Some unit of work
      below the goal level. Could be called epics, milestones, work
      packages — the noun doesn't matter; the decomposition does.
- [ ] **Is each epic at interface-level granularity?** "Build an X"
      without scope is too coarse. "Implement X interface backed by
      Y, exposing Z" is at the right grain. Apply this question to
      each unit of work in the plan.
- [ ] **Does each epic ground against an actor or integration
      boundary the plan cites?** No epic should be aspirational —
      every epic should connect to something the context names.
- [ ] **Are epics non-overlapping (or with explicit boundaries)?**
      Two epics covering the same scope is malformed unless the
      plan explicitly draws the boundary between them.

## 5. Revision-respect (only on retry)

- [ ] **Does the revised artifact address each prior finding?** If
      the upstream phase was respawned, the prior reviewer reason
      field carries the gaps. Each prior gap is either resolved with
      visible scope/epic change OR explicitly disambiguated in the
      revised plan.
- [ ] **No silent rebuttal.** If the planner's revision merely
      re-asserts a prior position without scope change, that's a
      gap.

## Verdict

If every question above has a clear "yes" answer in the plan content:
**approved**.

If one or more substance questions can't be answered from the plan:
**insufficient**, with bullet list naming each unanswered question
and what specifically is missing.

**Do not** reject for format. The plan does not need a `### Goal`
section; it needs a clear goal. The plan does not need an
`include / exclude / do_not_touch` triple bullet list; it needs a
clear in/out delineation. If you find yourself rejecting because
"the plan doesn't have section X" but the plan's content
*answers* the substance question that section would have answered,
**approve**.

You evaluate. You do not plan. If a gap requires a structural
decision (which epic boundary is right?), say so explicitly and let
the planner choose — do not author the choice for them.
