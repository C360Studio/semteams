---
name: semteams-preflight
description: >-
  Select and run the existing SemTeams verification gates for a concrete diff and record their evidence.
  Use before implementation pushes or when assessing local readiness.
---

# Verify a SemTeams change

The [shared protocol](../../protocol.md) owns claims and landing. Read it first. This skill selects existing checks and
records evidence; it grants no merge, tag, or cleanup authority.

## Establish the diff and its owner

Work in the claim's dedicated worktree (`../semteams-wt/<branch>` for CLI sessions or `.worktrees/<branch>` via
`move_to_worktree` in Enjoy desktop). Verify its branch, HEAD, dirty/untracked files, upstream, and PR target before
choosing checks. Compare the committed diff against the actual PR base; also inspect staged and unstaged work. Do not
assume the discovery checkout's local `main` is current. Before every commit or push, verify the branch still matches
the claimed PR head.

## Select the existing gates

`task --list` shows what exists; inspect `Taskfile.yml`, `taskfiles/`, `scripts/validate.sh`, and
`.github/workflows/ci.yml` when exact coverage matters.

| Task | What it does |
| --- | --- |
| `task lint` | Runs `gofmt -l`, `go vet`, and pinned `revive` (`revive.toml`) |
| `task test:race` | Runs Go unit and contract tests under `-race -count=1` |
| `task test:browser-runner` | Runs Python unit tests for `scripts/qualify-migration-browser.py` |
| `task test:integration` | Runs `-tags=integration` Go tests sequentially (`-p 1`) via testcontainers |
| `task schema:generate` / `task schema:check-changes` | Regenerates `schemas/` and `specs/openapi.v3.yaml` and checks for git drift |
| `task ui:lint` | Runs ESLint + Prettier checks in `ui/` |
| `task ui:check` | Runs `svelte-check` (TypeScript + Svelte 5 diagnostics) in `ui/` |
| `task ui:test` | Runs Vitest unit/component tests (`npm run test:unit -- --run`) in `ui/` |
| `task ui:build` | Builds the production SvelteKit bundle in `ui/` |
| `task ui:test:e2e` | Runs Playwright mock-LLM E2E journeys against the Docker stack (follow E2E Active Monitoring Protocol) |
| `task openspec:validate` | Strictly validates all OpenSpec changes and specs (`openspec validate --all --strict`) |
| `task openspec:queue` | Shows each in-flight OpenSpec change with its `Hold:` lines and blocking conditions |
| `task openspec:queue-test` | Runs fixture tests for `scripts/openspec-queue.sh` |
| `task publish:verify-test` | Runs fixture tests for `scripts/publish-verify.sh` |
| `task validate` | Read-only local parity gate mirroring `Repository CI` (Go, UI, and Governance lanes via `scripts/validate.sh`) without mutating tracked or untracked worktree files |

`Repository CI` (`.github/workflows/ci.yml`) runs three required jobs (`go`, `ui`, `governance`) feeding the
`CI Status Check` aggregate. `task validate` executes the exact same checks locally in read-only mode (writing temporary
build and schema artifacts to `mktemp -d` and diffing against tracked `schemas/` and `specs/openapi.v3.yaml`).

Select by what the diff changes during iteration, and run `task validate` before any push:

- **Documentation, `.agents/`, `.claude/`, or `.codex/` files:** `go test ./test/contract/... -run 'TestAgentSetupParity|TestValidateTaskContract' -v`, `task openspec:validate`, and `task openspec:queue-test`.
- **An OpenSpec change (`openspec/`):** `task openspec:validate`, `task openspec:queue-test`, then inspect `task openspec:queue` in this worktree.
- **Go backend, product shell (`cmd/semteams/`), catalog (`internal/runtimecatalog`), or rule packs (`configs/`):** focused `go test` first during iteration, then `task lint`, `task test:race`, `go build ./...`, and `task schema:generate && task schema:check-changes` when component/payload registrations change.
- **SvelteKit UI (`ui/`):** `task ui:lint`, `task ui:check`, `task ui:test`, `npm --prefix ui run generate-types:check`, and `task ui:build`; add focused `task ui:test:e2e` journeys when interactive browser flows or `/work` / `/pulse` workbench behavior change.
- **Before any implementation push:** `task validate`.

Do not hand-run a narrower command in place of a gate CI runs: the task owns flags and tool pins.

## Protect shared test infrastructure

Serialize heavy Docker/testcontainers work (`task test:integration`, `task ui:test:e2e`, `task validate`) on a shared
host. Preserve other sessions' containers and worktrees. When running `task ui:test:e2e`, follow the **E2E Active
Monitoring Protocol** (`AGENTS.md` § 4): monitor every 20–30s via test output,
`docker compose -f ui/docker-compose.agentic-e2e.yml logs --since=30s`, and
`curl -s http://localhost:3100/message-logger/entries?limit=10 | jq '.[].subject'`, aborting early on loops and quoting
log evidence.

## Report evidence and remaining gates

Record the tested HEAD (or dirty snapshot), exact command, exit status, assertions/tests exercised, and artifact
location. A log under a git-ignored or `/tmp/` path exists on this host only: say so, and include the output a reader
needs in the PR comment or body. When the implementation changes, older green results remain historical evidence for
the older revision.

Resolve failures from evidence; a successful re-run is not a fix for a flake. A red that your diff does not explain is
filed as an issue before the next push, labelled `class:flake` when it passes and fails on the same tree. After local
verification, assess the PR's current-head hosted checks (`CI Status Check`) and the protocol's reviewer/archive gates.
Local green is not merge authorization.
