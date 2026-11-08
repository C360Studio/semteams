# Adopt ATeam v0.3.9: repo-local `.tools/bin` shim and Enjoy agent review launch

Approved-by: Coby Leuschke (2026-11-08)

Track: design. This change edits gate files (`.github/workflows/ci.yml`, `.github/workflows/merge-check.yml`, and `.agents/review-policy.json`).

## Why

[ATeam v0.3.9](https://github.com/C360Studio/ateam/releases/tag/v0.3.9) (building on `v0.3.8`, `C360Studio/ateam#295`) adds agent-launched cross-provider reviews (`"review_launch": "agent"` in `.agents/review-policy.json`, `ateam review request`, `ateam review notify`, and host manifests under `.agents/hosts/`). SemTeams is still pinned at `v0.3.5`, where every cross-provider reviewer must be launched manually by the owner.

Locally, SemTeams also lacks a repo-local `ateam` install target: hooks, MCP configs, `Taskfile.yml`, and `scripts/doctor.sh` invoke bare `ateam` on global `PATH`. When `~/.local/bin/ateam` or `$(go env GOPATH)/bin/ateam` moves for a sister repository, `task doctor` fails and sessions fall back to one-off builds in `/tmp`. `semsource` (`#241`, `#242`) and `semengine` (`#538`) solved both before starting their port PR sequences.

## What changes

- Pin `ateam@v0.3.9` in `.github/workflows/ci.yml`, `.github/workflows/merge-check.yml`, `Taskfile.yml`, `scripts/doctor.sh`, `test/contract/workflow_gate_test.go`, `.agents/rules-index.md`, and `openspec/specs/repository-governance/spec.md`.
- Add `task tools:ateam` installing `v0.3.9` into `<main-checkout>/.tools/bin/ateam` (gitignored under `.tools/`), add `scripts/ateam` (~3.5 ms POSIX shim, no `git` fork) wired into all 7 hook/MCP configs, and prepend `.tools/bin` when present in `Taskfile.yml`, `scripts/doctor.sh`, and `scripts/validate.sh` (keeping `validate.sh` read-only).
- Enable `"review_launch": "agent"` in `.agents/review-policy.json`, add `v0.3.9`'s canonical `.agents/hosts/enjoy.toml`, sync `v0.3.9`'s `handoff` and `review-pr` skills, and update `.agents/protocol.md` § Cross-agent review.

## Non-goals

- Changing SemTeams runtime behavior, frozen SemStreams (`8b99efe9`) wiring, live categories (`research`, `autoresearch`), or parked dev packs (`create-change`, `proof-readiness`, `dev-from-task`, `dev-via-test` remain unwired under ADR-058 / ADR-061).
- Adding unexercised host manifests (`t3code.toml`, `herdr.toml`) or changing CI's `GOBIN="$RUNNER_TEMP"` isolation in `merge-check.yml`.
- Adding a GitHub `Gate files` label workflow (explicit non-goal under #372).

## Impact

- Once merged to `main`, implementing sessions launch cross-provider reviewers directly via `ateam review request --return-to <conv-id>` and `.agents/hosts/enjoy.toml`.
- `task tools:ateam` installs one repo-pinned `v0.3.9` binary shared by every linked `.worktrees/*` checkout without touching `/tmp` or global `PATH`.

Closes #413 (claimed on `gemini/ateam-v039-review-launch`, PR #414; addresses #412).
