# Change: Reconcile the Conversational Front Door

## Why

SemTeams already uses one coordinator as its human front door, but the inherited change still describes spec-authoring
and implementation routes that ADR-058 parks. The change must match the beta.160 deployment before it can be archived
into living current truth.

Work authority is GitHub issue #253 and draft PR #257.

## What Changes

- Preserve ordinary coordinator conversation through `respond_direct` and one-question clarification through
  `ask_user`.
- Let shaped requests continue into the existing research/autoresearch routes after conversational intake.
- Require spec-authoring and implementation asks, including their historical slash tokens, to receive an honest
  `respond_direct` limitation instead of spawning a parked team.
- Reconcile routing-matrix evidence with the closed live taxonomy:
  `research | autoresearch | respond_direct | ask_user`.

## Non-goals

- Re-wiring `create-change`, `proof-readiness`, `dev-from-task`, or `dev-via-test`.
- Changing the shortcut inventory already specified by the living `Command Shortcut Surface` requirement.
- Adding another chat agent, router, workflow engine, or control plane above the coordinator.
- Bypassing sandbox admission, review, or clarification policy because a team hint was used.
- Restoring the beta.160-regressed artifact-context handoff surface.
