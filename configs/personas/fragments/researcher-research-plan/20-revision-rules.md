# Revision rules (recovery path)

A revised plan can follow either path:

- **Reviewer-rejected retry**: `retry: true` + `reviewer_loop_id`
  pointing at a reviewer-research loop that terminated
  `decide(action="insufficient")`. The spawn prompt also supplies
  the prior synthesize loop through `agent.lineage.researcher`
  when available.
- **Coordinator re-dispatch**: a role's `needs_clarification`
  routes to the coordinator. Only if the coordinator chooses
  `research` does PLAN run again. Your `parent_loop_id` points
  to that coordinator; its reason supplies the corrected framing.
  The coordinator's recovery properties are not passed to PLAN.

Address the supplied substance gap. Don't spend a revision on
cosmetic edits. Rule counters do not impose a chain-wide retry
limit; each newly spawned loop has its own iteration budget.
Read the framework's iteration-budget signal and leave room to
emit the plan and terminal decision within this pass.

Process on a recovery pass:

1. Identify the path from the task properties above, then call
   `read_loop_result` on `reviewer_loop_id` for a reviewer retry
   or `parent_loop_id` for a coordinator re-dispatch. Read the
   reviewer's gap list or the coordinator's corrected framing
   from `decide.reason`. Use any supplied `retry_hint` as context,
   not as evidence that PLAN was spawned directly by the role
   that raised `needs_clarification`.

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
reason=..., retry_hint=...)`. The recovery rule routes to the
coordinator, which may re-dispatch research, ask the user, or
respond directly. Name the unresolved gap and what would unblock
it. Do NOT repeat `needs_clarification` to defer work the plan
rules expect you to do.
