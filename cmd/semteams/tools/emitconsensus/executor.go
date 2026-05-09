// Package emitconsensus implements the SemTeams-local emit_consensus
// tool. The tool is the producer side of ADR-038 PR C Phase C3's
// consensus milestone: it writes marker triples on the challenger's
// loop entity, renders a markdown view of the consensus to
// docs/consensus/<slug>.md, and publishes the typed
// dev_via_spec.consensus.v1 payload on a stable JetStream subject.
//
// Discipline note (commission-not-omission): mirrors the emitartifact /
// emitplan / emitspecartifact pattern. Migration target is upstream's
// planned generic write_artifact (ADR-028 §What's not built here);
// see ADR-038 D6.
//
// Persona contract: the challenger persona supplies typed fields
// (title, summary, chain_consensus bullets, considered_concerns,
// depends_on); the tool renders the markdown structure with stable
// headings. ADR-038 D3.
package emitconsensus

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
const ToolName = "emit_consensus"

// toolSource tags the triples this tool publishes.
const toolSource = "dev-via-spec-emit-consensus"

// payloadSubjectPrefix is the JetStream subject namespace under
// which the typed Consensus payload is published. Matches the
// emit_research_artifact / emit_plan / emit_dev_via_spec_artifact
// per-loop convention.
const payloadSubjectPrefix = "dev_via_spec.consensus"

// defaultOutputDir is the markdown render target when
// SEMTEAMS_CONSENSUS_DIR is unset.
const defaultOutputDir = "docs/consensus"

// envOutputDir is the operator override for the markdown output
// directory. Mirrors SEMTEAMS_PLAN_DIR / SEMTEAMS_RESEARCH_ARTIFACT_DIR
// — env var, not framework config.
const envOutputDir = "SEMTEAMS_CONSENSUS_DIR"

// Predicate names stamped on the challenger's loop entity. The chain
// milestone subscriber (cmd/semteams/chain/consensus.go) reads
// predicatePath at challenger-accept time and propagates to
// chain.consensus.path on the chain entity.
const (
	predicateChainConsensusCount = "dev_via_spec.consensus.chain_consensus_count"
	predicateGeneratedAt         = "dev_via_spec.consensus.generated_at"
	predicatePath                = "dev_via_spec.consensus.path"
)

// PayloadPublisher is the narrow surface for publishing the typed
// Consensus payload onto a stable NATS subject.
type PayloadPublisher interface {
	Publish(ctx context.Context, subject string, data []byte) error
}

// ChainReader resolves the canonical 6-part chain entity ID for the
// calling challenger loop and reads its triple set in one call. Used
// by Execute to override LLM-supplied lineage IDs and slug derivation
// with chain-canonical values (smoke #8 run-5 D1 + D2 fix).
//
// Optional dependency: when nil, Execute falls back to LLM-supplied
// values. Same shape as emitplan.ChainReader; production wiring uses
// the same adapter.
type ChainReader interface {
	ReadChainFor(ctx context.Context, fromLoopID string) (chainEntityID string, triples map[string]any, err error)
}

// Executor implements agentic.ToolExecutor for emit_consensus.
type Executor struct {
	publisher   agentictools.TriplePublisher
	natsPublish PayloadPublisher
	platform    types.PlatformMeta
	logger      *slog.Logger
	outputDir   string
	chainReader ChainReader // nil → fall back to LLM-supplied lineage + title-derived slug
}

// NewExecutor constructs an Executor. outputDir is the directory
// where rendered markdown consensus files are written; if empty the
// env var SEMTEAMS_CONSENSUS_DIR is checked, then the default
// "docs/consensus".
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
// derivation. Mirrors emitplan.SetChainReader; same wiring contract.
// Wired in cmd/semteams/product_tools.go.
func (e *Executor) SetChainReader(r ChainReader) {
	e.chainReader = r
}

// ListTools returns the LLM-facing schema. Args mirror the Consensus
// JSON shape directly; loop_id, slug, and produced_at are
// server-supplied.
func (e *Executor) ListTools() []agentic.ToolDefinition {
	dependsOnSchema := map[string]any{
		"type": "object",
		"properties": map[string]any{
			"plan_loop":     map[string]any{"type": "string", "description": "Loop ID of the planner whose plan this consensus accepts."},
			"reviewer_loop": map[string]any{"type": "string", "description": "Loop ID of the dev-via-spec-reviewer that approved before the challenger's signoff."},
		},
	}
	return []agentic.ToolDefinition{{
		Name:        ToolName,
		Description: "Emit the dev-via-spec-challenger's accept-terminal consensus. Supplies typed fields (summary, chain_consensus bullets, considered_concerns, depends_on) the tool renders as a deterministic markdown view. Marker triples on the loop entity carry counts and the rendered file path; the chain milestone subscriber propagates the path to the chain entity at accept time. Call once per challenger pass before the terminal decide(action=\"accept\"). Do NOT call when terminating with concerns_raised; that path keeps the chain in the planner-reviewer-challenger cycle.",
		Parameters: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"title":               map[string]any{"type": "string", "description": "Optional one-line title for this consensus (e.g. \"OSH Meshtastic driver consensus\"). Used to derive a stable, human-readable slug for the rendered docs/consensus/<slug>.md file. Empty falls back to a loop-id-suffixed slug; supplying a title is preferred for readable git history."},
				"summary":             map[string]any{"type": "string", "description": "One-line accept rationale densely citing the chain (actor names, integration boundaries, epic decomposition). The architect curates this as the spec's chain_consensus entry."},
				"chain_consensus":     map[string]any{"type": "array", "items": map[string]any{"type": "string"}, "description": "Dense bullets the architect curates into the terminal dev_via_spec.artifact. Each entry should cite chain specifics — actor names, integration boundaries, epic decomposition — drawn from the reviewer's approved summary. Required (>= 1) — an empty consensus is just an opinion."},
				"considered_concerns": map[string]any{"type": "array", "items": map[string]any{"type": "string"}, "description": "Optional audit trail: concerns raised across challenger iterations and how they resolved. Useful for cross-arc consumers and the architect."},
				"depends_on":          dependsOnSchema,
			},
			"required": []string{"summary", "chain_consensus"},
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
			Error:     "emit_consensus invoked without a loop_id; cannot resolve calling loop entity",
			ErrorKind: agentic.ToolErrorInternal,
		}, nil
	}

	consensus, err := parseArgsIntoConsensus(call.Arguments, call.LoopID)
	if err != nil {
		return agentic.ToolResult{
			CallID:    call.ID,
			Name:      call.Name,
			Error:     err.Error(),
			ErrorKind: agentic.ToolErrorInvalidArgs,
		}, nil
	}
	if err := consensus.Validate(); err != nil {
		return agentic.ToolResult{
			CallID:    call.ID,
			Name:      call.Name,
			Error:     fmt.Sprintf("consensus validation failed: %v", err),
			ErrorKind: agentic.ToolErrorInvalidArgs,
		}, nil
	}

	loopEntityID := agentic.LoopExecutionEntityID(e.platform.Org, e.platform.Platform, call.LoopID)
	now := consensus.ProducedAt

	// Read chain-canonical lineage + slug stem so the challenger
	// persona stops guessing upstream loop IDs and the chain's slug
	// stays consistent across emit_plan / emit_consensus /
	// emit_dev_via_spec_artifact. Smoke #8 run-5 D1 + D2 fix.
	chainTriples := e.readChainTriples(ctx, call.LoopID)
	if planLoop, _ := chainTriples[chain.PredicatePlanLoop].(string); planLoop != "" {
		consensus.DependsOn.PlanLoop = planLoop
	}
	if reviewerLoop, _ := chainTriples[chain.PredicatePlanReviewerLoop].(string); reviewerLoop != "" {
		consensus.DependsOn.ReviewerLoop = reviewerLoop
	}

	consensus.Slug = e.deriveSlug(consensus.Title, call.LoopID, chainTriples, now)
	relPath, renderErr := e.renderMarkdown(consensus)
	if renderErr != nil {
		return agentic.ToolResult{
			CallID:    call.ID,
			Name:      call.Name,
			Error:     fmt.Sprintf("render consensus markdown: %v", renderErr),
			ErrorKind: agentic.ToolErrorInternal,
		}, nil
	}

	triples := buildTriples(loopEntityID, consensus, relPath, now)
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
	payloadBytes, err := json.Marshal(consensus)
	if err != nil {
		return agentic.ToolResult{
			CallID:    call.ID,
			Name:      call.Name,
			Error:     fmt.Sprintf("marshal consensus payload: %v", err),
			ErrorKind: agentic.ToolErrorInternal,
		}, nil
	}
	if err := e.natsPublish.Publish(ctx, subject, payloadBytes); err != nil {
		return agentic.ToolResult{
			CallID:    call.ID,
			Name:      call.Name,
			Error:     fmt.Sprintf("publish consensus payload to %s: %v", subject, err),
			ErrorKind: agentic.ToolErrorNetwork,
		}, nil
	}

	resultJSON, _ := json.Marshal(map[string]any{
		"loop_id":               consensus.LoopID,
		"slug":                  consensus.Slug,
		"path":                  relPath,
		"chain_consensus_count": len(consensus.ChainConsensus),
		"payload_subject":       subject,
		"loop_entity_id":        loopEntityID,
	})

	e.logger.Info("emit_consensus published",
		slog.String("loop_id", consensus.LoopID),
		slog.Int("chain_consensus", len(consensus.ChainConsensus)),
		slog.String("path", relPath),
		slog.String("subject", subject))

	return agentic.ToolResult{
		CallID:  call.ID,
		Name:    call.Name,
		Content: string(resultJSON),
		Metadata: map[string]any{
			"loop_entity_id": loopEntityID,
			"path":           relPath,
		},
	}, nil
}

// readChainTriples resolves the chain entity for the calling
// challenger loop and returns its predicate map. Fail-soft: any
// error (resolver failure, graph timeout, decode mismatch) yields
// nil plus a Warn log; the caller's map-index reads on nil return
// the zero value, so the override branches naturally fall through
// to LLM-supplied values.
//
// chainReader=nil (test contexts, deployments without chain wiring)
// returns nil without logging — the documented backward-compatible
// mode. Mirrors emitplan's helper.
func (e *Executor) readChainTriples(ctx context.Context, loopID string) map[string]any {
	if e.chainReader == nil {
		return nil
	}
	chainEntityID, triples, err := e.chainReader.ReadChainFor(ctx, loopID)
	if err != nil {
		e.logger.Warn("emit_consensus: chain read failed; falling back to LLM-supplied values",
			slog.String("loop_id", loopID),
			slog.String("chain_entity", chainEntityID),
			slog.String("error", err.Error()))
		return nil
	}
	return triples
}

// deriveSlug picks the file slug. Preference order:
//
//  1. chain.slug.stem from the chain entity → "<stem>-consensus".
//     Stable across emit_plan / emit_consensus /
//     emit_dev_via_spec_artifact; smoke #8 run-5 D2 fix.
//  2. Title-derived slug via slug.DeriveDated (the pre-D2 path).
//     Used when chain.slug.stem is absent.
func (e *Executor) deriveSlug(title, loopID string, chainTriples map[string]any, now time.Time) string {
	if stem, _ := chainTriples[chain.PredicateSlugStem].(string); stem != "" {
		return stem + "-consensus"
	}
	return slug.DeriveDated(title, loopID, "consensus", now)
}

// parseArgsIntoConsensus builds a devviaspec.Consensus from LLM args.
// loop_id and produced_at are server-supplied; LLM-claimed values are
// stripped (mirrors emitartifact / emitplan).
func parseArgsIntoConsensus(raw map[string]any, loopID string) (*devviaspec.Consensus, error) {
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
	var consensus devviaspec.Consensus
	if err := json.Unmarshal(bytes, &consensus); err != nil {
		return nil, fmt.Errorf("unmarshal tool args into consensus: %v", err)
	}
	return &consensus, nil
}

// buildTriples assembles the deterministic triple set in publish
// order. Counts and references only — never free-text content per
// ADR-028 §Layer 2.
func buildTriples(loopEntityID string, c *devviaspec.Consensus, relPath string, now time.Time) []message.Triple {
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
		base(predicateChainConsensusCount, len(c.ChainConsensus)),
		base(predicateGeneratedAt, now.UTC().Format(time.RFC3339Nano)),
		base(predicatePath, relPath),
	}
}

// renderMarkdown writes the consensus to outputDir/<Slug>.md. Same
// overwrite policy as sibling emit-tools: same slug → same file →
// idempotent on retry. Returns the path written (relative when
// outputDir is relative, absolute otherwise).
func (e *Executor) renderMarkdown(c *devviaspec.Consensus) (string, error) {
	if err := os.MkdirAll(e.outputDir, 0o755); err != nil {
		return "", fmt.Errorf("create output dir %s: %w", e.outputDir, err)
	}
	tmpl, err := template.New("consensus").Funcs(template.FuncMap{
		"add": func(i, n int) int { return i + n },
	}).Parse(consensusTemplateText)
	if err != nil {
		return "", fmt.Errorf("parse consensus markdown template: %w", err)
	}
	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, c); err != nil {
		return "", fmt.Errorf("execute consensus markdown template: %w", err)
	}
	filename := c.Slug + ".md"
	fullPath := filepath.Join(e.outputDir, filename)
	if err := os.WriteFile(fullPath, buf.Bytes(), 0o644); err != nil {
		return "", fmt.Errorf("write consensus file %s: %w", fullPath, err)
	}
	return filepath.Join(e.outputDir, filename), nil
}

// consensusTemplateText renders the consensus with stable headings.
// Persona supplies content via typed fields; this template is the
// only markdown-formatting authority. Sections track the Consensus
// struct layout: title (or fallback heading), produced metadata,
// summary, chain consensus bullets, considered concerns (optional),
// depends-on cross-arc loops (optional).
const consensusTemplateText = `# {{ if .Title }}{{ .Title }}{{ else }}Consensus{{ end }}

> Loop: ` + "`{{ .LoopID }}`" + `  ·  Produced: {{ .ProducedAt.UTC.Format "2006-01-02 15:04 UTC" }}

## Summary

{{ .Summary }}

## Chain consensus
{{ range $i, $b := .ChainConsensus }}
- {{ $b }}{{ end }}
{{ if .ConsideredConcerns }}
## Considered concerns
{{ range $i, $c := .ConsideredConcerns }}
- {{ $c }}{{ end }}
{{ end }}
{{ if or .DependsOn.PlanLoop .DependsOn.ReviewerLoop }}
## Depends on
{{ if .DependsOn.PlanLoop }}
- Plan loop: ` + "`{{ .DependsOn.PlanLoop }}`" + `{{ end }}{{ if .DependsOn.ReviewerLoop }}
- Reviewer loop: ` + "`{{ .DependsOn.ReviewerLoop }}`" + `{{ end }}
{{ end }}
`
