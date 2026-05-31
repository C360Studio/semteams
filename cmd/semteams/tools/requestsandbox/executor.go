// Package requestsandbox implements the LLM-facing request_sandbox
// tool — the Coordinator's entry point into Layer 2 of the
// three-layer sandbox model (ADR-043).
//
// The Coordinator persona sketches a SandboxRequirements (typed
// capability contract — languages, tools, services, network,
// secrets, mounts, privileges, verification probes) and calls this
// tool. The executor delegates to sandboxmanager.Manager.Request,
// which matches → admits → materializes (devcontainer up) → probes
// → attests → stamps `sandbox.attestation.*` triples on the chain
// entity. The Attestation comes back as the tool result; the
// Coordinator then routes on attestation.ready / .degraded /
// .terminal / .admission_outcome.
//
// Chain entity resolution: prefers
// related_loops["chain-entity-id"] when the spawn rule pinned it
// (PR 4.3 rule pack does this). Falls back to walking the chain
// resolver to construct `{org}.{platform}.agent.chain.execution.{
// chainID}` from the calling loop_id. This lets the tool work
// both inside the rule-pack-wired chain and in isolated invocations
// (smoke tests, manual coordinator calls).
//
// Discipline note (framework-alignment review per
// cmd/semteams/tools/README.md): no upstream tool wraps a Go-side
// orchestrator that shells out to an external CLI + emits typed
// triples. The defensible product-shell-local case is identical to
// emit_bootstrap_plan's (verified semstreams beta.86, 2026-05-29).
// Migration target: when semstreams adds a generic
// `execute_external` or `provision_environment` primitive (not on
// the published roadmap), revisit.
package requestsandbox

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"

	"github.com/c360studio/semstreams/agentic"
	"github.com/c360studio/semstreams/message"
	"github.com/c360studio/semstreams/types"

	"github.com/c360studio/semteams/cmd/semteams/sandboxmanager"
)

// ToolName is the LLM-facing tool name.
const ToolName = "request_sandbox"

// chainEntityRoleKey is the related_loops key the spawn rule sets
// to the chain entity's 6-part ID. PR 4.3's admission-rule will pin
// it via `"related_loops": { "chain-entity-id": "$chain.entity.id" }`.
const chainEntityRoleKey = "chain-entity-id"

// Manager is the narrow surface this tool requires. Production
// supplies *sandboxmanager.Manager; tests inject a fake.
type Manager interface {
	Request(ctx context.Context, chainEntityID string, req sandboxmanager.SandboxRequirements) (sandboxmanager.Attestation, error)
	Catalog() *sandboxmanager.Catalog
}

// ChainResolver derives the chain entity ID from the calling loop
// when the spawn rule didn't pin it via related_loops. Production
// supplies *chain.Resolver; tests inject a fake.
type ChainResolver interface {
	ChainID(ctx context.Context, loopID string) (string, error)
}

// Executor implements agentic.ToolExecutor for request_sandbox.
type Executor struct {
	manager  Manager
	resolver ChainResolver
	platform types.PlatformMeta
	logger   *slog.Logger
}

// NewExecutor constructs an Executor. Manager and platform must be
// non-nil/non-empty (boot-required wiring). resolver may be nil —
// in that case the tool requires related_loops["chain-entity-id"]
// to be present and errors when it isn't.
func NewExecutor(manager Manager, resolver ChainResolver, platform types.PlatformMeta, logger *slog.Logger) *Executor {
	if manager == nil {
		panic("requestsandbox.NewExecutor: manager must not be nil")
	}
	if platform.Org == "" || platform.Platform == "" {
		panic("requestsandbox.NewExecutor: platform.Org and platform.Platform must be set for chain entity ID construction")
	}
	if logger == nil {
		logger = slog.Default()
	}
	return &Executor{manager: manager, resolver: resolver, platform: platform, logger: logger}
}

// ListTools returns the LLM-facing schema. SandboxRequirements is a
// typed capability contract — fields describe what the work needs
// (languages, tools), NOT how to provision it (image, mounts as
// paths). The latter live inside catalog profiles + admission
// policy, per ADR-043 §Decision.
func (e *Executor) ListTools() []agentic.ToolDefinition {
	return []agentic.ToolDefinition{{
		Name: ToolName,
		Description: "Request a sandbox environment for the work at hand. Synchronous: returns an Attestation that says ready/degraded/failed plus the verified capabilities. " +
			"Coordinator routes on the attestation shape (.ready/.degraded/.terminal/.admission_outcome). NEVER author container internals (image, mounts as paths, privileges as docker flags) — those live in catalog profiles. Describe what the WORK needs. " +
			"The manager handles profile-match + admission + devcontainer up + probes + attestation. Profile lookup is operator-curated; admission gates privileged + public-network + host-path-mounts + un-pre-provisioned secrets. " +
			"Call exactly once per work request. Before calling, consider whether a fresh attestation already covers your needs by calling query_sandbox_attestation first.",
		Parameters: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"languages": map[string]any{
					"type":        "array",
					"items":       map[string]any{"type": "string"},
					"description": "Language toolchains the work needs (e.g. ['go'], ['node'], ['go','node']). Matches against profile.advertises.languages.",
				},
				"tools": map[string]any{
					"type":        "array",
					"items":       map[string]any{"type": "string"},
					"description": "CLI tools the work needs (e.g. ['task','gh'], ['playwright']). Matches against profile.advertises.tools and is verified by preflight probes when listed in verification.",
				},
				"services": map[string]any{
					"type":        "array",
					"items":       map[string]any{"type": "string"},
					"description": "In-container services the work needs (e.g. ['nats']). Most workloads have none. Only full-stack-e2e profile advertises services.",
				},
				"network": map[string]any{
					"type":        "string",
					"enum":        []string{"restricted", "public", "none"},
					"description": "Network policy class. 'restricted' (default) admits without operator policy; 'public' pends on AllowPublicNetwork OR admission-reviewer approval; 'none' requires profile support.",
				},
				"secrets": map[string]any{
					"type":        "array",
					"items":       map[string]any{"type": "string"},
					"description": "Pre-provisioned secret identifiers (env-var shape, [A-Z_][A-Z0-9_]*). Each MUST be in operator AvailableSecrets policy — admission denies otherwise. Coordinator can't ask the user to enter a credential mid-chain.",
				},
				"mounts": map[string]any{
					"type":        "array",
					"items":       map[string]any{"type": "string", "enum": []string{"workspace-write", "workspace-read"}},
					"description": "Mount class enum. Default 'workspace-write' is the typical case. 'workspace-read' is audit/inspect-only. Host-path bind mounts go via privileges, not here.",
				},
				"privileges": map[string]any{
					"type":        "array",
					"items":       map[string]any{"type": "string", "enum": []string{"docker-socket", "privileged"}},
					"description": "Privileged capability classes. 'docker-socket' for testcontainers/compose workloads (requires full-stack-e2e profile + admission-reviewer approval). 'privileged' is forbidden by default (requires operator AllowPrivilegedContainers flag + profile allowlist + reviewer signoff).",
				},
				"verification": map[string]any{
					"type":        "array",
					"description": "Preflight probes the manager runs inside the container after devcontainer up. Each probe.name becomes a sandbox.attestation.verified.<name> triple the Coordinator can route on. Probes that fail produce Degraded=true (not Ready) and DegradedReasons.",
					"items": map[string]any{
						"type": "object",
						"properties": map[string]any{
							"name":        map[string]any{"type": "string", "description": "Capability identifier ([a-z0-9][a-z0-9._-]*). Becomes the verified.<name> predicate suffix; substituted by Coordinator routing."},
							"command":     map[string]any{"type": "string", "description": "Verbatim shell line the probe runs (e.g. 'go version', 'task --list'). This IS user-intent, not LLM-authored shell — the probe shape IS its command."},
							"expect_exit": map[string]any{"type": "integer", "description": "Required exit code. Default 0. Probes that intend non-zero pass must set explicitly."},
						},
						"required": []string{"name", "command"},
					},
				},
			},
		},
	}}
}

// Execute parses, delegates to Manager.Request, marshals the
// resulting Attestation as JSON. Manager errors are returned as
// agentic.ToolErrorKind classified failures; non-Ready attestations
// (admission_pending, denied, terminal, degraded) are returned as
// normal tool results — the Coordinator routes on the attestation
// fields, NOT on the ToolResult.Error field.
func (e *Executor) Execute(ctx context.Context, call agentic.ToolCall) (agentic.ToolResult, error) {
	if call.Name != ToolName {
		return errResult(call, agentic.ToolErrorNotFound, "unknown tool: %s", call.Name), nil
	}

	req, err := parseArgs(call.Arguments)
	if err != nil {
		return errResult(call, agentic.ToolErrorInvalidArgs, "%v", err), nil
	}

	chainEntityID, err := e.chainEntityFromCall(ctx, call)
	if err != nil {
		return errResult(call, agentic.ToolErrorInternal, "%v", err), nil
	}

	att, err := e.manager.Request(ctx, chainEntityID, req)
	if err != nil {
		// Stamp failures: the attestation was rendered correctly but
		// triple-publish failed. The Coordinator's chain-entity view
		// will be behind; this is a network/internal class, not
		// invalid_args.
		return errResult(call, agentic.ToolErrorNetwork, "manager request: %v", err), nil
	}

	body, marshalErr := json.Marshal(attestationToWire(att))
	if marshalErr != nil {
		return errResult(call, agentic.ToolErrorInternal, "marshal attestation: %v", marshalErr), nil
	}

	e.logger.Info("request_sandbox completed",
		slog.String("chain_entity", chainEntityID),
		slog.String("profile", att.ProfileID),
		slog.String("signature", att.Signature),
		slog.String("admission", string(att.AdmissionOutcome)),
		slog.Bool("ready", att.Ready),
		slog.Bool("degraded", att.Degraded),
		slog.Bool("terminal", att.Terminal))

	return agentic.ToolResult{
		CallID:  call.ID,
		Name:    call.Name,
		Content: string(body),
		Metadata: map[string]any{
			"profile":           att.ProfileID,
			"signature":         att.Signature,
			"ready":             att.Ready,
			"degraded":          att.Degraded,
			"terminal":          att.Terminal,
			"admission_outcome": string(att.AdmissionOutcome),
		},
	}, nil
}

// chainEntityFromCall resolves the chain entity ID for triple
// stamping. Three sources, in order of preference:
//
//  1. related_loops["chain-entity-id"] — set by the rule pack
//     (PR 4.3) so the calling rule explicitly names the chain
//     entity. Highest precedence.
//  2. chain.Resolver.ChainID(loop_id) → construct entity from
//     chain ID. Used when the tool is invoked outside the rule
//     pack (smoke tests, isolated coordinator calls).
//  3. None — return error. Tool requires a chain entity to stamp
//     on; without it the attestation triples are orphaned.
func (e *Executor) chainEntityFromCall(ctx context.Context, call agentic.ToolCall) (string, error) {
	if related, ok := call.Metadata[agentic.MetadataKeyRelatedLoops].(map[string]any); ok {
		if v, ok := related[chainEntityRoleKey].(string); ok && v != "" {
			return v, nil
		}
	}
	if e.resolver == nil {
		return "", fmt.Errorf("request_sandbox: related_loops[%q] missing AND no ChainResolver wired (chain entity required for attestation stamp)", chainEntityRoleKey)
	}
	// Spawn rule didn't pin chain-entity-id — fall back to the
	// resolver. Loud-warn so operators see the gap during PR 4.3
	// rule-pack bring-up (a typoed related_loops key would silently
	// land here forever otherwise).
	e.logger.Warn("request_sandbox: related_loops chain-entity-id missing; using chain.Resolver fallback",
		slog.String("loop_id", call.LoopID),
		slog.String("related_key", chainEntityRoleKey))
	if call.LoopID == "" {
		return "", fmt.Errorf("request_sandbox: related_loops[%q] missing AND tool call has no loop_id (resolver fallback unavailable)", chainEntityRoleKey)
	}
	chainID, err := e.resolver.ChainID(ctx, call.LoopID)
	if err != nil {
		return "", fmt.Errorf("request_sandbox: resolve chain id from loop %q: %w", call.LoopID, err)
	}
	if chainID == "" {
		return "", fmt.Errorf("request_sandbox: chain.Resolver returned empty chain id for loop %q", call.LoopID)
	}
	entityID := fmt.Sprintf("%s.%s.agent.chain.execution.%s", e.platform.Org, e.platform.Platform, chainID)
	if !message.IsValidEntityID(entityID) {
		return "", fmt.Errorf("request_sandbox: constructed chain entity id %q failed IsValidEntityID", entityID)
	}
	return entityID, nil
}

// parseArgs delegates to sandboxmanager.ParseRequirements — the
// shared parser between this tool and query_sandbox_attestation.
// Co-located there so the SandboxRequirements type and its decoder
// stay together; tools just call it.
func parseArgs(raw map[string]any) (sandboxmanager.SandboxRequirements, error) {
	return sandboxmanager.ParseRequirements(raw)
}

// attestationWire is the JSON shape returned to the LLM. Mirrors
// Attestation but flattens the time.Time and ignores TTL (in
// seconds for the wire) — keeps the LLM-facing surface small.
// Coordinator persona prose substitutes named fields via
// $entity.triple.sandbox.attestation.<predicate> against the chain
// entity; the Content body is for routing decisions.
type attestationWire struct {
	Profile          string            `json:"profile"`
	ImageDigest      string            `json:"image_digest,omitempty"`
	Ready            bool              `json:"ready"`
	Degraded         bool              `json:"degraded"`
	Terminal         bool              `json:"terminal"`
	TerminalReason   string            `json:"terminal_reason,omitempty"`
	AdmissionOutcome string            `json:"admission_outcome"`
	AdmissionReasons []string          `json:"admission_reasons,omitempty"`
	DegradedReasons  []string          `json:"degraded_reasons,omitempty"`
	FailedProbes     []string          `json:"failed_probes,omitempty"`
	Verified         map[string]string `json:"verified,omitempty"`
	RequirementsHash string            `json:"requirements_hash"`
	Signature        string            `json:"signature"`
	AttestedAt       string            `json:"attested_at"`
	TTLSeconds       int64             `json:"ttl_seconds"`
}

func attestationToWire(a sandboxmanager.Attestation) attestationWire {
	return attestationWire{
		Profile:          a.ProfileID,
		ImageDigest:      a.ImageDigest,
		Ready:            a.Ready,
		Degraded:         a.Degraded,
		Terminal:         a.Terminal,
		TerminalReason:   a.TerminalReason,
		AdmissionOutcome: string(a.AdmissionOutcome),
		AdmissionReasons: a.AdmissionReasons,
		DegradedReasons:  a.DegradedReasons,
		FailedProbes:     a.FailedProbes,
		Verified:         a.Verified,
		RequirementsHash: a.RequirementsHash,
		Signature:        a.Signature,
		AttestedAt:       a.AttestedAt.Format("2006-01-02T15:04:05.999999999Z07:00"),
		TTLSeconds:       int64(a.TTL.Seconds()),
	}
}

func errResult(call agentic.ToolCall, kind agentic.ToolErrorKind, format string, args ...any) agentic.ToolResult {
	return agentic.ToolResult{
		CallID:    call.ID,
		Name:      call.Name,
		Error:     fmt.Sprintf(format, args...),
		ErrorKind: kind,
	}
}
