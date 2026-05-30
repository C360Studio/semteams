// Package emitbootstrapexecute implements the SemTeams-local
// emit_bootstrap_execute tool — the forward-stamp tool the
// provisioner-bootstrap-execute persona calls after install_steps
// complete, before its terminal decide(verify).
//
// Purpose: bridge the rule 03 (execute → verify) substitution gap
// surfaced by smoke #9 (see [[smoke9-findings]] Finding 1 and
// [[pr3-1-smoke]]). emit_bootstrap_plan stamps sandbox.tenant.*
// triples on the run entity + the plan loop (PR 3.1 dual-target),
// which makes rule 02b's spawn-prompt substitution resolve when
// spawning execute. But execute's own loop entity has no
// sandbox.tenant.* triples, so rule 03's
// `$entity.triple.sandbox.tenant.{verify_command,
// expected_smoke_signature, container_name, workspace}`
// substitutions evaluate to literal tokens — verify's spawn prompt
// stranded without the smoke contract.
//
// This tool fixes that by stamping the SAME sandbox.tenant.*
// triples on the execute loop entity. The execute persona reads
// the values from its spawn prompt (already substituted from the
// plan loop's stamp), passes them as args here, and the tool
// forwards them onto execute's own loop entity. Rule 03 then
// substitutes them into verify's spawn prompt cleanly.
//
// NO registry mutation. The registry remains keyed off the run
// entity (authoritative). This tool is pure forward-plumbing per
// the multi-target stamping pattern established by PR 3.1.
//
// Discipline note (framework-alignment review per
// cmd/semteams/tools/README.md): mirror of emit_bootstrap_verify
// (same single-target stamp on the calling loop entity). Defensible
// product-shell-local; the upstream substitution semantics
// (semstreams v1.0.0-beta.86 processor/rule/execution_context.go)
// don't currently traverse $entity → related/lineage entities, so
// every cross-loop hop in a pack that needs triple substitution
// requires either dual-stamping at emit time (plan, committed) or
// forward-stamping at the receiving role (this tool). When upstream
// lands $lineage.X.triple.Y or $related.X.triple.Y, this tool can
// retire — rule 03 would substitute directly from the run entity.
package emitbootstrapexecute

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

	"github.com/c360studio/semteams/cmd/semteams/sandboxfleet"
)

// ToolName is the LLM-facing tool name.
const ToolName = "emit_bootstrap_execute"

// toolSource tags the triples this tool publishes.
const toolSource = "sandbox-bootstrap-emit-execute"

// Predicates stamped on the execute loop entity. Tenant predicates
// re-exported from sandboxfleet so the cross-tool predicate
// namespace stays single-sourced; audit predicates local to this
// tool. Rule 03's $entity.triple.sandbox.tenant.* substitutions
// resolve against these.
const (
	predicateSignature         = sandboxfleet.PredicateSignature
	predicateContainerName     = sandboxfleet.PredicateContainerName
	predicateImage             = sandboxfleet.PredicateImage
	predicateWorkspace         = sandboxfleet.PredicateWorkspace
	predicatePlanHash          = sandboxfleet.PredicatePlanHash
	predicatePlanAction        = sandboxfleet.PredicatePlanAction
	predicatePlanVerifyCommand = sandboxfleet.PredicatePlanVerifyCommand
	predicatePlanExpectedSmoke = sandboxfleet.PredicatePlanExpectedSmoke
	// Audit-only triples — not consumed by any rule substitution.
	// Stamped so an operator grepping the graph for an arc can see
	// which loop forwarded the plan triples (provenance trail).
	predicateExecuteForwardedAt     = "sandbox.tenant.execute_forwarded_at"
	predicateExecuteForwardedSource = "sandbox.tenant.execute_forwarded_source"
)

// Executor implements agentic.ToolExecutor for emit_bootstrap_execute.
type Executor struct {
	publisher agentictools.TriplePublisher
	platform  types.PlatformMeta
	logger    *slog.Logger
}

// NewExecutor constructs an Executor. Publisher must be non-nil;
// platform must have Org + Platform set so the executor can
// construct the calling-loop's 6-part entity ID.
func NewExecutor(publisher agentictools.TriplePublisher, platform types.PlatformMeta, logger *slog.Logger) *Executor {
	if publisher == nil {
		panic("emitbootstrapexecute.NewExecutor: publisher must not be nil")
	}
	if platform.Org == "" || platform.Platform == "" {
		panic("emitbootstrapexecute.NewExecutor: platform.Org and platform.Platform must be set for calling-loop entity ID construction")
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
		Description: "Forward the plan's sandbox.tenant.* values onto the calling execute loop entity. The execute persona reads container_name, image, workspace, verify_command, expected_smoke_signature, plan_hash, signature, plan_action from its spawn prompt (substituted from the plan loop's stamp); call this tool once with those values before terminating with decide(verify). The tool stamps them on YOUR loop entity so rule 03's $entity.triple.sandbox.tenant.* substitutions resolve when spawning verify. No registry mutation.",
		Parameters: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"signature":                map[string]any{"type": "string", "description": "Canonical signature hex (64 chars). Echo verbatim from your spawn prompt."},
				"container_name":           map[string]any{"type": "string", "description": "Tenant container name (e.g. 'semteams-tenant-abc123def456'). Echo verbatim."},
				"image":                    map[string]any{"type": "string", "description": "Base image:tag the tenant was provisioned from. Echo verbatim."},
				"workspace":                map[string]any{"type": "string", "description": "Container-internal workspace path. Echo verbatim."},
				"plan_hash":                map[string]any{"type": "string", "description": "Plan-hash computed at plan time. Echo verbatim."},
				"plan_action":              map[string]any{"type": "string", "description": "provision | reprovision | skip. Echo verbatim."},
				"verify_command":           map[string]any{"type": "string", "description": "Verify smoke command. Echo verbatim — verify reads this from your stamped triple."},
				"expected_smoke_signature": map[string]any{"type": "string", "description": "What verify will match against. Echo verbatim — verify reads this from your stamped triple."},
			},
			"required": []string{"signature", "container_name", "image", "workspace", "plan_hash", "verify_command", "expected_smoke_signature"},
		},
	}}
}

// Execute parses args, stamps tenant triples on the calling execute
// loop entity. The framework's agentic.TryLoopExecutionEntityID
// builds the subject from (org, platform, call.LoopID).
func (e *Executor) Execute(ctx context.Context, call agentic.ToolCall) (agentic.ToolResult, error) {
	if call.Name != ToolName {
		return errResult(call, agentic.ToolErrorNotFound, "unknown tool: %s", call.Name)
	}
	if call.LoopID == "" {
		return errResult(call, agentic.ToolErrorInternal,
			"emit_bootstrap_execute invoked without loop_id; cannot resolve calling loop entity")
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
		return errResult(call, agentic.ToolErrorNetwork, "stamp execute triples on %s: %v", loopEntityID, err)
	}

	body, _ := json.Marshal(map[string]any{
		"loop_entity_id": loopEntityID,
		"signature":      args.Signature,
		"container_name": args.ContainerName,
		"plan_action":    args.PlanAction,
	})

	e.logger.Info("emit_bootstrap_execute stamped",
		slog.String("loop_entity_id", loopEntityID),
		slog.String("signature", args.Signature),
		slog.String("container_name", args.ContainerName),
		slog.String("plan_action", args.PlanAction))

	return agentic.ToolResult{
		CallID:   call.ID,
		Name:     call.Name,
		Content:  string(body),
		Metadata: map[string]any{"loop_entity_id": loopEntityID, "container_name": args.ContainerName, "signature": args.Signature},
	}, nil
}

type parsedArgs struct {
	Signature              string
	ContainerName          string
	Image                  string
	Workspace              string
	PlanHash               string
	PlanAction             string
	VerifyCommand          string
	ExpectedSmokeSignature string
}

func parseArgs(raw map[string]any) (*parsedArgs, error) {
	p := &parsedArgs{}
	if v, ok := raw["signature"].(string); ok {
		p.Signature = v
	}
	if v, ok := raw["container_name"].(string); ok {
		p.ContainerName = v
	}
	if v, ok := raw["image"].(string); ok {
		p.Image = v
	}
	if v, ok := raw["workspace"].(string); ok {
		p.Workspace = v
	}
	if v, ok := raw["plan_hash"].(string); ok {
		p.PlanHash = v
	}
	if v, ok := raw["plan_action"].(string); ok {
		p.PlanAction = v
	}
	if v, ok := raw["verify_command"].(string); ok {
		p.VerifyCommand = v
	}
	if v, ok := raw["expected_smoke_signature"].(string); ok {
		p.ExpectedSmokeSignature = v
	}
	if p.Signature == "" {
		return nil, fmt.Errorf("signature is required")
	}
	if p.ContainerName == "" {
		return nil, fmt.Errorf("container_name is required")
	}
	if p.Image == "" {
		return nil, fmt.Errorf("image is required")
	}
	if p.Workspace == "" {
		return nil, fmt.Errorf("workspace is required")
	}
	if p.PlanHash == "" {
		return nil, fmt.Errorf("plan_hash is required")
	}
	if p.VerifyCommand == "" {
		return nil, fmt.Errorf("verify_command is required (the verify hop reads this from the stamped triple)")
	}
	if p.ExpectedSmokeSignature == "" {
		return nil, fmt.Errorf("expected_smoke_signature is required (the verify hop reads this from the stamped triple)")
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
		base(predicateSignature, p.Signature),
		base(predicateContainerName, p.ContainerName),
		base(predicateImage, p.Image),
		base(predicateWorkspace, p.Workspace),
		base(predicatePlanHash, p.PlanHash),
		base(predicatePlanVerifyCommand, p.VerifyCommand),
		base(predicatePlanExpectedSmoke, p.ExpectedSmokeSignature),
		base(predicateExecuteForwardedAt, now.Format(time.RFC3339Nano)),
		base(predicateExecuteForwardedSource, "provisioner-bootstrap-execute"),
	}
	if p.PlanAction != "" {
		out = append(out, base(predicatePlanAction, p.PlanAction))
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
