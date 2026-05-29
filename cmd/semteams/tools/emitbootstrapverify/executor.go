// Package emitbootstrapverify implements the SemTeams-local
// emit_bootstrap_verify tool — the producer side of the
// provisioner-bootstrap-verify persona's smoke-result emission.
//
// The verify persona runs the plan's verify_command against the
// tenant via bash (typically `docker exec <container> <verify_cmd>`),
// observes the smoke result, then calls this tool exactly once to
// stamp the structured verify-result triples on its own loop entity.
// The rule pack's $entity.triple.sandbox.verify.* substitutions (see
// configs/rules/sandbox-bootstrap/04-verify-to-reviewer.json) key off
// these triples.
//
// NO registry mutation. The reviewer-bootstrap holds the
// registry-commit authority (via emit_bootstrap_committed) per the
// rule pack's "registry_commit_in_reviewer" discipline: the
// reviewer's structural approval gate ensures the registry isn't
// committed on a malformed verify.
//
// Discipline note (framework-alignment review per
// cmd/semteams/tools/README.md): emit-style tool mirroring the
// existing emit_research_artifact / emit_plan pattern; defensible
// product-shell-local until upstream's planned generic write_artifact
// suite lands. Same migration target as the other emit tools.
package emitbootstrapverify

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"time"

	"github.com/c360studio/semstreams/agentic"
	"github.com/c360studio/semstreams/message"
	agentictools "github.com/c360studio/semstreams/processor/agentic-tools"
	"github.com/c360studio/semstreams/types"
)

// ToolName is the LLM-facing tool name.
const ToolName = "emit_bootstrap_verify"

// toolSource tags the triples this tool publishes.
const toolSource = "sandbox-bootstrap-emit-verify"

// Verify outcome vocabulary. Pinned in code so the rule pack's
// outcome-based routing (rule 05 reviewer-rejected-retry's
// recovery branches) can branch on stable string values.
const (
	OutcomeOK               = "ok"
	OutcomeContainerMissing = "container_missing"
	OutcomeSmokeFailed      = "smoke_failed"
)

// Predicate constants on the calling verify loop entity. Match the
// rule pack's $entity.triple.sandbox.verify.<name> substitutions
// — keep in sync if either changes.
const (
	predicateContainerName      = "sandbox.verify.container_name"
	predicateWorkspacePath      = "sandbox.verify.workspace_path"
	predicateSmokeExitCode      = "sandbox.verify.smoke_exit_code"
	predicateSmokeStdoutTail    = "sandbox.verify.smoke_stdout_tail"
	predicateSmokeMatchesExpect = "sandbox.verify.smoke_matches_expected"
	predicateVerifyOutcome      = "sandbox.verify.verify_outcome"
	predicateVerifyStampedAt    = "sandbox.verify.stamped_at"
)

// Executor implements agentic.ToolExecutor for emit_bootstrap_verify.
type Executor struct {
	publisher agentictools.TriplePublisher
	platform  types.PlatformMeta
	logger    *slog.Logger
}

// NewExecutor constructs an Executor. Publisher must be non-nil.
func NewExecutor(publisher agentictools.TriplePublisher, platform types.PlatformMeta, logger *slog.Logger) *Executor {
	if publisher == nil {
		panic("emitbootstrapverify.NewExecutor: publisher must not be nil")
	}
	if logger == nil {
		logger = slog.Default()
	}
	return &Executor{publisher: publisher, platform: platform, logger: logger}
}

// ListTools returns the LLM-facing schema.
func (e *Executor) ListTools() []agentic.ToolDefinition {
	return []agentic.ToolDefinition{{
		Name:        ToolName,
		Description: "Emit the provisioner-bootstrap-verify persona's smoke result. Stamps sandbox.verify.* triples on the calling loop entity so reviewer-bootstrap's $entity.triple.sandbox.verify.* substitutions resolve. No registry mutation — reviewer-bootstrap commits via emit_bootstrap_committed.",
		Parameters: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"container_name":         map[string]any{"type": "string", "description": "Tenant container name (e.g. 'semteams-tenant-abc123def456'). The verify persona ran the smoke against this container via docker exec."},
				"workspace_path":         map[string]any{"type": "string", "description": "Container-internal workspace the smoke ran in (typically /workspace)."},
				"smoke_exit_code":        map[string]any{"type": "integer", "description": "Exit code of the verify_command."},
				"smoke_stdout_tail":      map[string]any{"type": "string", "description": "Last ~500 bytes of the smoke stdout for audit (NOT the full output)."},
				"smoke_matches_expected": map[string]any{"type": "boolean", "description": "True if smoke output matched plan.expected_smoke_signature."},
				"verify_outcome":         map[string]any{"type": "string", "enum": []string{OutcomeOK, OutcomeContainerMissing, OutcomeSmokeFailed}, "description": "ok | container_missing | smoke_failed. Reviewer recovery branches on this value."},
			},
			"required": []string{"verify_outcome"},
		},
	}}
}

// Execute parses args, stamps verify triples on the calling loop
// entity. The framework's agentic.LoopExecutionEntityID builds the
// subject from (org, platform, call.LoopID).
func (e *Executor) Execute(ctx context.Context, call agentic.ToolCall) (agentic.ToolResult, error) {
	if call.Name != ToolName {
		return errResult(call, agentic.ToolErrorNotFound, "unknown tool: %s", call.Name)
	}
	if call.LoopID == "" {
		return errResult(call, agentic.ToolErrorInternal,
			"emit_bootstrap_verify invoked without loop_id; cannot resolve calling loop entity")
	}

	args, err := parseArgs(call.Arguments)
	if err != nil {
		return errResult(call, agentic.ToolErrorInvalidArgs, "%v", err)
	}

	loopEntityID, err := agentic.TryLoopExecutionEntityID(e.platform.Org, e.platform.Platform, call.LoopID)
	if err != nil {
		return errResult(call, agentic.ToolErrorInternal, "loop entity id: %v", err)
	}

	triples := args.triples(loopEntityID)
	if err := e.publisher.AddTriplesBatch(ctx, triples); err != nil {
		return errResult(call, agentic.ToolErrorNetwork, "stamp verify triples on %s: %v", loopEntityID, err)
	}

	body, _ := json.Marshal(map[string]any{
		"loop_entity_id":         loopEntityID,
		"verify_outcome":         args.VerifyOutcome,
		"container_name":         args.ContainerName,
		"smoke_matches_expected": args.SmokeMatchesExpected,
	})

	e.logger.Info("emit_bootstrap_verify stamped",
		slog.String("loop_entity_id", loopEntityID),
		slog.String("verify_outcome", args.VerifyOutcome),
		slog.String("container_name", args.ContainerName),
		slog.Int("smoke_exit_code", args.SmokeExitCode))

	return agentic.ToolResult{
		CallID:   call.ID,
		Name:     call.Name,
		Content:  string(body),
		Metadata: map[string]any{"loop_entity_id": loopEntityID, "verify_outcome": args.VerifyOutcome},
	}, nil
}

type parsedArgs struct {
	ContainerName        string
	WorkspacePath        string
	SmokeExitCode        int
	SmokeStdoutTail      string
	SmokeMatchesExpected bool
	VerifyOutcome        string
}

func parseArgs(raw map[string]any) (*parsedArgs, error) {
	p := &parsedArgs{}
	if v, ok := raw["container_name"].(string); ok {
		p.ContainerName = v
	}
	if v, ok := raw["workspace_path"].(string); ok {
		p.WorkspacePath = v
	}
	if v, ok := raw["smoke_exit_code"].(float64); ok {
		p.SmokeExitCode = int(v)
	}
	if v, ok := raw["smoke_stdout_tail"].(string); ok {
		p.SmokeStdoutTail = v
	}
	if v, ok := raw["smoke_matches_expected"].(bool); ok {
		p.SmokeMatchesExpected = v
	}
	if v, ok := raw["verify_outcome"].(string); ok {
		p.VerifyOutcome = v
	}

	switch p.VerifyOutcome {
	case OutcomeOK, OutcomeContainerMissing, OutcomeSmokeFailed:
		// ok
	default:
		return nil, fmt.Errorf("verify_outcome %q: must be ok|container_missing|smoke_failed", p.VerifyOutcome)
	}

	// Outcome-specific required-field checks. ok requires
	// container_name (defense-in-depth in reviewer needs it).
	// container_missing path can leave container_name blank — that
	// IS the signal. smoke_failed requires container_name for
	// reviewer's docker inspect re-check.
	if p.VerifyOutcome == OutcomeOK && p.ContainerName == "" {
		return nil, fmt.Errorf("container_name required when verify_outcome=ok")
	}
	if p.VerifyOutcome == OutcomeSmokeFailed && p.ContainerName == "" {
		return nil, fmt.Errorf("container_name required when verify_outcome=smoke_failed (reviewer needs it for docker inspect)")
	}
	return p, nil
}

func (p *parsedArgs) triples(loopEntityID string) []message.Triple {
	now := time.Now().UTC()
	base := func(pred string, obj any) message.Triple {
		return message.Triple{
			Subject:    loopEntityID,
			Predicate:  pred,
			Object:     obj,
			Source:     toolSource,
			Timestamp:  now,
			Confidence: 1.0,
		}
	}
	out := []message.Triple{
		base(predicateVerifyOutcome, p.VerifyOutcome),
		base(predicateSmokeExitCode, p.SmokeExitCode),
		base(predicateSmokeMatchesExpect, p.SmokeMatchesExpected),
		base(predicateVerifyStampedAt, now.Format(time.RFC3339Nano)),
	}
	if p.ContainerName != "" {
		out = append(out, base(predicateContainerName, p.ContainerName))
	}
	if p.WorkspacePath != "" {
		out = append(out, base(predicateWorkspacePath, p.WorkspacePath))
	}
	if p.SmokeStdoutTail != "" {
		out = append(out, base(predicateSmokeStdoutTail, p.SmokeStdoutTail))
	}
	return out
}

func errResult(call agentic.ToolCall, kind agentic.ToolErrorKind, format string, args ...any) (agentic.ToolResult, error) {
	return agentic.ToolResult{
		CallID:    call.ID,
		Name:      call.Name,
		Error:     fmt.Sprintf(format, args...),
		ErrorKind: kind,
	}, nil
}
