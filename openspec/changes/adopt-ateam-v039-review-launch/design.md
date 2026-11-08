# Design: Adopt ATeam v0.3.9, `.tools/bin` Shim, and Enjoy Review Launch

## Context

Line references are to `origin/main` (`a3a6d371` / current `HEAD`) and `C360Studio/ateam` `v0.3.9`:

- `.github/workflows/ci.yml:40,135`, `.github/workflows/merge-check.yml:55,102`, `scripts/doctor.sh:64`, and `test/contract/workflow_gate_test.go:19` pin `ateam@v0.3.5`.
- `.agents/review-policy.json:1-49` sets `"cross_provider": true` with no `review_launch` field; `.agents/hosts/` does not exist.
- `.mcp.json:4`, `.agents/plugins/ateam/mcp_config.json:4`, `.agents/hooks.json:6,11,16`, `.claude/settings.json:12,23,34,45`, `.codex/config.toml:8`, `.codex/hooks.json:8,18,28,38`, `.gemini/settings.json:14,25,36,47,55`, and `Taskfile.yml:49,97,103,108,116` all invoke bare `ateam` on `PATH`.
- `test/contract/specqueue_test.go:92-104` requires `spec:check` (`Taskfile.yml`) and `validate` (`taskfiles/openspec.yml`) to declare only `desc`, `summary`, and `cmds` (`openspec validate --all --strict` then `ateam state spec-check` — no `deps:` key).
- `test/contract/validate_task_contract_test.go:539-585` (`TestValidateScriptPrerequisiteFailures`) verifies `scripts/validate.sh` is strictly read-only and fails closed when `ateam` is absent in a clean fixture directory, while `:651-657` (`TestValidateTaskAndDocsContract`) pins the literal command strings in `Taskfile.yml`.
- `AGENTS.md` is 23,982 bytes (18 bytes below `ateam doctor`'s 24,000-byte cap) and mentions no `ateam` version pin.

## Goals / Non-Goals

Goals: adopt `ateam v0.3.9`, enable `review_launch: "agent"` with `.agents/hosts/enjoy.toml`, and isolate the repository's `ateam` binary in `<main-checkout>/.tools/bin/ateam` shared across linked worktrees while preserving every read-only `task validate` and contract-test invariant. Non-goals: runtime/pack changes, CI runner-temp changes in `merge-check.yml`, or `t3code`/`herdr` host manifests.

## Decisions

### D1. Repo-local `ateam` installation and worktree/gate resolution

- **A (selected, owner ruling 2026-11-08): `<main-checkout>/.tools/bin/ateam` + `scripts/ateam` shim + conditional `PATH` prepend (`semsource#241` pattern).**
  - `Taskfile.yml` defines `ATEAM_VERSION: v0.3.9`, resolves `ATEAM_BIN_DIR` as `$(dirname "$(git rev-parse --path-format=absolute --git-common-dir)")/.tools/bin`, sets `env.PATH: "{{.ATEAM_BIN_DIR}}:{{.PATH}}"`, and adds `task tools:ateam` (`GOBIN="{{.ATEAM_BIN_DIR}}" GOTOOLCHAIN=auto go install ...@{{.ATEAM_VERSION}}`).
  - `.gitignore` ignores `.tools/`. Every `.worktrees/<branch>` checkout shares the main checkout's `.tools/bin/ateam` without re-downloading or touching `/tmp`.
  - `scripts/ateam` is a 22-line POSIX `/bin/sh` shim that parses `.git` directly when `.git` is a worktree file (no `git` subprocess; ~3.5 ms, under `ateam doctor`'s 10 ms hook target), preferring `$main/.tools/bin/ateam`, then `$here/.tools/bin/ateam`, then `PATH`. All 7 hook and MCP configs call `scripts/ateam` (`"$CLAUDE_PROJECT_DIR"/scripts/ateam` in `.claude/settings.json`).
  - `scripts/doctor.sh` and `scripts/validate.sh` prepend `$main_root/.tools/bin` and `$REPO_ROOT/.tools/bin` to `PATH` **only when those directories exist** — keeping `validate.sh` strictly read-only and preserving `TestValidateScriptPrerequisiteFailures` (`missing_ateam`), `TestSpecCheckWiring`, and `TestValidateTaskAndDocsContract`.
  - Cost: one 22-line shell script + contract test. Forecloses nothing.
- **B: Per-worktree `.bin/ateam` as a `deps:` of `doctor` / `spec:check` (`semengine#545`).** Rejected: adding `deps:` to `spec:check` violates `TestSpecCheckWiring` (`test/contract/specqueue_test.go:103`), and installing inside `validate` violates SemTeams's read-only `validate.sh` contract.
- **C: Keep global `PATH` only.** Rejected: collides whenever a sister repository pins a different `ateam` release or a GUI host omits `~/go/bin`.

Adopter seam: run `task tools:ateam` once per clone (or after a pin bump); if omitted, `task doctor` and `scripts/ateam` print `run: task tools:ateam` (or fall back to `PATH` if a matching `ateam` is already installed).

### D2. Host manifests shipped for `review_launch: "agent"`

- **A (selected, owner ruling 2026-11-08): `.agents/hosts/enjoy.toml` only (copied verbatim from `ateam v0.3.9`).** Matches `semsource#241` and `semengine#538`. Terminal sessions (Claude Code, Codex CLI) need no manifest because `ateam review request <n>` prints the eligible reviewer commands directly.
- **B: Ship `enjoy.toml` + `t3code.toml`.** Rejected by owner ruling; `t3code.toml` is unused here.

Adopter seam: in Enjoy, the implementing agent posts `Review request`, runs `ateam review request --return-to <conversation-id> <pr>`, and invokes Enjoy's `start_conversation` MCP tool as `.agents/hosts/enjoy.toml` specifies; the reviewer posts its commit-bound record and runs `ateam review notify --record <file> --return-to <id> <pr>`.

## Risks / Trade-offs

- **Base-commit bootstrap:** `merge-check.yml` checks out `base.sha` (`main`), so this PR is judged by `v0.3.5` on `main` (which ignores unknown `review_launch` fields because it reads `main`'s `.agents/review-policy.json`) and `v0.3.9` + `review_launch: "agent"` take effect immediately after squash merge.
- **`AGENTS.md` 24 KB cap:** `AGENTS.md` is at 23,982 bytes; keep `AGENTS.md` unchanged (no rule row changes, only pin citations in `.agents/rules-index.md`).

## Migration Plan

Single squash-merged PR (`#414`). Run `task tools:ateam` once on the shared host so `<main-checkout>/.tools/bin/ateam` holds `v0.3.9`. CI workflows continue installing `ateam@v0.3.9` directly (`GOBIN="$RUNNER_TEMP"` in `merge-check.yml`).

## Open Questions

1. **Scope & sequence (resolved 2026-11-08):** Owner ruled to file `#413` in `semengine-port 1` and implement the PR immediately in a dedicated worktree.
2. **Host manifest selection (resolved 2026-11-08):** Owner ruled to ship `.agents/hosts/enjoy.toml` only.
