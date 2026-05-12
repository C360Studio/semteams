# Completeness checklist (substance-grounded)

For each question below, decide: does the artifact content answer
this? "Yes, clearly" → checked. "Implicit but I can't tell which
field or sentence addresses it" → flag as a gap with the specific
question. "No" → gap. **Approve when every substance question is
answered in a way the next phase (builder) could verify by
re-reading the artifact.**

The spec artifact you evaluate is the architect's
`emit_dev_via_spec_artifact` output. Its structured shape:

```
title, goal, context, actors[], integration_points[], tasks[],
checks[], provenance
```

Required fields (validated at the wire): `title`, `goal`, `context`,
`actors`, `tasks`, `provenance`. Conditionally required by the
architect's commitment contract: `integration_points[]` (when actors
relate across boundaries), `checks[]` (when `integration_points[]`
names an external actor).

## 1. Goal

- [ ] **Is `goal` a concretely named target capability?** A single
      identifiable outcome — a named interface, endpoint, component,
      or capability. "Build a driver" alone is not concrete; "Implement
      X interface backed by Y, exposing Z" is.
- [ ] **Is the goal testable in principle?** A downstream builder
      should be able to tell whether a working implementation has
      achieved it.

## 2. Context

- [ ] **Does `context` motivate the work?** The artifact should say
      *why* the work matters — system motivation, downstream consumer
      need, etc. Doesn't have to be a section called Context; the
      `context` field is the content channel.
- [ ] **Does `context` reference at least one actor from `actors[]`?**
      The narrative should name an actor explicitly — "the OSH driver
      framework's `IDriver` interface" — not just "the driver."

## 3. Actors and integration_points

- [ ] **Is every entry in `actors[]` named concretely?** Each actor
      has a `name` (a system, framework, or service) and a one-line
      `role`. Empty `role` is a gap; vague names like "the system"
      are a gap.
- [ ] **Does every `integration_points[]` entry name both sides?**
      Each entry has `from` and `to` as actor names appearing in
      `actors[]`, plus a `direction` (read/write) and `data`
      description. A `from`/`to` that doesn't appear in `actors[]`
      is a gap.
- [ ] **Are integration_points present when the goal crosses a
      system boundary?** If `goal` + `context` describe work that
      flows data between actors and `integration_points[]` is empty,
      that's a gap — the architect needs to enumerate the flows.
- [ ] **Does `data` on each integration_point name what flows
      concretely?** "Data" alone is not concrete; "MeshPacket
      protobuf messages" or "POSITION_APP packets" is.

## 4. Tasks decomposition

- [ ] **Does `tasks[]` decompose the work below goal-level?** Some
      unit of work below the goal level. The architect picks the
      grain; reviewer enforces presence.
- [ ] **Is each task at interface-level granularity?** "Build X"
      without scope is too coarse. Each task's `scope` should
      identify what the task delivers concretely.
- [ ] **Does each task ground against `actors[]` or
      `integration_points[]`?** Walk each task: `grounds_actors[]`
      should name actors the task touches, `grounds_integration_points[]`
      should name flows the task implements. Empty `grounds_actors[]`
      AND empty `grounds_integration_points[]` is a gap — every task
      should connect to something `context` cites.
- [ ] **Are tasks non-overlapping (or with explicit boundaries)?**
      Two tasks covering the same actor + integration_point scope
      is malformed unless the artifact draws the boundary between
      them in the `scope` text.
- [ ] **Does each `integration_points[]` entry map to at least one
      task?** Walk `integration_points[]`; for each, find a task
      whose `grounds_integration_points[]` names it. An
      integration_point with no implementing task is a gap.

## 5. Revision-respect (only on retry)

- [ ] **Does the revised artifact address each prior finding?** If
      the architect was respawned, the prior reviewer reason field
      carries the gaps. Each prior gap is either resolved with a
      visible field change (actors added, tasks split,
      integration_points enumerated) OR explicitly disambiguated in
      the revised artifact.
- [ ] **No silent rebuttal.** If the architect's revision merely
      re-asserts a prior position without field change, that's a
      gap.

## Verdict

If every question above has a clear "yes" answer in the artifact
fields: **approved**.

If one or more substance questions can't be answered from the
artifact: **insufficient**, with bullet list naming each unanswered
question and what specifically is missing.

**Do not** reject for format. The artifact's prose rendering can
phrase things however the template chooses — what matters is the
structured field content. Do not reject because the markdown
template chose one section order over another. Do not reject
because the `role` field on an actor uses "framework" instead of
"system."

If you find yourself rejecting because "the artifact's prose
doesn't have section X" but the structured field that section
would have rendered *is* populated, **approve**.

You evaluate. You do not author. If a gap requires a structural
decision (which task boundary is right?), say so explicitly and
let the architect choose — do not author the choice for them.
