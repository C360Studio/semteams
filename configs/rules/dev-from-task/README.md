# Dev From Task Rule Pack

This pack is the bridge from a reviewed OpenSpec change to the existing
dev-via-test execution rail.

It deliberately does not create another planning state machine. The approved
`change.<slug>.*` facts remain the planning authority, `project_spec_tasks`
projects those facts into the existing `plan.*` contract, and the existing
Ralph/CBG rules own task execution and chain-end acceptance.

The flow is explicit and HITL-friendly:

1. A coordinator emits `decide(action="dev_from_task")` from a run-attached
   loop. Rules 01a/01b stamp `dev_from_task.requested` onto the run entity.
2. Rule 02 fires only when the run is approved and proof-readiness has already
   stamped `proof_readiness.implementation_ready=true`.
3. Rule 02 spawns a coordinator walker. The walker calls `project_spec_tasks`,
   provisions or verifies the sandbox, creates the `plan.chain_start_git_tag`
   in the workspace, then emits `decide(action="dev_via_test",
   subtopics=["<one-task-id>"])`.
4. The existing dev-via-test rule 03 spawns Ralph. Existing rules 05-07 keep
   walking tasks and run CBG at the end.

There is no automatic implementation simply because proof readiness passed.
The request marker is the approval lever the UI and operator controls consume.
