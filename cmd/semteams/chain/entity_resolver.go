package chain

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/c360studio/semstreams/agentic"
	"github.com/c360studio/semstreams/message"
	"github.com/c360studio/semstreams/types"
)

// ChainEntityRoleKey is the related_loops key spawn rules set to the
// chain entity's 6-part ID. Single source of truth — tool packages
// reference this rather than re-declaring the literal so the rule
// pack contract stays grep-able from one constant.
const ChainEntityRoleKey = "chain-entity-id"

// IDResolver is the narrow surface ResolveChainEntityID needs to
// walk loop_id → chain_id when related_loops didn't pin the chain
// entity. *Resolver in this package satisfies it; tests inject fakes.
type IDResolver interface {
	ChainID(ctx context.Context, loopID string) (string, error)
}

// ResolveChainEntityID derives the chain entity's 6-part ID for a
// tool call. Three sources, in order of preference:
//
//  1. related_loops[ChainEntityRoleKey] — set by the rule pack when
//     the spawn rule explicitly names the chain entity. Highest
//     precedence; no log, no resolver hop, deterministic.
//
//  2. resolver.ChainID(call.LoopID) → construct
//     `{org}.{platform}.agent.chain.execution.{chainID}`. Used when
//     the tool is invoked outside the rule pack (smoke tests, isolated
//     coordinator calls). Logged at WARN so operators see the gap
//     during rule-pack bring-up — a typoed related_loops key would
//     silently land here forever otherwise.
//
//  3. None — return error. Callers requiring a chain entity (triple
//     stamping, attestation routing) cannot proceed.
//
// resolver may be nil; in that case sources 2+ are unavailable and
// the function returns an error if related_loops is missing.
//
// logger may be nil; nil means "no fallback-warn." Production
// callers should provide one so the bring-up gap is observable.
//
// Extracted from per-tool chainEntityFromCall implementations
// (requestsandbox, querysandboxattestation) once a third consumer
// (sandboxruntime.AttestationRunner) needed the same path.
// Errors are returned unprefixed; callers wrap with their tool name
// for log context.
func ResolveChainEntityID(ctx context.Context, call agentic.ToolCall, resolver IDResolver, platform types.PlatformMeta, logger *slog.Logger) (string, error) {
	if related, ok := call.Metadata[agentic.MetadataKeyRelatedLoops].(map[string]any); ok {
		if v, ok := related[ChainEntityRoleKey].(string); ok && v != "" {
			return v, nil
		}
	}
	if resolver == nil {
		return "", fmt.Errorf("related_loops[%q] missing AND no IDResolver wired (chain entity required)", ChainEntityRoleKey)
	}
	if logger != nil {
		logger.Warn("ResolveChainEntityID: related_loops chain-entity-id missing; using chain.Resolver fallback",
			slog.String("loop_id", call.LoopID),
			slog.String("related_key", ChainEntityRoleKey))
	}
	if call.LoopID == "" {
		return "", fmt.Errorf("related_loops[%q] missing AND tool call has no loop_id (resolver fallback unavailable)", ChainEntityRoleKey)
	}
	chainID, err := resolver.ChainID(ctx, call.LoopID)
	if err != nil {
		return "", fmt.Errorf("resolve chain id from loop %q: %w", call.LoopID, err)
	}
	if chainID == "" {
		return "", fmt.Errorf("chain.Resolver returned empty chain id for loop %q", call.LoopID)
	}
	entityID := fmt.Sprintf("%s.%s.agent.chain.execution.%s", platform.Org, platform.Platform, chainID)
	if !message.IsValidEntityID(entityID) {
		return "", fmt.Errorf("constructed chain entity id %q failed IsValidEntityID", entityID)
	}
	return entityID, nil
}
