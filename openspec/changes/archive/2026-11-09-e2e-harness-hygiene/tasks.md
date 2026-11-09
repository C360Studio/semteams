## 1. Approval

- [x] 1.1 Owner approves scope and autonomous execution in Enjoy conversation `SemTeams Work Pickup` (`Approved-by: Coby Leuschke (2026-11-09)`, `quick` track, proposal-only `skip_specs: true`).

## 2. E2E harness, spec, and fixture repairs

- [x] 2.1 Require HTTP `200` on `/teams-dispatch/loops` alongside `/teams-dispatch/commands` in `ui/Taskfile.yml` `test:e2e:agentic:stack:up` readiness polling (and mirror `/teams-dispatch/loops` wait in `scripts/qualify-migration-browser.py`).
- [x] 2.2 Update `ui/e2e/agentic/research-mvp.spec.ts` artifact path check to match `/artifacts/research/*.md` (compose bind mount) as well as `.artifacts/research/*.md`, and filter `listLoops` root selection by `task_id.startsWith("dispatch-")` in `ui/e2e/agentic/task-story-trace.spec.ts` and `ui/e2e/agentic/task-title-aliases.spec.ts`.
- [x] 2.3 Switch `approval-pause` and `approval-resume` (`ui/Taskfile.yml`, `scripts/semstreams-migration-matrix.json`, `scripts/qualify-migration-browser.py`, `test/fixtures/journeys/approval-{pause,resume}.yaml`, `ui/e2e/agentic/approval-{pause,resume}.spec.ts`) onto a derived `e2e-flow-bootstrap-approval-4c.gen.json` gating advertised `bash` on `research/06`'s recovery coordinator without editing shipped rule or bootstrap configs.
- [x] 2.4 Add Go contract test `test/contract/e2e_harness_hygiene_test.go` (`// spec:` cited + `cp` + SHA-256 mutant-detection verified) and Python runner unit coverage in `scripts/test_qualify_migration_browser.py` guarding `stack:up` `/teams-dispatch/loops` polling, root-loop `dispatch-` selection, artifact regex, and advertised-tool parity on `approval-pause`/`approval-resume`.

## 3. Verification

- [x] 3.1 Verify all 9 `#420`-affected Playwright E2E subtasks pass end to end against the Docker mock-LLM stack via `task ui:test:e2e:agentic:*`, and run `task spec:check`, `task test:spec`, and read-only `task validate` on a clean worktree.
