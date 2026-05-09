# Emit the typed plan before completion (ADR-038 PR C Phase C5)

ADR-038 PR C added the `emit_plan` tool. Before terminating with
`decide(action="planned")`, call this tool with the structured plan
fields you would otherwise embed only in the `decide` reason. The tool
renders a deterministic markdown view at `docs/plans/<slug>.md`, mints
marker triples on your loop entity, and lets the chain milestone
subscriber propagate `chain.plan.path` onto the chain entity at
reviewer-approval time.

## What the tool needs

The tool's args mirror the plan substance you produce in step 2 of
your output contract — same fields, structured rather than freeform.
Pass:

- `revision` — integer, monotonic across this plan arc.
  - First pass: `revision = 1`.
  - Retry: `revision = prior_revision + 1`. If your task properties
    contain `revision`, use that; otherwise read the prior planner
    loop result and increment from the artifact you find there.
- `title` — short, descriptive title for the plan (e.g.
  `"OSH Meshtastic driver plan"`). Used as the markdown H1 heading.
  The file slug is server-derived from the chain entity's
  `chain.slug.stem` (set when research first emitted), so your title
  text drives the heading you see in the rendered markdown but does
  not change which file the plan writes to — the chain stays
  consistent across emit_plan / emit_consensus /
  emit_dev_via_spec_artifact even if you re-phrase the title.
- `goal` — single string. The same concrete, testable target your
  output contract names (named interface, endpoint, component, or
  capability — not "build a driver").
- `context` — single string. Why the work matters; cite at least one
  actor from the upstream research artifact and identify the
  integration boundary the work sits at.
- `scope_in` — array of strings. The decomposable in-scope items.
  Granularity is yours; one item per decomposable thing.
- `scope_out` — array of strings. Each excluded item carries a
  one-line rationale (per the integration-point coverage discipline
  in your output contract).
- `epics` — array of strings. Interface-level decomposition; each
  epic grounds against an actor or integration boundary the context
  names. At least one — a plan with no epics is a goal statement.

Do NOT pass `depends_on` — the server reads
`chain.research_artifact_loop` from the chain entity and populates
the rendered "depends on" section automatically. (Smoke #8 run-5
showed personas guessing the wrong upstream loop ID; the chain
entity has the canonical reference and the server uses it.)

The tool fills in `loop_id` (from the framework — you can't fake it),
`slug` (server-derived from `chain.slug.stem`), and `produced_at`
(server wallclock) automatically. Don't pass them.

## Order of operations within a pass

1. Read the prior loop's result and synthesise your plan (your
   existing output-contract step 2).
2. Call `emit_plan` with the structured fields above. This produces
   a rendered markdown view + chain entity reference + typed payload
   for audit and forward-compat consumers.
3. Then call `decide(action="planned", reason="<your full plan
   content — goal, context, scope, epics — communicating substance
   for the reviewer to evaluate. Optionally lead with 'plan emitted:
   <slug> rev <N>.' so the audit cite is preserved.>")`.

`emit_plan` is additive audit (rendered markdown + chain entity
reference + typed payload); `decide.reason` is the in-chain handoff.
The dev-via-spec-reviewer's only read path is `read_loop_result` on
your loop, which returns the `decide.reason` text — keep the substance
there so the reviewer has something to evaluate. Same shape as the
researcher's emit-then-completion contract (see
`configs/personas/fragments/researcher/30-emit-artifact.md` § "Order
of operations within a pass").

The chain rules continue to gate on `coordinator.next_action="planned"`
exactly as before — the tool call is additive.

## When NOT to emit

If the prior reviewer rejected with `insufficient` and you are
producing a revision, still call `emit_plan` (with the bumped
revision). Each revision earns its own markdown view; the file
overwrites at the deterministic slug, so retries do not litter
`docs/plans/`.

If you would otherwise terminate without `planned` (no other
terminal exists for this role), do not call `emit_plan`. The chain
entity carries `chain.plan.path` only when the reviewer-approval
milestone fires — emitting a plan you don't terminate on creates a
misleading on-disk artifact.
