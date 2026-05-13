// Package emitplan implements the SemTeams-local emit_plan tool.
// The tool is the producer side of ADR-038 PR C Phase C2's plan
// milestone: it writes marker triples on the planner's loop entity
// (so downstream rules and the chain milestone subscriber can match
// without parsing JSON), renders a markdown view of the plan to
// docs/plans/<slug>.md, and publishes the typed dev_via_spec.plan.v1
// payload on a stable JetStream subject for audit.
//
// Discipline note (commission-not-omission): this tool mirrors the
// emitartifact / emitspecartifact pattern exactly. The migration
// target is upstream's planned generic write_artifact (ADR-028 §What's
// not built here). When upstream ships, evaluate migration of all
// emit_*_artifact / emit_plan / emit_evidence_summary tools to the
// generic primitive — see ADR-038 D6.
//
// Persona contract: the planner persona supplies typed fields
// (title, goal, context, scope_in/out, epics, depends_on); the tool
// renders the markdown structure with stable headings. Persona owns
// substance; tool owns format. ADR-038 D3.
package emitplan

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
)

// ToolName is the LLM-facing tool name.
const ToolName = "emit_plan"

// toolSource tags the triples this tool publishes.
const toolSource = "dev-via-spec-emit-plan"

// payloadSubjectPrefix is the JetStream subject namespace under
// which the typed Plan payload is published. Per-loop suffix matches
// the emit_research_artifact / emit_dev_via_spec_artifact convention.
const payloadSubjectPrefix = "dev_via_spec.plan"

// defaultOutputDir is the markdown render target when
// SEMTEAMS_PLAN_DIR is unset. Relative to process working
// directory (the repo root in normal operation).
const defaultOutputDir = "docs/plans"

// envOutputDir is the operator override for the markdown output
// directory. Mirrors SEMTEAMS_DEVVIASPEC_ARTIFACT_DIR /
// SEMTEAMS_RESEARCH_ARTIFACT_DIR — env var, not framework config,
// so it doesn't drift the upstream config schema.
const envOutputDir = "SEMTEAMS_PLAN_DIR"

// Predicate names stamped on the planner's loop entity. The chain
// milestone subscriber (cmd/semteams/chain/plan.go) reads
// predicatePath at reviewer-approval time and propagates to
// chain.plan.path on the chain entity.
const (
	predicateRevision    = "dev_via_spec.plan.revision"
	predicateEpicCount   = "dev_via_spec.plan.epic_count"
	predicateGeneratedAt = "dev_via_spec.plan.generated_at"
	predicatePath        = "dev_via_spec.plan.path"
)

// PayloadPublisher is the narrow surface the executor uses to
// publish the typed Plan payload onto a stable NATS subject.
// *natsclient.Client satisfies it via core Publish — no JetStream
// ack required (audit, not rule trigger).
type PayloadPublisher interface {
	Publish(ctx context.Context, subject string, data []byte) error
}

// ChainReader resolves the canonical 6-part chain entity ID for the
// calling planner loop and reads its triple set in one call. Used by
// Execute to override LLM-supplied lineage IDs and slug derivation
// with chain-canonical values (smoke #8 run-5 D1 + D2 fix).
//
// Optional dependency: when nil, Execute falls back to LLM-supplied
// values (test contexts, deployments without graph-query wired).
//
// Production wiring (cmd/semteams/product_tools.go) supplies a small
// adapter that composes chain.Resolver.ChainEntityID +
// chain.NATSEntityReader.ReadEntity. Tests typically pass a fake
// returning a fixed predicate map.
type ChainReader interface {
	ReadChainFor(ctx context.Context, fromLoopID string) (chainEntityID string, triples map[string]any, err error)
}

// Executor implements agentic.ToolExecutor for emit_plan.
type Executor struct {
	publisher   agentictools.TriplePublisher
	natsPublish PayloadPublisher
	platform    types.PlatformMeta
	logger      *slog.Logger
	outputDir   string
	chainReader ChainReader // nil → fall back to LLM-supplied lineage + title-derived slug
}

// NewExecutor constructs an Executor. outputDir is the directory
// where rendered markdown plans are written; if empty the env var
// SEMTEAMS_PLAN_DIR is checked, then the default "docs/plans".
func NewExecutor(publisher agentictools.TriplePublisher, natsPublish PayloadPublisher, platform types.PlatformMeta, logger *slog.Logger, outputDir string) *Executor {
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
		publisher:   publisher,
		natsPublish: natsPublish,
		platform:    platform,
		logger:      logger,
		outputDir:   outputDir,
	}
}

// SetChainReader enables chain-entity-driven lineage and slug
// derivation. Optional opt-in: leaving this unset keeps the executor
// backward-compatible (LLM-supplied depends_on + title-derived slug
// only). Mirrors emitspecartifact.SetChainResolver's pattern.
//
// Wired in cmd/semteams/product_tools.go after NewExecutor when the
// chain package's Resolver + NATSEntityReader are built.
func (e *Executor) SetChainReader(r ChainReader) {
	e.chainReader = r
}

// ListTools returns the LLM-facing schema. Args mirror the Plan
// JSON shape directly; loop_id, slug, and produced_at are
// server-supplied — the LLM neither sees nor sets them.
func (e *Executor) ListTools() []agentic.ToolDefinition {
	dependsOnSchema := map[string]any{
		"type": "object",
		"properties": map[string]any{
			"research_artifact_loop": map[string]any{"type": "string", "description": "Loop ID of the upstream research artifact this plan responds to."},
		},
	}
	return []agentic.ToolDefinition{{
		Name:        ToolName,
		Description: "Emit the planner's approved-pass plan. Supplies typed fields (goal, context, scope_in/out, epics) that the tool renders as a deterministic markdown view. Marker triples on the loop entity carry counts and the rendered file path; the chain milestone subscriber propagates the path to the chain entity at reviewer-approval time. Call once per planner pass before the terminal decide(action=\"planned\"); on retry after reviewer rejection, monotonically bump revision.",
		Parameters: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"revision":   map[string]any{"type": "integer", "minimum": 1, "description": "This pass's revision number, starting at 1 and monotonic across reviewer-rejection retries."},
				"title":      map[string]any{"type": "string", "description": "Optional one-line title for this plan (e.g. \"OSH Meshtastic driver plan\"). Used to derive a stable, human-readable slug for the rendered docs/plans/<slug>.md file. Empty falls back to a loop-id-suffixed slug; supplying a title is preferred for readable git history."},
				"goal":       map[string]any{"type": "string", "description": "Concrete, testable target capability (named interface, endpoint, component, or capability). Single string. Not 'build a driver' — something a downstream agent could verify."},
				"context":    map[string]any{"type": "string", "description": "Why the work matters, naming at least one actor from the upstream research artifact and identifying the integration boundary the work sits at. Single string."},
				"scope_in":   map[string]any{"type": "array", "items": map[string]any{"type": "string"}, "description": "Work items that are explicitly in-scope. Each item is one decomposable description. Persona owns granularity."},
				"scope_out":  map[string]any{"type": "array", "items": map[string]any{"type": "string"}, "description": "Work items explicitly excluded, each with a one-line rationale. Per the research-artifact integration_point coverage discipline."},
				"epics":      map[string]any{"type": "array", "items": map[string]any{"type": "string"}, "description": "Interface-level decomposition. Each epic grounds against an actor or integration boundary the context names. Required (>= 1) — a plan with no epics is a goal statement, not a plan."},
				"depends_on": dependsOnSchema,
			},
			"required": []string{"revision", "goal", "context", "epics"},
		},
	}}
}

// Execute parses, validates, renders, stamps triples, publishes payload.
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
			Error:     "emit_plan invoked without a loop_id; cannot resolve calling loop entity",
			ErrorKind: agentic.ToolErrorInternal,
		}, nil
	}

	plan, err := parseArgsIntoPlan(call.Arguments, call.LoopID)
	if err != nil {
		return agentic.ToolResult{
			CallID:    call.ID,
			Name:      call.Name,
			Error:     err.Error(),
			ErrorKind: agentic.ToolErrorInvalidArgs,
		}, nil
	}
	if err := plan.Validate(); err != nil {
		return agentic.ToolResult{
			CallID:    call.ID,
			Name:      call.Name,
			Error:     fmt.Sprintf("plan validation failed: %v", err),
			ErrorKind: agentic.ToolErrorInvalidArgs,
		}, nil
	}

	loopEntityID := agentic.LoopExecutionEntityID(e.platform.Org, e.platform.Platform, call.LoopID)
	now := plan.ProducedAt

	// Read chain-canonical lineage + slug stem so the planner persona
	// stops guessing upstream loop IDs and the chain's slug stays
	// consistent across emit_plan / emit_dev_via_spec_artifact.
	// Fail-soft: read errors fall back to LLM-supplied values + the
	// title-derived slug. Smoke #8 run-5 D1 + D2 fix.
	//
	// Anchor on a completed ancestor's loop_id (from task properties)
	// rather than the running loop's own LoopID — agent.loop.parent
	// is stamped at completion, so walking from a still-running loop
	// fails the ancestry walk. Smoke #8 run-6 hotfix.
	anchorLoopID := chain.AnchorFromMetadata(call.Metadata, call.LoopID)
	chainTriples := e.readChainTriples(ctx, anchorLoopID)
	if researchLoop, _ := chainTriples[chain.PredicateResearchArtifactLoop].(string); researchLoop != "" {
		plan.DependsOn.ResearchArtifactLoop = researchLoop
	}

	// Render markdown BEFORE triples so a partial-state retry never
	// stamps a path that doesn't exist on disk. Slug is deterministic
	// per (title, date), so re-renders idempotently overwrite.
	plan.Slug = e.deriveSlug(plan.Title, call.LoopID, chainTriples, now)
	relPath, renderErr := e.renderMarkdown(plan)
	if renderErr != nil {
		return agentic.ToolResult{
			CallID:    call.ID,
			Name:      call.Name,
			Error:     fmt.Sprintf("render plan markdown: %v", renderErr),
			ErrorKind: agentic.ToolErrorInternal,
		}, nil
	}

	triples := buildTriples(loopEntityID, plan, relPath, now)
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
	payloadBytes, err := json.Marshal(plan)
	if err != nil {
		return agentic.ToolResult{
			CallID:    call.ID,
			Name:      call.Name,
			Error:     fmt.Sprintf("marshal plan payload: %v", err),
			ErrorKind: agentic.ToolErrorInternal,
		}, nil
	}
	if err := e.natsPublish.Publish(ctx, subject, payloadBytes); err != nil {
		return agentic.ToolResult{
			CallID:    call.ID,
			Name:      call.Name,
			Error:     fmt.Sprintf("publish plan payload to %s: %v", subject, err),
			ErrorKind: agentic.ToolErrorNetwork,
		}, nil
	}

	resultJSON, _ := json.Marshal(map[string]any{
		"loop_id":         plan.LoopID,
		"revision":        plan.Revision,
		"slug":            plan.Slug,
		"path":            relPath,
		"epic_count":      len(plan.Epics),
		"payload_subject": subject,
		"loop_entity_id":  loopEntityID,
	})

	e.logger.Info("emit_plan published",
		slog.String("loop_id", plan.LoopID),
		slog.Int("revision", plan.Revision),
		slog.Int("epics", len(plan.Epics)),
		slog.String("path", relPath),
		slog.String("subject", subject))

	return agentic.ToolResult{
		CallID:  call.ID,
		Name:    call.Name,
		Content: string(resultJSON),
		Metadata: map[string]any{
			"loop_entity_id": loopEntityID,
			"revision":       plan.Revision,
			"path":           relPath,
		},
	}, nil
}

// readChainTriples resolves the chain entity for the calling planner
// loop and returns its predicate map. Fail-soft: any error (resolver
// failure, graph timeout, decode mismatch) yields nil plus a Warn
// log; the caller's map-index reads on nil return the zero value, so
// the override branches naturally fall through to LLM-supplied values.
//
// chainReader=nil (test contexts, deployments without chain wiring)
// returns nil without logging — the documented backward-compatible mode.
func (e *Executor) readChainTriples(ctx context.Context, loopID string) map[string]any {
	if e.chainReader == nil {
		return nil
	}
	chainEntityID, triples, err := e.chainReader.ReadChainFor(ctx, loopID)
	if err != nil {
		e.logger.Warn("emit_plan: chain read failed; falling back to LLM-supplied values",
			slog.String("loop_id", loopID),
			slog.String("chain_entity", chainEntityID),
			slog.String("error", err.Error()))
		return nil
	}
	return triples
}

// deriveSlug picks the file slug. Preference order:
//
//  1. chain.slug.stem from the chain entity → "<stem>-plan". Stable
//     across emit_plan / emit_dev_via_spec_artifact; this is the
//     smoke #8 run-5 D2 fix.
//  2. Title-derived slug via slug.DeriveDated (the pre-D2 path). Used
//     when chain.slug.stem is absent (legacy chains, chainReader unset,
//     or the researcher's path predicate didn't render).
func (e *Executor) deriveSlug(title, loopID string, chainTriples map[string]any, now time.Time) string {
	if stem, _ := chainTriples[chain.PredicateSlugStem].(string); stem != "" {
		return stem + "-plan"
	}
	return slug.DeriveDated(title, loopID, "plan", now)
}

// parseArgsIntoPlan builds a devviaspec.Plan from LLM args. loop_id
// and produced_at are server-supplied; LLM-claimed values are
// stripped (mirrors emitartifact pattern).
func parseArgsIntoPlan(raw map[string]any, loopID string) (*devviaspec.Plan, error) {
	clean := make(map[string]any, len(raw))
	for k, v := range raw {
		if k == "loop_id" || k == "produced_at" || k == "slug" {
			continue
		}
		clean[k] = v
	}
	clean["loop_id"] = loopID
	clean["produced_at"] = time.Now().UTC()

	bytes, err := json.Marshal(clean)
	if err != nil {
		return nil, fmt.Errorf("marshal tool args: %v", err)
	}
	var plan devviaspec.Plan
	if err := json.Unmarshal(bytes, &plan); err != nil {
		return nil, fmt.Errorf("unmarshal tool args into plan: %v", err)
	}
	return &plan, nil
}

// buildTriples assembles the deterministic triple set in publish order.
// Counts and references only — never free-text content (ADR-028 §Layer 2).
func buildTriples(loopEntityID string, p *devviaspec.Plan, relPath string, now time.Time) []message.Triple {
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
		base(predicateRevision, p.Revision),
		base(predicateEpicCount, len(p.Epics)),
		base(predicateGeneratedAt, now.UTC().Format(time.RFC3339Nano)),
		base(predicatePath, relPath),
	}
}

// renderMarkdown writes the plan to outputDir/<Slug>.md. Same overwrite
// policy as emitartifact / emitspecartifact: same slug → same file →
// idempotent on retry. Returns the path written (relative when
// outputDir is relative, absolute when outputDir is absolute);
// downstream consumers must tolerate both shapes.
func (e *Executor) renderMarkdown(p *devviaspec.Plan) (string, error) {
	if err := os.MkdirAll(e.outputDir, 0o755); err != nil {
		return "", fmt.Errorf("create output dir %s: %w", e.outputDir, err)
	}
	tmpl, err := template.New("plan").Funcs(template.FuncMap{
		"add": func(i, n int) int { return i + n },
	}).Parse(planTemplateText)
	if err != nil {
		return "", fmt.Errorf("parse plan markdown template: %w", err)
	}
	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, p); err != nil {
		return "", fmt.Errorf("execute plan markdown template: %w", err)
	}

	filename := p.Slug + ".md"
	fullPath := filepath.Join(e.outputDir, filename)
	if err := os.WriteFile(fullPath, buf.Bytes(), 0o644); err != nil {
		return "", fmt.Errorf("write plan file %s: %w", fullPath, err)
	}
	return filepath.Join(e.outputDir, filename), nil
}

// planTemplateText renders the plan with stable headings. Persona
// supplies content via typed fields; this template is the only
// markdown-formatting authority. Sections track the Plan struct
// layout: title (or fallback heading), revision metadata, goal,
// context, scope (in/out), epics, depends-on if present.
const planTemplateText = `# {{ if .Title }}{{ .Title }}{{ else }}Plan{{ end }}

> Loop: ` + "`{{ .LoopID }}`" + `  ·  Revision: **{{ .Revision }}**  ·  Produced: {{ .ProducedAt.UTC.Format "2006-01-02 15:04 UTC" }}

## Goal

{{ .Goal }}

## Context

{{ .Context }}

{{ if .ScopeIn }}## Scope — In
{{ range $i, $s := .ScopeIn }}
- {{ $s }}{{ end }}
{{ end }}
{{ if .ScopeOut }}## Scope — Out
{{ range $i, $s := .ScopeOut }}
- {{ $s }}{{ end }}
{{ end }}
## Epics
{{ range $i, $e := .Epics }}
{{ add $i 1 }}. {{ $e }}{{ end }}
{{ if .DependsOn.ResearchArtifactLoop }}
## Depends on

- Research artifact loop: ` + "`{{ .DependsOn.ResearchArtifactLoop }}`" + `
{{ end }}
`
