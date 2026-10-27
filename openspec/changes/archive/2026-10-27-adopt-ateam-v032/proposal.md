# Adopt ATeam v0.3.2 cleanly across harness, CI/merge gate, and `/work` review records

## Why

[Issue #362](https://github.com/C360Studio/semteams/issues/362), claimed by [PR #363](https://github.com/C360Studio/semteams/pull/363), adopts the released [`C360Studio/ateam` `v0.3.2` (`45b6db8`)](https://github.com/C360Studio/ateam/releases/tag/v0.3.2) unchanged as SemTeams's first-customer pilot (`C360Studio/ateam#157`). Replacing bespoke duplicate workflow scripts and skills with the pinned upstream `ateam` binary keeps state reconciliation, hooks, MCP tools, parity checks, and merge-gate mechanics shared across `sem*`, while SemTeams retains product architecture rules, specialist role contracts, domain skills, and `task validate` verification under [ADR-061](../../../../docs/adr/061-agent-runtime-ownership-and-observable-work.md).

## What changes

- Initialize `ateam` `v0.3.2` (`45b6db8`) without `--force` and reconcile client hooks and MCP configs (`.claude/settings.json`, `.mcp.json`, `.codex/config.toml`, `.codex/hooks.json`, `.gemini/settings.json`, `.gemini/commands/*.toml`, `.agents/hooks.json`, `.agents/plugins/ateam/`), `.agents/contracts/{developer,reviewer}.md`, `.agents/protocol.md`, and `.gitignore` so `ateam doctor --check` passes (`"passed": true`).
- Consolidate generic `semteams-{pickup,handoff,preflight}` skills onto ATeam's canonical `pickup`, `handoff`, and `preflight` (preserving SemTeams's diff-based gate selection inside `preflight`), while keeping SemTeams's four domain skills (`kv-or-stream`, `new-payload`, `orchestration-check`, `query-pattern`) as `adopter_skills` and SemTeams's six specialist role contracts (`architect`, `go-developer`, `go-reviewer`, `svelte-developer`, `svelte-reviewer`, `technical-writer`).
- Configure `.agents/review-policy.json` with `"cross_provider": true` and SemTeams's peer frontier model tiers (`claude:opus`, `codex:gpt-6-astra`, `codex:gpt-6-sol`, `gemini:gemini-4-argon`), without enabling ATeam's dormant `routes` / JSON `ateam review check` path.
- Pin `ateam@v0.3.2` in `.github/workflows/ci.yml` (`governance` job) and `scripts/doctor.sh`, run `ateam doctor --check` in both `Repository CI` and `scripts/validate.sh` without mutating worktree state, rename the `Repository CI` aggregate check-run and `scripts/publish-verify.sh` target from `CI Status Check` to `Required` to match `ateam review gate`, and wrap `ateam review gate` in `scripts/merge-check.sh` (`task merge:check`).
- Update `parseReviewComments` in `ui/src/lib/server/work/githubSource.ts` to recognize ATeam's `reviewed-by:` + `Reviewed at <sha>` + `Verdict: APPROVE | REQUEST CHANGES` (and `### Independent Cross-Agent Review`) PR comment format alongside historical `<!-- semteams-review-* -->` comments.

## Non-goals

- Do not modify SemTeams runtime components, live category packs (`research`, `autoresearch`), support packs (`coordinator`, `agent-run`, `ops`), schemas, or OpenAPI contracts, and do not activate implemented-but-parked development packs (`create-change`, `proof-readiness`, `dev-from-task`, `dev-via-test`).
- Do not fork or patch the `ateam` `v0.3.2` binary locally, copy `ateam`'s development `Taskfile.yml`, or activate ATeam's dormant `routes` / JSON `ateam review check` path.
- Do not introduce a competing execution journal or workflow engine outside ADR-061 and SemEngine's substrate boundary.

## Impact

Updates agent/client harness configuration, CI/validation/merge-gate scripts and contract tests, documentation (`AGENTS.md`, `.agents/*`, `docs/release.md`), and the read-only `/work` GitHub comment parser (`ui/src/lib/server/work/githubSource.ts`). No live runtime or parked pack behavior changes.
