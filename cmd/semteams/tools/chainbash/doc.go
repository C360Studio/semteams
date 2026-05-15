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
//  1. The framework calls Execute with call.LoopID populated from the
//     active loop's loop_id.
//  2. We resolve loop_id → chain_id via chain.Resolver (walks
//     agent.loop.parent triples back to the chain root).
//  3. We inject Metadata["task_id"] = chain_id on a shallow copy of
//     the ToolCall. Upstream's BashExecutor prefers Metadata["task_id"]
//     over Metadata["loop_id"] when picking the sandbox bucket, so
//     every role in the same chain talks to the same worktree.
//  4. We delegate to upstream's BashExecutor unchanged.
//
// Fail-soft: a chain resolution failure (no parent triples yet, NATS
// timeout, etc.) does not abort the call. We log at Debug and let
// upstream fall back to loop_id, preserving behaviour for non-chain
// invocations and during graceful-degradation paths.
//
// Naming: the wrapper is registered under the canonical "bash" name.
// LLMs are trained on the `bash` token, and renaming risks behavioural
// drift (see memory note feedback_tool_names_match_training_data).
// Upstream beta.72 ships the SkipBuiltins API, which omits the
// framework's BashExecutor from RegisterBuiltins and leaves the slot
// free for the wrapper to claim.
package chainbash
