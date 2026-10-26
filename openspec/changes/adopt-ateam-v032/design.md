# Design: Clean Upstream Adoption of ATeam v0.3.2

## Context

`C360Studio/ateam` `v0.3.2` (`45b6db8`) resolves SemTeams's brownfield adoption obstacles (`C360Studio/ateam#157`):
1. `ateam doctor --check` validates numbered rule-section and table-row parity between `AGENTS.md` and `.agents/rules-index.md` using SemTeams's own five rule domains rather than ATeam's three-domain taxonomy, and checks `.agents/protocol.md` by required harness phrases (`Deliverable`, `non-goals`, `Definition of Done`, `Stop boundary`, `ateam state handoff`, `openspec/changes/archive/`).
2. Strict skill checks apply only to the five canonical skills (`pickup`, `handoff`, `preflight`, `ponytail`, `review-pr`), while SemTeams's four domain skills (`kv-or-stream`, `new-payload`, `orchestration-check`, `query-pattern`) coexist as `adopter_skills`.
3. `ateam review gate` provides the cross-forge merge gate (main ruleset up-to-date check on `Required`, `implemented-by:` / `reviewed-by:` tier-and-band comparison via `.agents/review-policy.json`, commit-bound PR review record verification, and open `class:flake` defense).

## Key Decisions

### D1. Consolidate generic workflow skills; retain SemTeams specialist roles and domain skills

- Run `ateam init` without `--force` so `AGENTS.md`, `CLAUDE.md`, `.agents/README.md`, `.agents/protocol.md`, `.agents/rules-index.md`, and `Taskfile.yml` remain preserved.
- Retire `.agents/skills/semteams-{pickup,handoff,preflight}` and `.claude/skills/semteams-{pickup,handoff}` in favor of ATeam's canonical `pickup`, `handoff`, `preflight`, `ponytail`, and `review-pr` skills (and matching `.claude/skills/*` + `.gemini/commands/*.toml` adapters).
- Enrich `.agents/skills/preflight/SKILL.md` with SemTeams's diff-based gate matrix (`task validate` / `task verify`, Go, UI, Governance, Playwright E2E) while keeping ATeam's required canonical markers (`## 1.`, `task verify`, `Record Evidence`).
- Retain SemTeams's six specialist role contracts (`architect`, `go-developer`, `go-reviewer`, `svelte-developer`, `svelte-reviewer`, `technical-writer`) and their `.claude/agents/*.md` and `.codex/agents/*.toml` adapters alongside ATeam's `.agents/contracts/{developer,reviewer}.md`, verified by `TestAgentSetupParity` (`test/contract/shared_work_protocol_contract_test.go`).

### D2. Peer frontier model policy (`cross_provider: true`, dormant `routes` off)

- Add `.agents/review-policy.json` without `routes` (keeping `ateam review check` dormant per ATeam ADR 0001) and with `"cross_provider": true`.
- Place `claude:opus`, `claude:fable`, `codex:gpt-6-astra`, `codex:gpt-6-sol`, `codex:gpt-6.1-sol`, and `gemini:gemini-4-argon` in `"frontier"`, and `claude:sonnet` / `gemini:gemini-3.8-flash` / `gemini:gemini-3.1-pro` in `"workhorse"` / `"light"`, matching SemTeams's `.agents/README.md` role-routing contract where frontier Claude, Codex, and Gemini reviewers cross-review one another's PRs.

### D3. Unified `Required` CI aggregate, read-only `ateam doctor --check`, and `ateam review gate`

- Rename the aggregate job in `.github/workflows/ci.yml` (`status-check` -> `required`, `name: Required`) and update `scripts/publish-verify.sh` (+ `scripts/publish-verify_fixture_test.sh`, `docs/release.md`, and `openspec/specs/repository-governance/spec.md`) to check `Required`, aligning SemTeams's merge and release gates with `ateam review gate`'s GitHub check name (`internal/review/gate_github.go:160`).
- Install `github.com/c360studio/ateam/cmd/ateam@v0.3.2` in the `Repository CI` `governance` job (using Go `1.26.4` in `governance` so `ateam@v0.3.2`'s `go 1.26.4` directive builds cleanly without mutating SemTeams's `go.mod`), and run `ateam doctor --check` in both `ci.yml` and `scripts/validate.sh` with session-ID environment variables scrubbed (`env -u ATEAM_SESSION_ID -u CLAUDE_SESSION_ID -u CODEX_SESSION_ID -u GEMINI_SESSION_ID ateam doctor --check`) so `doctor`'s warm-up `session-start` hook probe never writes `.ateam/writer.json` or `.ateam/mcp-checked-sessions.json` in a read-only validation run.
- Replace the bespoke shell implementation in `scripts/merge-check.sh` with `exec "${ATEAM:-ateam}" review gate "$@"` and update `test/contract/mergecheck_test.go` to exercise the wrapper and `ateam review gate` contract.
- Wire `task spec:check` (`task openspec:validate`) and `task spec:queue` (`task openspec:queue`) to preserve strict OpenSpec validation and `Hold:` reporting while keeping `scripts/openspec-queue.sh` fixture coverage intact.

### D4. Dual-format `/work` review-record reader (`githubSource.ts`)

- Extend `parseReviewComments` in `ui/src/lib/server/work/githubSource.ts` so each PR comment is checked for:
  1. Legacy SemTeams HTML markers (`<!-- semteams-review-request -->` and `<!-- semteams-review-record -->`) for historical PRs.
  2. ATeam's canonical PR review record contract (`.agents/contracts/reviewer.md` § Record and `internal/review/gate.go`): a comment carrying `reviewed-by: <agent> (<model>, <effort>)` (or `### Independent Cross-Agent Review` heading naming one agent), designating a single commit via `Reviewed at <7..40-hex-sha>` (ignoring fenced code blocks and rejecting comments that designate multiple distinct `Reviewed at` / `Head:` commits, matching `ateam review gate`), and `Verdict: APPROVE` (`"pass"`) or `Verdict: REQUEST CHANGES` / `CHANGES REQUESTED` (`"blocking"`), plus plain `## Review request` / `Review request` comments naming a commit SHA and optional reviewer lane.
- Later valid comments in chronological order supersede earlier ones, preserving existing `deriveBatonOverlays` (`not_requested`, `requested`, `current`, `stale`, `changes_requested`) without changing `/api/work/*` response schemas.
