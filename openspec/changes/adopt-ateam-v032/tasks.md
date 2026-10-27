## 1. Harness initialization, skill consolidation, and review policy

- [x] 1.1 Run `ateam init` (`v0.3.2`, `45b6db8`) without `--force`, reconcile hooks/MCP settings (`.claude/settings.json`, `.mcp.json`, `.codex/config.toml`, `.codex/hooks.json`, `.gemini/settings.json`, `.gemini/commands/*.toml`, `.agents/hooks.json`, `.agents/plugins/ateam/`), `.agents/contracts/{developer,reviewer}.md`, `.agents/protocol.md` markers, and `.gitignore`, and add `.agents/review-policy.json` (`cross_provider: true` with SemTeams's frontier tiers; see `notes/1.1.md`).
- [x] 1.2 Consolidate `semteams-{pickup,handoff,preflight}` onto canonical `pickup`, `handoff`, and `preflight` (retaining SemTeams's gate selection matrix in `preflight`), update `AGENTS.md`, `.agents/README.md`, `.agents/protocol.md`, `.agents/rules-index.md`, and `test/contract/shared_work_protocol_contract_test.go`, and verify `ateam doctor --check` reports `"passed": true`.

## 2. CI, `task validate`, `Required` aggregate check, and merge gate

- [x] 2.1 Pin `ateam` `v0.3.2` in `.github/workflows/ci.yml` (`governance` and `go` jobs), `scripts/doctor.sh`, and `scripts/validate.sh`, running `ateam doctor --check` in read-only mode (session env vars scrubbed via `task ateam:doctor`) and updating `test/contract/validate_task_contract_test.go`.
- [x] 2.2 Rename the `Repository CI` aggregate check-run and `scripts/publish-verify.sh` (+ `scripts/publish-verify_fixture_test.sh` and `docs/release.md`) from `CI Status Check` to `Required`, and wrap `ateam review gate` in `scripts/merge-check.sh` with updated `test/contract/mergecheck_test.go` tests.

## 3. `/work` review-record compatibility (`ui/`)

- [x] 3.1 Update `parseReviewComments` in `ui/src/lib/server/work/githubSource.ts` to support ATeam's `reviewed-by:` / `Reviewed at <sha>` / `Verdict:` (and `### Independent Cross-Agent Review`) PR comment records and `Review request` comments while preserving historical `<!-- semteams-review-* -->` parsing.
- [x] 3.2 Add focused Vitest unit tests in `ui/src/lib/server/work/githubSource.test.ts` covering ATeam `APPROVE` and `REQUEST CHANGES` records, fenced-example immunity, multi-SHA ambiguity rejection, chronological superseding, and legacy `<!-- semteams-review-* -->` records.

## 4. Verification, upstream bug reporting, and specialist review

- [x] 4.1 Verify live `ateam doctor --check`, `ateam mcp` (`ateam_state`, `ateam_task`), hook execution, and `ateam review compare` / `gate` at `v0.3.2` (`45b6db8`), and file verified upstream brownfield/runtime bugs on `C360Studio/ateam` (`#204`, `#205`, `#206`, `#207`, plus evidence comments on `#193` and `#201`).
- [x] 4.2 Run `go-reviewer` and `svelte-reviewer` audits and pass `task validate` on a clean worktree.
