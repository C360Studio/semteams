// Package chainbash provides a chain-scoped wrapper around upstream's
// bash executor.
//
// Why this exists (ADR-041 Phase 4): non-builder roles (architect,
// reviewer-spec, reviewer-qa, reviewer-research) need to read artifact
// markdown and inspect builder workspace state via bash to do their jobs
// well — "document grader" reviewers miss transcription errors and
// fabricated references. Upstream's BashExecutor routes commands to a
// sandbox keyed by the caller's loop_id, so each role-loop sees a
// different empty worktree. The fix is to scope worktree identity to
// the *chain* rather than each loop: one worktree per dispatch, shared
// across every role-loop in the chain.
//
// How the wrapper works:
//
//  1. The framework calls Execute with the run anchor stamped on
//     ToolCall.Metadata by agentic-loop dispatch (MetadataKeyRunID = the
//     run's bare loop-id = chain root; semstreams#250, beta.105).
//  2. We read it via runanchor.Anchor — no NATS ancestry walk. (This
//     replaced the hand-rolled chain.Resolver loop_id→chain_id walk in
//     ADR-053 Phase 5; the framework now resolves the run at dispatch.)
//  3. We inject Metadata["task_id"] = run_id (the BARE id, not the 6-part
//     entity id — the sandbox uses it as a worktree dir name and the
//     AttestationRunner prepends the prefix) on a shallow copy of the
//     ToolCall. Upstream's BashExecutor prefers Metadata["task_id"] over
//     Metadata["loop_id"] when picking the sandbox bucket, so every role
//     in the same chain talks to the same worktree.
//  4. We delegate to upstream's BashExecutor unchanged.
//
// Fail-soft: no run anchor (standalone loop / pre-#250 framework) skips
// the rewrite — upstream falls back to loop_id, preserving behaviour for
// non-chain invocations.
//
// Naming: the wrapper is registered under the canonical "bash" name.
// LLMs are trained on the `bash` token, and renaming risks behavioural
// drift (see memory note feedback_tool_names_match_training_data).
// Upstream beta.72 ships the SkipBuiltins API, which omits the
// framework's BashExecutor from RegisterBuiltins and leaves the slot
// free for the wrapper to claim.
package chainbash
