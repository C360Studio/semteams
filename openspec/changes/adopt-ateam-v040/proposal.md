# Adopt ATeam v0.4.0: `.agents/ateam.json` pin, digest-bound approval, and standing-pass probe

Approved-by: Coby Leuschke (2026-11-10) sha256:df10fd8c42f8

Track: design. This change edits gate workflows (`.github/workflows/ci.yml` and `.github/workflows/merge-check.yml`).

## Why

[ATeam v0.4.0](https://github.com/C360Studio/ateam/releases/tag/v0.4.0) (`59a64735`) introduces committed team settings (`.agents/ateam.json` with `"version": 1` and `"ateam_version": "v0.4.0"`, `C360Studio/ateam#333`, `#334`, `#341`), task-scoped `.agents/protocol.md` reads and digest-bound proposal approval (`Approved-by: <name> (<YYYY-MM-DD>) sha256:<12 hex>`, `C360Studio/ateam#200`, `#272`), simplified Ponytail levels (`on`/`off` only, `C360Studio/ateam#275`), and the standing-pass `Merge check` probe (`checks: read` on `awaiting`, `C360Studio/ateam#359`). SemTeams is currently pinned at `v0.3.9` (`#413` / PR `#414`).

## What changes

- Add `.agents/ateam.json` (`{"version": 1, "ateam_version": "v0.4.0"}`) and bump `ateam@v0.3.9` → `ateam@v0.4.0` in `Taskfile.yml`, `scripts/doctor.sh`, `.github/workflows/ci.yml`, `.github/workflows/merge-check.yml`, `test/contract/workflow_gate_test.go`, `.agents/rules-index.md`, and `openspec/specs/repository-governance/spec.md`.
- Grant `checks: read` to the `awaiting` probe job in `.github/workflows/merge-check.yml` and enforce it in `TestMergeCheckWorkflow` (`test/contract/workflow_gate_test.go`) so `ateam review gate --awaiting` can inspect whether a passing `Merge check` stands at `HEAD` (`C360Studio/ateam#359`).
- Sync `.agents/contracts/{architect,developer,reviewer}.md` base headers, `.agents/skills/{design,handoff,pickup,ponytail,preflight,review-pr}/SKILL.md`, `.claude/skills/*/SKILL.md`, and `.agents/protocol.md` to `ateam v0.4.0`.

## Non-goals

- Changing SemTeams runtime behavior, frozen SemStreams (`8b99efe9`) wiring, live categories (`research`, `autoresearch`), or parked dev packs (`create-change`, `proof-readiness`, `dev-from-task`, `dev-via-test`).
- Committing explicit `forges` in `.agents/ateam.json` (forge detection from `git remote` remains authoritative per owner ruling).

## Impact

- `task ateam:doctor` (`v0.4.0`) verifies the running and hook binaries against `.agents/ateam.json`'s `ateam_version: "v0.4.0"` pin.
- `Merge check` stays skipped on non-draft code PRs until `reviewed-by:` is in the PR description or a passing `Merge check` stands at `HEAD`.

Closes #429 (claimed on `gemini/ateam-v040`; addresses #412).
