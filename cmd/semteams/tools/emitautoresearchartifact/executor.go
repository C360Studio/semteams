// Package emitautoresearchartifact implements the SemTeams-local
// emit_autoresearch_artifact tool — the producer side of the
// autoresearch-synthesize persona's rollup emission (see
// configs/rules/autoresearch/07-synthesize-to-reviewer.json).
//
// The tool renders a structured markdown artifact summarizing the
// autoresearch run (baseline → best, per-iteration journey,
// recommended action) to .artifacts/autoresearch/<slug>.md and
// stamps autoresearch.artifact.{title,path,revision,produced_at}
// on the calling synthesize loop entity. reviewer-autoresearch
// reads the rendered markdown via `bash cat $entity.triple.
// autoresearch.artifact.path` for substance review.
//
// Mirrors the emit_plan / emit_research_artifact pattern exactly:
// persona supplies typed fields; tool owns format (stable markdown
// headings). Migration target same as other emit tools.
package emitautoresearchartifact

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

	"github.com/c360studio/semteams/cmd/semteams/slug"
)

// ToolName is the LLM-facing tool name.
const ToolName = "emit_autoresearch_artifact"

// toolSource tags the triples this tool publishes.
const toolSource = "autoresearch-emit-artifact"

// defaultOutputDir is the markdown render target. Relative to the
// process working directory.
const defaultOutputDir = ".artifacts/autoresearch"

// envOutputDir is the operator override env var.
const envOutputDir = "SEMTEAMS_AUTORESEARCH_ARTIFACT_DIR"

// Predicates stamped on the synthesize loop entity. Match rule
// 07/08 (synthesize-to-reviewer / reviewer-approved-to-coordinator)
// substitutions.
const (
	predicateTitle           = "autoresearch.artifact.title"
	predicatePath            = "autoresearch.artifact.path"
	predicateRevision        = "autoresearch.artifact.revision"
	predicateProducedAt      = "autoresearch.artifact.produced_at"
	predicateImprovementPct  = "autoresearch.artifact.improvement_pct"
	predicateIterationsKept  = "autoresearch.artifact.iterations_kept"
	predicateIterationsTotal = "autoresearch.artifact.iterations_completed"
)

// Executor implements agentic.ToolExecutor for emit_autoresearch_artifact.
type Executor struct {
	publisher agentictools.TriplePublisher
	platform  types.PlatformMeta
	logger    *slog.Logger
	outputDir string
}

// NewExecutor constructs an Executor. outputDir empty → reads
// SEMTEAMS_AUTORESEARCH_ARTIFACT_DIR or falls back to the package
// default.
func NewExecutor(publisher agentictools.TriplePublisher, platform types.PlatformMeta, logger *slog.Logger, outputDir string) *Executor {
	if publisher == nil {
		panic("emitautoresearchartifact.NewExecutor: publisher must not be nil")
	}
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
	return &Executor{publisher: publisher, platform: platform, logger: logger, outputDir: outputDir}
}

// ListTools returns the LLM-facing schema.
func (e *Executor) ListTools() []agentic.ToolDefinition {
	return []agentic.ToolDefinition{{
		Name:        ToolName,
		Description: "Emit the autoresearch-synthesize rollup. Renders structured markdown (baseline → best journey, per-iteration breakdown, recommended action) to .artifacts/autoresearch/<slug>.md and stamps autoresearch.artifact.{title,path,revision,produced_at,improvement_pct,iterations_*} on the calling synthesize loop entity. Call once before terminating with decide(action=emit).",
		Parameters: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"title":                map[string]any{"type": "string", "description": "Title for the rollup (e.g. 'Autoresearch on task test:integration')."},
				"command":              map[string]any{"type": "string", "description": "The measurement command (echoed from baseline)."},
				"baseline_value":       map[string]any{"type": "number", "description": "Baseline measurement."},
				"best_value":           map[string]any{"type": "number", "description": "Best measurement reached during the run."},
				"improvement_pct":      map[string]any{"type": "number", "description": "(baseline - best) / baseline * 100. Can be negative."},
				"iterations_completed": map[string]any{"type": "integer", "description": "Total iterations (matches cap or the early-stop count)."},
				"iterations_kept":      map[string]any{"type": "integer", "description": "Iterations whose diff improved the metric and was kept."},
				"iterations_reverted":  map[string]any{"type": "integer", "description": "Iterations reverted (no improvement)."},
				"iterations_crashed":   map[string]any{"type": "integer", "description": "Iterations whose measurement command failed."},
				"best_diff_summary":    map[string]any{"type": "string", "description": "Prose summary of the diff that won (referencing files + change pattern)."},
				"journey":              map[string]any{"type": "array", "items": map[string]any{"type": "string"}, "description": "One entry per iteration: '<iter>: hypothesis → value (outcome)'."},
				"open_opportunities":   map[string]any{"type": "array", "items": map[string]any{"type": "string"}, "description": "Hypotheses worth exploring in a follow-up run."},
				"recommended_action":   map[string]any{"type": "string", "description": "What the operator should do with this run's result (commit best, re-run with different surface, etc.)."},
				"revision":             map[string]any{"type": "integer", "minimum": 1, "description": "Monotonic across reviewer-rejection retries."},
			},
			"required": []string{"title", "command", "baseline_value", "best_value", "iterations_completed", "journey"},
		},
	}}
}

// Execute parses, renders markdown, stamps triples.
func (e *Executor) Execute(ctx context.Context, call agentic.ToolCall) (agentic.ToolResult, error) {
	if call.Name != ToolName {
		return errResult(call, agentic.ToolErrorNotFound, "unknown tool: %s", call.Name)
	}
	if call.LoopID == "" {
		return errResult(call, agentic.ToolErrorInternal,
			"emit_autoresearch_artifact invoked without loop_id; cannot resolve synthesize loop entity")
	}

	args, err := parseArgs(call.Arguments)
	if err != nil {
		return errResult(call, agentic.ToolErrorInvalidArgs, "%v", err)
	}

	loopEntityID, err := agentic.TryLoopExecutionEntityID(e.platform.Org, e.platform.Platform, call.LoopID)
	if err != nil {
		return errResult(call, agentic.ToolErrorInternal, "loop entity id: %v", err)
	}

	now := time.Now().UTC()
	artifactSlug := slug.DeriveDated(args.Title, call.LoopID, "autoresearch", now)
	relPath, err := e.renderMarkdown(args, artifactSlug, now)
	if err != nil {
		return errResult(call, agentic.ToolErrorInternal, "render artifact: %v", err)
	}

	triples := args.triples(loopEntityID, relPath, now)
	if err := e.publisher.AddTriplesBatch(ctx, triples); err != nil {
		return errResult(call, agentic.ToolErrorNetwork, "stamp artifact triples on %s: %v", loopEntityID, err)
	}

	body, _ := json.Marshal(map[string]any{
		"loop_entity_id":  loopEntityID,
		"path":            relPath,
		"revision":        args.Revision,
		"slug":            artifactSlug,
		"improvement_pct": args.ImprovementPct,
	})

	e.logger.Info("emit_autoresearch_artifact stamped",
		slog.String("loop_entity_id", loopEntityID),
		slog.String("path", relPath),
		slog.Int("revision", args.Revision),
		slog.Float64("improvement_pct", args.ImprovementPct))

	return agentic.ToolResult{
		CallID:   call.ID,
		Name:     call.Name,
		Content:  string(body),
		Metadata: map[string]any{"loop_entity_id": loopEntityID, "path": relPath},
	}, nil
}

type parsedArgs struct {
	Title               string
	Command             string
	BaselineValue       float64
	BestValue           float64
	ImprovementPct      float64
	IterationsCompleted int
	IterationsKept      int
	IterationsReverted  int
	IterationsCrashed   int
	BestDiffSummary     string
	Journey             []string
	OpenOpportunities   []string
	RecommendedAction   string
	Revision            int
	ProducedAt          time.Time
}

func parseArgs(raw map[string]any) (*parsedArgs, error) {
	p := &parsedArgs{Revision: 1, ProducedAt: time.Now().UTC()}
	if v, ok := raw["title"].(string); ok {
		p.Title = v
	}
	if v, ok := raw["command"].(string); ok {
		p.Command = v
	}
	if v, ok := raw["baseline_value"].(float64); ok {
		p.BaselineValue = v
	}
	if v, ok := raw["best_value"].(float64); ok {
		p.BestValue = v
	}
	if v, ok := raw["improvement_pct"].(float64); ok {
		p.ImprovementPct = v
	} else if p.BaselineValue != 0 {
		p.ImprovementPct = (p.BaselineValue - p.BestValue) / p.BaselineValue * 100
	}
	if v, ok := raw["iterations_completed"].(float64); ok {
		p.IterationsCompleted = int(v)
	}
	if v, ok := raw["iterations_kept"].(float64); ok {
		p.IterationsKept = int(v)
	}
	if v, ok := raw["iterations_reverted"].(float64); ok {
		p.IterationsReverted = int(v)
	}
	if v, ok := raw["iterations_crashed"].(float64); ok {
		p.IterationsCrashed = int(v)
	}
	if v, ok := raw["best_diff_summary"].(string); ok {
		p.BestDiffSummary = v
	}
	if v, ok := raw["journey"].([]any); ok {
		for i, s := range v {
			str, ok := s.(string)
			if !ok {
				return nil, fmt.Errorf("journey[%d]: expected string, got %T", i, s)
			}
			p.Journey = append(p.Journey, str)
		}
	}
	if v, ok := raw["open_opportunities"].([]any); ok {
		for _, s := range v {
			if str, ok := s.(string); ok {
				p.OpenOpportunities = append(p.OpenOpportunities, str)
			}
		}
	}
	if v, ok := raw["recommended_action"].(string); ok {
		p.RecommendedAction = v
	}
	if v, ok := raw["revision"].(float64); ok && v > 0 {
		p.Revision = int(v)
	}

	if p.Title == "" {
		return nil, fmt.Errorf("title is required")
	}
	if p.Command == "" {
		return nil, fmt.Errorf("command is required")
	}
	if len(p.Journey) == 0 {
		return nil, fmt.Errorf("journey must have at least one entry")
	}
	return p, nil
}

func (p *parsedArgs) triples(loopEntityID, relPath string, now time.Time) []message.Triple {
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
		base(predicateTitle, p.Title),
		base(predicatePath, relPath),
		base(predicateRevision, p.Revision),
		base(predicateProducedAt, now.Format(time.RFC3339Nano)),
		base(predicateImprovementPct, p.ImprovementPct),
		base(predicateIterationsKept, p.IterationsKept),
		base(predicateIterationsTotal, p.IterationsCompleted),
	}
}

func (e *Executor) renderMarkdown(p *parsedArgs, artifactSlug string, now time.Time) (string, error) {
	if err := os.MkdirAll(e.outputDir, 0o755); err != nil {
		return "", fmt.Errorf("create output dir %s: %w", e.outputDir, err)
	}
	tmpl, err := template.New("autoresearch-artifact").Funcs(template.FuncMap{
		"add": func(i, n int) int { return i + n },
	}).Parse(artifactTemplate)
	if err != nil {
		return "", fmt.Errorf("parse artifact template: %w", err)
	}
	view := struct {
		*parsedArgs
		Slug      string
		Generated string
	}{
		parsedArgs: p,
		Slug:       artifactSlug,
		Generated:  now.UTC().Format("2006-01-02 15:04 UTC"),
	}
	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, view); err != nil {
		return "", fmt.Errorf("execute template: %w", err)
	}
	filename := artifactSlug + ".md"
	fullPath := filepath.Join(e.outputDir, filename)
	if err := os.WriteFile(fullPath, buf.Bytes(), 0o644); err != nil {
		return "", fmt.Errorf("write file %s: %w", fullPath, err)
	}
	return filepath.Join(e.outputDir, filename), nil
}

const artifactTemplate = `# {{ .Title }}

> Slug: ` + "`{{ .Slug }}`" + `  ·  Revision: **{{ .Revision }}**  ·  Generated: {{ .Generated }}

## Summary

Command: ` + "`{{ .Command }}`" + `

| Measurement | Value |
|---|---|
| Baseline | {{ .BaselineValue }} |
| Best     | {{ .BestValue }} |
| Improvement | {{ printf "%.2f" .ImprovementPct }}% |

| Iterations | Count |
|---|---|
| Completed | {{ .IterationsCompleted }} |
| Kept | {{ .IterationsKept }} |
| Reverted | {{ .IterationsReverted }} |
| Crashed | {{ .IterationsCrashed }} |

{{ if .BestDiffSummary }}## Best diff

{{ .BestDiffSummary }}
{{ end }}
## Journey
{{ range $i, $e := .Journey }}
{{ add $i 1 }}. {{ $e }}{{ end }}
{{ if .OpenOpportunities }}
## Open opportunities
{{ range $i, $o := .OpenOpportunities }}
- {{ $o }}{{ end }}
{{ end }}
{{ if .RecommendedAction }}
## Recommended action

{{ .RecommendedAction }}
{{ end }}`

func errResult(call agentic.ToolCall, kind agentic.ToolErrorKind, format string, args ...any) (agentic.ToolResult, error) {
	return agentic.ToolResult{
		CallID:    call.ID,
		Name:      call.Name,
		Error:     fmt.Sprintf(format, args...),
		ErrorKind: kind,
	}, nil
}
