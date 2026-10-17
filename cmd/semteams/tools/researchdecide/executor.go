package researchdecide

import (
	"context"
	"fmt"
	"log/slog"
	"maps"
	"strings"

	"github.com/c360studio/semstreams/agentic"

	"github.com/c360studio/semteams/cmd/semteams/research"
)

// ToolName is the canonical LLM-facing tool name. Registered under "decide"
// after upstream's RegisterBuiltins is configured with SkipBuiltins=["bash", "decide"].
const ToolName = agentic.DecideToolName

// Inner is the wrapped decide executor. Upstream's *agentictools.DecideExecutor
// satisfies it; unit tests can inject a recording fake or a real DecideExecutor.
type Inner interface {
	Execute(ctx context.Context, call agentic.ToolCall) (agentic.ToolResult, error)
	ListTools() []agentic.ToolDefinition
}

// Executor wraps Inner with pre-mutation fan-out validation on subtopics
// (1..4 items per ADR-060 §7 / #272).
type Executor struct {
	inner             Inner
	restrictedActions map[string]struct{}
	logger            *slog.Logger
}

// NewExecutor constructs the decide wrapper. inner must be non-nil.
// restrictedActions mirrors the deployment's restricted_decide_actions list so
// restricted actions are delegated straight to inner (preserving upstream's
// rejection message and Prometheus counter) before any subtopics check runs.
func NewExecutor(inner Inner, restrictedActions []string, logger *slog.Logger) *Executor {
	if logger == nil {
		logger = slog.Default()
	}
	return &Executor{
		inner:             inner,
		restrictedActions: buildRestrictedSet(restrictedActions),
		logger:            logger,
	}
}

// ListTools clones the inner executor's tool definitions and annotates
// parameters.properties.subtopics with minItems=1 and maxItems=4. All other
// fields (including Description, which upstream tests guard against action
// example leakage) are preserved verbatim.
func (e *Executor) ListTools() []agentic.ToolDefinition {
	base := e.inner.ListTools()
	out := make([]agentic.ToolDefinition, len(base))
	for i, def := range base {
		cloned := def
		if def.Parameters != nil {
			paramsCopy := maps.Clone(def.Parameters)
			if props, ok := paramsCopy["properties"].(map[string]any); ok {
				propsCopy := maps.Clone(props)
				if sub, ok := propsCopy["subtopics"].(map[string]any); ok {
					subCopy := maps.Clone(sub)
					subCopy["minItems"] = research.MinSubtopics
					subCopy["maxItems"] = research.MaxSubtopics
					propsCopy["subtopics"] = subCopy
				}
				paramsCopy["properties"] = propsCopy
			}
			cloned.Parameters = paramsCopy
		}
		out[i] = cloned
	}
	return out
}

// Execute validates subtopics bounds (1..4 non-empty strings) before delegating
// to the inner DecideExecutor. On a bounds violation, Execute returns
// ToolErrorInvalidArgs with StopLoop=false without invoking inner.Execute,
// ensuring zero triples are published to graph-ingest and the calling loop
// can self-correct on its next turn.
func (e *Executor) Execute(ctx context.Context, call agentic.ToolCall) (agentic.ToolResult, error) {
	if call.Name != ToolName {
		return e.inner.Execute(ctx, call)
	}

	// Let upstream handle missing/wrong-typed action, reason, or retry_hint.
	rawAction, ok := call.Arguments["action"].(string)
	if !ok || rawAction == "" {
		return e.inner.Execute(ctx, call)
	}
	rawReason, ok := call.Arguments["reason"].(string)
	if !ok || rawReason == "" {
		return e.inner.Execute(ctx, call)
	}
	if hint, present := call.Arguments["retry_hint"]; present && hint != nil {
		if _, ok := hint.(string); !ok {
			return e.inner.Execute(ctx, call)
		}
	}

	// Resolve against per-spawn action_allowlist and deployment-level
	// restricted_decide_actions first. If off-allowlist or restricted,
	// delegate to inner so upstream emits its exact error and metrics.
	canonicalAction, allowed := resolveAllowedAction(call.Metadata, rawAction)
	if !allowed || e.isRestricted(canonicalAction) {
		return e.inner.Execute(ctx, call)
	}

	rawSubtopics, hasSubtopics := call.Arguments["subtopics"]
	isGather := normaliseAction(canonicalAction) == research.GatherAction

	if isGather && (!hasSubtopics || rawSubtopics == nil) {
		return e.rejectSubtopics(call, fmt.Sprintf(
			"decide(action=%q) requires subtopics with %d..%d non-empty items (got 0); supply 1..%d subtopics matching emit_plan epics",
			canonicalAction, research.MinSubtopics, research.MaxSubtopics, research.MaxSubtopics,
		)), nil
	}

	if hasSubtopics && rawSubtopics != nil {
		items, normalizedAny, ok := extractStringSlice(rawSubtopics)
		if !ok {
			// Wrong type (not an array, or non-string element) — delegate so
			// upstream returns its exact parseDecideArgs type error.
			return e.inner.Execute(ctx, call)
		}
		if len(items) < research.MinSubtopics || len(items) > research.MaxSubtopics {
			return e.rejectSubtopics(call, fmt.Sprintf(
				"subtopics count must be between %d and %d (got %d); consolidate or split subtopics to fit the %d..%d fan-out budget",
				research.MinSubtopics, research.MaxSubtopics, len(items), research.MinSubtopics, research.MaxSubtopics,
			)), nil
		}
		for i, item := range items {
			if strings.TrimSpace(item) == "" {
				return e.rejectSubtopics(call, fmt.Sprintf(
					"subtopics[%d] must be a non-empty string", i,
				)), nil
			}
		}
		// If caller supplied []string in-process, normalize to []any on a
		// cloned Arguments map so upstream parseDecideArgs accepts it.
		if normalizedAny != nil {
			clonedArgs := maps.Clone(call.Arguments)
			clonedArgs["subtopics"] = normalizedAny
			call.Arguments = clonedArgs
		}
	}

	return e.inner.Execute(ctx, call)
}

func (e *Executor) rejectSubtopics(call agentic.ToolCall, msg string) agentic.ToolResult {
	e.logger.Info("decide tool rejected subtopics outside fan-out containment bounds",
		slog.String("loop_id", call.LoopID),
		slog.String("error", msg))
	return agentic.ToolResult{
		CallID:    call.ID,
		Name:      call.Name,
		Error:     msg,
		ErrorKind: agentic.ToolErrorInvalidArgs,
		StopLoop:  false,
	}
}

func extractStringSlice(raw any) ([]string, []any, bool) {
	switch v := raw.(type) {
	case []any:
		out := make([]string, 0, len(v))
		for _, elem := range v {
			s, ok := elem.(string)
			if !ok {
				return nil, nil, false
			}
			out = append(out, s)
		}
		return out, nil, true
	case []string:
		out := make([]string, len(v))
		copy(out, v)
		asAny := make([]any, len(v))
		for i, s := range v {
			asAny[i] = s
		}
		return out, asAny, true
	default:
		return nil, nil, false
	}
}

func resolveAllowedAction(metadata map[string]any, action string) (string, bool) {
	if metadata == nil {
		return action, true
	}
	raw, ok := metadata[agentic.MetadataKeyDecideActionAllowlist]
	if !ok || raw == nil {
		return action, true
	}
	allowlist := coerceStringList(raw)
	if len(allowlist) == 0 {
		return action, true
	}
	for _, a := range allowlist {
		if a == action {
			return action, true
		}
	}
	norm := normaliseAction(action)
	for _, a := range allowlist {
		if normaliseAction(a) == norm {
			return a, true
		}
	}
	return action, false
}

func coerceStringList(raw any) []string {
	switch v := raw.(type) {
	case []string:
		return v
	case []any:
		out := make([]string, 0, len(v))
		for _, item := range v {
			if s, ok := item.(string); ok && s != "" {
				out = append(out, s)
			}
		}
		return out
	default:
		return nil
	}
}

func buildRestrictedSet(actions []string) map[string]struct{} {
	if len(actions) == 0 {
		return nil
	}
	set := make(map[string]struct{}, len(actions))
	for _, a := range actions {
		if n := normaliseAction(a); n != "" {
			set[n] = struct{}{}
		}
	}
	if len(set) == 0 {
		return nil
	}
	return set
}

func (e *Executor) isRestricted(action string) bool {
	if len(e.restrictedActions) == 0 {
		return false
	}
	_, ok := e.restrictedActions[normaliseAction(action)]
	return ok
}

func normaliseAction(s string) string {
	s = strings.TrimSpace(s)
	s = strings.ToLower(s)
	s = strings.ReplaceAll(s, "-", "_")
	return s
}
