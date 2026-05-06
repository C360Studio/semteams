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
	"regexp"
	"strings"
	"text/template"
	"time"

	"github.com/c360studio/semstreams/agentic"
	"github.com/c360studio/semstreams/message"
	agentictools "github.com/c360studio/semstreams/processor/agentic-tools"
	"github.com/c360studio/semstreams/types"

	"github.com/c360studio/semteams/cmd/semteams/devviaspec"
	"github.com/c360studio/semteams/cmd/semteams/harness"
)

// HarnessResolver looks up a harness catalog entry by name. Returns
// nil + non-nil error on lookup failure (catalog miss, transport
// error). Used by the renderer to project the architect-cited
// harness NAME into the concrete fields the builder needs to
// transcribe (Image, TCP exposes) — without those fields rendered
// inline in SPEC.md, the builder has no in-sandbox path to resolve
// the catalog (the catalog file lives on the backend host, not in
// the builder's sandbox workspace). R3.7.2.h′ closes that gap.
//
// nil is permitted; the executor falls back to rendering the
// commitment without resolved fields (the architect's name still
// ships, just no image/port projection). Operators running
// dev-via-spec with no harness manager wired will see commitments
// without resolution; operators running it WITH the manager will
// see commitments fully grounded.
type HarnessResolver func(ctx context.Context, name string) (*harness.Harness, error)

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
	predicateSeedRequirementCount  = "dev_via_spec.artifact.seed_requirement_count"
	predicateResearchRootLoop      = "dev_via_spec.artifact.research_root_loop"
	// predicateCommitmentCount is emitted regardless of whether commitments
	// are populated (zero is a meaningful signal: "architect emitted but
	// claimed no verification surface"). R3.7.2.h's evidence gate fires
	// per-commitment, so the count is its branching predicate.
	predicateCommitmentCount = "dev_via_spec.artifact.commitment_count"
)

// PayloadPublisher is the narrow surface the executor uses to publish the
// typed Artifact payload onto a stable NATS subject. *natsclient.Client
// satisfies it via its core Publish. No JetStream ack required — forward-compat
// audit; if durability is needed operators can add a stream binding without
// touching this code. Named for parity with agentictools.TriplePublisher.
type PayloadPublisher interface {
	Publish(ctx context.Context, subject string, data []byte) error
}

// Executor implements agentic.ToolExecutor for emit_dev_via_spec_artifact.
// It renders the Artifact as markdown to disk, writes a deterministic set
// of marker triples on the calling loop entity, and publishes the typed
// payload on a stable subject.
type Executor struct {
	publisher       agentictools.TriplePublisher
	natsPublish     PayloadPublisher
	platform        types.PlatformMeta
	logger          *slog.Logger
	outputDir       string
	harnessResolver HarnessResolver
}

// NewExecutor constructs an Executor. outputDir is the directory where
// rendered markdown specs are written; if empty the env var
// SEMTEAMS_DEVVIASPEC_ARTIFACT_DIR is checked, then the default "docs/specs".
// Injecting outputDir directly lets tests write to a temp directory without
// environment-variable side effects.
//
// resolver may be nil — the renderer falls back to rendering the
// architect's harness NAME without resolved fields (Image, TCP
// exposes). Production wiring (cmd/semteams/product_tools.go) passes
// the harness manager's Get adapter; tests typically pass nil.
func NewExecutor(publisher agentictools.TriplePublisher, natsPublish PayloadPublisher, platform types.PlatformMeta, logger *slog.Logger, outputDir string, resolver HarnessResolver) *Executor {
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
		publisher:       publisher,
		natsPublish:     natsPublish,
		platform:        platform,
		logger:          logger,
		outputDir:       outputDir,
		harnessResolver: resolver,
	}
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
	seedRequirementSchema := map[string]any{
		"type": "object",
		"properties": map[string]any{
			"title": map[string]any{"type": "string", "description": "Short requirement title."},
			"scope": map[string]any{"type": "string", "description": "Implementation scope (e.g. \"backend\", \"ui\", \"infra\")."},
			"grounds_actors": map[string]any{
				"type":        "array",
				"items":       map[string]any{"type": "string"},
				"description": "Actor names this requirement is grounded in.",
			},
			"grounds_integration_points": map[string]any{
				"type":        "array",
				"items":       map[string]any{"type": "string"},
				"description": "Freeform \"from→to\" strings identifying integration points this requirement touches.",
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

	// R3.7.2.b: verification_commitments[] is the architect's structured
	// statement of WHAT is verified, AGAINST WHAT, and with WHAT
	// EVIDENCE. Each commitment fills a verification surface (unit /
	// testcontainer / sidecar / browser-flow / static-analysis) and
	// names a harness from configs/harnesses.json when applicable.
	// The architect persona contract (R3.7.2.f′,
	// configs/personas/fragments/dev-via-spec-architect/30-commitment-
	// contract.md) requires at least one commitment for any artifact
	// whose integration_points[] names an external actor. The field
	// stays optional at the wire level so v1 consumers see no schema
	// drift; the dvs-reviewer (R3.7.2.j′) enforces coverage adequacy.
	conventionRefSchema := map[string]any{
		"type": "object",
		"properties": map[string]any{
			"type": map[string]any{"type": "string", "enum": []string{"filepath", "template_id"}, "description": "Discriminates the union: filepath for brownfield (cite an existing test file in the repo); template_id for greenfield (name a framework-shipped template)."},
			"path": map[string]any{"type": "string", "description": "Workspace-relative path to the convention test file. Required when type=filepath; must be empty otherwise."},
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
	commitmentSchema := map[string]any{
		"type": "object",
		"properties": map[string]any{
			"target":     map[string]any{"type": "string", "description": "Natural-language description of WHAT is verified by this commitment."},
			"approach":   map[string]any{"type": "string", "enum": []string{"in-process-unit", "process-local-testcontainer", "external-sidecar", "browser-flow", "static-analysis"}, "description": "Verification approach. testcontainer/sidecar/browser-flow REQUIRE harness; unit/static-analysis FORBID it. testcontainer/sidecar/browser-flow REQUIRE runtime."},
			"harness":    map[string]any{"type": "string", "description": "Name of a catalog entry from configs/harnesses.json. Required for testcontainer/sidecar/browser-flow approaches; must be omitted otherwise."},
			"runtime":    map[string]any{"type": "string", "description": "Test runtime name (e.g. \"java-junit-testcontainers\", \"go-testing-net\", \"playwright-typescript\"). Required when harness is named."},
			"convention": conventionRefSchema,
			"evidence":   map[string]any{"type": "array", "items": evidenceRuleSchema, "description": "Structurally-checkable assertions the evidence gate runs post-build. Empty is permitted; the reviewer may flag commitments without evidence as under-specified."},
		},
		"required": []string{"target", "approach", "convention"},
	}

	return []agentic.ToolDefinition{{
		Name:        ToolName,
		Description: "Emit the dev-via-spec artifact as the architect's terminal action. Writes a human-readable markdown spec to disk, mints marker triples on this loop entity for downstream rule matching, and publishes the typed dev_via_spec.artifact.v1 payload for audit. Call once after the planner/reviewer/challenger chain converges, before submit_work.",
		Parameters: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"title":                    map[string]any{"type": "string", "description": "Short artifact title, used as the markdown H1 and to derive the file slug."},
				"goal":                     map[string]any{"type": "string", "description": "The 'why' — what this work achieves."},
				"context":                  map[string]any{"type": "string", "description": "Background that grounds the goal in the research corpus."},
				"actors":                   map[string]any{"type": "array", "items": actorSchema, "description": "Systems, frameworks, or services this work touches."},
				"integration_points":       map[string]any{"type": "array", "items": integrationPointSchema, "description": "Actor-to-actor data flows with direction."},
				"seed_requirements":        map[string]any{"type": "array", "items": seedRequirementSchema, "description": "Decomposable-grain requirements, each grounded in at least one actor."},
				"verification_commitments": map[string]any{"type": "array", "items": commitmentSchema, "description": "Structured commitments to verification surfaces. Each commitment names target / approach / harness / runtime / convention / evidence. Multi-layer is normal — typically a unit-level commitment for in-language behaviour PLUS a real-stack commitment (testcontainer/sidecar/browser-flow) for external integration. Architect persona contract (R3.7.2.f′, see 30-commitment-contract.md): REQUIRED when integration_points[] names any external actor; optional at the wire level so v1 consumers see no schema drift. The dvs-reviewer (R3.7.2.j′) enforces coverage adequacy and rejects artifacts with external integration_points but empty verification_commitments[]."},
				"provenance":               provenanceSchema,
			},
			"required": []string{"title", "goal", "context", "actors", "seed_requirements", "provenance"},
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

	// Reject titles whose ASCII-alphanumeric content is empty (e.g. "!!!" or
	// "日本語タイトル") — deriveSlug would produce a slug ending in "-" that
	// silently maps two distinct non-ASCII calls to the same file on the same day.
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
		"seed_requirement_count":  len(artifact.SeedRequirements),
		"commitment_count":        len(artifact.VerificationCommitments),
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
		slog.Int("seed_requirements", len(artifact.SeedRequirements)),
		slog.Int("commitments", len(artifact.VerificationCommitments)),
		slog.String("research_root_loop", artifact.Provenance.ResearchArtifactLoop),
		slog.String("subject", subject))

	return agentic.ToolResult{
		CallID:  call.ID,
		Name:    call.Name,
		Content: string(resultJSON),
		Metadata: map[string]any{
			"loop_entity_id":         loopEntityID,
			"slug":                   artifact.Slug,
			"path":                   relPath,
			"seed_requirement_count": len(artifact.SeedRequirements),
			"commitment_count":       len(artifact.VerificationCommitments),
		},
	}, nil
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
	artifact.Slug = deriveSlug(artifact.Title, now)
	return &artifact, nil
}

// deriveSlug produces a YYYY-MM-DD-lower-kebab-case slug from the artifact
// title. Non-alphanumeric characters are collapsed to single hyphens; leading
// and trailing hyphens are trimmed. The date prefix ensures lexicographic sort
// order across artifacts and avoids slug collisions across different days.
var nonAlnum = regexp.MustCompile(`[^a-z0-9]+`)

func deriveSlug(title string, t time.Time) string {
	lower := strings.ToLower(title)
	slug := nonAlnum.ReplaceAllString(lower, "-")
	slug = strings.Trim(slug, "-")
	return t.Format("2006-01-02") + "-" + slug
}

// renderMarkdown writes the artifact as a markdown file to e.outputDir.
// The file is named <slug>.md. The directory is created if it does not exist
// (os.MkdirAll). Existing files are overwritten — idempotent on re-run of
// the same arc (same title → same slug → same path). Returns the relative
// path written (for the marker triple and tool result).
func (e *Executor) renderMarkdown(ctx context.Context, a *devviaspec.Artifact) (string, error) {
	if err := os.MkdirAll(e.outputDir, 0o755); err != nil {
		return "", fmt.Errorf("create output dir %s: %w", e.outputDir, err)
	}

	// Resolve any architect-cited harness names into catalog entries
	// up front so the per-call template's lookup funcs read from a
	// closure-captured map rather than re-querying the KV bucket per
	// commitment. A failed lookup is silently dropped from the map —
	// the rendered SPEC ships the architect's harness name without
	// the resolved Image/TCP, which the builder will surface as a
	// `needs_clarification` rather than fabricate. Catalog churn
	// between architect emit-time and builder iteration would
	// otherwise produce a worse failure mode (silent stale fields).
	harnessByName := e.resolveHarnesses(ctx, a)

	tmpl, err := template.New("artifact").Funcs(template.FuncMap{
		"add":  func(i, n int) int { return i + n },
		"join": strings.Join,
		"harnessImage": func(name string) string {
			if h, ok := harnessByName[name]; ok {
				return h.Image
			}
			return ""
		},
		"harnessTCP": func(name string) []harness.PortExpose {
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

	// Markdown is written first (human-readable artifact), then the
	// commitments sidecar. If the sidecar write fails, the caller returns
	// early before any triples are minted — same abort policy as above.
	if err := e.renderCommitmentsJSON(a); err != nil {
		return "", err
	}

	return filepath.Join(e.outputDir, filename), nil
}

// renderCommitmentsJSON writes <slug>.commitments.json next to the markdown
// file. Skipped when the artifact carries no commitments — file presence is a
// signal to PR #2's preprocessor (absent file → no-commitments summary, not a
// hard error). Uses json.MarshalIndent for human-diffable output, ensuring
// deterministic content for the same input.
func (e *Executor) renderCommitmentsJSON(a *devviaspec.Artifact) error {
	if len(a.VerificationCommitments) == 0 {
		return nil
	}
	data, err := json.MarshalIndent(a.VerificationCommitments, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal commitments JSON for slug %s: %w", a.Slug, err)
	}
	fullPath := filepath.Join(e.outputDir, a.Slug+".commitments.json")
	if err := os.WriteFile(fullPath, data, 0o644); err != nil {
		return fmt.Errorf("write commitments file %s: %w", fullPath, err)
	}
	return nil
}

// resolveHarnesses returns a name → catalog-entry map for every
// distinct harness named on a's verification commitments. nil
// resolver returns an empty map; lookup errors are logged at WARN
// and the entry is dropped from the map (the renderer just omits
// the resolved fields rather than aborting emission). Distinct
// names are looked up once even if the artifact carries multiple
// commitments referencing the same harness.
func (e *Executor) resolveHarnesses(ctx context.Context, a *devviaspec.Artifact) map[string]*harness.Harness {
	out := map[string]*harness.Harness{}
	if e.harnessResolver == nil {
		return out
	}
	for i := range a.VerificationCommitments {
		name := a.VerificationCommitments[i].Harness
		if name == "" {
			continue
		}
		if _, seen := out[name]; seen {
			continue
		}
		h, err := e.harnessResolver(ctx, name)
		if err != nil {
			e.logger.Warn("harness lookup failed; SPEC will render name without resolved fields",
				slog.String("harness", name),
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
		base(predicateSeedRequirementCount, len(a.SeedRequirements)),
		base(predicateCommitmentCount, len(a.VerificationCommitments)),
		base(predicateResearchRootLoop, a.Provenance.ResearchArtifactLoop),
	}
}

// artifactTemplateText is the markdown template body for the rendered
// spec file. Parsed per-call by renderMarkdown so the harness lookup
// funcs can close over a per-call resolver map (see resolveHarnesses).
// Per-call parse cost is sub-millisecond for a ~2 KB template; cheaper
// than threading a richer data shape through the template that would
// otherwise need to carry the harness projection per-VC at value level.
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
## Seed Requirements

{{range $i, $sr := .SeedRequirements}}### SR{{add $i 1}} — {{$sr.Title}}

- **Scope**: {{$sr.Scope}}
- **Grounds (actors)**: {{if $sr.GroundsActors}}{{join $sr.GroundsActors ", "}}{{else}}_flagged: missing grounding_{{end}}
- **Grounds (integration)**: {{if $sr.GroundsIntegrationPoints}}{{join $sr.GroundsIntegrationPoints "; "}}{{else}}_flagged: missing grounding_{{end}}

{{end}}
## Verification Commitments

{{if .VerificationCommitments}}{{range $i, $c := .VerificationCommitments}}### VC{{add $i 1}} — {{$c.Target}}

- **Approach**: ` + "`" + `{{$c.Approach}}` + "`" + `
{{if $c.Harness}}- **Harness**: ` + "`" + `{{$c.Harness}}` + "`" + `
{{with harnessImage $c.Harness}}- **Image**: ` + "`" + `{{.}}` + "`" + `
{{end}}{{with harnessTCP $c.Harness}}- **TCP exposes**: {{range $j, $p := .}}{{if $j}}, {{end}}port {{$p.Port}} (` + "`" + `{{$p.Protocol}}` + "`" + `){{end}}
{{end}}{{end}}{{if $c.Runtime}}- **Runtime**: ` + "`" + `{{$c.Runtime}}` + "`" + `
{{end}}- **Convention**: {{if eq (printf "%s" $c.Convention.Type) "filepath"}}filepath ` + "`" + `{{$c.Convention.Path}}` + "`" + `{{else}}template ` + "`" + `{{$c.Convention.ID}}` + "`" + `{{end}}
{{if $c.Evidence}}- **Evidence rules**:
{{range $c.Evidence}}  - ` + "`" + `{{.Kind}}` + "`" + `
{{end}}{{else}}- **Evidence rules**: _none — reviewer may flag as under-specified_
{{end}}
{{end}}{{else}}_No verification commitments emitted. The reviewer is expected to flag this for any artifact whose integration_points reference external actors._

{{end}}## Provenance

- Research artifact loop: ` + "`" + `{{.Provenance.ResearchArtifactLoop}}` + "`" + `
- Approved planner loop: ` + "`" + `{{.Provenance.PlannerLoop}}` + "`" + `
- Approving reviewer loop: ` + "`" + `{{.Provenance.ReviewerLoop}}` + "`" + `
- Accepting challenger loop: ` + "`" + `{{.Provenance.ChallengerLoop}}` + "`" + `
- Architect terminal loop: ` + "`" + `{{.Provenance.ArchitectLoop}}` + "`" + `

---

*Generated by semteams dev-via-spec arc. This document is the structured terminal output of an LLM-mediated chain (research → mode-transition → planner → reviewer → challenger → architect). Hand-edits to this file will be overwritten if the chain re-runs.*
`
