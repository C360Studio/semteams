// Package emitbootstrapcommitted implements the SemTeams-local
// emit_bootstrap_committed tool — the cross-run reuse commit invoked
// by reviewer-bootstrap on approval (see
// configs/rules/sandbox-bootstrap/04-verify-to-reviewer.json
// iteration 4).
//
// Two writes per call:
//
//  1. Stamp sandbox.tenant.{signature, container_name, image,
//     workspace, plan_hash} on the calling reviewer loop entity so
//     rule 07's $entity.triple.sandbox.tenant.* substitutions
//     resolve. This is what threads the tenant handoff state into
//     the chained wake-up coordinator's spawn properties.
//
//  2. Transition the registry entity (canonical 6-part
//     {org}.{platform}.sandbox.tenant.execution.{hash}) to
//     state=ready_running with ready_at + last_used + plan_hash.
//     Subsequent bootstrap arcs on the same signature take the
//     skip path on the strength of these triples.
//
// The reviewer is the structural gate that prevents a malformed
// verify from committing the registry. Per the rule pack's
// "registry_commit_in_reviewer" discipline.
//
// Discipline note (framework-alignment review per
// cmd/semteams/tools/README.md): defensible product-shell-local;
// migration target documented alongside the sandboxfleet package.
package emitbootstrapcommitted

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
const ToolName = "emit_bootstrap_committed"

// toolSource tags the triples this tool publishes.
const toolSource = "sandbox-bootstrap-emit-committed"

// Predicates stamped on the reviewer loop. Match rule 07's
// $entity.triple.sandbox.tenant.* substitution targets — keep
// in sync if either side changes.
const (
	predicateSignature       = sandboxfleet.PredicateSignature
	predicateContainerName   = sandboxfleet.PredicateContainerName
	predicateImage           = sandboxfleet.PredicateImage
	predicateWorkspace       = sandboxfleet.PredicateWorkspace
	predicatePlanHash        = sandboxfleet.PredicatePlanHash
	predicateCommittedAt     = "sandbox.tenant.committed_at"
	predicateCommittedSource = "sandbox.tenant.committed_source"
)

// Registry is the narrow registry-mutation surface this executor
// uses. CommitByHash is keyed by the signature hash; emit_bootstrap
// _committed only receives the precomputed hash (the reviewer
// persona reads it off the run entity via read_loop_result and
// echoes it back).
type Registry interface {
	CommitByHash(ctx context.Context, sigHash, containerName, image, workspace, planHash string) error
}

// Executor implements agentic.ToolExecutor for emit_bootstrap_committed.
type Executor struct {
	publisher agentictools.TriplePublisher
	registry  Registry
	platform  types.PlatformMeta
	logger    *slog.Logger
}

// NewExecutor constructs an Executor. All deps must be non-nil.
func NewExecutor(publisher agentictools.TriplePublisher, registry Registry, platform types.PlatformMeta, logger *slog.Logger) *Executor {
	if publisher == nil {
		panic("emitbootstrapcommitted.NewExecutor: publisher must not be nil")
	}
	if registry == nil {
		panic("emitbootstrapcommitted.NewExecutor: registry must not be nil")
	}
	if logger == nil {
		logger = slog.Default()
	}
	return &Executor{publisher: publisher, registry: registry, platform: platform, logger: logger}
}

// ListTools returns the LLM-facing schema.
func (e *Executor) ListTools() []agentic.ToolDefinition {
	return []agentic.ToolDefinition{{
		Name:        ToolName,
		Description: "Commit the tenant to ready_running. Stamps sandbox.tenant.{signature,container_name,image,workspace,plan_hash} on the calling reviewer loop entity so the chained wake-up coordinator's spawn properties carry the tenant handoff state, AND transitions the registry entity to state=ready_running with ready_at + last_used. Called by reviewer-bootstrap exactly once on approval.",
		Parameters: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"signature":      map[string]any{"type": "string", "description": "Canonical signature hex (64 chars). Echoed verbatim from the run entity (read via read_loop_result)."},
				"container_name": map[string]any{"type": "string", "description": "Tenant container name (e.g. 'semteams-tenant-abc123def456')."},
				"image":          map[string]any{"type": "string", "description": "Base image:tag the tenant was provisioned from."},
				"workspace":      map[string]any{"type": "string", "description": "Container-internal workspace path (typically /workspace)."},
				"plan_hash":      map[string]any{"type": "string", "description": "Plan-hash computed at plan time. Echoed verbatim."},
			},
			"required": []string{"signature", "container_name", "image", "workspace", "plan_hash"},
		},
	}}
}

// Execute parses args, stamps reviewer-loop triples, commits to
// registry. Order: stamp first, then registry. If the registry
// commit fails, the reviewer loop still carries the tenant handoff
// triples (rule 07 substitution will resolve), but the tenant's
// state stays at provisioning — the next bootstrap arc on this
// signature will retry. Acceptable: the alternative (registry first,
// then stamp) would create a worse failure mode where rule 07
// substitution fails but the registry claims ready.
func (e *Executor) Execute(ctx context.Context, call agentic.ToolCall) (agentic.ToolResult, error) {
	if call.Name != ToolName {
		return errResult(call, agentic.ToolErrorNotFound, "unknown tool: %s", call.Name)
	}
	if call.LoopID == "" {
		return errResult(call, agentic.ToolErrorInternal,
			"emit_bootstrap_committed invoked without loop_id; cannot resolve reviewer loop entity")
	}

	args, err := parseArgs(call.Arguments)
	if err != nil {
		return errResult(call, agentic.ToolErrorInvalidArgs, "%v", err)
	}

	reviewerEntityID, err := agentic.TryLoopExecutionEntityID(e.platform.Org, e.platform.Platform, call.LoopID)
	if err != nil {
		return errResult(call, agentic.ToolErrorInternal, "reviewer loop entity id: %v", err)
	}

	triples := args.triples(reviewerEntityID)
	if err := e.publisher.AddTriplesBatch(ctx, triples); err != nil {
		return errResult(call, agentic.ToolErrorNetwork, "stamp committed triples on %s: %v", reviewerEntityID, err)
	}

	if err := e.registry.CommitByHash(ctx, args.Signature, args.ContainerName, args.Image, args.Workspace, args.PlanHash); err != nil {
		return errResult(call, agentic.ToolErrorNetwork, "registry commit: %v", err)
	}

	body, _ := json.Marshal(map[string]any{
		"reviewer_entity_id": reviewerEntityID,
		"signature":          args.Signature,
		"container_name":     args.ContainerName,
		"state":              "ready_running",
	})

	e.logger.Info("emit_bootstrap_committed",
		slog.String("reviewer_entity_id", reviewerEntityID),
		slog.String("signature", args.Signature),
		slog.String("container_name", args.ContainerName))

	return agentic.ToolResult{
		CallID:   call.ID,
		Name:     call.Name,
		Content:  string(body),
		Metadata: map[string]any{"signature": args.Signature, "container_name": args.ContainerName},
	}, nil
}

type parsedArgs struct {
	Signature     string
	ContainerName string
	Image         string
	Workspace     string
	PlanHash      string
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
	return p, nil
}

func (p *parsedArgs) triples(reviewerEntityID string) []message.Triple {
	now := time.Now().UTC()
	base := func(pred string, obj any) message.Triple {
		return message.Triple{
			Subject:    reviewerEntityID,
			Predicate:  pred,
			Object:     obj,
			Source:     toolSource,
			Timestamp:  now,
			Confidence: 1.0,
		}
	}
	return []message.Triple{
		base(predicateSignature, p.Signature),
		base(predicateContainerName, p.ContainerName),
		base(predicateImage, p.Image),
		base(predicateWorkspace, p.Workspace),
		base(predicatePlanHash, p.PlanHash),
		base(predicateCommittedAt, now.Format(time.RFC3339Nano)),
		base(predicateCommittedSource, "reviewer-bootstrap"),
	}
}

func errResult(call agentic.ToolCall, kind agentic.ToolErrorKind, format string, args ...any) (agentic.ToolResult, error) {
	return agentic.ToolResult{
		CallID:    call.ID,
		Name:      call.Name,
		Error:     fmt.Sprintf(format, args...),
		ErrorKind: kind,
	}, nil
}
