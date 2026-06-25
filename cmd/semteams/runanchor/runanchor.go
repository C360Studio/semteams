// Package runanchor reads the run anchor that agentic-loop dispatch stamps onto
// every run-attached ToolCall (semstreams#250, beta.105). It replaces the
// hand-rolled cmd/semteams/chain ancestry-walk Resolver (ADR-053 Phase 5): the
// framework now resolves the run/chain entity at dispatch and carries it on
// ToolCall.Metadata, so a tool executor reads it directly instead of walking
// agent.loop.parent triples over graph.query.entity.
//
// It has zero cmd/semteams/chain dependency (reads agentic.ToolCall.Metadata +
// agentic entity-ID helpers) and lives in its own package so the tool
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

// FallbackLoopID returns the loop_id carried by front-door standalone tool
// calls. beta.115 does not stamp agent.run_* metadata on a top-level
// coordinator loop before it has selected a specialist run; those calls still
// need a stable, already-born graph entity for preflight tools such as
// request_sandbox.
func FallbackLoopID(call agentic.ToolCall) string {
	if call.LoopID != "" {
		return call.LoopID
	}
	if call.Metadata == nil {
		return ""
	}
	if loopID, ok := call.Metadata["loop_id"].(string); ok {
		return loopID
	}
	return ""
}

// ChainEntityID resolves the attestation target graph entity ID, in
// precedence order: (1) related_loops[ChainEntityRoleKey] — pinned by the spawn
// rule, deterministic; (2) the dispatch-stamped run anchor (Anchor's
// runEntityID); (3) the tool call's loop_id as a front-door coordinator
// fallback. For a run-scope=new chain the run entity IS the chain entity, so
// the first two sources converge. The loop_id fallback intentionally targets
// the existing agentic-loop execution entity because the coordinator may need
// pre-routing facts before any chain entity has been minted.
// Returns an error when none is present — the caller cannot route to a stable
// graph entity and must fail soft.
//
// Replaces the retired chain.ResolveChainEntityID (ADR-053 Phase 5): the Resolver
// ancestry-walk fallback is gone; the run anchor on ToolCall.Metadata is it.
func ChainEntityID(call agentic.ToolCall, org, platform string) (string, error) {
	if v := relatedLoopChainEntityID(call.Metadata); v != "" {
		return v, nil
	}
	runID, runEntityID := Anchor(call, org, platform)
	if runEntityID != "" {
		return runEntityID, nil
	}
	if runID != "" || hasRunAnchorKey(call.Metadata) {
		return "", fmt.Errorf("runanchor.ChainEntityID: run anchor present on ToolCall.Metadata but no usable chain entity resolved")
	}
	if loopID := FallbackLoopID(call); loopID != "" {
		if id, err := agentic.TryLoopExecutionEntityID(org, platform, loopID); err == nil {
			return id, nil
		}
	}
	return "", fmt.Errorf("runanchor.ChainEntityID: related_loops[%q] missing, no run anchor on ToolCall.Metadata, and no usable loop_id fallback", ChainEntityRoleKey)
}

func relatedLoopChainEntityID(metadata map[string]any) string {
	switch related := metadata[agentic.MetadataKeyRelatedLoops].(type) {
	case map[string]any:
		if v, ok := related[ChainEntityRoleKey].(string); ok {
			return v
		}
	case map[string]string:
		return related[ChainEntityRoleKey]
	}
	return ""
}

func hasRunAnchorKey(metadata map[string]any) bool {
	if metadata == nil {
		return false
	}
	if _, ok := metadata[agentic.MetadataKeyRunID]; ok {
		return true
	}
	if _, ok := metadata[agentic.MetadataKeyRunEntityID]; ok {
		return true
	}
	return false
}
