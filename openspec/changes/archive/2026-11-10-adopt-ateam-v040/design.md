# Design: Adopt ATeam v0.4.0, `.agents/ateam.json` Pin, and Standing-Pass Probe

## Context

Line references are to `origin/main` (`413ef964`) and `C360Studio/ateam` `v0.4.0` (`59a64735`):

- `Taskfile.yml:10`, `scripts/doctor.sh:67`, `.github/workflows/ci.yml:40,135`, `.github/workflows/merge-check.yml:55,102`, `test/contract/workflow_gate_test.go:19`, `.agents/rules-index.md:48,49,63`, and `openspec/specs/repository-governance/spec.md:64,70,180` pin `ateam@v0.3.9`.
- `.agents/ateam.json` does not exist; `ateam v0.4.0` (`internal/state/settings.go`, `internal/bootstrap/pin.go`) reads `.agents/ateam.json` (`{"version": 1, "ateam_version": "v0.4.0"}`) in `ateam doctor --check` to verify that both the running `ateam` binary and the hook binary (`scripts/ateam`) match the declared release.
- Running `ateam v0.4.0` `doctor --check` against `origin/main` fails on `.agents/contracts/{architect,developer,reviewer}.md` because `v0.4.0` updated the base contract opening lines to task-scoped `.agents/protocol.md` section reads (`ateam#200`) and updated `architect.md` paragraph 2 to name `Approved-by: <name> (<YYYY-MM-DD>) sha256:<12 hex>` (`ateam#272`).
- `.github/workflows/merge-check.yml:33-36` grants `contents: read`, `issues: read`, and `pull-requests: read` to `awaiting`, whereas `ateam v0.4.0`'s `ateam review gate --awaiting` queries `repos/{owner}/{repo}/commits/<head>/check-runs?check_name=Merge+check` (`ateam#359`) and requires `checks: read` on `awaiting`.
- `AGENTS.md` is 23,982 bytes (18 bytes below `ateam doctor`'s 24,000-byte cap).

## Goals / Non-Goals

Goals: adopt `ateam v0.4.0`, commit `.agents/ateam.json` with `ateam_version: "v0.4.0"`, grant `checks: read` to the `awaiting` probe job in `merge-check.yml`, and sync base contracts, canonical skills, and `.agents/protocol.md` while keeping `task validate` green and `AGENTS.md` under 24,000 bytes. Non-goals: runtime/pack changes or committing explicit `forges` in `.agents/ateam.json`.

## Decisions

### D1. `.agents/ateam.json` scope

- **A (selected, owner ruling 2026-11-10): Commit `{"version": 1, "ateam_version": "v0.4.0"}` only.**
  - `ateam v0.4.0` (`internal/state/settings.go:75-77`) requires `"version": 1` and validates `"ateam_version"` in `PinStatus` (`internal/bootstrap/pin.go:25-56`), failing `ateam doctor --check` if either the running binary or `scripts/ateam` hook binary drifts from `v0.4.0`. Omitting `forges`, `merge_method`, `default_branch`, and `commit_statuses` preserves auto-detected GitHub forge settings and existing squash/main defaults.
  - Cost: one 4-line JSON file. Forecloses nothing.
- **B: Include explicit `forges` array in `.agents/ateam.json`.** Rejected by owner ruling; git remote auto-detection is sufficient and avoids locking the board's forge settings endpoint with HTTP 409.

Adopter seam: `task tools:ateam` installs `v0.4.0` into `<main-checkout>/.tools/bin/ateam`; `task ateam:doctor` checks `.agents/ateam.json` against both the running binary and `scripts/ateam`.

### D2. `awaiting` probe permissions in `.github/workflows/merge-check.yml`

- **A (selected): Add `checks: read` to the `awaiting` job permissions and keep `merge-check` job permissions at `contents: read`, `issues: read`, `pull-requests: read`.**
  - Matches `ateam v0.4.0`'s `.github/workflows/merge-check.yml:34-40` and `internal/bootstrap/mergecheck_test.go:1165-1166`. `ateam review gate --awaiting` checks whether a passing `Merge check` check-run already stands at the PR head (`ateam#359`).
  - `test/contract/workflow_gate_test.go` (`mergeCheckWorkflowViolations` and `TestMergeCheckWorkflow`) is updated with paired sensitivity mutations verifying `checks: read` is required on `awaiting`.
- **B: Leave `awaiting` without `checks: read`.** Rejected: GitHub Actions tokens without `checks: read` cannot read check-runs when `ateam v0.4.0`'s probe checks for a standing pass at `HEAD`.

Adopter seam: `.github/workflows/merge-check.yml` `awaiting` job and `TestMergeCheckWorkflow`.

## Risks / Trade-offs

- **Base-commit bootstrap:** `merge-check.yml` runs from `base.sha` (`main`), so this PR is evaluated by `v0.3.9` on `main` and `v0.4.0` takes effect on `main` as soon as this PR squash-merges.
- **`AGENTS.md` 24 KB cap:** `AGENTS.md` remains untouched at 23,982 bytes; protocol details live in `.agents/protocol.md` and `.agents/rules-index.md`.

## Migration Plan

Single squash-merged PR (`Closes #429`, `Addresses #412`). Run `task tools:ateam` so `<main-checkout>/.tools/bin/ateam` updates from `v0.3.9` to `v0.4.0`.

## Open Questions

1. **Scope & sequence (resolved 2026-11-10):** Owner ruled in Enjoy conversation `Upgrade ateams to v0.4.0 plan` to file `#429` in `semengine-port 1` under `#412`, move to a worktree, and implement the full upgrade PR now.
2. **`.agents/ateam.json` contents (resolved 2026-11-10):** Owner ruled to set `ateam_version` only and rely on git remote origin detection.
