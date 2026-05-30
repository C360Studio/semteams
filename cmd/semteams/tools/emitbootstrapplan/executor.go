// Package emitbootstrapplan implements the SemTeams-local
// emit_bootstrap_plan tool — the producer side of the
// sandbox-bootstrap pack's plan-emission contract.
//
// The tool is the terminal call of the
// provisioner-bootstrap-plan persona's iteration 5 (per
// configs/rules/sandbox-bootstrap/01-coordinator-bootstrap-spawn.json
// iteration 5): given typed plan fields supplied by the LLM, the
// executor canonicalizes the target via sandboxfleet.Canonicalize,
// computes a deterministic plan_hash over the provisioning fields,
// stamps the sandbox.tenant.* triples on the run entity (the
// coordinator's loop entity, picked from related_loops), and
// transitions the registry to provisioning state (for provision /
// reprovision paths; skip preserves the cached ready state).
//
// Discipline note (framework-alignment review per
// cmd/semteams/tools/README.md): no upstream tenant-fleet primitive
// exists or is planned (semstreams beta.86, 2026-05-29). Defensible
// product-shell-local. Migration target documented alongside the
// sandboxfleet package per ADR-042 §addendum 2026-05-29 §A.
package emitbootstrapplan

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log/slog"
	"sort"
	"strings"
	"time"

	"github.com/c360studio/semstreams/agentic"
	"github.com/c360studio/semstreams/message"
	agentictools "github.com/c360studio/semstreams/processor/agentic-tools"
	"github.com/c360studio/semstreams/types"

	"github.com/c360studio/semteams/cmd/semteams/sandboxfleet"
)

// ToolName is the LLM-facing tool name.
const ToolName = "emit_bootstrap_plan"

// toolSource tags the triples this tool publishes.
const toolSource = "sandbox-bootstrap-emit-plan"

// runEntityRoleKey is the related_loops key the spawn rule sets to
// the run entity's 6-part ID. Per rule 01:
//
//	"related_loops": { "run-loop-entity-id": "$entity.id" }
//
// The framework propagates this into ToolCall.Metadata under
// MetadataKeyRelatedLoops. The plan tool reads it directly — no
// chain.Resolver walk needed because the spawn rule already pinned
// the run entity at chain start.
const runEntityRoleKey = "run-loop-entity-id"

// Plan action vocabulary. Pinned in code so the executor can branch
// on registry-mutation behavior; persona prompts reference the same
// string values.
const (
	ActionProvision   = "provision"
	ActionReprovision = "reprovision"
	ActionSkip        = "skip"
)

// Predicate constants on the run entity. Re-exported from
// sandboxfleet for the "tenant" cluster so callers (tests, rule-pack
// reviewers) have one place to look. Add new predicates here only
// if the rule pack contracts on them.
const (
	predicateSignature              = sandboxfleet.PredicateSignature
	predicateContainerName          = sandboxfleet.PredicateContainerName
	predicateImage                  = sandboxfleet.PredicateImage
	predicateWorkspace              = sandboxfleet.PredicateWorkspace
	predicatePlanHash               = sandboxfleet.PredicatePlanHash
	predicatePlanAction             = sandboxfleet.PredicatePlanAction
	predicatePlanCloneCommand       = sandboxfleet.PredicatePlanCloneCommand
	predicatePlanInstallSteps       = sandboxfleet.PredicatePlanInstallSteps
	predicatePlanVolumeMounts       = sandboxfleet.PredicatePlanVolumeMounts
	predicatePlanDockerSocketMount  = sandboxfleet.PredicatePlanDockerSocketMount
	predicatePlanVerifyCommand      = sandboxfleet.PredicatePlanVerifyCommand
	predicatePlanExpectedSmoke      = sandboxfleet.PredicatePlanExpectedSmoke
	predicatePlanForceRefresh       = sandboxfleet.PredicatePlanForceRefresh
	predicatePlanRevision           = sandboxfleet.PredicatePlanRevision
	predicatePlanCanonicalCommand   = sandboxfleet.PredicatePlanCanonicalCommand
	predicatePlanCanonicalRepoURL   = sandboxfleet.PredicatePlanCanonicalRepoURL
	predicatePlanCanonicalRepoRef   = sandboxfleet.PredicatePlanCanonicalRepoRef
	predicatePlanCanonicalBaseImage = sandboxfleet.PredicatePlanCanonicalBaseImage
	predicatePlanCanonicalToolchain = sandboxfleet.PredicatePlanCanonicalToolchain
	predicatePlanStampedAt          = sandboxfleet.PredicatePlanStampedAt
)

// Registry is the narrow registry-mutation surface this executor
// uses. Production wiring supplies the concrete
// sandboxfleet.TenantRegistry. The interface is here so unit tests
// can verify the executor's call shape against a stub.
type Registry interface {
	MarkProvisioning(ctx context.Context, sig sandboxfleet.TargetSignature, planHash string) error
}

// Executor implements agentic.ToolExecutor for emit_bootstrap_plan.
type Executor struct {
	publisher agentictools.TriplePublisher
	registry  Registry
	platform  types.PlatformMeta
	logger    *slog.Logger
}

// NewExecutor constructs an Executor. All deps must be non-nil
// (this is boot-required wiring; foundation PR has no
// graceful-degradation mode). Platform is required so the executor
// can construct the calling loop's 6-part entity ID when dual-
// stamping triples (PR 3.1 wiring fix — see Execute for the why).
func NewExecutor(publisher agentictools.TriplePublisher, registry Registry, platform types.PlatformMeta, logger *slog.Logger) *Executor {
	if publisher == nil {
		panic("emitbootstrapplan.NewExecutor: publisher must not be nil")
	}
	if registry == nil {
		panic("emitbootstrapplan.NewExecutor: registry must not be nil")
	}
	if platform.Org == "" || platform.Platform == "" {
		panic("emitbootstrapplan.NewExecutor: platform.Org and platform.Platform must be set for calling-loop entity ID construction")
	}
	if logger == nil {
		logger = slog.Default()
	}
	return &Executor{publisher: publisher, registry: registry, platform: platform, logger: logger}
}

// ListTools returns the LLM-facing schema. Persona supplies typed
// fields; the tool canonicalizes the signature and computes
// plan_hash. Server-derived: signature, container_name, plan_hash,
// stamped_at.
func (e *Executor) ListTools() []agentic.ToolDefinition {
	return []agentic.ToolDefinition{{
		Name: ToolName,
		Description: "Emit the provisioner-bootstrap-plan's terminal plan. Canonicalizes the target signature server-side (cache-key correctness invariant); computes plan_hash from the provisioning fields; stamps sandbox.tenant.* triples on the run entity; transitions registry state to provisioning for provision/reprovision (skip preserves the cached ready state). " +
			"Call exactly once per plan-persona pass before terminating with decide(action=execute|skip).",
		Parameters: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"command":                  map[string]any{"type": "string", "description": "Measurement / smoke command verbatim (e.g. 'task test:integration')."},
				"repo_url":                 map[string]any{"type": "string", "description": "Source repo URL (ssh or https). Empty for self-contained targets."},
				"repo_ref":                 map[string]any{"type": "string", "description": "Source ref (SHA, tag, or branch). Required when repo_url is set."},
				"toolchain":                map[string]any{"type": "object", "description": "Toolchain version map (e.g. {\"go\":\"1.26\"})."},
				"base_image":               map[string]any{"type": "string", "description": "Docker image:tag for the tenant base (no :latest)."},
				"clone_command":            map[string]any{"type": "string", "description": "Full `git clone <url> <workspace>` (or 'none' for no-clone)."},
				"install_steps":            map[string]any{"type": "array", "items": map[string]any{"type": "string"}, "description": "Ordered shell commands to install deps inside the tenant. One step per line."},
				"volume_mounts":            map[string]any{"type": "array", "items": map[string]any{"type": "string"}, "description": "Docker volume mounts (e.g. 'semteams-tenant-<sig>-workspace:/workspace')."},
				"docker_socket_mount":      map[string]any{"type": "boolean", "description": "true if the target's measurement needs Docker (testcontainers, etc.). Conservative gate; default false."},
				"verify_command":           map[string]any{"type": "string", "description": "Fast smoke command (<60s) the verify persona runs to grade provisioning."},
				"expected_smoke_signature": map[string]any{"type": "string", "description": "What the verify persona matches against (exit code, stdout substring, etc.)."},
				"plan_action":              map[string]any{"type": "string", "enum": []string{ActionProvision, ActionReprovision, ActionSkip}, "description": "provision (fresh), reprovision (rm + provision; for stale tenants), or skip (registry hit + fresh)."},
				"force_refresh":            map[string]any{"type": "boolean", "description": "If true, bypass registry freshness check (operator-explicit rebuild)."},
				"plan_revision":            map[string]any{"type": "integer", "minimum": 1, "description": "Monotonic across reviewer-rejection retries. 1 on first pass."},
				"workspace":                map[string]any{"type": "string", "description": "Container-internal workspace path. Typically /workspace. Optional; defaults to /workspace."},
			},
			"required": []string{"command", "base_image", "plan_action"},
		},
	}}
}

// Execute parses, canonicalizes, stamps triples, transitions registry.
func (e *Executor) Execute(ctx context.Context, call agentic.ToolCall) (agentic.ToolResult, error) {
	if call.Name != ToolName {
		return errResult(call, agentic.ToolErrorNotFound, "unknown tool: %s", call.Name)
	}

	args, err := parseArgs(call.Arguments)
	if err != nil {
		return errResult(call, agentic.ToolErrorInvalidArgs, "%v", err)
	}

	sig, err := sandboxfleet.Canonicalize(args.canonicalizeInput())
	if err != nil {
		return errResult(call, agentic.ToolErrorInvalidArgs, "canonicalize: %v", err)
	}

	planHash := args.planHash(sig)

	runEntityID, err := runEntityFromCall(call)
	if err != nil {
		return errResult(call, agentic.ToolErrorInternal, "%v", err)
	}

	// PR 3.1 dual-target stamping: build sandbox.tenant.* triples on
	// (a) the run entity — authoritative for the registry namespace,
	// unchanged from PR 1, AND (b) the calling loop's entity (the
	// plan loop). The downstream spawn rule (sandbox-bootstrap/02b)
	// fires with the plan loop as its trigger entity; its prompt
	// substitutes `$entity.triple.sandbox.tenant.*` which resolves
	// against the trigger entity per upstream semantics
	// (processor/rule/execution_context.go:595). Without the dual
	// stamp the substitution leaves literal tokens in execute's
	// spawn prompt; smoke #9 run-1 surfaced this as the load-bearing
	// plan→execute hop wedge (Finding 1 in [[smoke9-findings]]).
	//
	// The duplication is bounded: per-arc, per-plan-call. Subsequent
	// rules that read sandbox.tenant.* from the run entity (the
	// registry, the wake-up coordinator) continue to use the run
	// entity stamp as the source of truth.
	callingLoopEntityID, err := agentic.TryLoopExecutionEntityID(e.platform.Org, e.platform.Platform, call.LoopID)
	if err != nil {
		return errResult(call, agentic.ToolErrorInternal, "construct calling-loop entity ID from loop_id %q: %v", call.LoopID, err)
	}

	triples := args.triples(runEntityID, sig, planHash)
	triples = append(triples, args.triples(callingLoopEntityID, sig, planHash)...)
	if err := e.publisher.AddTriplesBatch(ctx, triples); err != nil {
		return errResult(call, agentic.ToolErrorNetwork, "stamp plan triples on %s + %s: %v", runEntityID, callingLoopEntityID, err)
	}

	// Registry mutation: provision/reprovision → MarkProvisioning;
	// skip → no-op (preserve cached ready state per the rule pack's
	// skip contract). The plan persona's skip path still routes
	// through verify; if verify fails (container_missing or
	// smoke_failed), the reviewer recovery loop kicks in.
	if args.PlanAction == ActionProvision || args.PlanAction == ActionReprovision {
		if err := e.registry.MarkProvisioning(ctx, sig, planHash); err != nil {
			return errResult(call, agentic.ToolErrorNetwork, "mark provisioning: %v", err)
		}
	}

	body, _ := json.Marshal(map[string]any{
		"signature":      sig.Hash(),
		"container_name": sig.ContainerName(),
		"plan_hash":      planHash,
		"plan_action":    args.PlanAction,
		"run_entity_id":  runEntityID,
		"workspace":      args.Workspace,
	})

	e.logger.Info("emit_bootstrap_plan stamped",
		slog.String("run_entity_id", runEntityID),
		slog.String("signature", sig.Hash()),
		slog.String("plan_hash", planHash),
		slog.String("plan_action", args.PlanAction))

	return agentic.ToolResult{
		CallID:  call.ID,
		Name:    call.Name,
		Content: string(body),
		Metadata: map[string]any{
			"signature":      sig.Hash(),
			"plan_hash":      planHash,
			"container_name": sig.ContainerName(),
		},
	}, nil
}

// runEntityFromCall reads the run entity ID the spawn rule pinned in
// related_loops["run-loop-entity-id"]. Error if absent — the tool
// has no fallback that produces a coherent run-entity stamp (writing
// to the calling plan loop pollutes the lineage).
func runEntityFromCall(call agentic.ToolCall) (string, error) {
	related, _ := call.Metadata[agentic.MetadataKeyRelatedLoops].(map[string]any)
	if v, ok := related[runEntityRoleKey].(string); ok && v != "" {
		return v, nil
	}
	return "", fmt.Errorf("emit_bootstrap_plan: related_loops[%q] missing or empty in call metadata; spawn rule must pin run-loop-entity-id at chain start", runEntityRoleKey)
}

// parsedArgs is the post-decode plan shape. Fields use the JSON tag
// the LLM sent; defaults landed in parseArgs / Validate.
type parsedArgs struct {
	Command                string
	RepoURL                string
	RepoRef                string
	Toolchain              map[string]string
	BaseImage              string
	CloneCommand           string
	InstallSteps           []string
	VolumeMounts           []string
	DockerSocketMount      bool
	VerifyCommand          string
	ExpectedSmokeSignature string
	PlanAction             string
	ForceRefresh           bool
	PlanRevision           int
	Workspace              string
}

func parseArgs(raw map[string]any) (*parsedArgs, error) {
	p := &parsedArgs{
		PlanRevision: 1,
		Workspace:    "/workspace",
	}

	if v, ok := raw["command"].(string); ok {
		p.Command = v
	}
	if v, ok := raw["repo_url"].(string); ok {
		p.RepoURL = v
	}
	if v, ok := raw["repo_ref"].(string); ok {
		p.RepoRef = v
	}
	if v, ok := raw["base_image"].(string); ok {
		p.BaseImage = v
	}
	if tc, ok := raw["toolchain"].(map[string]any); ok {
		p.Toolchain = make(map[string]string, len(tc))
		for k, v := range tc {
			s, ok := v.(string)
			if !ok {
				return nil, fmt.Errorf("toolchain[%s]: expected string, got %T", k, v)
			}
			p.Toolchain[k] = s
		}
	}
	if v, ok := raw["clone_command"].(string); ok {
		p.CloneCommand = v
	}
	if v, ok := raw["install_steps"].([]any); ok {
		for i, s := range v {
			str, ok := s.(string)
			if !ok {
				return nil, fmt.Errorf("install_steps[%d]: expected string, got %T", i, s)
			}
			p.InstallSteps = append(p.InstallSteps, str)
		}
	}
	if v, ok := raw["volume_mounts"].([]any); ok {
		for i, s := range v {
			str, ok := s.(string)
			if !ok {
				return nil, fmt.Errorf("volume_mounts[%d]: expected string, got %T", i, s)
			}
			p.VolumeMounts = append(p.VolumeMounts, str)
		}
	}
	if v, ok := raw["docker_socket_mount"].(bool); ok {
		p.DockerSocketMount = v
	}
	if v, ok := raw["verify_command"].(string); ok {
		p.VerifyCommand = v
	}
	if v, ok := raw["expected_smoke_signature"].(string); ok {
		p.ExpectedSmokeSignature = v
	}
	if v, ok := raw["plan_action"].(string); ok {
		p.PlanAction = v
	}
	if v, ok := raw["force_refresh"].(bool); ok {
		p.ForceRefresh = v
	}
	if v, ok := raw["plan_revision"].(float64); ok && v > 0 {
		p.PlanRevision = int(v)
	}
	if v, ok := raw["workspace"].(string); ok && strings.TrimSpace(v) != "" {
		p.Workspace = v
	}

	if err := p.validate(); err != nil {
		return nil, err
	}
	return p, nil
}

func (p *parsedArgs) validate() error {
	// The plan_action enum is declared in ListTools' JSON schema
	// AND validated here in Go. Defense-in-depth: backends that
	// honor strict-mode JSON schemas (Anthropic strict, OpenAI
	// strict tool_call) reject invalid enum values at the model-
	// output stage; non-strict backends (text-only fallbacks,
	// older models) can still hand us garbage and we surface it
	// as ToolErrorInvalidArgs instead of a panic or a silent miss-
	// stamp downstream. Mirror the validation in both places.
	switch p.PlanAction {
	case ActionProvision, ActionReprovision, ActionSkip:
		// ok
	default:
		return fmt.Errorf("plan_action %q: must be one of provision|reprovision|skip", p.PlanAction)
	}
	if p.PlanAction != ActionSkip {
		if p.VerifyCommand == "" {
			return fmt.Errorf("verify_command required for plan_action=%s", p.PlanAction)
		}
		if p.ExpectedSmokeSignature == "" {
			return fmt.Errorf("expected_smoke_signature required for plan_action=%s", p.PlanAction)
		}
	}
	return nil
}

func (p *parsedArgs) canonicalizeInput() sandboxfleet.CanonicalizeInput {
	return sandboxfleet.CanonicalizeInput{
		Command:   p.Command,
		RepoURL:   p.RepoURL,
		RepoRef:   p.RepoRef,
		Toolchain: p.Toolchain,
		BaseImage: p.BaseImage,
	}
}

// planHash hashes the provisioning fields that affect cache validity:
// canonical base_image, clone_command, canonical install_steps
// (sorted), canonical volume_mounts (sorted), docker_socket_mount,
// pinned to the canonical signature hash. Toolchain + repo are
// already in the signature (the outer cache key); plan_hash is the
// inner key that distinguishes "same target, different recipe."
//
// Workspace deliberately omitted from the hash — per reviewer H3
// 2026-05-29, workspace is a provisioning SIDE-OUTPUT (the
// container-internal mount point) not a provisioning INPUT, and
// including the raw string in the hash makes "/workspace",
// "/workspace/", and "//workspace" produce distinct plan_hashes
// without semantic meaning. Workspace lands as an audit triple
// (sandbox.tenant.workspace) but not in the cache key.
//
// Stable across runs: sorted-key marshal of a sorted-input struct.
func (p *parsedArgs) planHash(sig sandboxfleet.TargetSignature) string {
	type planHashInput struct {
		BaseImage         string   `json:"base_image"`
		CloneCommand      string   `json:"clone_command,omitempty"`
		InstallSteps      []string `json:"install_steps,omitempty"`
		VolumeMounts      []string `json:"volume_mounts,omitempty"`
		DockerSocketMount bool     `json:"docker_socket_mount"`
		// Pin to the canonical signature so two prose-framings of
		// the same target with the same recipe produce the same
		// plan_hash too.
		SignatureHash string `json:"signature"`
	}
	in := planHashInput{
		BaseImage:         sig.BaseImage,
		CloneCommand:      p.CloneCommand,
		InstallSteps:      sortedCopy(p.InstallSteps),
		VolumeMounts:      sortedCopy(p.VolumeMounts),
		DockerSocketMount: p.DockerSocketMount,
		SignatureHash:     sig.Hash(),
	}
	b, _ := json.Marshal(in)
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:])
}

func sortedCopy(in []string) []string {
	if len(in) == 0 {
		return nil
	}
	out := append([]string(nil), in...)
	sort.Strings(out)
	return out
}

// triples builds the deterministic predicate set for the run entity.
// Workspace is derived from the parsed input (defaulting to
// "/workspace"); ContainerName from the canonical signature.
func (p *parsedArgs) triples(runEntityID string, sig sandboxfleet.TargetSignature, planHash string) []message.Triple {
	now := time.Now().UTC()
	base := func(pred string, obj any) message.Triple {
		return message.Triple{
			Subject:    runEntityID,
			Predicate:  pred,
			Object:     obj,
			Source:     toolSource,
			Timestamp:  now,
			Confidence: 1.0,
		}
	}

	installStepsJSON, _ := json.Marshal(p.InstallSteps)
	volumeMountsJSON, _ := json.Marshal(p.VolumeMounts)
	toolchainJSON, _ := json.Marshal(sig.Toolchain)

	out := []message.Triple{
		base(predicateSignature, sig.Hash()),
		base(predicateContainerName, sig.ContainerName()),
		base(predicateImage, sig.BaseImage),
		base(predicateWorkspace, p.Workspace),
		base(predicatePlanHash, planHash),
		base(predicatePlanAction, p.PlanAction),
		base(predicatePlanRevision, p.PlanRevision),
		base(predicatePlanStampedAt, now.Format(time.RFC3339Nano)),

		// Canonical signature fields for audit. Persona supplies
		// raw; tool stamps canonical so consumers see what the
		// hash was actually computed over.
		base(predicatePlanCanonicalCommand, sig.Command),
		base(predicatePlanCanonicalBaseImage, sig.BaseImage),
		base(predicatePlanCanonicalToolchain, string(toolchainJSON)),

		// Recipe fields.
		base(predicatePlanCloneCommand, p.CloneCommand),
		base(predicatePlanInstallSteps, string(installStepsJSON)),
		base(predicatePlanVolumeMounts, string(volumeMountsJSON)),
		base(predicatePlanDockerSocketMount, p.DockerSocketMount),
		base(predicatePlanForceRefresh, p.ForceRefresh),
	}

	// Conditional: repo fields only when present (skip path may
	// omit them; canonicalizer enforces consistency).
	if sig.RepoURL != "" {
		out = append(out, base(predicatePlanCanonicalRepoURL, sig.RepoURL))
		out = append(out, base(predicatePlanCanonicalRepoRef, sig.RepoRef))
	}
	if p.VerifyCommand != "" {
		out = append(out, base(predicatePlanVerifyCommand, p.VerifyCommand))
	}
	if p.ExpectedSmokeSignature != "" {
		out = append(out, base(predicatePlanExpectedSmoke, p.ExpectedSmokeSignature))
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
