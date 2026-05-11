// Package emitspecartifact implements the SemTeams-local
// emit_dev_via_spec_artifact tool. This tool is the architect role's
// terminal action in the dev-via-spec arc (research → mode-transition →
// planner → reviewer → challenger → architect): it writes the structured
// Artifact to disk as a human-readable markdown spec, mints marker triples
// on the calling loop entity for downstream rule matching, and publishes the
// typed dev_via_spec.artifact.v1 payload on a stable NATS subject for audit.
//
// See cmd/semteams/devviaspec/artifact.go for the payload type and the
// ADR-031 §R3.3 addendum for the design rationale.
//
// Discipline note (commission-not-omission): this tool is the dev-via-spec
// domain instance of upstream's planned generic write_artifact suite
// (ADR-028 §What's not built here, verified absent in semstreams beta.36).
// It mirrors the canonical agent-terminal-tool emission shape of upstream
// `decide`, `emit_diagnosis`, and the sibling `emit_research_artifact` tool.
//
// If you are reading this because you want to:
//
//   - Add a similar emit-* tool for another domain → read
//     cmd/semteams/tools/README.md "Discipline" section first. Survey
//     upstream; if a near-equivalent ships in semstreams, port don't fork.
//   - Generalise across emit_research_artifact and this tool → STOP.
//     Migration target is upstream's planned write_artifact, not a
//     SemTeams-local generalisation. Open an ADR addendum when upstream ships.
//   - Stuff additional content into the marker triples → STOP. Triples carry
//     references and counts only. Content lives in the typed payload and the
//     rendered markdown file. See ADR-028 §Layer 2.
package emitspecartifact

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"text/template"
	"time"

	"github.com/c360studio/semstreams/agentic"
	"github.com/c360studio/semstreams/message"
	agentictools "github.com/c360studio/semstreams/processor/agentic-tools"
	"github.com/c360studio/semstreams/types"

	"github.com/c360studio/semteams/cmd/semteams/chain"
	"github.com/c360studio/semteams/cmd/semteams/devviaspec"
	"github.com/c360studio/semteams/cmd/semteams/slug"
	"github.com/c360studio/semteams/cmd/semteams/testharness"
	"github.com/c360studio/semteams/cmd/semteams/verification"
)

// TestHarnessResolver looks up a test-harness catalog entry by name. Returns
// nil + non-nil error on lookup failure (catalog miss, transport
// error). Used by the renderer to project the architect-cited
// test_harness NAME into the concrete fields the builder needs to
// transcribe (Image, TCP exposes) — without those fields rendered
// inline in SPEC.md, the builder has no in-sandbox path to resolve
// the catalog (the catalog file lives on the backend host, not in
// the builder's sandbox workspace). R3.7.2.h′ closes that gap.
//
// nil is permitted; the executor falls back to rendering the
// check without resolved fields (the architect's name still
// ships, just no image/port projection). Operators running
// dev-via-spec with no test-harness manager wired will see checks
// without resolution; operators running it WITH the manager will
// see checks fully grounded.
//
// The resolver is ALSO used for sidecar generation (ADR-036 Phase 1):
// when a check names a test_harness, the resolved manifest is embedded
// in <slug>.checks.json so bootstrapworkspace can project it into
// the builder's workspace as .test-harness/manifest.json without
// requiring in-sandbox catalog access. A resolver error on the sidecar
// path is fatal (unlike the markdown path) — it returns ToolErrorInvalidArgs
// with the specific catalog ID so the operator can fix the catalog entry
// before the builder loop boots.
type TestHarnessResolver func(ctx context.Context, name string) (*testharness.TestHarness, error)

// SidecarPayload is the on-disk shape of <slug>.checks.json written
// adjacent to <slug>.md by renderChecksJSON. It embeds the architect's
// check slice plus the resolved harness manifests for every unique
// test_harness reference appearing in any check.
//
// Harnesses is keyed by catalog ID (TestHarness.Name) — one entry per
// unique ID even when multiple checks reference the same test_harness.
// Empty/nil when no checks carry a test_harness reference.
//
// This file is read by bootstrapworkspace to seed:
//   - .evidence/checks.json — the checks slice for the evidence preprocessor (Slice 4).
//   - .test-harness/manifest.json — the harnesses map for the builder's Testcontainers config (ADR-036 §D2).
type SidecarPayload struct {
	Checks    []verification.Check                    `json:"checks"`
	Harnesses map[string]testharness.ResolvedManifest `json:"harnesses,omitempty"`
}

// ToolName is the LLM-facing tool name. Listed in agentic-tools
// allowed_tools per deployment that runs the dev-via-spec flow.
const ToolName = "emit_dev_via_spec_artifact"

// toolSource is the Source field on triples this tool publishes. Lets
// operators distinguish dev-via-spec emitter triples from other sources
// at a glance in graph queries.
const toolSource = "dev-via-spec-emit-artifact"

// payloadSubjectPrefix is the NATS subject namespace under which the
// typed Artifact payload is published. Per-loop suffix: one subject per
// architect loop. Stable so future audit subscribers can scope to a single
// arc. Core NATS (not JetStream) — same audit-trail pattern as emitartifact.
const payloadSubjectPrefix = "dev_via_spec.artifact"

// defaultOutputDir is the output directory when SEMTEAMS_DEVVIASPEC_ARTIFACT_DIR
// is unset. Relative to the process working directory (the repo root in normal
// operation).
const defaultOutputDir = "docs/specs"

// envOutputDir is the environment variable operators set to override the output
// directory. Kept as env var (not framework config) so it does not drift the
// upstream config schema — same pattern as emitartifact's sibling tools.
const envOutputDir = "SEMTEAMS_DEVVIASPEC_ARTIFACT_DIR"

// Predicate names stamped on the loop entity. Kept as string constants
// (rather than a separate vocabulary package) until a second consumer needs
// them — promote to vocabulary/devviaspec/ if a graph-query tool filters on
// them. Counts and references only — never free-text content (ADR-028 §Layer 2).
const (
	predicatePath                  = "dev_via_spec.artifact.path"
	predicateSlug                  = "dev_via_spec.artifact.slug"
	predicateGeneratedAt           = "dev_via_spec.artifact.generated_at"
	predicateActorCount            = "dev_via_spec.artifact.actor_count"
	predicateIntegrationPointCount = "dev_via_spec.artifact.integration_point_count"
	predicateTaskCount             = "dev_via_spec.artifact.task_count"
	predicateResearchRootLoop      = "dev_via_spec.artifact.research_root_loop"
	// predicateCheckCount is emitted regardless of whether checks
	// are populated (zero is a meaningful signal: "architect emitted but
	// claimed no verification surface"). R3.7.2.h's evidence gate fires
	// per-check, so the count is its branching predicate.
	predicateCheckCount = "dev_via_spec.artifact.check_count"

	// ADR-038 D2 spec_artifact milestone — predicates land on the chain
	// entity (cross-arc subject), not the loop entity. Cluster shape
	// matches the per-arc convention chain.<milestone>.<field>.
	chainPredicateSpecArtifactLoop       = "chain.spec_artifact.loop"
	chainPredicateSpecArtifactPath       = "chain.spec_artifact.path"
	chainPredicateSpecArtifactCheckCount = "chain.spec_artifact.check_count"

	// chainTripleSource tags chain entity triples this tool writes so a
	// graph query can scope to the emitter (parallel to chainpause's
	// "chainpause" and DispatchedStamper's "chain.dispatched").
	chainTripleSource = "chain.spec_artifact"
)

// PayloadPublisher is the narrow surface the executor uses to publish the
// typed Artifact payload onto a stable NATS subject. *natsclient.Client
// satisfies it via its core Publish. No JetStream ack required — forward-compat
// audit; if durability is needed operators can add a stream binding without
// touching this code. Named for parity with agentictools.TriplePublisher.
type PayloadPublisher interface {
	Publish(ctx context.Context, subject string, data []byte) error
}

// ChainResolver is the narrow surface the executor uses to derive the
// canonical 6-part chain entity ID for the running chain. cmd/semteams/chain
// .Resolver satisfies it structurally; tests inject a stub.
//
// Optional — leave unset to skip chain entity triple writes. ADR-038 PR B
// drift-safe phasing: existing dev_via_spec.artifact.* triples on the
// loop entity continue regardless; chain entity triples land on top
// when the resolver is wired.
type ChainResolver interface {
	ChainEntityID(ctx context.Context, loopID string) (string, error)
}

// ChainReader resolves the chain entity ID for the calling architect
// loop and reads its triple set in one call. Used by Execute to
// override LLM-supplied lineage IDs (provenance.{research_artifact_loop,
// planner_loop, reviewer_loop, challenger_loop}) and the slug with
// chain-canonical values (smoke #8 run-5 D1 + D2 fix).
//
// Optional — leave unset to keep the executor backward-compatible
// (LLM-supplied provenance + title-derived slug only). Same shape as
// emitplan.ChainReader / emitconsensus.ChainReader; production wiring
// uses the same adapter.
type ChainReader interface {
	ReadChainFor(ctx context.Context, fromLoopID string) (chainEntityID string, triples map[string]any, err error)
}

// Executor implements agentic.ToolExecutor for emit_dev_via_spec_artifact.
// It renders the Artifact as markdown to disk, writes a deterministic set
// of marker triples on the calling loop entity, and publishes the typed
// payload on a stable subject.
type Executor struct {
	publisher           agentictools.TriplePublisher
	natsPublish         PayloadPublisher
	platform            types.PlatformMeta
	logger              *slog.Logger
	outputDir           string
	testHarnessResolver TestHarnessResolver
	chainResolver       ChainResolver // nil → skip chain entity triples
	chainReader         ChainReader   // nil → fall back to LLM-supplied provenance + title-derived slug
}

// NewExecutor constructs an Executor. outputDir is the directory where
// rendered markdown specs are written; if empty the env var
// SEMTEAMS_DEVVIASPEC_ARTIFACT_DIR is checked, then the default "docs/specs".
// Injecting outputDir directly lets tests write to a temp directory without
// environment-variable side effects.
//
// resolver may be nil — the renderer falls back to rendering the
// architect's test_harness NAME without resolved fields (Image, TCP
// exposes). Production wiring (cmd/semteams/product_tools.go) passes
// the testharness manager's Get adapter; tests typically pass nil.
func NewExecutor(publisher agentictools.TriplePublisher, natsPublish PayloadPublisher, platform types.PlatformMeta, logger *slog.Logger, outputDir string, resolver TestHarnessResolver) *Executor {
	if logger == nil {
		logger = slog.Default()
	}
	if outputDir == "" {
		if envDir := strings.TrimSpace(os.Getenv(envOutputDir)); envDir != "" {
			outputDir = envDir
		} else {
			outputDir = defaultOutputDir
		}
	}
	return &Executor{
		publisher:           publisher,
		natsPublish:         natsPublish,
		platform:            platform,
		logger:              logger,
		outputDir:           outputDir,
		testHarnessResolver: resolver,
	}
}

// SetChainResolver enables ADR-038 chain entity triple writes alongside
// the existing loop-entity triples. Optional opt-in: leaving this unset
// keeps the executor backward-compatible (loop-entity triples only).
//
// Wired in cmd/semteams/product_tools.go after NewExecutor when the
// chain package's Resolver is built.
func (e *Executor) SetChainResolver(r ChainResolver) {
	e.chainResolver = r
}

// SetChainReader enables chain-entity-driven provenance and slug
// derivation. Mirrors the SetChainReader pattern used by emitplan and
// emitconsensus; same wiring contract. Smoke #8 run-5 D1 + D2 fix.
func (e *Executor) SetChainReader(r ChainReader) {
	e.chainReader = r
}

// ListTools returns the LLM-facing schema. The args mirror the Artifact
// JSON shape (excluding server-supplied fields). GeneratedAt and Slug are
// derived server-side; ArchitectLoop in Provenance is taken from ToolCall.LoopID.
func (e *Executor) ListTools() []agentic.ToolDefinition {
	actorSchema := map[string]any{
		"type": "object",
		"properties": map[string]any{
			"name": map[string]any{"type": "string", "description": "Actor name (e.g. \"OSH driver framework\")."},
			"role": map[string]any{"type": "string", "description": "One-line role description for this actor."},
		},
		"required": []string{"name", "role"},
	}
	integrationPointSchema := map[string]any{
		"type": "object",
		"properties": map[string]any{
			"from":      map[string]any{"type": "string", "description": "Source actor name."},
			"to":        map[string]any{"type": "string", "description": "Destination actor name."},
			"direction": map[string]any{"type": "string", "enum": []string{"read", "write"}, "description": "Direction of the data flow."},
			"data":      map[string]any{"type": "string", "description": "What data flows across this point."},
		},
		"required": []string{"from", "to"},
	}
	taskSchema := map[string]any{
		"type": "object",
		"properties": map[string]any{
			"title": map[string]any{"type": "string", "description": "Short task title."},
			"scope": map[string]any{"type": "string", "description": "Implementation scope (e.g. \"backend\", \"ui\", \"infra\")."},
			"grounds_actors": map[string]any{
				"type":        "array",
				"items":       map[string]any{"type": "string"},
				"description": "Actor names this task is grounded in.",
			},
			"grounds_integration_points": map[string]any{
				"type":        "array",
				"items":       map[string]any{"type": "string"},
				"description": "Freeform \"from→to\" strings identifying integration points this task touches.",
			},
		},
		"required": []string{"title", "scope"},
	}
	provenanceSchema := map[string]any{
		"type": "object",
		"properties": map[string]any{
			"research_artifact_loop": map[string]any{"type": "string", "description": "Loop ID of the research arc that produced the upstream artifact."},
			"planner_loop":           map[string]any{"type": "string", "description": "Loop ID of the approved planner pass."},
			"reviewer_loop":          map[string]any{"type": "string", "description": "Loop ID of the approving reviewer pass."},
			"challenger_loop":        map[string]any{"type": "string", "description": "Loop ID of the accepting challenger pass."},
		},
		"required": []string{"research_artifact_loop", "planner_loop", "reviewer_loop", "challenger_loop"},
	}

	// R3.7.2.b: checks[] is the architect's structured statement of WHAT is
	// verified, AGAINST WHAT, and with WHAT EVIDENCE. Each check fills a
	// verification surface (unit / testcontainer / sidecar / browser-flow /
	// static-analysis) and names a test_harness from configs/harnesses.json
	// when applicable. The architect persona contract (R3.7.2.f′,
	// configs/personas/fragments/dev-via-spec-architect/30-commitment-
	// contract.md) requires at least one check for any artifact whose
	// integration_points[] names an external actor. The field stays optional
	// at the wire level so v1 consumers see no schema drift; the
	// dvs-reviewer (R3.7.2.j′) enforces coverage adequacy.
	refSchema := map[string]any{
		"type": "object",
		"properties": map[string]any{
			"type": map[string]any{"type": "string", "enum": []string{"filepath", "template_id"}, "description": "Discriminates the union: filepath for brownfield (cite an existing test file in the repo); template_id for greenfield (name a framework-shipped template)."},
			"path": map[string]any{"type": "string", "description": "Workspace-relative path to the reference test file. Required when type=filepath; must be empty otherwise."},
			"id":   map[string]any{"type": "string", "description": "Framework-shipped template identifier. Required when type=template_id; must be empty otherwise."},
		},
		"required": []string{"type"},
	}
	evidenceRuleSchema := map[string]any{
		"type": "object",
		"properties": map[string]any{
			"kind": map[string]any{"type": "string", "description": "Evidence rule kind. Lowercase ASCII + digits + underscores, must start with a letter (matches ^[a-z][a-z0-9_]*$). Concrete checker kinds ship with the evidence-rule registry in R3.7.2.e."},
			"args": map[string]any{"type": "object", "description": "Kind-specific arguments. Shape is registry-validated at evidence-gate time; not constrained here."},
		},
		"required": []string{"kind"},
	}
	checkSchema := map[string]any{
		"type": "object",
		"properties": map[string]any{
			"target":       map[string]any{"type": "string", "description": "Natural-language description of WHAT is verified by this check."},
			"runtime":      map[string]any{"type": "string", "enum": []string{"in-process-unit", "process-local-testcontainer", "external-sidecar", "browser-flow", "static-analysis"}, "description": "Verification runtime. testcontainer/sidecar/browser-flow REQUIRE test_harness; unit/static-analysis FORBID it. testcontainer/sidecar/browser-flow REQUIRE test_runtime."},
			"test_harness": map[string]any{"type": "string", "description": "Name of a catalog entry from configs/harnesses.json. Required for testcontainer/sidecar/browser-flow runtimes; must be omitted otherwise."},
			"test_runtime": map[string]any{"type": "string", "description": "Test runtime name (e.g. \"java-junit-testcontainers\", \"go-testing-net\", \"playwright-typescript\"). Required when test_harness is named."},
			"ref":          refSchema,
			"evidence":     map[string]any{"type": "array", "items": evidenceRuleSchema, "minItems": 1, "description": "Structurally-checkable assertions the evidence gate runs post-build. REQUIRED — at least one rule per check, because the dvs-qa-reviewer (R3.7.2.j′) cannot render a verdict without rules to evaluate. Registered kinds: test_file_exists, test_uses_build_tag, surefire_passing_count."},
		},
		"required": []string{"target", "runtime", "ref", "evidence"},
	}

	return []agentic.ToolDefinition{{
		Name:        ToolName,
		Description: "Emit the dev-via-spec artifact as the architect's terminal action. Writes a human-readable markdown spec to disk, mints marker triples on this loop entity for downstream rule matching, and publishes the typed dev_via_spec.artifact.v1 payload for audit. Call once after the planner/reviewer/challenger chain converges, before submit_work.",
		Parameters: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"title":              map[string]any{"type": "string", "description": "Short artifact title, used as the markdown H1 and to derive the file slug."},
				"goal":               map[string]any{"type": "string", "description": "The 'why' — what this work achieves."},
				"context":            map[string]any{"type": "string", "description": "Background that grounds the goal in the research corpus."},
				"actors":             map[string]any{"type": "array", "items": actorSchema, "description": "Systems, frameworks, or services this work touches."},
				"integration_points": map[string]any{"type": "array", "items": integrationPointSchema, "description": "Actor-to-actor data flows with direction."},
				"tasks":              map[string]any{"type": "array", "items": taskSchema, "description": "Decomposable-grain tasks, each grounded in at least one actor."},
				"checks":             map[string]any{"type": "array", "items": checkSchema, "description": "Structured checks of verification surfaces. Each check names target / runtime / test_harness / test_runtime / ref / evidence. Multi-layer is normal — typically a unit-level check for in-language behaviour PLUS a real-stack check (testcontainer/sidecar/browser-flow) for external integration. Architect persona contract (R3.7.2.f′, see 30-commitment-contract.md): REQUIRED when integration_points[] names any external actor; optional at the wire level so v1 consumers see no schema drift. The dvs-reviewer (R3.7.2.j′) enforces coverage adequacy and rejects artifacts with external integration_points but empty checks[]."},
				"provenance":         provenanceSchema,
			},
			"required": []string{"title", "goal", "context", "actors", "tasks", "provenance"},
		},
	}}
}

// Execute parses the args into a devviaspec.Artifact, validates it, renders
// markdown to disk, publishes marker triples, and publishes the typed payload.
// LLM-facing argument errors land on Result.Error with nil error so the
// framework retry loop can surface them; transport failures land on
// Result.Error with the appropriate ErrorKind.
func (e *Executor) Execute(ctx context.Context, call agentic.ToolCall) (agentic.ToolResult, error) {
	if call.Name != ToolName {
		return agentic.ToolResult{
			CallID:    call.ID,
			Name:      call.Name,
			Error:     fmt.Sprintf("unknown tool: %s", call.Name),
			ErrorKind: agentic.ToolErrorNotFound,
		}, nil
	}

	if call.LoopID == "" {
		return agentic.ToolResult{
			CallID:    call.ID,
			Name:      call.Name,
			Error:     "emit_dev_via_spec_artifact invoked without a loop_id; cannot resolve calling loop entity",
			ErrorKind: agentic.ToolErrorInternal,
		}, nil
	}

	// Capture now once so the artifact's GeneratedAt, the predicateGeneratedAt
	// triple object, and the published payload all carry the byte-identical
	// timestamp. UTC-normalised for parity with emit_research_artifact.
	now := time.Now().UTC()

	artifact, err := parseArgsIntoArtifact(call.Arguments, call.LoopID, now)
	if err != nil {
		return agentic.ToolResult{
			CallID:    call.ID,
			Name:      call.Name,
			Error:     err.Error(),
			ErrorKind: agentic.ToolErrorInvalidArgs,
		}, nil
	}

	// Read chain-canonical lineage + slug stem so the architect persona
	// stops guessing upstream loop IDs and the chain's slug stays
	// consistent across emit_plan / emit_consensus /
	// emit_dev_via_spec_artifact. Smoke #8 run-5 D1 + D2 fix.
	//
	// Anchor on a completed ancestor's loop_id (from task properties)
	// rather than the running loop's own LoopID — agent.loop.parent
	// is stamped at completion, so walking from a still-running loop
	// fails the ancestry walk. Smoke #8 run-6 hotfix.
	anchorLoopID := chain.AnchorFromMetadata(call.Metadata, call.LoopID)
	chainTriples := e.readChainTriples(ctx, anchorLoopID)
	overrideProvenanceFromChain(&artifact.Provenance, chainTriples)
	if stem, _ := chainTriples[chain.PredicateSlugStem].(string); stem != "" {
		// Chain stem present → override the title-derived slug. "<stem>-implementation"
		// is non-empty by construction so the trailing-dash rejection
		// below safely passes.
		artifact.Slug = stem + "-implementation"
	}

	// Reject titles whose ASCII-alphanumeric content is empty (e.g. "!!!" or
	// "日本語タイトル") — slug.DeriveDated with no fallback (the
	// emitspecartifact contract path) produces a slug ending in "-"
	// that silently maps two distinct non-ASCII calls to the same file
	// on the same day. The HasSuffix("-") check converts that into a
	// 400 to the LLM so the persona retries with valid input. Skipped
	// when the chain stem overrode the title-derived slug above.
	if strings.HasSuffix(artifact.Slug, "-") {
		return agentic.ToolResult{
			CallID:    call.ID,
			Name:      call.Name,
			Error:     fmt.Sprintf("title must contain at least one ASCII alphanumeric character (got %q)", artifact.Title),
			ErrorKind: agentic.ToolErrorInvalidArgs,
		}, nil
	}

	if err := artifact.Validate(); err != nil {
		return agentic.ToolResult{
			CallID:    call.ID,
			Name:      call.Name,
			Error:     fmt.Sprintf("artifact validation failed: %v", err),
			ErrorKind: agentic.ToolErrorInvalidArgs,
		}, nil
	}

	// Resolve test-harness catalog entries for sidecar embedding BEFORE any
	// file writes. A resolver error on the sidecar path is fatal: a builder
	// that boots without a manifest would fabricate stubs against itself
	// (exactly what smoke #7 exposed). Resolving upfront and hard-erroring
	// here prevents that, and leaves the filesystem and triple store clean
	// so the LLM can retry after the operator fixes the catalog entry.
	var sidecarHarnesses map[string]testharness.ResolvedManifest
	if len(artifact.Checks) > 0 {
		var toolErr *agentic.ToolResult
		sidecarHarnesses, toolErr = e.buildSidecarHarnesses(ctx, artifact)
		if toolErr != nil {
			toolErr.CallID = call.ID
			return *toolErr, nil
		}
	}

	// Render markdown to disk before minting triples. If the disk write
	// fails, we return early without any side effects so the LLM can
	// retry after an operator fixes the path or permissions.
	relPath, err := e.renderMarkdown(ctx, artifact)
	if err != nil {
		return agentic.ToolResult{
			CallID:    call.ID,
			Name:      call.Name,
			Error:     fmt.Sprintf("render markdown: %v", err),
			ErrorKind: agentic.ToolErrorInternal,
		}, nil
	}

	// Write the sidecar <slug>.checks.json after the markdown (file ordering:
	// markdown first, sidecar second). Uses already-resolved harnesses so no
	// second catalog query. Failure aborts before any triples mint — a missing
	// sidecar means bootstrapworkspace can't seed the workspace.
	if len(artifact.Checks) > 0 {
		if toolErr := e.renderChecksJSON(artifact, sidecarHarnesses); toolErr != nil {
			toolErr.CallID = call.ID
			return *toolErr, nil
		}
	}

	loopEntityID := agentic.LoopExecutionEntityID(e.platform.Org, e.platform.Platform, call.LoopID)
	triples := buildTriples(loopEntityID, artifact, relPath, now)

	// Short-circuit on the first triple failure — same rationale as emitartifact:
	// partial triple sets would produce a half-fired rule that is hard to debug.
	// Triples are last-wins on the loop entity, so the next successful call
	// overwrites any partial set.
	for _, triple := range triples {
		if err := e.publisher.AddTriple(ctx, triple); err != nil {
			return agentic.ToolResult{
				CallID:    call.ID,
				Name:      call.Name,
				Error:     fmt.Sprintf("publish %s triple: %v", triple.Predicate, err),
				ErrorKind: agentic.ToolErrorNetwork,
			}, nil
		}
	}

	// ADR-038 PR B Phase 4: in addition to the per-loop dev_via_spec.artifact.*
	// triples, mint the cross-arc chain.spec_artifact.* cluster on the
	// canonical 6-part chain entity. Drift-safe — the loop-entity triples
	// above continue to land regardless. PR D removes them once the chain
	// entity is the sole subject. Fail-soft on resolver/publish errors:
	// the architect's tool call already succeeded against the loop entity;
	// a transient graph blip on the chain side should not return an LLM
	// error and re-trigger the architect.
	e.writeChainTriples(ctx, artifact, relPath, call.LoopID, now)

	subject := payloadSubjectPrefix + "." + call.LoopID
	payloadBytes, err := json.Marshal(artifact)
	if err != nil {
		// Marshalling a struct we own and just validated should be infallible;
		// report as Internal rather than Network.
		return agentic.ToolResult{
			CallID:    call.ID,
			Name:      call.Name,
			Error:     fmt.Sprintf("marshal artifact payload: %v", err),
			ErrorKind: agentic.ToolErrorInternal,
		}, nil
	}
	if err := e.natsPublish.Publish(ctx, subject, payloadBytes); err != nil {
		return agentic.ToolResult{
			CallID:    call.ID,
			Name:      call.Name,
			Error:     fmt.Sprintf("publish artifact payload to %s: %v", subject, err),
			ErrorKind: agentic.ToolErrorNetwork,
		}, nil
	}

	resultJSON, err := json.Marshal(map[string]any{
		"slug":                    artifact.Slug,
		"path":                    relPath,
		"generated_at":            artifact.GeneratedAt,
		"actor_count":             len(artifact.Actors),
		"integration_point_count": len(artifact.IntegrationPoints),
		"task_count":              len(artifact.Tasks),
		"check_count":             len(artifact.Checks),
		"research_root_loop":      artifact.Provenance.ResearchArtifactLoop,
		"payload_subject":         subject,
		"loop_entity_id":          loopEntityID,
	})
	if err != nil {
		return agentic.ToolResult{
			CallID:    call.ID,
			Name:      call.Name,
			Error:     fmt.Sprintf("marshal tool result: %v", err),
			ErrorKind: agentic.ToolErrorInternal,
		}, nil
	}

	e.logger.Info("emit_dev_via_spec_artifact published",
		slog.String("loop_id", call.LoopID),
		slog.String("slug", artifact.Slug),
		slog.String("path", relPath),
		slog.Int("actors", len(artifact.Actors)),
		slog.Int("integration_points", len(artifact.IntegrationPoints)),
		slog.Int("tasks", len(artifact.Tasks)),
		slog.Int("checks", len(artifact.Checks)),
		slog.String("research_root_loop", artifact.Provenance.ResearchArtifactLoop),
		slog.String("subject", subject))

	return agentic.ToolResult{
		CallID:  call.ID,
		Name:    call.Name,
		Content: string(resultJSON),
		Metadata: map[string]any{
			"loop_entity_id": loopEntityID,
			"slug":           artifact.Slug,
			"path":           relPath,
			"task_count":     len(artifact.Tasks),
			"check_count":    len(artifact.Checks),
		},
	}, nil
}

// readChainTriples resolves the chain entity for the calling architect
// loop and returns its predicate map. Fail-soft: any error (resolver
// failure, graph timeout, decode mismatch) yields nil plus a Warn log;
// the caller's map-index reads on nil return the zero value, so the
// override branches naturally fall through to LLM-supplied values +
// title-derived slug.
//
// chainReader=nil (test contexts, deployments without chain wiring)
// returns nil without logging — the documented backward-compatible
// mode. Mirrors emitplan / emitconsensus.
func (e *Executor) readChainTriples(ctx context.Context, loopID string) map[string]any {
	if e.chainReader == nil {
		return nil
	}
	chainEntityID, triples, err := e.chainReader.ReadChainFor(ctx, loopID)
	if err != nil {
		e.logger.Warn("emit_dev_via_spec_artifact: chain read failed; falling back to LLM-supplied values",
			slog.String("loop_id", loopID),
			slog.String("chain_entity", chainEntityID),
			slog.String("error", err.Error()))
		return nil
	}
	return triples
}

// overrideProvenanceFromChain replaces LLM-supplied lineage IDs with
// chain-canonical values. Smoke #8 run-5 D1 evidence: architect filled
// provenance.planner_loop with the challenger's loop ID and other
// similar drift. Each predicate is a single hop on the chain entity
// stamped earlier in the chain's lifecycle (research, plan, consensus
// milestones). Absent predicates leave the LLM-supplied value
// untouched (legacy / chain-not-wired fallback).
//
// Mutates p in place; the caller continues with the corrected value.
// Field by field (rather than reflection) so a future addition of a
// new provenance slot doesn't silently land without an explicit chain
// predicate to back it.
func overrideProvenanceFromChain(p *devviaspec.Provenance, chainTriples map[string]any) {
	if researchLoop, _ := chainTriples[chain.PredicateResearchArtifactLoop].(string); researchLoop != "" {
		p.ResearchArtifactLoop = researchLoop
	}
	if plannerLoop, _ := chainTriples[chain.PredicatePlanLoop].(string); plannerLoop != "" {
		p.PlannerLoop = plannerLoop
	}
	if reviewerLoop, _ := chainTriples[chain.PredicatePlanReviewerLoop].(string); reviewerLoop != "" {
		p.ReviewerLoop = reviewerLoop
	}
	if challengerLoop, _ := chainTriples[chain.PredicateConsensusLoop].(string); challengerLoop != "" {
		p.ChallengerLoop = challengerLoop
	}
}

// parseArgsIntoArtifact builds a devviaspec.Artifact from the LLM-supplied
// tool args. GeneratedAt and Slug are derived server-side from the caller-
// supplied now (never LLM-supplied); Provenance.ArchitectLoop is set from
// ToolCall.LoopID so the LLM cannot claim a different loop as the architect.
// Round-trips through JSON to reuse the struct's UnmarshalJSON and avoid
// re-implementing field-level type checks.
func parseArgsIntoArtifact(raw map[string]any, loopID string, now time.Time) (*devviaspec.Artifact, error) {
	// Strip server-supplied fields so LLM-provided values cannot override them.
	clean := make(map[string]any, len(raw))
	for k, v := range raw {
		if k == "generated_at" || k == "slug" {
			continue
		}
		clean[k] = v
	}

	// Set architect_loop unconditionally from the ToolCall, not from LLM args.
	// If provenance is not a map (e.g. LLM passed a string, array, or null),
	// we still build a provenance map with architect_loop set; Validate will
	// catch the missing required fields (research_artifact_loop etc.) naturally.
	provMap, _ := clean["provenance"].(map[string]any)
	provCopy := make(map[string]any, len(provMap)+1)
	for k, v := range provMap {
		if k == "architect_loop" {
			continue
		}
		provCopy[k] = v
	}
	provCopy["architect_loop"] = loopID
	clean["provenance"] = provCopy

	// Derive GeneratedAt server-side using the caller-supplied now so the
	// artifact's timestamp and the predicateGeneratedAt triple object are
	// byte-identical. RFC3339Nano for parity with emit_research_artifact.
	clean["generated_at"] = now.Format(time.RFC3339Nano)

	// Marshal/unmarshal into the typed struct to normalise field types (e.g.
	// JSON arrays from map[string]any land correctly on []Actor etc.).
	b, err := json.Marshal(clean)
	if err != nil {
		return nil, fmt.Errorf("marshal tool args: %v", err)
	}
	var artifact devviaspec.Artifact
	if err := json.Unmarshal(b, &artifact); err != nil {
		return nil, fmt.Errorf("unmarshal tool args into artifact: %v", err)
	}

	// Derive slug after unmarshal so we can read the normalised Title.
	// emitspecartifact's persona contract requires a non-empty title
	// with ASCII content. Pass empty fallback args so a degenerate
	// title (all-non-ASCII or empty) yields a slug ending in "-",
	// which the HasSuffix("-") check above rejects with a 400 to the
	// LLM. See cmd/semteams/slug for the full behavior matrix.
	artifact.Slug = slug.DeriveDated(artifact.Title, "", "", now)
	return &artifact, nil
}

// renderMarkdown writes the artifact as a markdown file to e.outputDir.
// The file is named <slug>.md. The directory is created if it does not exist
// (os.MkdirAll). Existing files are overwritten — idempotent on re-run of
// the same arc (same title → same slug → same path). Returns the relative
// path written (for the marker triple and tool result).
//
// Markdown's harness lookup is independent of buildSidecarHarnesses' result —
// the markdown template needs *TestHarness (with .Exposes.TCP), the sidecar
// needs ResolvedManifest. They share the catalog but not the projection.
func (e *Executor) renderMarkdown(ctx context.Context, a *devviaspec.Artifact) (string, error) {
	if err := os.MkdirAll(e.outputDir, 0o755); err != nil {
		return "", fmt.Errorf("create output dir %s: %w", e.outputDir, err)
	}

	harnessByName := e.buildMarkdownHarnessMap(ctx, a)

	tmpl, err := template.New("artifact").Funcs(template.FuncMap{
		"add":  func(i, n int) int { return i + n },
		"join": strings.Join,
		"harnessImage": func(name string) string {
			if h, ok := harnessByName[name]; ok {
				return h.Image
			}
			return ""
		},
		"harnessTCP": func(name string) []testharness.PortExpose {
			if h, ok := harnessByName[name]; ok {
				return h.Exposes.TCP
			}
			return nil
		},
	}).Parse(artifactTemplateText)
	if err != nil {
		return "", fmt.Errorf("parse markdown template: %w", err)
	}

	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, a); err != nil {
		return "", fmt.Errorf("execute markdown template: %w", err)
	}

	filename := a.Slug + ".md"
	fullPath := filepath.Join(e.outputDir, filename)
	if err := os.WriteFile(fullPath, buf.Bytes(), 0o644); err != nil {
		return "", fmt.Errorf("write spec file %s: %w", fullPath, err)
	}

	return filepath.Join(e.outputDir, filename), nil
}

// renderChecksJSON writes <slug>.checks.json adjacent to <slug>.md.
// The sidecar embeds the checks slice plus the pre-resolved harness
// manifests map (built by Execute before any file write).
// Called only when len(a.Checks) > 0.
//
// Error contract:
//   - os.WriteFile failure → returns ToolErrorInternal.
//
// The caller (Execute) must check for a non-nil return and short-circuit
// before minting triples.
func (e *Executor) renderChecksJSON(a *devviaspec.Artifact, harnesses map[string]testharness.ResolvedManifest) *agentic.ToolResult {
	sidecar := SidecarPayload{
		Checks:    a.Checks,
		Harnesses: harnesses,
	}

	data, err := json.MarshalIndent(sidecar, "", "  ")
	if err != nil {
		return &agentic.ToolResult{
			Name:      ToolName,
			Error:     fmt.Sprintf("marshal checks sidecar: %v", err),
			ErrorKind: agentic.ToolErrorInternal,
		}
	}

	fullPath := filepath.Join(e.outputDir, a.Slug+".checks.json")
	if err := os.WriteFile(fullPath, data, 0o644); err != nil {
		return &agentic.ToolResult{
			Name:      ToolName,
			Error:     fmt.Sprintf("write checks sidecar %s: %v", fullPath, err),
			ErrorKind: agentic.ToolErrorInternal,
		}
	}
	return nil
}

// buildSidecarHarnesses resolves every unique test_harness ID appearing
// across the artifact's checks, returning a map keyed by catalog ID.
// A resolver error is fatal and returns a ToolErrorInvalidArgs result
// so the operator can fix the catalog before the builder loop boots.
// Checks with no test_harness (e.g. in-process-unit) are skipped.
func (e *Executor) buildSidecarHarnesses(ctx context.Context, a *devviaspec.Artifact) (map[string]testharness.ResolvedManifest, *agentic.ToolResult) {
	out := map[string]testharness.ResolvedManifest{}
	if e.testHarnessResolver == nil {
		return out, nil
	}
	for i := range a.Checks {
		name := a.Checks[i].TestHarness
		if name == "" {
			continue
		}
		if _, seen := out[name]; seen {
			continue
		}
		h, err := e.testHarnessResolver(ctx, name)
		if err != nil {
			return nil, &agentic.ToolResult{
				Name:      ToolName,
				Error:     fmt.Sprintf("test_harness %q not found in catalog: %v", name, err),
				ErrorKind: agentic.ToolErrorInvalidArgs,
			}
		}
		if h == nil {
			continue
		}
		out[name] = h.Resolve()
	}
	if len(out) == 0 {
		return nil, nil
	}
	return out, nil
}

// buildMarkdownHarnessMap builds the name → *TestHarness lookup map used
// by the SPEC.md template funcs (harnessImage, harnessTCP). One call per
// unique name, soft-drop on error — the template renders name-only on miss.
// When there are no checks with a test_harness reference, or the resolver
// is nil, returns empty.
//
// Design note: the markdown path uses *TestHarness (for .Exposes.TCP) while
// the sidecar path uses ResolvedManifest. They are separate consumers of the
// same catalog data. A second lookup here is acceptable (sub-ms for KV, and
// the alternative — threading two different types through a shared map —
// is worse).
func (e *Executor) buildMarkdownHarnessMap(ctx context.Context, a *devviaspec.Artifact) map[string]*testharness.TestHarness {
	out := map[string]*testharness.TestHarness{}
	if e.testHarnessResolver == nil {
		return out
	}
	for i := range a.Checks {
		name := a.Checks[i].TestHarness
		if name == "" {
			continue
		}
		if _, seen := out[name]; seen {
			continue
		}
		h, err := e.testHarnessResolver(ctx, name)
		if err != nil {
			e.logger.Warn("test_harness lookup failed; SPEC will render name without resolved fields",
				slog.String("test_harness", name),
				slog.String("error", err.Error()))
			continue
		}
		if h == nil {
			continue
		}
		out[name] = h
	}
	return out
}

// writeChainTriples mints the ADR-038 spec_artifact milestone cluster on
// the canonical 6-part chain entity. The chain entity ID is derived by
// walking ancestry from artifact.Provenance.ResearchArtifactLoop — a
// completed researcher loop with full agent.loop.parent ancestry stamped
// (the architect's own loop has not completed yet, so its parent triple
// is absent at write time; walking from a completed ancestor closes that
// gap).
//
// Fail-soft: every step that can fail logs a Warn and returns. The
// loop-entity triples written above are unaffected; PR D's contract test
// surfaces drift where chain triples should have landed but didn't.
func (e *Executor) writeChainTriples(ctx context.Context, a *devviaspec.Artifact, relPath, architectLoopID string, now time.Time) {
	if e.chainResolver == nil {
		// Chain resolution not wired (test contexts, deployments without the
		// chain package available). Drift-safe: loop-entity triples already
		// landed.
		return
	}
	anchorLoopID := a.Provenance.ResearchArtifactLoop
	if anchorLoopID == "" {
		// Architect violated the persona contract by submitting an artifact
		// without a research_root_loop. Loop-entity triple
		// dev_via_spec.artifact.research_root_loop is also empty in this
		// case; downstream review will catch it. Skip chain emission rather
		// than fabricate an anchor.
		e.logger.Warn("emit_dev_via_spec_artifact: skipping chain triples — empty research_root_loop in provenance",
			slog.String("architect_loop_id", architectLoopID),
			slog.String("slug", a.Slug))
		return
	}

	chainEntityID, err := e.chainResolver.ChainEntityID(ctx, anchorLoopID)
	if err != nil {
		e.logger.Warn("emit_dev_via_spec_artifact: chain ancestry walk failed; skipping chain triples",
			slog.String("architect_loop_id", architectLoopID),
			slog.String("research_anchor", anchorLoopID),
			slog.String("error", err.Error()))
		return
	}

	chainBase := func(pred string, obj any) message.Triple {
		return message.Triple{
			Subject:    chainEntityID,
			Predicate:  pred,
			Object:     obj,
			Source:     chainTripleSource,
			Timestamp:  now,
			Confidence: 1.0,
		}
	}
	chainTriples := []message.Triple{
		chainBase(chainPredicateSpecArtifactLoop, architectLoopID),
		chainBase(chainPredicateSpecArtifactPath, relPath),
		chainBase(chainPredicateSpecArtifactCheckCount, len(a.Checks)),
	}
	for _, t := range chainTriples {
		if err := e.publisher.AddTriple(ctx, t); err != nil {
			// Log + continue across siblings: a partial chain cluster is
			// queryable for debugging, a half-fired silence is not.
			e.logger.Warn("emit_dev_via_spec_artifact: chain triple write failed",
				slog.String("predicate", t.Predicate),
				slog.String("chain_entity", chainEntityID),
				slog.String("error", err.Error()))
		}
	}
}

// buildTriples assembles the deterministic triple set in publish order.
// Counts and references only — never free-text content (ADR-028 §Layer 2).
// Timestamp is RFC3339Nano for correlation with the published payload.
func buildTriples(loopEntityID string, a *devviaspec.Artifact, relPath string, now time.Time) []message.Triple {
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
	return []message.Triple{
		base(predicatePath, relPath),
		base(predicateSlug, a.Slug),
		base(predicateGeneratedAt, now.Format(time.RFC3339Nano)),
		base(predicateActorCount, len(a.Actors)),
		base(predicateIntegrationPointCount, len(a.IntegrationPoints)),
		base(predicateTaskCount, len(a.Tasks)),
		base(predicateCheckCount, len(a.Checks)),
		base(predicateResearchRootLoop, a.Provenance.ResearchArtifactLoop),
	}
}

// artifactTemplateText is the markdown template body for the rendered
// spec file. Parsed per-call by renderMarkdown so the test_harness lookup
// funcs can close over a per-call resolver map (see resolveTestHarnesses).
// Per-call parse cost is sub-millisecond for a ~2 KB template; cheaper
// than threading a richer data shape through the template that would
// otherwise need to carry the test_harness projection per-check at value level.
//
// Template functions:
//   - add(i, n): integer addition for 1-indexed SR headings.
//   - join(elems, sep): strings.Join wrapper.
//   - harnessImage(name): catalog-resolved image string, "" when miss.
//   - harnessTCP(name): catalog-resolved TCP exposes, nil when miss.
//
// Overwrite policy: same slug (same title + same day) overwrites the existing
// file. This makes the architect's terminal tool idempotent on retry and
// avoids stale drafts accumulating for the same arc. If a second distinct arc
// produces the same slug on the same day (unlikely in practice), the second
// arc overwrites the first. Operators who need to preserve all versions should
// use git history or the payload audit trail on dev_via_spec.artifact.{loop_id}.
//
// Note: the {{else}} branch on `+ "`"+`{{if $c.Evidence}}`+"`"+` (rendering
// "_none — reviewer may flag as under-specified_") is unreachable in v1
// because verification.Check.Validate rejects checks with empty Evidence
// (smoke #8 run-8 fix). Kept in case a future relaxation re-engages it
// (e.g. operator-bypass mode for greenfield bring-up).
const artifactTemplateText = `# {{.Title}}

> **Generated**: {{.GeneratedAt}}
> **Chain root**: ` + "`" + `{{.Provenance.ResearchArtifactLoop}}` + "`" + `
> **Slug**: ` + "`" + `{{.Slug}}` + "`" + `

## Goal

{{.Goal}}

## Context

{{.Context}}

## Actors

{{range .Actors}}- **{{.Name}}** — {{.Role}}
{{end}}
## Integration Points

{{range .IntegrationPoints}}- **{{.From}} → {{.To}}**{{if .Direction}} ({{.Direction}}){{end}}: {{.Data}}
{{end}}
## Tasks

{{range $i, $t := .Tasks}}### T{{add $i 1}} — {{$t.Title}}

- **Scope**: {{$t.Scope}}
- **Grounds (actors)**: {{if $t.GroundsActors}}{{join $t.GroundsActors ", "}}{{else}}_flagged: missing grounding_{{end}}
- **Grounds (integration)**: {{if $t.GroundsIntegrationPoints}}{{join $t.GroundsIntegrationPoints "; "}}{{else}}_flagged: missing grounding_{{end}}

{{end}}
## Verification Checks

{{if .Checks}}{{range $i, $c := .Checks}}### C{{add $i 1}} — {{$c.Target}}

- **Runtime**: ` + "`" + `{{$c.Runtime}}` + "`" + `
{{if $c.TestHarness}}- **Test harness**: ` + "`" + `{{$c.TestHarness}}` + "`" + `
{{with harnessImage $c.TestHarness}}- **Image**: ` + "`" + `{{.}}` + "`" + `
{{end}}{{with harnessTCP $c.TestHarness}}- **TCP exposes**: {{range $j, $p := .}}{{if $j}}, {{end}}port {{$p.Port}} (` + "`" + `{{$p.Protocol}}` + "`" + `){{end}}
{{end}}{{end}}{{if $c.TestRuntime}}- **Test runtime**: ` + "`" + `{{$c.TestRuntime}}` + "`" + `
{{end}}- **Ref**: {{if eq (printf "%s" $c.Ref.Type) "filepath"}}filepath ` + "`" + `{{$c.Ref.Path}}` + "`" + `{{else}}template ` + "`" + `{{$c.Ref.ID}}` + "`" + `{{end}}
{{if $c.Evidence}}- **Evidence rules**:
{{range $c.Evidence}}  - ` + "`" + `{{.Kind}}` + "`" + `
{{end}}{{else}}- **Evidence rules**: _none — reviewer may flag as under-specified_
{{end}}
{{end}}{{else}}_No verification checks emitted. The reviewer is expected to flag this for any artifact whose integration_points reference external actors._

{{end}}## Provenance

- Research artifact loop: ` + "`" + `{{.Provenance.ResearchArtifactLoop}}` + "`" + `
- Approved planner loop: ` + "`" + `{{.Provenance.PlannerLoop}}` + "`" + `
- Approving reviewer loop: ` + "`" + `{{.Provenance.ReviewerLoop}}` + "`" + `
- Accepting challenger loop: ` + "`" + `{{.Provenance.ChallengerLoop}}` + "`" + `
- Architect terminal loop: ` + "`" + `{{.Provenance.ArchitectLoop}}` + "`" + `

---

*Generated by semteams dev-via-spec arc. This document is the structured terminal output of an LLM-mediated chain (research → mode-transition → planner → reviewer → challenger → architect). Hand-edits to this file will be overwritten if the chain re-runs.*
`
