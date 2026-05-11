# Emit the typed consensus before accepting

Before terminating with
`decide(action="accept")`, call this tool with the structured consensus
fields. The tool renders a deterministic markdown view at
`docs/consensus/<slug>.md`, mints marker triples on your loop entity,
and lets the chain milestone subscriber propagate `chain.consensus.path`
onto the chain entity at accept time.

The downstream architect reads your `accept` reason as their primary
input — `emit_consensus` is the structured form of that same substance,
so the architect can also pull from the chain entity / rendered file
for cross-referencing.

## What the tool needs

The tool's args mirror the consensus substance you produce in step 3 of
your probing contract — same fields, structured rather than freeform
prose. Pass:

- `title` — short, descriptive title for the consensus (e.g.
  `"OSH Meshtastic driver consensus"`). Used as the markdown H1
  heading. The file slug is server-derived from the chain entity's
  `chain.slug.stem` (set when research first emitted), so your title
  text drives the heading you see in the rendered markdown but does
  not change which file the consensus writes to — the chain stays
  consistent across emit_plan / emit_consensus /
  emit_dev_via_spec_artifact even if you re-phrase the title.
- `summary` — single string. One-line accept rationale densely citing
  the chain (actor names, integration boundaries, epic decomposition).
  This is what the architect curates into the spec's chain consensus.
- `chain_consensus` — array of strings. Dense bullets the architect
  uses verbatim. Each bullet should cite chain specifics — actor
  names, integration boundaries, epic decomposition — drawn from the
  reviewer's approved summary. Required (>= 1) — an empty consensus
  is just an opinion.
- `considered_concerns` — array of strings. Optional audit trail of
  concerns raised across challenger iterations and how they resolved.
  Useful for cross-arc consumers and the architect; include when you
  iterated through `concerns_raised → planner-revision → accept`.

Do NOT pass `depends_on` — the server reads `chain.plan_loop` and
`chain.plan_reviewer_loop` from the chain entity and populates the
rendered "depends on" section automatically. (Smoke #8 run-5 showed
the challenger filling both `plan_loop` and `reviewer_loop` slots
with the same reviewer ID; the chain entity has the canonical pair
distinct and the server uses it.)

The tool fills in `loop_id` (from the framework — you can't fake it),
`slug` (server-derived from `chain.slug.stem`), and `produced_at`
(server wallclock) automatically. Don't pass them.

## Order of operations within a pass

1. Read the prior reviewer's result and walk the failure-class probes
   (your existing probing-contract steps 1-2).
2. **Form your verdict internally** — accept or concerns_raised —
   before reaching for any terminal tool. The branch determines what
   you call next.
3. **If your verdict is `accept`:**
   - Call `emit_consensus` with the structured fields above. This
     produces a rendered markdown view + chain entity reference +
     typed payload for audit and the architect's cross-referencing.
   - Then call `decide(action="accept", reason="<dense one-line
     accept rationale citing actors, integration boundaries, epic
     decomposition — same substance as `summary` and
     `chain_consensus` bullets, in prose form, for the architect to
     curate. Optionally lead with 'consensus emitted: <slug>.' so
     the audit cite is preserved.>")`.
4. **If your verdict is `concerns_raised`:**
   - Do **not** call `emit_consensus`. Concerns are not consensus;
     emitting one would create a misleading on-disk artifact and
     stamp `chain.consensus.path` on a chain that never reached
     accept.
   - Call `decide(action="concerns_raised", reason="<bullet list,
     each concern naming the failure class, the specific evidence in
     the plan, and what would resolve it>")` directly.

`emit_consensus` is additive audit (rendered markdown + chain entity
reference + typed payload); `decide.reason` is the in-chain handoff.
The architect's primary input is the challenger's accept reason —
keep the substance there so the architect has something to curate
without reading off-loop files.

Downstream rule `05-challenger-accept-to-architect` continues to gate
on `coordinator.next_action="accept"` exactly as before — the tool
call is additive. The chain entity carries `chain.consensus_loop` +
`chain.consensus.path` only when the accept terminal fires; the
concerns-raised path keeps the chain in the planner-reviewer-challenger
cycle without a consensus milestone.

Once you have committed to a verdict in step 2, do not flip it
between `emit_consensus` and `decide` — the rendered markdown becomes
misleading evidence on disk if the verdict reverses to
`concerns_raised` after emission.
