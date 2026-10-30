## 1. Approval

- [x] 1.1 Owner approves `proposal.md` and `design.md` by filling in `Approved-by: <name> (<YYYY-MM-DD>)` and removing
  this hold in the same edit. The approval rules on design Open Questions 1-2 (#386, #374). Evidence: the owner
  approved in Enjoy on 2026-10-30 ("lift the #374 deferral, D1–D3 all A"); Claude wrote the header at their request.

## 2. Pin and harness

- [x] 2.1 Pin `ateam@v0.3.5` in `.github/workflows/ci.yml`, `.github/workflows/merge-check.yml`, and `scripts/doctor.sh`.
  Update the version comments and the contract-test pin rows. Acceptance: `task doctor` reports ateam v0.3.5.
  Evidence: commit `66bf6f7e`; `task doctor` prints `ok    ateam           v0.3.5`.
- [x] 2.2 Sync `.agents/contracts/reviewer.md` to the v0.3.5 template. Acceptance: `task ateam:doctor` (v0.3.5) reports
  no violations. Evidence: commit `66bf6f7e` copies the `ateam init` v0.3.5 `reviewer.md` and `review-pr` § 5 verbatim;
  `task ateam:doctor` prints `"violations": null`.

## 3. Test-to-spec gate

- [x] 3.1 Add `task test:spec` (`ateam test-spec --base ${TEST_SPEC_BASE:-origin/main}`), wire it into the Governance
  job and the governance lane of `scripts/validate.sh` per design D1, and update `validate_task_contract_test.go`.
  Acceptance: a contract-test row fails when the step or the `--base` flag is dropped, and a planted uncited new test
  fails `task test:spec`. Evidence: commit `2ab5ffa4`; `TestCIWorkflowGate` and `TestValidateTaskMirrorsRepositoryCI`
  rows fail when the step or base is dropped, and `TestValidateTaskAndDocsContract` failed with the `--base` form
  removed from `Taskfile.yml`; planted uncited Go and Vitest tests each reported `missing spec citation`.
- [x] 3.2 Rewrite the five malformed citations per design D3 A in precursor PR #392, which merges before this PR (owner
  sequencing ruling, 2026-10-30; notes/3.2.md), then merge `origin/main` here. Acceptance: `ateam test-spec --base
  origin/main` exits 0 on this branch. Evidence: #392 squash-merged as `4de5b097`; merged into this branch at
  `7078b344`; `ateam test-spec --base origin/main` (v0.3.5) prints "all discovered supported tests have valid
  citations", exit 0.

## 4. Merge check probe

- [x] 4.1 Add the `awaiting` job (`Merge check probe`) and the conditional skip to `merge-check.yml` per design D2.
  Extend `TestMergeCheckWorkflow` so sensitivity rows fail when any of these is dropped: the probe's base checkout, its
  read-only permissions, `needs: awaiting`, or the awaiting condition. Evidence: commit `6b36f312`;
  `TestMergeCheckWorkflow` passes with 27 sensitivity rows, including each of these and `persist-credentials: false`
  and the awaiting rename.

## 5. Docs and spec

- [x] 5.1 Replace the deferral paragraph at `.agents/protocol.md:38-41` with the adopted rule. Add the rule row to
  `AGENTS.md` and `.agents/rules-index.md`, and update `review-pr` § 5 if the v0.3.5 template changed it.
  Acceptance: `AGENTS.md` is under 24,000 bytes, and `TestAgentSetupParity` passes. Evidence: `wc -c AGENTS.md` is
  23982; `go test ./test/contract/...` passes; `task ateam:doctor` reports a § 4 row-count violation when the
  rules-index row is removed.
- [x] 5.2 Apply the `repository-governance` delta. Acceptance: `task spec:check` and `task validate` pass on a clean
  tree with ateam v0.3.5 first on PATH. Evidence: `openspec archive adopt-ateam-v035` synced the delta in the archive commit; `task validate` passed on that clean tree.
