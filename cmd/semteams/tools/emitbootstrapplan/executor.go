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

// ListTools returns the LLM-facing schema. PR 3.3: persona supplies
// STRUCTURED INTENT — source kind, dependency intent (apt/go_mod/
// npm_ci/pip_install/raw), mount intent (volume_suffix + path), smoke
// expects (exit_code + stdout_contains). The tool's Go composer
// (sandboxfleet.Compose) deterministically renders those into the
// verbatim shell strings stamped on the run + plan-loop entities, so
// the rule-02b → execute persona contract is unchanged downstream.
//
// CLI grammar (git --branch ordering, apt-get -y --no-install-
// recommends, docker volume-name conventions) lives in the composer,
// NOT in persona prose. See [[personas-should-not-author-shell]] +
// ADR-042 §addendum PR 3.3 for the rationale.
//
// Server-derived: signature, container_name, plan_hash, clone_command,
// install_steps, volume_mounts, expected_smoke_signature, stamped_at.
func (e *Executor) ListTools() []agentic.ToolDefinition {
	return []agentic.ToolDefinition{{
		Name: ToolName,
		Description: "Emit the provisioner-bootstrap-plan's terminal plan as STRUCTURED INTENT. Canonicalizes the target signature server-side, composes provisioning shell from typed fields (no LLM-authored shell strings — the composer owns CLI grammar), stamps sandbox.tenant.* triples on the run entity AND the calling loop entity, transitions registry state to provisioning for provision/reprovision (skip preserves the cached ready state). " +
			"Call exactly once per plan-persona pass before terminating with decide(action=execute|skip).",
		Parameters: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"command":    map[string]any{"type": "string", "description": "Measurement / smoke command verbatim (e.g. 'task test:integration'). Signature input."},
				"repo_url":   map[string]any{"type": "string", "description": "Source repo URL (ssh or https). Empty for self-contained targets. Signature input."},
				"repo_ref":   map[string]any{"type": "string", "description": "Source ref (SHA, tag, or branch). Required when repo_url is set. Signature input."},
				"toolchain":  map[string]any{"type": "object", "description": "Toolchain version map (e.g. {\"go\":\"1.26\"}). Signature input."},
				"base_image": map[string]any{"type": "string", "description": "Docker image:tag for the tenant base (no :latest). Signature input."},
				"source": map[string]any{
					"type":        "object",
					"description": "Source intent. kind=git clones repo_url@repo_ref into workspace (composer emits shallow + single-branch by default; depth/all_branches are explicit opt-outs). kind=none leaves workspace empty.",
					"properties": map[string]any{
						"kind":         map[string]any{"type": "string", "enum": []string{"git", "none"}, "description": "git | none. Default git when repo_url present, else none."},
						"depth":        map[string]any{"type": "integer", "description": "Optional: 0 = default shallow (1), negative = full clone (omit --depth), positive = literal depth."},
						"all_branches": map[string]any{"type": "boolean", "description": "Optional: true omits --single-branch (fetch all refs). Default false."},
					},
				},
				"dependencies": map[string]any{
					"type":        "array",
					"description": "Ordered install intent. Composer emits one shell line per entry with idempotency flags (apt-get -y --no-install-recommends, pip --no-cache-dir, npm ci --no-audit, etc.). Persona owns order; composer owns flags.",
					"items": map[string]any{
						"type": "object",
						"properties": map[string]any{
							"kind":     map[string]any{"type": "string", "enum": []string{"apt", "go_mod_download", "npm_ci", "pip_install", "toolchain_go", "toolchain_node", "raw"}, "description": "Dependency shape. raw is an escape hatch — use sparingly; recurring raw uses motivate adding a structured kind."},
							"packages": map[string]any{"type": "array", "items": map[string]any{"type": "string"}, "description": "apt: package list. Sorted server-side for stable hashing."},
							"manifest": map[string]any{"type": "string", "description": "pip_install: requirements.txt path OR a pip CLI spec starting with '-' (e.g. '-e .[test]')."},
							"command":  map[string]any{"type": "string", "description": "raw: verbatim shell line. Last-resort escape hatch."},
						},
						"required": []string{"kind"},
					},
				},
				"mounts": map[string]any{
					"type":        "array",
					"description": "Volume mounts. Composer derives the full volume name from the signature prefix (semteams-tenant-<prefix>-<volume_suffix>).",
					"items": map[string]any{
						"type": "object",
						"properties": map[string]any{
							"volume_suffix": map[string]any{"type": "string", "description": "Suffix differentiator (e.g. 'workspace', 'deps'). Must match [a-z0-9-]+."},
							"path":          map[string]any{"type": "string", "description": "In-container mount path."},
						},
						"required": []string{"volume_suffix", "path"},
					},
				},
				"docker_socket_mount": map[string]any{"type": "boolean", "description": "true if the target's measurement needs Docker (testcontainers, etc.). Conservative gate; default false."},
				"smoke": map[string]any{
					"type":        "object",
					"description": "Verify-phase smoke contract. command is the actual shell line verify will run (legitimately LLM-authored — IS the user-intent unit of work). expects encodes the grading rule structurally.",
					"properties": map[string]any{
						"command": map[string]any{"type": "string", "description": "Fast smoke command (<60s) the verify persona runs."},
						"expects": map[string]any{
							"type": "object",
							"properties": map[string]any{
								"exit_code":       map[string]any{"type": "integer", "description": "Required exit status. Default 0."},
								"stdout_contains": map[string]any{"type": "string", "description": "Optional substring match on stdout."},
							},
						},
					},
				},
				"plan_action":   map[string]any{"type": "string", "enum": []string{ActionProvision, ActionReprovision, ActionSkip}, "description": "provision (fresh), reprovision (rm + provision; for stale tenants), or skip (registry hit + fresh)."},
				"force_refresh": map[string]any{"type": "boolean", "description": "If true, bypass registry freshness check (operator-explicit rebuild)."},
				"plan_revision": map[string]any{"type": "integer", "minimum": 1, "description": "Monotonic across reviewer-rejection retries. 1 on first pass."},
				"workspace":     map[string]any{"type": "string", "description": "Container-internal workspace path. Typically /workspace. Optional; defaults to /workspace."},
			},
			"required": []string{"command", "base_image", "plan_action"},
		},
	}}
}

// Execute parses, canonicalizes, composes shell, stamps triples,
// transitions registry. The structured intent → shell composition
// happens in sandboxfleet.Compose; the executor stamps the composed
// shell strings on the run + plan-loop entities so downstream
// substitution (rule 02b → execute persona) reads literal commands.
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

	recipe, err := sandboxfleet.Compose(args.recipeIntent(), sig, args.Workspace)
	if err != nil {
		return errResult(call, agentic.ToolErrorInvalidArgs, "compose recipe: %v", err)
	}

	planHash := args.planHash(sig, recipe)

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

	triples := args.triples(runEntityID, sig, recipe, planHash)
	triples = append(triples, args.triples(callingLoopEntityID, sig, recipe, planHash)...)
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

// parsedArgs is the post-decode plan shape. PR 3.3 split: the
// signature inputs (Command, RepoURL, RepoRef, Toolchain, BaseImage,
// PlanAction, ForceRefresh, PlanRevision, Workspace,
// DockerSocketMount) stay flat; the recipe intent (Source,
// Dependencies, Mounts, Smoke) lives in a nested struct mirrored on
// sandboxfleet.RecipeIntent and renders through Compose.
type parsedArgs struct {
	Command           string
	RepoURL           string
	RepoRef           string
	Toolchain         map[string]string
	BaseImage         string
	Source            sandboxfleet.SourceIntent
	Dependencies      []sandboxfleet.Dependency
	Mounts            []sandboxfleet.Mount
	Smoke             sandboxfleet.SmokeIntent
	DockerSocketMount bool
	PlanAction        string
	ForceRefresh      bool
	PlanRevision      int
	Workspace         string
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
	if err := parseSource(raw, p); err != nil {
		return nil, err
	}
	if err := parseDependencies(raw, p); err != nil {
		return nil, err
	}
	if err := parseMounts(raw, p); err != nil {
		return nil, err
	}
	if err := parseSmoke(raw, p); err != nil {
		return nil, err
	}
	if v, ok := raw["docker_socket_mount"].(bool); ok {
		p.DockerSocketMount = v
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

	// Source kind defaulting: zero-value kind with a repo_url
	// supplied implies git; without a repo_url, none. Persona can
	// always override explicitly.
	if p.Source.Kind == "" {
		if strings.TrimSpace(p.RepoURL) != "" {
			p.Source.Kind = sandboxfleet.SourceKindGit
		} else {
			p.Source.Kind = sandboxfleet.SourceKindNone
		}
	}

	if err := p.validate(); err != nil {
		return nil, err
	}
	return p, nil
}

func parseSource(raw map[string]any, p *parsedArgs) error {
	srcRaw, present := raw["source"]
	if !present {
		return nil
	}
	src, ok := srcRaw.(map[string]any)
	if !ok {
		return fmt.Errorf("source: expected object, got %T", srcRaw)
	}
	if v, present := src["kind"]; present {
		s, ok := v.(string)
		if !ok {
			return fmt.Errorf("source.kind: expected string, got %T", v)
		}
		p.Source.Kind = sandboxfleet.SourceKind(s)
	}
	if v, present := src["depth"]; present {
		// Per PR 3.3 reviewer REC-5: error on wrong type instead of
		// silently dropping. A persona passing "depth": "shallow"
		// today would land Depth=0 (default), masking its intent.
		n, ok := v.(float64)
		if !ok {
			return fmt.Errorf("source.depth: expected integer, got %T", v)
		}
		p.Source.Depth = int(n)
	}
	if v, present := src["all_branches"]; present {
		b, ok := v.(bool)
		if !ok {
			return fmt.Errorf("source.all_branches: expected bool, got %T", v)
		}
		p.Source.AllBranches = b
	}
	return nil
}

func parseDependencies(raw map[string]any, p *parsedArgs) error {
	depsRaw, present := raw["dependencies"]
	if !present {
		return nil
	}
	deps, ok := depsRaw.([]any)
	if !ok {
		return fmt.Errorf("dependencies: expected array, got %T", depsRaw)
	}
	for i, d := range deps {
		m, ok := d.(map[string]any)
		if !ok {
			return fmt.Errorf("dependencies[%d]: expected object, got %T", i, d)
		}
		var dep sandboxfleet.Dependency
		if v, present := m["kind"]; present {
			s, ok := v.(string)
			if !ok {
				return fmt.Errorf("dependencies[%d].kind: expected string, got %T", i, v)
			}
			dep.Kind = sandboxfleet.DependencyKind(s)
		}
		if dep.Kind == "" {
			return fmt.Errorf("dependencies[%d]: kind required", i)
		}
		if pkgsRaw, present := m["packages"]; present {
			pkgs, ok := pkgsRaw.([]any)
			if !ok {
				return fmt.Errorf("dependencies[%d].packages: expected array, got %T", i, pkgsRaw)
			}
			for j, pkg := range pkgs {
				s, ok := pkg.(string)
				if !ok {
					return fmt.Errorf("dependencies[%d].packages[%d]: expected string, got %T", i, j, pkg)
				}
				dep.Packages = append(dep.Packages, s)
			}
		}
		if v, present := m["manifest"]; present {
			s, ok := v.(string)
			if !ok {
				return fmt.Errorf("dependencies[%d].manifest: expected string, got %T", i, v)
			}
			dep.Manifest = s
		}
		if v, present := m["command"]; present {
			s, ok := v.(string)
			if !ok {
				return fmt.Errorf("dependencies[%d].command: expected string, got %T", i, v)
			}
			dep.Command = s
		}
		p.Dependencies = append(p.Dependencies, dep)
	}
	return nil
}

func parseMounts(raw map[string]any, p *parsedArgs) error {
	mountsRaw, present := raw["mounts"]
	if !present {
		return nil
	}
	mounts, ok := mountsRaw.([]any)
	if !ok {
		return fmt.Errorf("mounts: expected array, got %T", mountsRaw)
	}
	for i, m := range mounts {
		obj, ok := m.(map[string]any)
		if !ok {
			return fmt.Errorf("mounts[%d]: expected object, got %T", i, m)
		}
		var mount sandboxfleet.Mount
		if v, present := obj["volume_suffix"]; present {
			s, ok := v.(string)
			if !ok {
				return fmt.Errorf("mounts[%d].volume_suffix: expected string, got %T", i, v)
			}
			mount.VolumeSuffix = s
		}
		if v, present := obj["path"]; present {
			s, ok := v.(string)
			if !ok {
				return fmt.Errorf("mounts[%d].path: expected string, got %T", i, v)
			}
			mount.Path = s
		}
		p.Mounts = append(p.Mounts, mount)
	}
	return nil
}

func parseSmoke(raw map[string]any, p *parsedArgs) error {
	smokeRaw, present := raw["smoke"]
	if !present {
		return nil
	}
	smoke, ok := smokeRaw.(map[string]any)
	if !ok {
		return fmt.Errorf("smoke: expected object, got %T", smokeRaw)
	}
	if v, present := smoke["command"]; present {
		s, ok := v.(string)
		if !ok {
			return fmt.Errorf("smoke.command: expected string, got %T", v)
		}
		p.Smoke.Command = s
	}
	if expRaw, present := smoke["expects"]; present {
		exp, ok := expRaw.(map[string]any)
		if !ok {
			return fmt.Errorf("smoke.expects: expected object, got %T", expRaw)
		}
		if v, present := exp["exit_code"]; present {
			n, ok := v.(float64)
			if !ok {
				return fmt.Errorf("smoke.expects.exit_code: expected integer, got %T", v)
			}
			p.Smoke.Expects.ExitCode = int(n)
		}
		if v, present := exp["stdout_contains"]; present {
			s, ok := v.(string)
			if !ok {
				return fmt.Errorf("smoke.expects.stdout_contains: expected string, got %T", v)
			}
			p.Smoke.Expects.StdoutContains = s
		}
	}
	return nil
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
		if strings.TrimSpace(p.Smoke.Command) == "" {
			return fmt.Errorf("smoke.command required for plan_action=%s", p.PlanAction)
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

// recipeIntent projects parsedArgs into the composer's input shape.
// DockerSocketMount mirrors through so future composer logic that
// branches on socket-need has a single read point.
func (p *parsedArgs) recipeIntent() sandboxfleet.RecipeIntent {
	return sandboxfleet.RecipeIntent{
		Source:            p.Source,
		Dependencies:      p.Dependencies,
		Mounts:            p.Mounts,
		Smoke:             p.Smoke,
		DockerSocketMount: p.DockerSocketMount,
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
// PR 3.3: hashes the COMPOSED shell strings (recipe.CloneCommand,
// recipe.InstallSteps, recipe.VolumeMounts), not the raw intent. The
// composer is deterministic over intent, so this is equivalent to
// hashing the intent + the composer version — and downstream
// consumers of plan_hash (the registry's cache-validity check)
// continue to see the same hash semantics they did pre-PR-3.3 when
// the persona authored shell directly.
//
// ComposerVersion is folded in (PR 3.3 reviewer REC-4) so that bumps
// to the composer's output semantics deterministically rotate
// cached plan_hashes. If we change the composer to (say) add a new
// idempotency flag, bumping sandboxfleet.ComposerVersion forces
// re-provision on the next arc that hits a cached tenant — even
// when the composed string happens to be identical for that arc's
// specific recipe.
func (p *parsedArgs) planHash(sig sandboxfleet.TargetSignature, recipe sandboxfleet.Recipe) string {
	type planHashInput struct {
		BaseImage         string   `json:"base_image"`
		CloneCommand      string   `json:"clone_command,omitempty"`
		InstallSteps      []string `json:"install_steps,omitempty"`
		VolumeMounts      []string `json:"volume_mounts,omitempty"`
		DockerSocketMount bool     `json:"docker_socket_mount"`
		// Pin to the canonical signature so two prose-framings of
		// the same target with the same recipe produce the same
		// plan_hash too.
		SignatureHash   string `json:"signature"`
		ComposerVersion int    `json:"composer_version"`
	}
	in := planHashInput{
		BaseImage:         sig.BaseImage,
		CloneCommand:      recipe.CloneCommand,
		InstallSteps:      sortedCopy(recipe.InstallSteps),
		VolumeMounts:      sortedCopy(recipe.VolumeMounts),
		DockerSocketMount: p.DockerSocketMount,
		SignatureHash:     sig.Hash(),
		ComposerVersion:   sandboxfleet.ComposerVersion,
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
// "/workspace"); ContainerName from the canonical signature. PR 3.3:
// recipe fields come from the composer, not from raw LLM input. The
// triple shape (predicate names + value types) is unchanged so
// downstream substitution and consumers continue to work without
// modification.
func (p *parsedArgs) triples(runEntityID string, sig sandboxfleet.TargetSignature, recipe sandboxfleet.Recipe, planHash string) []message.Triple {
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

	installStepsJSON, _ := json.Marshal(recipe.InstallSteps)
	volumeMountsJSON, _ := json.Marshal(recipe.VolumeMounts)
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

		// Recipe fields — composed from structured intent (PR 3.3).
		base(predicatePlanCloneCommand, recipe.CloneCommand),
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
	if smoke := strings.TrimSpace(p.Smoke.Command); smoke != "" {
		out = append(out, base(predicatePlanVerifyCommand, smoke))
	}
	if recipe.ExpectedSmokeSignature != "" {
		out = append(out, base(predicatePlanExpectedSmoke, recipe.ExpectedSmokeSignature))
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
