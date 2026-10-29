## 1. Approval

- [x] 1.1 Owner approves `proposal.md` and `design.md` by filling `Approved-by: <name> (<YYYY-MM-DD>)` and removing
  this hold in the same edit, ruling on design Open Questions 1-2 (#369, #375). Evidence: the owner approved in Enjoy
  on 2026-10-29, ruling D3 A (keep the explicit strict step) and authorizing the ruleset change; Claude wrote the
  header at their request and added `Merge check` to the `main-required-checks` ruleset (id 25924145) the same day.

## 2. Pin and harness

- [x] 2.1 Pin `ateam@v0.3.4` in `.github/workflows/ci.yml`, the new `merge-check.yml`, and `scripts/doctor.sh`.
  Update the version comments in `Taskfile.yml` and `test/contract/shared_work_protocol_contract_test.go`.
  Acceptance: `task doctor` reports ateam v0.3.4. Evidence: commit ac16f5b0; `task doctor` printed `ok ateam v0.3.4` with `~/go/bin` first on PATH
  (`ateam version`: `v0.3.4 (module)`).
- [x] 2.2 Sync `.agents/contracts/{developer,reviewer}.md` and `.agents/skills/review-pr/SKILL.md` to the v0.3.4
  templates. Acceptance: `task ateam:doctor` (v0.3.4) reports no violations. Evidence: commit ac16f5b0 copies the drifted
  sentences from a fresh v0.3.4 `ateam init` in a temp dir (`diff -q` against the templates is silent); `task ateam:doctor`
  printed `"violations": null`.

## 3. Workflows

- [x] 3.1 Move the `merge-check` job into `.github/workflows/merge-check.yml`, following design D1 and D2, and make
  `required` need only `go`, `ui`, and `governance`. Acceptance: contract tests pin the trigger list (including
  `edited`), the concurrency setting, base checkout, read-only permissions, and `ATEAM_REQUIRED_CHECKS`. Each has a
  sensitivity row, plus a row that fails if a `merge-check` job is left in `ci.yml`. Evidence: commit d8c2338f;
  `TestMergeCheckWorkflow` (13 sensitivity rows) and `TestCIWorkflowGate` (8 rows, including `merge-check` job left in
  `ci.yml` and `required` still needing `merge-check`) pass; `actionlint` is clean on both workflows.
- [x] 3.2 Add the draft guard and rename to every `Repository CI` job and to `Merge check`, following design D4.
  Acceptance: a contract test fails when the guard or the rename is dropped from `required`. Evidence: commit d8c2338f;
  `TestCIWorkflowGate` rows `draft guard dropped on required` and `rename dropped on required` (plus `go` guard and
  `governance` rename rows), and `TestMergeCheckWorkflow` rows for its own guard and rename, pass.

## 4. Retire openspec-queue.sh (#369)

- [x] 4.1 Remove `scripts/openspec-queue.sh`, `scripts/openspec-queue_fixture_test.sh`, and the `queue-strict` and
  `queue-test` tasks. Wire `spec:check` and `openspec:validate` per design D3, and drop
  `task openspec:queue-test` from `ci.yml` and `scripts/validate.sh`. Rewrite `test/contract/specqueue_test.go` to
  assert the new wiring, and update `validate_task_contract_test.go`. Acceptance: `rg openspec-queue` finds matches
  only under `openspec/changes/archive/` and `docs/migrations/`, and `TestSpecCheckWiring` fails when
  `ateam state spec-check` is dropped. Evidence: commit 2728fc64; `TestSpecCheckWiring` row `ateam spec-check line removed`
  passes; `rg -l openspec-queue` outside `openspec/changes/archive/` and `docs/migrations/` matches only this change's
  files, and `rg -l queue-test` adds only `openspec/specs/repository-governance/spec.md:50`, which archive (5.2) rewrites.

## 5. Docs and spec

- [x] 5.1 Update `AGENTS.md`, `.agents/rules-index.md`, `.agents/protocol.md`, `.agents/skills/preflight/SKILL.md`,
  `.agents/contracts/technical-writer.md`, and `README.md` for the new `Merge check` context and the retired script.
  Acceptance: `AGENTS.md` is under 24,000 bytes and `TestAgentSetupParity` passes. Evidence: commit d6a1d2d5;
  `wc -c AGENTS.md` is 23,800; `go test ./test/contract/... -count=1` passes; `task ateam:doctor` reports no violations;
  `task validate` passed on a clean tree at d6a1d2d5 with ateam v0.3.4 first on PATH.
- [ ] 5.2 Apply the `repository-governance` delta in this change. Acceptance: `task spec:check` and
  `task validate` pass on a clean tree, run with ateam v0.3.4 first on PATH.
