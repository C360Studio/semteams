package chainbash

import (
	"context"
	"fmt"
	"log/slog"
	"maps"

	"github.com/c360studio/semstreams/agentic"
	"github.com/c360studio/semstreams/types"

	"github.com/c360studio/semteams/cmd/semteams/runanchor"
)

// ToolName is the canonical LLM-facing name. The wrapper registers
// under "bash" after upstream's RegisterBuiltins is told to SkipBuiltins
// the framework bash executor.
const ToolName = "bash"

// MetadataKeyTaskID is the upstream BashExecutor's sandbox-bucket key
// (see semstreams executors/bash.go:116-123). When present and non-empty,
// upstream uses it instead of Metadata["loop_id"]; setting it to
// chain_id pins every role in the chain to one worktree.
const MetadataKeyTaskID = "task_id"

// Inner is the wrapped bash executor. Upstream's *executors.BashExecutor
// satisfies it; tests inject fakes that record the rewritten ToolCall so
// we can assert task_id was rewritten before delegation.
type Inner interface {
	Execute(ctx context.Context, call agentic.ToolCall) (agentic.ToolResult, error)
	ListTools() []agentic.ToolDefinition
}

// Executor wraps Inner with chain-scoped task_id rewriting. The chain anchor
// (the run's bare loop-id = chain root) arrives on ToolCall.Metadata, stamped by
// agentic-loop dispatch (semstreams#250, beta.105) — the wrapper reads it via
// runanchor.Anchor instead of the retired ancestry-walk Resolver (ADR-053 Phase 5).
type Executor struct {
	inner    Inner
	platform types.PlatformMeta
	logger   *slog.Logger
}

// NewExecutor constructs the wrapper. inner is upstream's BashExecutor (or a
// test fake satisfying Inner) and must be non-nil. platform supplies org/platform
// for the run-anchor read (the 6-part reconstruction fallback). nil logger
// defaults to slog.Default.
func NewExecutor(inner Inner, platform types.PlatformMeta, logger *slog.Logger) *Executor {
	if logger == nil {
		logger = slog.Default()
	}
	return &Executor{
		inner:    inner,
		platform: platform,
		logger:   logger,
	}
}

// ListTools delegates to the inner executor. The LLM-facing schema
// (description, parameters) is upstream's — we don't restate it here so
// schema drift between framework and product cannot happen.
func (e *Executor) ListTools() []agentic.ToolDefinition {
	return e.inner.ListTools()
}

// Execute reads the run anchor off ToolCall.Metadata (the run's bare loop-id =
// chain root, stamped by dispatch), injects it as Metadata["task_id"] on a
// shallow copy, and delegates. The task_id is the BARE run id, NOT the 6-part
// entity id: upstream BashExecutor feeds task_id straight to the sandbox as a
// worktree dir name, and the AttestationRunner reconstructs the 6-part entity by
// prepending the prefix — a dotted entity id would double-prefix.
//
// Fail-soft when there is no run anchor (standalone loop / pre-#250 framework):
// upstream falls back to Metadata["loop_id"], keeping the existing per-loop
// behaviour. This subsumes both the old resolve-error path and the no-loop path.
func (e *Executor) Execute(ctx context.Context, call agentic.ToolCall) (agentic.ToolResult, error) {
	// Defence in depth: registry routes by name, but a misrouted call
	// (config typo, registry bug) should produce a clean tool error
	// rather than silently rewriting metadata on the wrong executor.
	if call.Name != ToolName {
		return agentic.ToolResult{
			CallID:    call.ID,
			Name:      call.Name,
			Error:     fmt.Sprintf("chainbash: unexpected tool name %q (wrapper registered as %q)", call.Name, ToolName),
			ErrorKind: agentic.ToolErrorNotFound,
		}, nil
	}

	runID, _ := runanchor.Anchor(call, e.platform.Org, e.platform.Platform)
	if runID == "" {
		// Not part of a run (standalone loop) or a pre-#250 framework — no
		// chain anchor to pin. Delegate as-is; upstream's task_id fallback to
		// loop_id keeps the command useful.
		return e.inner.Execute(ctx, call)
	}
	if runID == loopIDFromCall(call) {
		// The call's loop IS the chain/run root; loop_id == chain_id, so no
		// rewrite is needed. Delegating unchanged saves an allocation on the
		// hot path (every chain-root bash call).
		return e.inner.Execute(ctx, call)
	}

	rewritten := withTaskID(call, runID)
	return e.inner.Execute(ctx, rewritten)
}

// loopIDFromCall picks loop_id off the typed field first (the framework
// sets it on every dispatched call) then falls back to Metadata for
// callers that only thread it through metadata. Empty string when
// neither is present.
func loopIDFromCall(call agentic.ToolCall) string {
	if call.LoopID != "" {
		return call.LoopID
	}
	if call.Metadata != nil {
		if v, ok := call.Metadata["loop_id"].(string); ok {
			return v
		}
	}
	return ""
}

// withTaskID returns a shallow copy of call with Metadata["task_id"]
// set to chainID. The original ToolCall is not mutated — important
// because the framework reuses ToolCall structs across log sinks and
// retry paths. Metadata map is cloned to avoid aliasing the caller's
// map: upstream's BashExecutor only reads, but other tools in the same
// dispatch step may read concurrently from the original.
func withTaskID(call agentic.ToolCall, chainID string) agentic.ToolCall {
	clonedMeta := make(map[string]any, len(call.Metadata)+1)
	maps.Copy(clonedMeta, call.Metadata)
	clonedMeta[MetadataKeyTaskID] = chainID
	call.Metadata = clonedMeta
	return call
}
