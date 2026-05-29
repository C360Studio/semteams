// Package querysandboxtenant implements the SemTeams-local
// query_sandbox_tenant tool — the read side of the sandbox-bootstrap
// pack's registry contract.
//
// The tool is the deterministic registry probe the
// provisioner-bootstrap-plan persona calls in its iteration 3
// (per configs/rules/sandbox-bootstrap/01-coordinator-bootstrap-spawn.json):
// given a target signature, return the current registry record
// ({state, container_name, image, workspace, ready_at, plan_hash,
// last_used}) or null on miss. Plan persona's freshness gate keys
// off the returned state + ready_at + plan_hash.
//
// Discipline note (framework-alignment review per
// cmd/semteams/tools/README.md): no upstream "tenant container fleet"
// primitive exists or is planned (verified semstreams beta.86,
// 2026-05-29). The tool is defensible product-shell-local. Migration
// target: graduate alongside the sandboxfleet package upstream if
// semspec / semdragons gain similar multi-tenant containerization
// needs. ADR-042 §addendum 2026-05-29 §A.
package querysandboxtenant

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"time"

	"github.com/c360studio/semstreams/agentic"

	"github.com/c360studio/semteams/cmd/semteams/sandboxfleet"
)

// ToolName is the LLM-facing tool name. Wired into the
// provisioner-bootstrap-plan role's allowed_tools at rule
// configs/rules/sandbox-bootstrap/01-coordinator-bootstrap-spawn.json.
const ToolName = "query_sandbox_tenant"

// Registry is the narrow read surface the executor uses. The
// production wiring (cmd/semteams/product_tools.go) supplies the
// concrete sandboxfleet.TenantRegistry; tests inject a fake.
type Registry interface {
	Lookup(ctx context.Context, sig sandboxfleet.TargetSignature) (*sandboxfleet.Record, error)
}

// Executor implements agentic.ToolExecutor for query_sandbox_tenant.
type Executor struct {
	registry Registry
	logger   *slog.Logger
}

// NewExecutor constructs an Executor. Both registry and logger must
// be non-nil — the read tool has no graceful-degradation mode for a
// missing registry (the foundation PR is part of boot-required
// wiring).
func NewExecutor(registry Registry, logger *slog.Logger) *Executor {
	if registry == nil {
		panic("querysandboxtenant.NewExecutor: registry must not be nil")
	}
	if logger == nil {
		logger = slog.Default()
	}
	return &Executor{registry: registry, logger: logger}
}

// ListTools returns the LLM-facing schema. Args mirror
// sandboxfleet.CanonicalizeInput — the tool canonicalizes server-side
// so two prose-framing variants of the same target hit the same
// registry record (cache-key correctness per ADR-042 §addendum review
// H2 fix).
func (e *Executor) ListTools() []agentic.ToolDefinition {
	return []agentic.ToolDefinition{{
		Name: ToolName,
		Description: "Look up the sandbox tenant registry for a target signature. Returns the current registry record or null on miss. Plan persona's freshness gate keys off the returned state + ready_at + plan_hash. " +
			"Canonicalization is server-side: variants of the same target description (case, ssh-vs-https repo url, version short-form, etc.) hit the same record. " +
			"The canonical 6-part tenant entity ID is {org}.{platform}.sandbox.tenant.execution.{hash} — siblings to the agentic-loop and chain entity namespaces.",
		Parameters: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"command":    map[string]any{"type": "string", "description": "The measurement / smoke command verbatim (e.g. 'task test:integration')."},
				"repo_url":   map[string]any{"type": "string", "description": "Optional source repo URL (ssh or https accepted). Empty for self-contained targets."},
				"repo_ref":   map[string]any{"type": "string", "description": "Optional source ref (commit SHA, tag, or branch). Required when repo_url is set."},
				"toolchain":  map[string]any{"type": "object", "description": "Optional toolchain version map (e.g. {\"go\":\"1.26\",\"node\":\"22\"}). Lowercased keys; versions normalized to full semver."},
				"base_image": map[string]any{"type": "string", "description": "Docker image:tag for the tenant base (no :latest, no missing tag — cache-correctness requires explicit tags)."},
			},
			"required": []string{"command", "base_image"},
		},
	}}
}

// Execute parses args, canonicalizes, queries the registry, returns
// the record as a JSON object (or null on miss).
func (e *Executor) Execute(ctx context.Context, call agentic.ToolCall) (agentic.ToolResult, error) {
	if call.Name != ToolName {
		return agentic.ToolResult{
			CallID:    call.ID,
			Name:      call.Name,
			Error:     fmt.Sprintf("unknown tool: %s", call.Name),
			ErrorKind: agentic.ToolErrorNotFound,
		}, nil
	}
	input, err := parseArgs(call.Arguments)
	if err != nil {
		return agentic.ToolResult{
			CallID:    call.ID,
			Name:      call.Name,
			Error:     err.Error(),
			ErrorKind: agentic.ToolErrorInvalidArgs,
		}, nil
	}
	sig, err := sandboxfleet.Canonicalize(input)
	if err != nil {
		return agentic.ToolResult{
			CallID:    call.ID,
			Name:      call.Name,
			Error:     fmt.Sprintf("canonicalize: %v", err),
			ErrorKind: agentic.ToolErrorInvalidArgs,
		}, nil
	}
	rec, err := e.registry.Lookup(ctx, sig)
	if err != nil {
		return agentic.ToolResult{
			CallID:    call.ID,
			Name:      call.Name,
			Error:     fmt.Sprintf("registry lookup: %v", err),
			ErrorKind: agentic.ToolErrorNetwork,
		}, nil
	}

	result := buildResult(sig, rec)
	body, err := json.Marshal(result)
	if err != nil {
		return agentic.ToolResult{
			CallID:    call.ID,
			Name:      call.Name,
			Error:     fmt.Sprintf("marshal result: %v", err),
			ErrorKind: agentic.ToolErrorInternal,
		}, nil
	}

	e.logger.Info("query_sandbox_tenant",
		slog.String("signature", sig.Hash()),
		slog.Bool("hit", rec != nil))

	return agentic.ToolResult{
		CallID:  call.ID,
		Name:    call.Name,
		Content: string(body),
		Metadata: map[string]any{
			"signature": sig.Hash(),
			"hit":       rec != nil,
		},
	}, nil
}

// parseArgs decodes the LLM-supplied tool args into a
// sandboxfleet.CanonicalizeInput. Unrecognized fields are ignored
// (forward-compat: callers can pass canonical fields we haven't
// pulled into the schema yet without erroring).
func parseArgs(raw map[string]any) (sandboxfleet.CanonicalizeInput, error) {
	out := sandboxfleet.CanonicalizeInput{}
	if v, ok := raw["command"].(string); ok {
		out.Command = v
	}
	if v, ok := raw["repo_url"].(string); ok {
		out.RepoURL = v
	}
	if v, ok := raw["repo_ref"].(string); ok {
		out.RepoRef = v
	}
	if v, ok := raw["base_image"].(string); ok {
		out.BaseImage = v
	}
	if tc, ok := raw["toolchain"].(map[string]any); ok {
		out.Toolchain = make(map[string]string, len(tc))
		for k, v := range tc {
			if s, ok := v.(string); ok {
				out.Toolchain[k] = s
			} else {
				return sandboxfleet.CanonicalizeInput{}, fmt.Errorf("toolchain[%s]: expected string, got %T", k, v)
			}
		}
	}
	return out, nil
}

// resultBody is the JSON-marshaled return shape. Mirrors the
// "{state, container_name, image, workspace, ready_at, plan_hash,
// last_used}" contract from the sandbox-bootstrap pack README.
// When Record is nil (registry miss), Hit=false and the typed
// fields stay at their zero values. The plan persona inspects
// Hit first; if true, walks the typed fields.
type resultBody struct {
	Signature     string `json:"signature"`
	Hit           bool   `json:"hit"`
	State         string `json:"state,omitempty"`
	ContainerName string `json:"container_name,omitempty"`
	Image         string `json:"image,omitempty"`
	Workspace     string `json:"workspace,omitempty"`
	ReadyAt       string `json:"ready_at,omitempty"`
	LastUsed      string `json:"last_used,omitempty"`
	PlanHash      string `json:"plan_hash,omitempty"`
}

func buildResult(sig sandboxfleet.TargetSignature, rec *sandboxfleet.Record) resultBody {
	out := resultBody{Signature: sig.Hash()}
	if rec == nil {
		return out
	}
	out.Hit = true
	out.State = string(rec.State)
	out.ContainerName = rec.ContainerName
	out.Image = rec.Image
	out.Workspace = rec.Workspace
	out.PlanHash = rec.PlanHash
	if !rec.ReadyAt.IsZero() {
		out.ReadyAt = rec.ReadyAt.Format(time.RFC3339Nano)
	}
	if !rec.LastUsed.IsZero() {
		out.LastUsed = rec.LastUsed.Format(time.RFC3339Nano)
	}
	return out
}
