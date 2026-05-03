// Package main implements the SemTeams builder sandbox — a per-task
// workspace service for the dev-via-spec builder agent (R3.6, ADR-032).
//
// Wire format conforms to upstream `processor/agentic-tools/sandbox.Client`
// (semstreams beta.36) for the subset the BashExecutor route needs:
// CreateWorktree (POST /worktree), DeleteWorktree, Exec, ReadFile,
// WriteFile. ListDir, Search, GitStatus, GitCommit, GitDiff are
// intentionally NOT exposed — bash subsumes them, and the LLM-visible
// tool surface stays minimal per the "fewer rich tools" principle.
// One product-shell extension: GET /worktree/{taskID}/archive returns
// a zip for forensics (ADR-032 §10).
//
// Posture: standalone binary mirroring semdragon's cmd/sandbox/ shape.
// Runs in its own hardened container (cap_drop ALL + minimal cap_add,
// no-new-privileges, read-only root, tmpfs); no NATS dependency.
//
// Simplifications vs semdragon (per ADR-032 Decided rows):
//   - No repo-backed workspaces / git worktrees — bare-seed posture
//     (Decision #17) means workspaces are fresh scratch dirs; the
//     builder writes pom.xml, OSGi metadata, and source from spec.
//   - No merge-to-main endpoint — serial chain has no shared-main
//     merge concept (Decision #4).
//   - No orphan-collector cron — explicit DELETE only (Decision #9).
//
// R3.6.1 phasing (this package grows across slices):
//   - .a (this slice): workspace lifecycle (POST/GET-zip/DELETE)
//   - .b: file ops + structural path confinement
//   - .c: exec + toolchain image + shared cache volume
//   - .d: egress filter + per-workspace disk quota
//
// See docs/adr/032-r36-sandbox-design.md for the full design.
package main
