// Package emitautoresearchbaseline implements the SemTeams-local
// emit_autoresearch_baseline tool — the producer side of the
// autoresearch-baseline persona's terminal commit (see
// configs/rules/autoresearch/01-coordinator-autoresearch-spawn.json
// iteration 3).
//
// The baseline persona parses four autoresearch parameters from the
// coordinator's reason text (command, surface, cap, metric_parser),
// runs the command via bash to establish a baseline measurement,
// then calls this tool to stamp the run parameters AND initialize
// best.{value,experiment_id} so the empirical-compare executor in
// emit_autoresearch_measurement has a reference to compare against.
//
// Stamps on the RUN entity (read from related_loops):
//
//   - autoresearch.command, .surface, .cap, .metric_parser
//   - autoresearch.baseline.value, .baseline.pass, .baseline.stdout_tail
//   - autoresearch.best.value (= baseline.value initially)
//   - autoresearch.best.experiment_id (= "baseline" initially)
//
// Mirrors the run-entity-stamp pattern of emit_bootstrap_plan; uses
// related_loops["run-loop-entity-id"] as the spawn-time-stable
// reference (sidesteps the semstreams#159 publish-vs-write race).
//
// Discipline note (framework-alignment review): defensible
// product-shell-local; migration target same as other emit tools.
package emitautoresearchbaseline

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"time"

	"github.com/c360studio/semstreams/agentic"
	"github.com/c360studio/semstreams/message"
	agentictools "github.com/c360studio/semstreams/processor/agentic-tools"
)

// ToolName is the LLM-facing tool name.
const ToolName = "emit_autoresearch_baseline"

// toolSource tags the triples this tool publishes.
const toolSource = "autoresearch-emit-baseline"

// runEntityRoleKey is the related_loops key the spawn rule pins to
// the run entity's 6-part ID.
const runEntityRoleKey = "run-loop-entity-id"

// Predicate constants on the run entity. Rules 05/06 (continue/stop)
// and emit_autoresearch_measurement's compare key off these.
const (
	predicateCommand            = "autoresearch.command"
	predicateSurface            = "autoresearch.surface"
	predicateCap                = "autoresearch.cap"
	predicateMetricParser       = "autoresearch.metric_parser"
	predicateBaselineValue      = "autoresearch.baseline.value"
	predicateBaselinePass       = "autoresearch.baseline.pass"
	predicateBaselineStdoutTail = "autoresearch.baseline.stdout_tail"
	predicateBestValue          = "autoresearch.best.value"
	predicateBestExperimentID   = "autoresearch.best.experiment_id"
	predicateBaselineStampedAt  = "autoresearch.baseline.stamped_at"
)

// Executor implements agentic.ToolExecutor for emit_autoresearch_baseline.
type Executor struct {
	publisher agentictools.TriplePublisher
	logger    *slog.Logger
}

// NewExecutor constructs an Executor. Publisher must be non-nil.
func NewExecutor(publisher agentictools.TriplePublisher, logger *slog.Logger) *Executor {
	if publisher == nil {
		panic("emitautoresearchbaseline.NewExecutor: publisher must not be nil")
	}
	if logger == nil {
		logger = slog.Default()
	}
	return &Executor{publisher: publisher, logger: logger}
}

// ListTools returns the LLM-facing schema.
func (e *Executor) ListTools() []agentic.ToolDefinition {
	return []agentic.ToolDefinition{{
		Name:        ToolName,
		Description: "Emit the autoresearch run parameters + baseline measurement. Stamps autoresearch.{command,surface,cap,metric_parser,baseline.value,baseline.pass,baseline.stdout_tail,best.value,best.experiment_id} on the run entity. best.value initializes to baseline.value so the empirical-compare executor in emit_autoresearch_measurement has an initial reference. Call exactly once per autoresearch arc.",
		Parameters: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"command":              map[string]any{"type": "string", "description": "The executable measurement command (e.g. 'task test:integration')."},
				"surface":              map[string]any{"type": "string", "description": "Comma-separated file globs the propose phase may edit (e.g. 'test/, Taskfile.yml')."},
				"cap":                  map[string]any{"type": "integer", "minimum": 1, "description": "Iteration cap. Typical: 10 for cheap measurements, 5 for slow ones."},
				"metric_parser":        map[string]any{"type": "string", "description": "Instructions for extracting a numeric value from the command's stdout. Lower-is-better."},
				"baseline_value":       map[string]any{"type": "number", "description": "The baseline measurement extracted via metric_parser."},
				"baseline_pass":        map[string]any{"type": "boolean", "description": "True iff baseline command exited 0."},
				"baseline_stdout_tail": map[string]any{"type": "string", "description": "Last ~200 chars of baseline stdout for audit."},
			},
			"required": []string{"command", "surface", "cap", "metric_parser", "baseline_value", "baseline_pass"},
		},
	}}
}

// Execute parses args, stamps run-entity triples.
func (e *Executor) Execute(ctx context.Context, call agentic.ToolCall) (agentic.ToolResult, error) {
	if call.Name != ToolName {
		return errResult(call, agentic.ToolErrorNotFound, "unknown tool: %s", call.Name)
	}

	args, err := parseArgs(call.Arguments)
	if err != nil {
		return errResult(call, agentic.ToolErrorInvalidArgs, "%v", err)
	}

	runEntityID, err := runEntityFromCall(call)
	if err != nil {
		return errResult(call, agentic.ToolErrorInternal, "%v", err)
	}

	triples := args.triples(runEntityID)
	if err := e.publisher.AddTriplesBatch(ctx, triples); err != nil {
		return errResult(call, agentic.ToolErrorNetwork, "stamp baseline triples on %s: %v", runEntityID, err)
	}

	body, _ := json.Marshal(map[string]any{
		"run_entity_id":  runEntityID,
		"baseline_value": args.BaselineValue,
		"baseline_pass":  args.BaselinePass,
		"best_value":     args.BaselineValue,
		"cap":            args.Cap,
	})

	e.logger.Info("emit_autoresearch_baseline stamped",
		slog.String("run_entity_id", runEntityID),
		slog.Float64("baseline_value", args.BaselineValue),
		slog.Bool("baseline_pass", args.BaselinePass),
		slog.Int("cap", args.Cap))

	return agentic.ToolResult{
		CallID:   call.ID,
		Name:     call.Name,
		Content:  string(body),
		Metadata: map[string]any{"run_entity_id": runEntityID, "cap": args.Cap},
	}, nil
}

func runEntityFromCall(call agentic.ToolCall) (string, error) {
	related, _ := call.Metadata[agentic.MetadataKeyRelatedLoops].(map[string]any)
	if v, ok := related[runEntityRoleKey].(string); ok && v != "" {
		return v, nil
	}
	return "", fmt.Errorf("emit_autoresearch_baseline: related_loops[%q] missing or empty in call metadata; spawn rule must pin run-loop-entity-id at chain start", runEntityRoleKey)
}

type parsedArgs struct {
	Command            string
	Surface            string
	Cap                int
	MetricParser       string
	BaselineValue      float64
	BaselinePass       bool
	BaselineStdoutTail string
}

func parseArgs(raw map[string]any) (*parsedArgs, error) {
	p := &parsedArgs{}
	if v, ok := raw["command"].(string); ok {
		p.Command = v
	}
	if v, ok := raw["surface"].(string); ok {
		p.Surface = v
	}
	if v, ok := raw["cap"].(float64); ok {
		p.Cap = int(v)
	}
	if v, ok := raw["metric_parser"].(string); ok {
		p.MetricParser = v
	}
	if v, ok := raw["baseline_value"].(float64); ok {
		p.BaselineValue = v
	}
	if v, ok := raw["baseline_pass"].(bool); ok {
		p.BaselinePass = v
	}
	if v, ok := raw["baseline_stdout_tail"].(string); ok {
		p.BaselineStdoutTail = v
	}

	if p.Command == "" {
		return nil, fmt.Errorf("command is required")
	}
	if p.Surface == "" {
		return nil, fmt.Errorf("surface is required")
	}
	if p.Cap < 1 {
		return nil, fmt.Errorf("cap must be >= 1 (got %d)", p.Cap)
	}
	if p.MetricParser == "" {
		return nil, fmt.Errorf("metric_parser is required")
	}
	return p, nil
}

func (p *parsedArgs) triples(runEntityID string) []message.Triple {
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
	out := []message.Triple{
		base(predicateCommand, p.Command),
		base(predicateSurface, p.Surface),
		base(predicateCap, p.Cap),
		base(predicateMetricParser, p.MetricParser),
		base(predicateBaselineValue, p.BaselineValue),
		base(predicateBaselinePass, p.BaselinePass),
		// Seed best.* from baseline.* — the empirical-compare
		// executor in emit_autoresearch_measurement reads
		// best.value to decide kept/reverted on the FIRST
		// propose iteration.
		base(predicateBestValue, p.BaselineValue),
		base(predicateBestExperimentID, "baseline"),
		base(predicateBaselineStampedAt, now.Format(time.RFC3339Nano)),
	}
	if p.BaselineStdoutTail != "" {
		out = append(out, base(predicateBaselineStdoutTail, p.BaselineStdoutTail))
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
