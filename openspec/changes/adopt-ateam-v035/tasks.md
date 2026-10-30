## 1. Approval

- [x] 1.1 Owner approves `proposal.md` and `design.md` by filling in `Approved-by: <name> (<YYYY-MM-DD>)` and removing
  this hold in the same edit. The approval rules on design Open Questions 1-2 (#386, #374). Evidence: the owner
  approved in Enjoy on 2026-10-30 ("lift the #374 deferral, D1–D3 all A"); Claude wrote the header at their request.

## 2. Pin and harness

- [ ] 2.1 Pin `ateam@v0.3.5` in `.github/workflows/ci.yml`, `.github/workflows/merge-check.yml`, and `scripts/doctor.sh`.
  Update the version comments and the contract-test pin rows. Acceptance: `task doctor` reports ateam v0.3.5.
- [ ] 2.2 Sync `.agents/contracts/reviewer.md` to the v0.3.5 template. Acceptance: `task ateam:doctor` (v0.3.5) reports
  no violations.

## 3. Test-to-spec gate

- [ ] 3.1 Add `task test:spec` (`ateam test-spec --base ${TEST_SPEC_BASE:-origin/main}`), wire it into the Governance
  job and the governance lane of `scripts/validate.sh` per design D1, and update `validate_task_contract_test.go`.
  Acceptance: a contract-test row fails when the step or the `--base` flag is dropped, and a planted uncited new test
  fails `task test:spec`.
- [ ] 3.2 Rewrite the three malformed citations per design D3. Acceptance: `ateam test-spec --base origin/main` exits 0.

## 4. Merge check probe

- [ ] 4.1 Add the `awaiting` job (`Merge check probe`) and the conditional skip to `merge-check.yml` per design D2.
  Extend `TestMergeCheckWorkflow` so sensitivity rows fail when any of these is dropped: the probe's base checkout, its
  read-only permissions, `needs: awaiting`, or the awaiting condition.

## 5. Docs and spec

- [ ] 5.1 Replace the deferral paragraph at `.agents/protocol.md:38-41` with the adopted rule. Add the rule row to
  `AGENTS.md` and `.agents/rules-index.md`, and update `review-pr` § 5 if the v0.3.5 template changed it.
  Acceptance: `AGENTS.md` is under 24,000 bytes, and `TestAgentSetupParity` passes.
- [ ] 5.2 Apply the `repository-governance` delta. Acceptance: `task spec:check` and `task validate` pass on a clean
  tree with ateam v0.3.5 first on PATH.
