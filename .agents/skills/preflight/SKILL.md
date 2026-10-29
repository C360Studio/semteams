---
name: preflight
description: >-
  Select and execute SemTeams Taskfile verification gates for the current diff and record reproducible evidence.
  Use before implementation pushes, handoffs, or concluding a task.
---

# Verify Changes (Preflight)

Read [the shared protocol](../../protocol.md) first. One `Taskfile.yml` entrypoint serves local verification, MCP (`ateam_task`), and CI. This skill selects existing checks and records evidence; it grants no merge, tag, or cleanup authority.

## 1. Establish the Diff and Select Gates by Scope

Work in the claim's dedicated worktree (`../semteams-wt/<branch>` for CLI sessions or `.worktrees/<branch>` via `move_to_worktree` in Enjoy desktop). Inspect `git status --short` and `git diff --stat`, and verify branch, `HEAD` SHA, dirty/untracked files, upstream, and PR target before choosing checks.

| Task | What it does |
| --- | --- |
| `task ateam:doctor` | Runs `ateam doctor --check` (with session env vars scrubbed) to verify multi-agent setup, hooks, MCP config, skills, and review-policy parity |
| `task lint` | Runs `gofmt -l`, `go mod tidy -diff`, `go vet`, pinned `revive` (`revive.toml`), `cleanup-roots:check`, and `lint-test-ports` |
| `task test:race` | Runs Go unit and contract tests under `-race -count=1` |
| `task test:browser-runner` | Runs Python unit tests for `scripts/qualify-migration-browser.py` |
| `task test:integration` | Runs `-tags=integration` Go tests sequentially (`-p 1`) via testcontainers |
| `task schema:generate` / `task schema:check-changes` | Regenerates `schemas/` and `specs/openapi.v3.yaml` and checks for git drift |
| `task ui:lint` | Runs ESLint + Prettier checks in `ui/` |
| `task ui:check` | Runs `svelte-check` (TypeScript + Svelte 5 diagnostics) in `ui/` |
| `task ui:test` | Runs Vitest unit/component tests (`npm run test:unit -- --run`) in `ui/` |
| `task ui:build` | Builds the production SvelteKit bundle in `ui/` |
| `task ui:test:e2e` | Runs Playwright mock-LLM E2E journeys against the Docker stack (follow E2E Active Monitoring Protocol) |
| `task openspec:validate` | Strictly validates OpenSpec changes/specs (`openspec validate --all --strict`), then runs `ateam state spec-check` (stray `Hold:` checks) (alias: `task spec:check`) |
| `task openspec:queue` | Shows each in-flight OpenSpec change with its `Hold:` lines and blocking conditions (alias: `task spec:queue`) |
| `task publish:verify-test` | Runs fixture tests for `scripts/publish-verify.sh` |
| `task mutate:check -- <flags>` | Runs `ateam mutate` (`-pkg -test -file -mutant -expect`): a non-destructive Go `-overlay` mutation check against a mutant kept outside the repository, reporting `detection`, `survivor`, `invalid`, or `inconclusive` (`GOFLAGS=-tags=integration` reaches integration-tagged tests) |
| `task merge:check -- <pr>` | Runs `scripts/merge-check.sh` (`ateam review gate`) against the GitHub ruleset (`Required` and `Merge check`), review policy/record, and `class:flake` issues |
| `task validate` / `task verify` | Read-only local parity gate mirroring `Repository CI` (`Required` aggregate across Go, UI, and Governance lanes via `scripts/validate.sh`) without mutating tracked or untracked worktree files |

Select gates by diff scope during iteration, and run `task validate` (alias `task verify`, via CLI or `ateam_task`) before any push or completion:

- **Documentation, `.agents/`, `.claude/`, `.codex/`, or `.gemini/` files:** `task ateam:doctor`, `go test ./test/contract/... -run 'TestAgentSetupParity|TestValidateTask' -v`, and `task openspec:validate`.
- **An OpenSpec change (`openspec/`):** `task openspec:validate`, then inspect `task openspec:queue`.
- **Go backend, product shell (`cmd/semteams/`), catalog (`internal/runtimecatalog`), or rule packs (`configs/`):** focused `go test` first during iteration, then `task lint`, `task test:race`, `go build ./...`, and `task schema:generate && task schema:check-changes` when component/payload registrations change. Prove non-trivial logic able to fail with `task mutate:check` (`ateam mutate`), or `cp` + SHA-256 where `go test -overlay` cannot reach.
- **SvelteKit UI (`ui/`):** `task ui:lint`, `task ui:check`, `task ui:test`, `npm --prefix ui run generate-types:check`, and `task ui:build`; prove non-trivial logic able to fail with a `cp` + SHA-256 restore check (`ateam mutate` is Go-only); add focused `task ui:test:e2e` journeys when interactive browser flows or `/work` / `/pulse` workbench behavior change.
- **Before any implementation push:** `task validate` (`task verify`). Do not hand-run a narrower command in place of a gate CI runs.

## 2. Protect Shared Test Infrastructure

Serialize heavy Docker/testcontainers work (`task test:integration`, `task ui:test:e2e`, `task validate`) on a shared host and preserve other sessions' containers and worktrees. When running `task ui:test:e2e`, follow the **E2E Active Monitoring Protocol** (`AGENTS.md` § 4): monitor every 20–30s via test output, `docker compose -f ui/docker-compose.agentic-e2e.yml logs --since=30s`, and `curl -s http://localhost:3100/message-logger/entries?limit=10 | jq '.[].subject'`, aborting early on loops and quoting log evidence.

## 3. Record Evidence

Record the exact command, exit status, tested `HEAD` SHA (or dirty snapshot), assertions/tests exercised, and artifact location. A log under a git-ignored or `/tmp/` path exists on this host only: say so, and include the output a reader needs in the PR comment or body. When the implementation changes, older green results remain historical evidence for the older revision.

Resolve failures from evidence; a successful re-run is not a fix for a flake. A red that your diff does not explain is filed as an issue before the next push, labelled `class:flake` when it passes and fails on the same tree. After local verification, assess the PR's current-head hosted checks (`Required`) and the protocol's reviewer/archive gates (`task merge:check -- <pr>`), ending with Ponytail's 1–2 line skipped/risk summary.
