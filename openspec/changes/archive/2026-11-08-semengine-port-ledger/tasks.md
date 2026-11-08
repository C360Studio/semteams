## 1. Approval

- [x] 1.1 Owner pre-approves `proposal.md` (`Approved-by: Coby Leuschke (2026-11-08)`) in Enjoy conversation `SemEngine Milestone 1 Pickup` ("Run end-to-end: pre-approve OpenSpec, implement, pass task validate, and launch cross-agent review").

## 2. Measurement script and upstream substrate verification

- [x] 2.1 Inspect `C360Studio/semengine` (`main`, open issues `#25`–`#30`, `#77`, `#155`, `#361`, and ADRs `108`, `109`, `110`, `112`, `113`) against all SemStreams imports in SemTeams; verify whether any unfiled substrate gap exists outside D4 `defer-exclude` packages and file upstream issues if found (`0` unfiled substrate gaps found against `C360Studio/semengine@01567cc48d4d89ede05dfdf0aa1675a68bf9402b`).
- [x] 2.2 Add stdlib-only measurement tool `scripts/measure-semengine-port-ledger.go` and unit/contract tests in `test/contract/semengine_port_ledger_test.go` (proven able to fail via mutation check) to compute exact non-test, `_test.go`, and `scripts/` import counts per SemStreams path. Acceptance: `go test ./test/contract/... -run TestSemEnginePortLedger` passes.

## 3. Crosswalk ledger and verification

- [x] 3.1 Write `docs/migrations/semengine-port-ledger.md` with the complete 56-path repository crosswalk (+ 2 `scripts/`-only paths), file counts, SemEngine dispositions (`carry` / `adapt` / `defer-exclude` / unported), upstream issue links, Milestone 1/2 consumer actions, consumer ADR adoption notes (`ADR-108`..`ADR-113`), and exact regeneration commands.
- [x] 3.2 Run `task spec:check` and read-only `task validate` on a clean worktree before requesting cross-provider review.
