# Bound live research-pack loops using the frozen runtime

## Why

[Issue #272](https://github.com/C360Studio/semteams/issues/272) requires hard containment before unattended research.
Existing research and autoresearch spawns inherit a component ceiling of 50 instead of declaring role budgets.
This first slice uses the frozen public `loop_max_iterations` action field; it does not need SemEngine adoption.
The claim is branch `codex/research-loop-containment`; its draft PR is recorded here once created.

## What changes

- Declare explicit positive per-role iteration ceilings on every research and autoresearch agent spawn.
- Qualify authored action → canonical task → effective loop budget using frozen public boundaries.
- Verify component ceilings still narrow task budgets and iteration exhaustion is not successful research.
- Correct stale research guidance about clarification routing and the per-entity recovery counter.
- Refresh the affected rule-family fixture provenance without relaxing its source identity check.

## Non-goals

No runtime extraction, dependency bump, engine adapter, new tool/action/payload, generic journal, global retry counter,
fan-out schema change, scheduled trigger, paid model run, deep-profile activation, UI work or parked-pack activation.
This is partial #272: schema fan-out, run-wide recovery and scheduling qualification remain in that issue.
No merge or issue closure is authorized; owner CONFIRM-CLOSE remains required.

## Acceptance and risks

Use a meaningful missing-budget regression before changing rules, then test each authored spawn through the real
frozen action executor and public loop intake. Qualify narrowing and exhaustion without copying private runtime policy.
Role budgets are conservative engineering defaults, not measured optimal costs. Unbounded fan-out and experiment counts
can still exhaust synthesis; this slice establishes no total-run budget or unattended-operation guarantee.
