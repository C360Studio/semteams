package chainbash

import (
	"context"
	"fmt"
	"log/slog"
	"maps"

	"github.com/c360studio/semstreams/agentic"
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

// ChainResolver narrows chain.Resolver to the single method the wrapper
// uses. Keeps tests free of NATS plumbing — fakes implement ChainID
// directly. Production passes *chain.Resolver.
type ChainResolver interface {
	ChainID(ctx context.Context, loopID string) (string, error)
}

// Inner is the wrapped bash executor. Upstream's *executors.BashExecutor
// satisfies it; tests inject fakes that record the rewritten ToolCall so
// we can assert task_id was rewritten before delegation.
type Inner interface {
	Execute(ctx context.Context, call agentic.ToolCall) (agentic.ToolResult, error)
	ListTools() []agentic.ToolDefinition
}

// Executor wraps Inner with chain-scoped task_id rewriting.
type Executor struct {
	inner    Inner
	resolver ChainResolver
	logger   *slog.Logger
}

// NewExecutor constructs the wrapper. inner is upstream's BashExecutor
// (or a test fake satisfying Inner) and must be non-nil. resolver is
// expected to be non-nil in production — chain_id resolution is the
// whole point of the wrapper — but the wrapper does not nil-check it;
// callers without NATS pass identityChainResolver (see
// product_tools.go) so every call cleanly degrades to "loop is the
// chain root". nil logger defaults to slog.Default.
func NewExecutor(inner Inner, resolver ChainResolver, logger *slog.Logger) *Executor {
	if logger == nil {
		logger = slog.Default()
	}
	return &Executor{
		inner:    inner,
		resolver: resolver,
		logger:   logger,
	}
}

// ListTools delegates to the inner executor. The LLM-facing schema
// (description, parameters) is upstream's — we don't restate it here so
// schema drift between framework and product cannot happen.
func (e *Executor) ListTools() []agentic.ToolDefinition {
	return e.inner.ListTools()
}

// Execute resolves call.LoopID → chain_id, injects Metadata["task_id"]
// on a shallow copy, and delegates. Fail-soft on resolve error: upstream
// falls back to Metadata["loop_id"] which keeps the existing per-loop
// behaviour for non-chain or pre-stamped-parent calls.
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
	loopID := loopIDFromCall(call)
	if loopID == "" {
		// No loop context at all — nothing to resolve. Delegate as-is;
		// upstream BashExecutor's fallback ("default" task) handles it.
		return e.inner.Execute(ctx, call)
	}

	chainID, err := e.resolver.ChainID(ctx, loopID)
	if err != nil {
		// Ancestry walk failed (typical causes: parent triple not yet
		// stamped, NATS timeout, malformed entity ID). Don't abort the
		// tool call — upstream's task_id fallback to loop_id keeps the
		// command useful, and the operator can debug via the log.
		e.logger.Warn("chainbash: ChainID resolve failed; falling back to loop_id (chain isolation lost for this call)",
			slog.String("loop_id", loopID),
			slog.String("error", err.Error()))
		return e.inner.Execute(ctx, call)
	}
	if chainID == "" || chainID == loopID {
		// Loop is itself the chain root (no parent walk), or resolver
		// returned empty. Either way, loop_id == chain_id; no rewrite
		// needed. Delegating unchanged saves an allocation on the hot
		// path (every chain-root bash call).
		return e.inner.Execute(ctx, call)
	}

	rewritten := withTaskID(call, chainID)
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
