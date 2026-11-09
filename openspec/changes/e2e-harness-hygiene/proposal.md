# Repair post-v0.2.0 E2E harness, spec, and fixture defects on main

Approved-by: Coby Leuschke (2026-11-09)

Track: quick. Proposal-only change (`skip_specs: true`): repairs four pre-existing Playwright E2E harness, spec, and mock-fixture defects recorded in `#420` without changing any spec Requirement, Go runtime package, or shipped bootstrap config.

## Why

Recording the post-`v0.3.0` Milestone 1 SemEngine port acceptance floor (`#403`, `docs/migrations/semengine-port-baseline/`) surfaced four pre-existing mock-LLM Playwright E2E defects that accumulated on `main` because `Repository CI` (`scripts/validate.sh`) does not boot Docker E2E stacks:

1. `ui/Taskfile.yml` `stack:up` polls `/health` and `/teams-dispatch/commands` without `/teams-dispatch/loops`, racing fail-closed `graphview` `503` readiness across 4 API-first subtasks.
2. `ui/e2e/agentic/research-mvp.spec.ts:348` expects relative `/\.artifacts\/research\/.+\.md$/` while `ui/docker-compose.agentic-e2e.yml` sets `SEMTEAMS_RESEARCH_ARTIFACT_DIR=/artifacts/research`.
3. `ui/e2e/agentic/task-story-trace.spec.ts` and `task-title-aliases.spec.ts` pick the first lexicographical loop UUID from `/teams-dispatch/loops` without filtering root `dispatch-*` tasks.
4. `approval-pause` and `approval-resume` call `create_rule` on `research/06`'s recovery coordinator, which advertises `["read_loop_result", "bash", "decide"]` (omitting `create_rule`), so `admitToolCall` rejects the call before `ApprovalFilter` and fails the run.

## What changes

- Update `ui/Taskfile.yml` `test:e2e:agentic:stack:up` readiness polling to require HTTP `200` on both `/teams-dispatch/commands` and `/teams-dispatch/loops`.
- Update `ui/e2e/agentic/research-mvp.spec.ts` to accept both container-absolute `/artifacts/research/<file>.md` and local-default `.artifacts/research/<file>.md` (`/(?:^|\/)\.?artifacts\/research\/.+\.md$/`).
- Filter `listLoops` selection in `ui/e2e/agentic/task-story-trace.spec.ts` and `ui/e2e/agentic/task-title-aliases.spec.ts` to root dispatch tasks (`l.task_id?.startsWith("dispatch-")`) so `/activity` top-level `task-card` assertions never select a child loop UUID.
- Derive a gitignored temporary `e2e-flow-bootstrap-approval-4c.gen.json` config (gating `bash`, advertised on `research/06`'s recovery coordinator) in `ui/Taskfile.yml` and `scripts/qualify-migration-browser.py` (`scripts/semstreams-migration-matrix.json`), and update `test/fixtures/journeys/approval-{pause,resume}.yaml` + `ui/e2e/agentic/approval-{pause,resume}.spec.ts` to exercise `bash` while keeping `configs/e2e-flow-bootstrap.json` and `configs/rules/research/06-needs-clarification-replan.json` untouched.
- Add Go contract regression checks in `test/contract/e2e_harness_hygiene_test.go` (`// spec:` cited + `task mutate:check` verified) and a Python unit test in `scripts/test_qualify_migration_browser.py` covering the four invariants.

## Non-goals

- Modifying Go runtime behavior, `configs/flow-bootstrap.json`, `configs/e2e-flow-bootstrap.json`, `configs/rules/research/06-needs-clarification-replan.json`, or the immutable `#403` historical floor (`docs/migrations/semengine-port-baseline/`).
- Re-pointing parked ADR-058 `dev_via_test` journeys (`ask-user-pause`, `clarification-resume`, `run-failed-coordinator*`) or `#261` (`artifact-context-handoff`).

## Impact

- Restores all 9 `#420`-affected Playwright E2E subtasks (`18/18` active subtasks green, `5` explicit skips) ahead of Milestone 3 (`#408`).

Closes #420.
