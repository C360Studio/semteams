## 1. Approval

- [x] 1.1 Owner approves `proposal.md` and `design.md` (`Approved-by: Coby Leuschke (2026-11-10) sha256:abe0ea81009d`), ruling on Clarify questions 1–3 in Enjoy conversation `Upgrade ateams to v0.4.0 plan` ("File the issue, move to a worktree, and implement the full upgrade PR now", "Milestone 'semengine-port 1' and linked under epic #412", and "Only {\"ateam_version\": \"v0.4.0\"}").

## 2. Pin `ateam@v0.4.0`, `.agents/ateam.json`, and `merge-check.yml` probe permissions

- [x] 2.1 Add `.agents/ateam.json` (`{"version": 1, "ateam_version": "v0.4.0"}`), bump `v0.3.9` to `v0.4.0` in `Taskfile.yml` (`:10`), `scripts/doctor.sh` (`:67`), `.github/workflows/ci.yml` (`:40,135`), `.github/workflows/merge-check.yml` (`:55,102`), `test/contract/workflow_gate_test.go` (`:19`), and `.agents/rules-index.md` (`:48,49,63`), and grant `checks: read` to the `awaiting` job in `.github/workflows/merge-check.yml` with paired sensitivity coverage in `TestMergeCheckWorkflow`. Acceptance: `task tools:ateam`, `task doctor`, and `go test ./test/contract/... -run 'TestMergeCheckWorkflow|TestCIWorkflowGate'` pass.

## 3. Sync `v0.4.0` role contracts, canonical skills, protocol, and spec delta

- [x] 3.1 Update base role contract headers in `.agents/contracts/{architect,developer,reviewer}.md` (preserving SemTeams's additions in `architect.md`), sync `.agents/skills/{design,handoff,pickup,ponytail,preflight,review-pr}/SKILL.md` (and `.agents/skills/handoff/t3-card.html`) to `v0.4.0`, number `.agents/protocol.md` sections (`§ 1`–`§ 4` and `*Test-to-spec attribution*`) with `readingViolations` contract coverage in `test/contract/shared_work_protocol_contract_test.go`, and update `.agents/protocol.md` for `Approved-by: <name> (<YYYY-MM-DD>) sha256:<12 hex>` (`ateam#272`) and standing-pass `ateam review gate --awaiting` (`ateam#359`). Acceptance: `task ateam:doctor` (`v0.4.0`) and `go test ./test/contract/... -run 'TestAgentSetupParity'` pass with zero violations.
- [x] 3.2 Sync `repository-governance` spec delta, archive `adopt-ateam-v040`, and verify full local parity. Acceptance: `task spec:check` and `task validate` pass on a clean worktree.
