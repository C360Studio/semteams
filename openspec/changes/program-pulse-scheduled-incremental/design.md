# Design: Scheduled, Incremental, and Replay-Safe Program Pulse (#268)

## Context

Under ADR-060 (`#267`, `#272`, `#273`), Program Pulse runs as an operator profile (`/pulse`) of the existing `research` pack (`coordinator → researcher-research-plan → researcher-research-gather ×1..4 → researcher-research-synthesize → reviewer-research → coordinator`) using `github_read` for read-only portfolio evidence. Issue `#268` requires Program Pulse to support:
1. Scheduled execution via the existing `research` pack without introducing a duplicate `program-report` pack or role set, with overlap/cooldown protection and Pattern-B runtime enable/disable (`RULE_CONFIG` KV).
2. Incremental evidence reads where `github_read` distinguishes initial repository onboarding (`baseline: true`) from subsequent delta reads, and distinguishes `LastCheckedAt` from `LastPersistedAt` so no-change runs (`no_change: true`) do not overwrite `Watermark` or `LastPersistedAt`.
3. Replay safety where cursors do not permanently advance on a failed run before synthesis persists the pulse artifact (`emit_research_artifact`), and where a retried/replayed `github_read` within the same run observes the same pre-run baseline cursor.
4. Partial-repository failure resilience where one unavailable repository in a multi-repository pulse does not abort healthy repository reads, leaves the failed repository's cursor unadvanced, and is mechanically required to be disclosed in `artifact.OpenGaps`.

## Decisions

### 1. Run-Scoped Cursor Staging and Commit on `emit_research_artifact`

`githubread.CursorEntry` is extended with:
- `Watermark time.Time` (`json:"watermark"`)
- `Seen map[string]string` (`json:"seen,omitempty"`)
- `BaselineEstablished bool` (`json:"baseline_established,omitempty"`)
- `LastCheckedAt time.Time` (`json:"last_checked_at,omitempty"`)
- `LastPersistedAt time.Time` (`json:"last_persisted_at,omitempty"`)

`githubread.CursorStore` (`MemoryCursorStore` and `FileCursorStore`) adds run-scoped staging methods:
- `GetForRun(runID, repoKey, kind string) CursorEntry`: returns the committed `CursorEntry` snapshot captured before any staging in `runID` (so a retry or second `github_read` call within the same `runID` reads against the exact same pre-run baseline cursor and returns identical delta records).
- `Stage(runID, repoKey, kind string, next CursorEntry, hasNewRecords bool) error`:
  - When `runID == ""` (standalone calls without a run anchor), commits immediately (`BaselineEstablished = true`, `LastCheckedAt = next.LastCheckedAt`, and if `hasNewRecords || prev.LastPersistedAt.IsZero()` updates `Watermark`, `Seen`, and `LastPersistedAt`).
  - When `runID != ""`, records the committed `LastCheckedAt` immediately (since the check occurred) and stages `next` under `runID` without mutating the committed `Watermark`, `Seen`, `BaselineEstablished`, or `LastPersistedAt`.
- `CommitRun(runID string, committedAt time.Time) error`: promotes all staged `(repoKey, kind)` entries for `runID` into the committed store. For each staged entry:
  - Sets `BaselineEstablished = true`.
  - If the staged entry observed new/updated records (or established the initial baseline), promotes `Seen`, `Watermark`, and sets `LastPersistedAt = committedAt.UTC()`.
  - If the staged entry was a no-change check (`!hasNewRecords` on an already-baselined entry), preserves the prior committed `Watermark` and `LastPersistedAt` while keeping `LastCheckedAt`.
  - Clears the staged buffer for `runID` (idempotent if `CommitRun` is called again for the same `runID`).

How `runID` is resolved:
- `githubread.Executor.Execute` resolves the run key via `runanchor.Anchor(call, "", "")` (extracting `agent.run_id` / `agent.run_entity_id` from `call.Metadata`, or falling back to `call.Metadata["pulse_run_id"]`).
- In `cmd/semteams/product_tools.go`, `registerProductTools` constructs the shared `githubread.CursorStore` and wires it into both `githubread.Executor` and `emitartifact.Executor` (`SetCursorCommitter`).
- When `emitartifact.Executor.Execute` succeeds (after `VerifyDerivation`, markdown rendering, triple append, and NATS payload publish), it invokes `CommitRun` for the resolved caller run ID (and any loop IDs resolved during derivation).

### 2. Partial-Repository Failure Disclosure

In `githubread.Executor.Execute`:
- Each `(repo, kind)` pair is fetched independently.
- If `fetchKind` fails for `(repo, kind)`, `Execute` appends a `RepositoryFailure{Repository: repoKey, Kind: kind, Error: err.Error(), ErrorKind: string(errKind), StatusCode: statusCode, RateLimited: rateLimited}` to `Result.Failures`, skips cursor staging/advancement for that `(repo, kind)`, and continues fetching the remaining `(repo, kind)` pairs.
- Only when *all* `(repo, kind)` pairs fail (zero successful `(repo, kind)` fetches) does `Execute` return a failed `agentic.ToolResult{Error: ..., ErrorKind: ...}`. When at least one `(repo, kind)` succeeds, `Execute` returns a normal JSON `Result` containing both `Records` (from healthy repositories) and `Failures` (for unavailable repositories).

In `emitartifact.VerifyDerivation`:
- While inspecting resolved tool results cited by `artifact.Actors` and `artifact.IntegrationPoints`, `VerifyDerivation` also checks if any cited `github_read` JSON envelope contains non-empty `failures`.
- For every `failure` in `failures`, `VerifyDerivation` requires at least one entry in `artifact.OpenGaps` to mention `failure.Repository` (case-insensitive substring match). If any failed repository from a cited `github_read` tool result is omitted from `artifact.OpenGaps`, `VerifyDerivation` returns a `ToolErrorInvalidArgs` `DerivationError` instructing the synthesizer to disclose the unavailable repository in `open_gaps`.

### 3. Scheduled Program Pulse Cron Rule (`10-scheduled-program-pulse.json`)

We add `configs/rules/research/10-scheduled-program-pulse.json`:
- `id`: `"scheduled_program_pulse"`
- `type`: `"cron"`
- `enabled`: `false` (default disabled in `configs/flow-bootstrap.json` and `configs/e2e-flow-bootstrap.json`; operators enable or adjust schedule at runtime via Pattern-B `RULE_CONFIG` KV without process restart)
- `schedule`: `"0 9 * * *"`
- `cooldown`: `"20h"`
- `actions`: single `publish_agent` action with `run_scope: "new"`, `subject: "agent.task.coordinator"`, `role: "coordinator"`, `loop_max_iterations: "8"`, `model: "coordinator"`, `tools: ["decide"]`, `tool_choice: {"mode": "required"}`, `action_allowlist: ["research", "respond_direct"]`, and an `@pulse` prompt referencing `$schedule.last_fired_at` so the coordinator routes directly into the existing `research` pack (`01-coordinator-research-spawn.json`).
