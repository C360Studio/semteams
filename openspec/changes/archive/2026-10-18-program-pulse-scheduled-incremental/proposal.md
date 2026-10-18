# Proposal: Make Program Pulse Scheduled, Incremental, and Replay-Safe (#268)

## Why

Issue `#267` delivered on-demand `/pulse` Program Pulse reporting over the `research` pack and `github_read` (`#273`), and `#272` established the loop, fan-out (`1..4`), and cron inflight/cooldown/kill-switch containment budget. However, `github_read` still advances its per-`(repo, kind)` cursor immediately during gather before synthesis/derivation succeeds, conflates initial onboarding with incremental delta checks, does not distinguish `LastCheckedAt` from `LastPersistedAt` on no-change runs, aborts multi-repository reads when one repository is unavailable, and lacks a wired scheduled cron rule (`10-scheduled-program-pulse.json`) in the bootstrap configs.

Issue `#268` is the final `v0.3.0` milestone slice that turns Program Pulse into a scheduled, incremental, and replay-safe capability without introducing a separate `program-report` pack or custom workflow engine.

## What Changes

1. **Run-Scoped Cursor Staging, Commit-on-Persist, Baseline Onboarding, and No-Change Timestamps (`cmd/semteams/tools/githubread`, `cmd/semteams/tools/emitartifact`, `cmd/semteams/product_tools.go`):**
   - Extend `githubread.CursorEntry` with `BaselineEstablished bool`, `LastCheckedAt time.Time`, and `LastPersistedAt time.Time`.
   - Add run-scoped staging to `githubread.CursorStore` (`MemoryCursorStore` and `FileCursorStore`) keyed by the active research run ID (or loop ID fallback when running inside a run):
     - On a run's first `github_read` for `(repo, kind)`, stage the candidate cursor against the committed pre-run baseline cursor so a retry/replay within the same run observes the identical pre-run baseline and returns the exact same delta records idempotently.
     - If a run fails before `emit_research_artifact` succeeds, the staged cursor is never promoted to the committed `CursorEntry`, preserving the delta for the next run.
     - Wire a `CursorCommitter` onto `emitartifact.Executor` from `registerProductTools` so `emit_research_artifact` commits staged cursors for the run (and discovered gather loops) only after `VerifyDerivation`, markdown rendering, and triple/payload publication succeed.
     - When `github_read` is invoked without a run anchor (`runID == ""` in standalone unit tests), preserve immediate commit semantics for backward compatibility while still tracking `BaselineEstablished`, `LastCheckedAt`, and `LastPersistedAt`.
     - Distinguish first-run onboarding (`Result.Baseline = true` when `!prev.BaselineEstablished && len(prev.Seen) == 0`) from subsequent incremental delta runs (`Result.Baseline = false`), and distinguish no-change runs (`Result.NoChange = true` when `new_count == 0`, advancing `LastCheckedAt` while keeping `Watermark` and `LastPersistedAt` unchanged).

2. **Partial-Repository Failure Tolerance and Mandatory `open_gaps` Disclosure (`cmd/semteams/tools/githubread`, `cmd/semteams/tools/emitartifact/derivation.go`):**
   - Update `githubread.Executor.Execute` so a fetch failure on one `(repo, kind)` in a multi-repository/multi-kind call records a structured `RepositoryFailure` entry on `Result.Failures` without advancing that `(repo, kind)` cursor, while continuing to collect evidence from healthy repositories/kinds. Return a top-level `ToolResult.Error` only when zero `(repo, kind)` targets succeed.
   - Update `emitartifact.VerifyDerivation` so that whenever a cited `github_read` tool output contains non-empty `Failures`, every failed repository (`owner/name`) must be explicitly disclosed in `artifact.OpenGaps` (returning `ToolErrorInvalidArgs` if omitted).

3. **Scheduled Program Pulse Cron Rule Wiring (`configs/rules/research/10-scheduled-program-pulse.json`, `configs/flow-bootstrap.json`, `configs/e2e-flow-bootstrap.json`):**
   - Author `configs/rules/research/10-scheduled-program-pulse.json` (`id: "scheduled_program_pulse"`, `type: "cron"`, `schedule: "0 9 * * *"`, `cooldown: "20h"`, `enabled: false`, `loop_max_iterations: "8"`, spawning `coordinator` with an `@pulse` incremental portfolio synthesis prompt).
   - Wire `/app/configs/rules/research/10-scheduled-program-pulse.json` into `rules_files` in both `configs/flow-bootstrap.json` and `configs/e2e-flow-bootstrap.json`, keeping `enabled: false` by default and toggling live via Pattern-B `RULE_CONFIG` KV (`rule.ConfigManager.SaveRule`).

4. **Personas, Documentation, and Verification (`configs/personas/`, `docs/product/program-manager.md`, `AGENTS.md`, tests):**
   - Update `researcher-research-gather` and `researcher-research-synthesize` persona fragments to guide baseline vs. incremental delta vs. no-change reporting and partial-repository `open_gaps` disclosure.
   - Update `AGENTS.md`, `configs/rules/research/README.md`, and `docs/product/program-manager.md` to reflect shipped `#268` scheduled/incremental/replay-safe Program Pulse behavior.
   - Add unit, contract (with paired sensitivity tests), and integration tests covering initial baseline onboarding, no-change rerun, incremental update, run-scoped retry/replay before and after `CommitRun`, partial-repository failure disclosure, and live scheduled cron enable/cooldown/kill-switch behavior.

## Capabilities

- Modified: `program-pulse-github-read`
- Modified: `research-containment`

## Impact

- Closes `#268` (completing Milestone `v0.3.0`).
- Preserves existing single-pack `research` architecture and frozen SemStreams `v1.0.0-beta.162` runtime contracts.
