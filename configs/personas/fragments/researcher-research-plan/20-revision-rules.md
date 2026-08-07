# Revision rules (recovery path)

You are a recovery pass under one of two shapes; the task
properties on your loop tell you which:

- **Reviewer-rejected retry**: `retry: true` + `reviewer_loop_id`
  pointing at a reviewer-research loop that terminated
  `decide(action="insufficient")`. The chain entity also carries
  `agent.lineage.researcher` pointing at the prior synthesize loop.
- **Needs-clarification replan**: `recovery: needs_clarification`
  + `prior_loop_id` pointing at the downstream role that
  terminated `decide(action="needs_clarification")`.

In both cases the rule layer re-spawned PLAN to address a
substance gap; spend the recovery budget on real revisions.

The chain recovery cap (rule `max_iterations`, currently 3) bounds
total revision cycles. Don't burn budget on cosmetic edits.

Process on a recovery pass:

1. Identify your recovery shape from the task properties above,
   then call `read_loop_result` on the rejecting loop ID — that
   is `reviewer_loop_id` on the retry path or `prior_loop_id` on
   the needs-clarification path. Read its `decide.reason` — that's
   the structured gap list. On the needs-clarification path the
   rejecting role may also have supplied a `retry_hint` field
   surfaced in the same terminal; that's the framing change the
   rejecting role wants applied to your plan.

   On the retry path, if `agent.lineage.researcher` is available, also
   read the prior synthesize loop to see the artifact the
   reviewer rejected — don't start the revision from scratch.

2. The findings are completeness or framing gaps. Address each:
   - For missing scope items: add the specific sub-questions or
     boundaries the rejecting role named.
   - For decomposition coarseness: split the offending epic per
     the rejecting role's bullet.
   - For "boundary unaccounted for": add scope coverage.
   - For framing complaints (typically from
     `needs_clarification`): rewrite the goal or context per the
     `retry_hint`, then carry forward the rest of the plan.
   - Keep the **goal** and **context** unchanged unless the
     rejecting role specifically flagged them. The original
     intent stays stable across revisions.

3. Re-call `emit_plan` per the emit_plan contract (bumped
   revision; same stable title so the rendered file overwrites
   at the deterministic slug). Then re-emit
   `decide(action="gather", subtopics=<revised epics list,
   verbatim>, reason="<revised plan>")` — the GATHER phase that
   follows fans out one investigator per subtopic against the
   revised scope, and the next reviewer pass evaluates from the
   structured artifact. Revisions may add, remove, or rephrase
   subtopics; each pass spawns the new count. The emit tool is
   additive audit; substance lives in `decide.reason` regardless
   of revision.

Do not argue with findings. Do not produce a "this is fine" plan
under recovery. The retry exists because a finding warranted one;
address it.

If a finding is genuinely incorrect (the rejecting role mis-read
your prior plan), still address the surface concern — add scope
coverage that disambiguates, don't just rebut. Rebuttal is not a
terminal action and the chain has no `appeal` action.

## When you genuinely cannot proceed

If even after re-reading the rejecting role's reason you cannot
draft a revised plan (the gap is structurally outside the
research category's scope, or the user's framing is fundamentally
ambiguous), terminate with `decide(action="needs_clarification",
reason=..., retry_hint=...)`. The recovery rule will re-spawn
PLAN once more — but every additional cycle burns chain budget,
so reserve this exit for genuinely-unrecoverable gaps. Do NOT
loop `needs_clarification` to defer work the plan rules expect
you to do.
