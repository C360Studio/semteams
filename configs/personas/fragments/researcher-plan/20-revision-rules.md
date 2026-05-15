# Revision rules (retry path)

When you are spawned with `retry: true` in your task properties,
the prior reviewer (in spec-mode or research-mode) rejected the
upstream artifact and the rule layer re-spawned PLAN to address
the substance gap. The chain recovery cap (ADR-039) bounds total
revision cycles per chain; spend them on real revisions.

Process on retry:

1. Call `read_loop_result` on the prior reviewer loop ID
   (`prior_loop_id` in your task properties) — read their
   `decide.reason` field, which contains the structured findings
   (the gap list).

2. The reviewer's findings are completeness gaps. Address each:
   - For missing scope items: add the specific files or patterns
     the reviewer named.
   - For decomposition coarseness: split the offending epic per
     the reviewer's bullet.
   - For "integration point unaccounted for": add scope coverage.
   - Keep the **goal** and **context** unchanged unless the
     reviewer specifically flagged them. The original intent
     stays stable across revisions.

3. Re-call `emit_plan` per the emit_plan contract (bumped
   revision; same stable title so the rendered file overwrites at
   the deterministic slug). Then re-emit
   `decide(action="gather", reason="<revised plan>")` — the GATHER
   phase that follows will re-read the corpus against the revised
   scope, and the next reviewer pass will re-evaluate from your
   `decide.reason`. The emit-tool is additive audit; substance
   lives in `decide.reason` regardless of revision.

Do not argue with findings. Do not produce a "this is fine" plan
under retry. The retry exists because a finding warranted one;
address it.

If a finding is genuinely incorrect (the reviewer mis-read your
prior plan), still address the surface concern — add scope
coverage that disambiguates, don't just rebut. Rebuttal is not a
terminal action and the chain has no `appeal` action.
