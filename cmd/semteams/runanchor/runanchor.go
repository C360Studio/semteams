// Package runanchor reads the run anchor that agentic-loop dispatch stamps onto
// every run-attached ToolCall (semstreams#250, beta.105). It replaces the
// hand-rolled cmd/semteams/chain ancestry-walk Resolver (ADR-053 Phase 5): the
// framework now resolves the run/chain entity at dispatch and carries it on
// ToolCall.Metadata, so a tool executor reads it directly instead of walking
// agent.loop.parent triples over graph.query.entity.
//
// It has zero cmd/semteams/chain dependency (reads agentic.ToolCall.Metadata +
// agentic.TryChainExecutionEntityID) and lives in its own package so the tool
// subpackages AND product_tools.go (package main) can share one copy.
package runanchor

import (
	"fmt"

	"github.com/c360studio/semstreams/agentic"
)

// ChainEntityRoleKey is the related_loops key a spawn rule sets to the chain
// entity's 6-part ID. When present it pins the chain entity explicitly (highest
// precedence, deterministic), ahead of the framework run anchor.
const ChainEntityRoleKey = "chain-entity-id"

// Anchor returns the run anchor stamped on a ToolCall by agentic-loop dispatch
// (MetadataKeyRunID = the bare run loop-id / chain root; MetadataKeyRunEntityID =
// the resolved 6-part org.platform.agent.chain.execution.<runID>).
//
// Returns ("", "") for a standalone loop (not part of a run) or a pre-#250
// framework — callers fail soft on that, exactly as they did on a Resolver miss.
// When only RunID is present (the framework lacked a platform identity at
// dispatch) it reconstructs the 6-part entity from the caller's org/platform via
// the non-panicking TryChainExecutionEntityID.
func Anchor(call agentic.ToolCall, org, platform string) (runID, runEntityID string) {
	m := call.Metadata
	if m == nil {
		return "", ""
	}
	runID, _ = m[agentic.MetadataKeyRunID].(string)
	runEntityID, _ = m[agentic.MetadataKeyRunEntityID].(string)
	if runEntityID == "" && runID != "" {
		if id, err := agentic.TryChainExecutionEntityID(org, platform, runID); err == nil {
			runEntityID = id
		}
	}
	return runID, runEntityID
}

// ChainEntityID resolves the 6-part chain entity ID a tool call belongs to, in
// precedence order: (1) related_loops[ChainEntityRoleKey] — pinned by the spawn
// rule, deterministic; (2) the dispatch-stamped run anchor (Anchor's
// runEntityID). For a run-scope=new chain the run entity IS the chain entity, so
// both sources converge. Returns an error when neither is present (a standalone
// loop, or a pre-#250 framework with no related_loops pin) — the caller cannot
// route to a chain entity and must fail soft.
//
// Replaces the retired chain.ResolveChainEntityID (ADR-053 Phase 5): the Resolver
// ancestry-walk fallback is gone; the run anchor on ToolCall.Metadata is it.
func ChainEntityID(call agentic.ToolCall, org, platform string) (string, error) {
	if related, ok := call.Metadata[agentic.MetadataKeyRelatedLoops].(map[string]any); ok {
		if v, ok := related[ChainEntityRoleKey].(string); ok && v != "" {
			return v, nil
		}
	}
	if _, runEntityID := Anchor(call, org, platform); runEntityID != "" {
		return runEntityID, nil
	}
	return "", fmt.Errorf("runanchor.ChainEntityID: related_loops[%q] missing AND no run anchor on ToolCall.Metadata (not part of a run)", ChainEntityRoleKey)
}
