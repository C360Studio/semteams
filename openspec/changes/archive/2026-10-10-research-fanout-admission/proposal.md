# Authoritative planner fan-out admission design

## Why

[Issue #272](https://github.com/C360Studio/semteams/issues/272) requires a schema-enforced research fan-out ceiling.
The frozen `decide` executor owns the terminal subtopic list; `emit_plan` is additive audit. A schema limit on the
latter cannot prevent a planner from skipping it or supplying another list to `decide`. The measured 15-item bypass
and reproducible probe are in [the issue handoff](https://github.com/C360Studio/semteams/issues/272#issuecomment-6101060504).

## What changes

This design-only claim defines the authoritative pre-mutation validation boundary, one pack policy source, schema
agreement, error/correction behavior, integration ownership and proving cases. It records the frozen source survey,
rejected shortcuts and migration posture in an ADR and design. It does not implement enforcement.

[Draft PR #312](https://github.com/C360Studio/semteams/pull/312) on `codex/research-fanout-boundary` owns this design claim.
The parent issue remains open for implementation, run-wide recovery and scheduling. No issue closure is authorized.

## Non-goals

No runtime copying, tool wrapper implementation, dependency bump, new tool/action/payload/storage, rule-routing
change, generic execution mechanism, SemEngine cutover, adaptive rounds, scheduling, parked-pack activation or UI
change. No paid model runs or retained-state operations. Existing research and autoresearch behavior stays frozen.

## Impact

Deliver ADR/OpenSpec design and independent architecture review. Specify implementation admission without promoting
unimplemented fan-out enforcement into the living spec. The numerical policy and integration choice must be explicit;
source inspection and the prior executor probe are not end-to-end qualification.
