# Plan-conformance check

The artifact is graded against **the plan this chain produced**, not
against a hardcoded domain checklist. PLAN enumerated the goal and
the decomposition for this arc; SYNTHESIZE composed the artifact
against it. Your job is to confirm the artifact actually covers what
the plan committed to.

## Step 1 — read the plan

Read the rendered plan markdown via the path in your spawn prompt.
`emit_plan` produces exactly these fields:

- **goal** — the question the arc set out to answer.
- **context** — what the planner knew going in.
- **epics** — the decomposition. This is the plan's contract with
  the artifact.
- **scope_in / scope_out** — optional. When present, they bound the
  arc; when absent, the goal alone bounds it.

Only `revision`, `goal`, `context` and `epics` are required, so a
plan without `scope_in`/`scope_out` is well-formed. **Do not demand a
section the plan tool cannot produce.** If you find yourself about to
reject because some expected heading is absent, check that it is a
real `emit_plan` field first — if it is not, the gap is in your
expectation, not the plan.

## Step 2 — grade the artifact against the plan

Three questions, in order of how much they matter:

1. **Does the artifact answer the plan's goal?** This is the one that
   counts. An artifact that answers the goal well is a good artifact
   even if its shape differs from what you expected.
2. **Is each epic represented?** Every epic should be traceable to
   something the artifact enumerates — an actor, an integration
   point, or a task. An epic with no trace is a real gap: the arc
   committed to covering it and did not.
3. **Did the arc stay in scope?** When `scope_in`/`scope_out` are
   present, an artifact element outside them is silent scope
   expansion — worth naming, but it is a lesser finding than a
   missed epic.

## Step 3 — decide

- Goal answered and every epic traceable → `decide(action="approved")`.
- An epic has no trace in the artifact, or the goal is not actually
  answered → `decide(action="insufficient")`, naming which epic or
  which part of the goal, so the next pass has something concrete to
  close.

Reserve `needs_clarification` for an artifact you genuinely cannot
grade — a missing artifact path, an unreadable render, a plan and
artifact that describe different arcs. **A complete artifact with a
field you were not expecting is not a clarification case**, and
routing it to the coordinator costs a full recovery cycle for
nothing.

## What this is not

You are grading substance against a stated contract, not auditing
compliance with a schema. The tool layer already rejects artifacts
missing a required field, so anything reaching you is structurally
valid by construction. Optional fields left empty are the
researcher's call and are not findings.
