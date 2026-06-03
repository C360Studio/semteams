# Execute loop discipline

## Per-iteration discipline

The iteration pattern is `edit → test → emit → branch`. Keep each
iteration *tight*. The framework runs you until you converge or
escalate; there is no per-attempt budget you should reason about
(per ADR-044 §Stuck-task recovery).

**One edit per iteration.** Don't batch six file edits then run
the test once. Running the test after each meaningful change makes
the failure-to-success transition observable — the LLM can attribute
the pass to the right change. Batching obscures cause.

**Exception:** when a single logical change spans multiple files
(e.g., add a function in `pkg/foo/parse.go` + its test in
`pkg/foo/parse_test.go`), make both edits and run once. The unit
of work is the *change*, not the file.

## Reading the spec

Your spawn prompt names your task ID. The plan triples live on
the run entity, threaded into your loop via the
`run-loop-entity-id` related_loops key. Read them via
`bash $entity.triple` substitution (the rule engine pre-substitutes
into your spawn prompt) or by calling `read_loop_result` on
upstream loops.

Key triples you care about (substituted into your spawn prompt
by the dispatch rule):

- `plan.task.<your-id>.goal` — what success means
- `plan.task.<your-id>.target_files` — your edit scope (JSON-encoded
  array — parse it)
- `plan.task.<your-id>.test_command` — what to run for convergence
- `plan.task.<your-id>.expected_outcome` — human-readable "done
  looks like" (optional, helps disambiguate)
- `plan.task.<your-id>.assumptions` / `plan.task.<your-id>.non_goals`
  — context for what's in / out of scope

## Handling test failures

When `pass=false`:

1. **Parse the actual error.** Don't read the test output as a
   wall of text — find the specific failing assertion or panic.
2. **Locate the relevant code.** The test names should hint at
   the function under test. `bash grep` or `bash rg` if you need
   to find it; use `bash cat` to read the current state.
3. **Form a hypothesis.** Use scratchpad. Write down: "the test
   expects X; the code produces Y; therefore I'll change Z."
4. **Make the surgical change.** Edit only what supports the
   hypothesis. Don't refactor adjacent code.
5. **Run the test again.** Single iteration commits to one change.

If you cycle through the same edit twice (or see the same failure
after a change you thought would fix it), you're guessing. Use
scratchpad to think more carefully — quote the actual failure
text, list two alternative hypotheses, pick one with explicit
reasoning. If you genuinely can't form a hypothesis, escalate via
`decide(needs_clarification)`.

## Anti-patterns

- **Testing your test.** Once Ralph passes, don't run other test
  commands to "make sure." The task is done; terminate.
- **Refactor drift.** "While I'm here, this code could be cleaner."
  No. Karpathy Rule 3 says surgical. Refactoring outside
  `target_files` is the cross-task contamination the ADR §Risks
  warns about.
- **Test-gaming.** Writing test stubs that pass trivially
  (`assert.True(t, true)`), or modifying the test command to
  always exit 0, or commenting out the failing assertion. CBG's
  full integration test (Slice 4) re-runs everything; gaming
  here just delays the failure to chain-end where it costs more.
- **Premature `needs_clarification`.** Don't bail on iteration 2.
  Try at least a few hypotheses on the actual error. Escalation
  is for genuine blockers (spec is internally contradictory, test
  command depends on a binary that doesn't exist, target_files
  cover the wrong code), not for "this is hard."
- **Pad the audit.** Don't write a paragraph to scratchpad each
  iteration — short, structured thinking only. Long scratchpad
  dumps don't help you converge and bloat the trajectory.

## Terminal contract

Exactly one `decide` call per loop, at the end:

- `decide(action="measured", reason="<one-line summary of the
  change that fixed it>")` — convergence. The rule 04a wake-up
  routes this to the coordinator, which stamps `plan.task.<id>.status
  = done` and walks to the next task.
- `decide(action="needs_clarification", reason="<specific blocker
  in concrete terms — quote the failure, name what's missing>")`
  — escalation. Coordinator wakes and routes to ask_user.

If the framework's `max_iterations` safety ceiling is hit without
either decision, the loop terminates with outcome=failed and rule
04b stamps the failure for coordinator pickup. That is not your
choice — the framework bounds runaway for safety. Aim to converge
OR escalate explicitly; don't pace yourself against the ceiling.
