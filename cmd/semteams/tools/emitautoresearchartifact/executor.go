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
// Attestation-aware write target (semteams#194):
//
// When the calling chain has a ready per-tenant devcontainer
// (sandbox.attestation.{ready=true, host_workspace_folder=<wsf>} on
// the chain entity), the artifact is written INTO that workspace via
// the sandbox /exec endpoint at <wsf>/.artifacts/autoresearch/<slug>.md.
// The reviewer's `bash cat` runs through chainbash → AttestationRunner →
// `devcontainer exec --workspace-folder <wsf>`, landing inside the
// per-tenant container whose cwd is the wsf bind-mount — so a relative
// path like `.artifacts/autoresearch/<slug>.md` resolves to the same
// file from both sides. Without this routing, the write went to the
// backend container's filesystem and the reviewer's cat saw an empty
// path.
//
// When no attestation is wired (dev shells, unit tests, deployments
// without the sandbox stack), the writer falls back to local
// os.WriteFile at e.outputDir — same behavior as before #194.
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
	predicateTitle            = "autoresearch.artifact.title"
	predicatePath             = "autoresearch.artifact.path"
	predicateRevision         = "autoresearch.artifact.revision"
	predicateProducedAt       = "autoresearch.artifact.produced-at"
	predicateImprovementPct   = "autoresearch.artifact.improvement-pct"
	predicateIterationsKept   = "autoresearch.artifact.iterations-kept"
	predicateIterationsTotal  = "autoresearch.artifact.iterations-completed"
	predicateCap              = "autoresearch.artifact.cap"
	predicateBestExperimentID = "autoresearch.artifact.best-experiment-id"
)

// ChainAttestationReader resolves the per-tenant devcontainer's host
// workspace folder for the chain that owns the calling tool
// invocation. Returns ("", nil) when no attestation is present,
// not ready, or the host_workspace_folder triple is empty — all of
// which route to the local-fs fallback. Errors are propagated for
// graph-read failures so the caller can log + degrade gracefully.
//
// Implemented in product_tools.go via runanchor.Anchor (the run anchor on
// ToolCall.Metadata, ADR-053 Phase 5) + chain.NATSEntityReader;
// consumer-defines-interface mirrors the pattern in querysandboxattestation +
// sandboxruntime.AttestationRunner.
type ChainAttestationReader interface {
	HostWorkspaceFolder(ctx context.Context, call agentic.ToolCall) (string, error)
}

// SandboxFileWriter writes content to <hostWorkspaceFolder>/<relPath>
// via the sandbox /exec endpoint. The artifact lands on the host
// filesystem at the tenant workspace, which the per-tenant
// devcontainer sees through the SEMTEAMS_TENANT_ROOT symmetric bind
// mount (see sandboxmanager/runner_api.go PR 4.5 F6) — so the
// reviewer's `devcontainer exec ... cat .artifacts/autoresearch/...`
// resolves to the same file from inside the container.
//
// Implemented in product_tools.go via upstream's runner.Client
// targeting SANDBOX_URL.
type SandboxFileWriter interface {
	WriteFile(ctx context.Context, hostWorkspaceFolder, relPath string, content []byte) error
}

// Executor implements agentic.ToolExecutor for emit_autoresearch_artifact.
type Executor struct {
	publisher     agentictools.TriplePublisher
	platform      types.PlatformMeta
	logger        *slog.Logger
	outputDir     string
	chainReader   ChainAttestationReader
	sandboxWriter SandboxFileWriter
}

// NewExecutor constructs an Executor. outputDir empty → reads
// SEMTEAMS_AUTORESEARCH_ARTIFACT_DIR or falls back to the package
// default.
//
// Attestation-aware routing is opt-in via SetAttestationWriter; without
// it, the executor writes to the local backend filesystem (legacy
// behavior preserved for dev shells + unit tests).
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

// SetAttestationWriter wires the attestation-aware write path. When
// both reader and writer are set AND the chain has a ready per-tenant
// devcontainer, renderMarkdown writes the artifact into the tenant
// workspace via /exec instead of the local backend filesystem. Either
// argument nil → keep the local-fs fallback (no-op).
func (e *Executor) SetAttestationWriter(reader ChainAttestationReader, writer SandboxFileWriter) {
	e.chainReader = reader
	e.sandboxWriter = writer
}

// ListTools returns the LLM-facing schema.
func (e *Executor) ListTools() []agentic.ToolDefinition {
	return []agentic.ToolDefinition{{
		Name:        ToolName,
		Description: "Emit the autoresearch-synthesize rollup. Renders structured markdown (baseline → best journey, per-iteration breakdown, recommended action) to .artifacts/autoresearch/<slug>.md and stamps autoresearch.artifact.{title,path,revision,produced_at,improvement_pct,iterations_*,cap,best_experiment_id} on the calling synthesize loop entity. Call once before terminating with decide(action=emit).",
		Parameters: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"title":                map[string]any{"type": "string", "description": "Title for the rollup (e.g. 'Autoresearch on task test:integration')."},
				"command":              map[string]any{"type": "string", "description": "The measurement command (echoed from baseline)."},
				"baseline_value":       map[string]any{"type": "number", "description": "Baseline measurement."},
				"best_value":           map[string]any{"type": "number", "description": "Best measurement reached during the run."},
				"improvement_pct":      map[string]any{"type": "number", "description": "(baseline - best) / baseline * 100. Can be negative."},
				"cap":                  map[string]any{"type": "integer", "minimum": 1, "description": "Iteration cap from the run entity (autoresearch.run.cap). Required for the reviewer's consistency check."},
				"iterations_completed": map[string]any{"type": "integer", "description": "Total iterations (matches cap or the early-stop count)."},
				"iterations_kept":      map[string]any{"type": "integer", "description": "Iterations whose diff improved the metric and was kept."},
				"iterations_reverted":  map[string]any{"type": "integer", "description": "Iterations reverted (no improvement)."},
				"iterations_crashed":   map[string]any{"type": "integer", "description": "Iterations whose measurement command failed."},
				"best_experiment_id":   map[string]any{"type": "string", "description": "The experiment_id that achieved best_value (autoresearch.best.experiment-id from the run entity). Use literal 'baseline' when iterations_kept == 0. Required for the reviewer's consistency check that best.experiment_id resolves to a kept journey entry."},
				"best_diff_summary":    map[string]any{"type": "string", "description": "Prose summary of the diff that won (referencing files + change pattern)."},
				"journey":              map[string]any{"type": "array", "items": map[string]any{"type": "string"}, "description": "One entry per iteration: '<iter>: hypothesis → value (outcome)'."},
				"open_opportunities":   map[string]any{"type": "array", "items": map[string]any{"type": "string"}, "description": "Hypotheses worth exploring in a follow-up run."},
				"recommended_action":   map[string]any{"type": "string", "description": "What the operator should do with this run's result (commit best, re-run with different surface, etc.)."},
				"revision":             map[string]any{"type": "integer", "minimum": 1, "description": "Monotonic across reviewer-rejection retries."},
			},
			"required": []string{"title", "command", "baseline_value", "best_value", "cap", "iterations_completed", "journey", "best_experiment_id"},
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

	// Attestation-aware routing per semteams#194: when the chain has a
	// ready per-tenant devcontainer, write into <wsf>/.artifacts/...
	// via /exec so the reviewer's `devcontainer exec ... cat` resolves
	// the same file. Read failures DON'T fail the tool — log and fall
	// back to local fs (dev shells, e2e fixtures without sandbox stack
	// up). Empty wsf → no ready container → local fallback.
	hostWsf := ""
	if e.chainReader != nil {
		wsf, err := e.chainReader.HostWorkspaceFolder(ctx, call)
		if err != nil {
			e.logger.Warn("emit_autoresearch_artifact: chain attestation lookup failed; using local-fs fallback",
				slog.String("loop_id", call.LoopID),
				slog.Any("error", err))
		} else {
			hostWsf = wsf
		}
	}

	relPath, err := e.renderMarkdown(ctx, args, artifactSlug, now, hostWsf)
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
	Cap                 int
	IterationsCompleted int
	IterationsKept      int
	IterationsReverted  int
	IterationsCrashed   int
	BestExperimentID    string
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
	if v, ok := raw["cap"].(float64); ok {
		p.Cap = int(v)
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
	if v, ok := raw["best_experiment_id"].(string); ok {
		p.BestExperimentID = v
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

	if err := p.validate(); err != nil {
		return nil, err
	}
	return p, nil
}

// validate enforces required-field + consistency rules on parsedArgs.
// Split out so parseArgs stays under revive's function-length cap and
// validation can be unit-tested independently of JSON shape parsing.
func (p *parsedArgs) validate() error {
	if p.Title == "" {
		return fmt.Errorf("title is required")
	}
	if p.Command == "" {
		return fmt.Errorf("command is required")
	}
	if p.Cap <= 0 {
		return fmt.Errorf("cap is required and must be > 0")
	}
	if p.BestExperimentID == "" {
		return fmt.Errorf("best_experiment_id is required (use literal 'baseline' when iterations_kept == 0)")
	}
	// Structural check: the reviewer's consistency contract requires
	// best_experiment_id == "baseline" exactly when no kept iteration
	// improved on baseline. Catching this at tool-time pushes the
	// failure to a debuggable point instead of round-tripping through
	// reviewer-rejection → resynthesize (smoke #15 surfaced the latter
	// path as expensive).
	if p.IterationsKept == 0 && p.BestExperimentID != "baseline" {
		return fmt.Errorf("best_experiment_id=%q but iterations_kept=0; use literal 'baseline' for negative-result runs", p.BestExperimentID)
	}
	if p.IterationsKept > 0 && p.BestExperimentID == "baseline" {
		return fmt.Errorf("best_experiment_id='baseline' but iterations_kept=%d; pass the kept iteration's experiment_id", p.IterationsKept)
	}
	if len(p.Journey) == 0 {
		return fmt.Errorf("journey must have at least one entry")
	}
	return nil
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
		base(predicateCap, p.Cap),
		base(predicateBestExperimentID, p.BestExperimentID),
	}
}

// tenantArtifactSubdir is the path under the per-tenant workspace
// the attestation-aware write lands at. Matches the LOCAL outputDir's
// trailing component so the relative form (`.artifacts/autoresearch/
// <slug>.md`) is identical regardless of write target — readers don't
// need to know which path the writer took.
const tenantArtifactSubdir = ".artifacts/autoresearch"

func (e *Executor) renderMarkdown(ctx context.Context, p *parsedArgs, artifactSlug string, now time.Time, hostWsf string) (string, error) {
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

	// Attestation-aware path: write into the per-tenant workspace via
	// /exec so the path the reviewer's bash-cat (which runs inside the
	// per-tenant devcontainer) resolves to the same file. The stamped
	// path is workspace-relative — the devcontainer's cwd at the wsf
	// bind-mount makes it resolve cleanly.
	if hostWsf != "" && e.sandboxWriter != nil {
		relPath := filepath.Join(tenantArtifactSubdir, filename)
		if err := e.sandboxWriter.WriteFile(ctx, hostWsf, relPath, buf.Bytes()); err != nil {
			return "", fmt.Errorf("write artifact to tenant workspace %s: %w", hostWsf, err)
		}
		return relPath, nil
	}

	// Local-fs fallback: dev shells, unit tests, deployments without
	// the sandbox stack. Behavior pre-#194.
	if err := os.MkdirAll(e.outputDir, 0o755); err != nil {
		return "", fmt.Errorf("create output dir %s: %w", e.outputDir, err)
	}
	fullPath := filepath.Join(e.outputDir, filename)
	if err := os.WriteFile(fullPath, buf.Bytes(), 0o644); err != nil {
		return "", fmt.Errorf("write file %s: %w", fullPath, err)
	}
	return fullPath, nil
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
| Cap | {{ .Cap }} |
| Completed | {{ .IterationsCompleted }} |
| Kept | {{ .IterationsKept }} |
| Reverted | {{ .IterationsReverted }} |
| Crashed | {{ .IterationsCrashed }} |

**Best experiment:** ` + "`{{ .BestExperimentID }}`" + `

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
