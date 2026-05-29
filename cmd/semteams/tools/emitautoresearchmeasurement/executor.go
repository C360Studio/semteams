// Package emitautoresearchmeasurement implements the SemTeams-local
// emit_autoresearch_measurement tool — the EMPIRICAL REVIEWER of the
// autoresearch category, per ADR-042 §addendum 2026-05-29 §A.
//
// This tool is structurally the load-bearing piece of the
// autoresearch contract: routing the keep-vs-revert decision through
// the LLM forfeits the empirical-reviewer property (the LLM could
// fabricate "improved"); routing through the rule engine needs
// conditional numeric updates the engine does not support today. So
// the compare-and-stamp logic lives in this Go executor.
//
// On every call, the tool:
//
//  1. Reads autoresearch.best.value from the run entity (the
//     coordinator's loop entity, identified via related_loops).
//
//  2. Compares the measurement against best.value with lower-
//     is-better semantics:
//
//     - pass=false                    → outcome=crashed
//     - pass=true && value < best     → outcome=kept
//     - pass=true && value >= best    → outcome=reverted
//
//  3. Stamps autoresearch.measurement.{value,pass,outcome,delta,
//     stdout_tail,stderr_tail} on the calling execute loop entity
//     so rules 04a + 04b (clean-completion / loop-failed counters)
//     resolve their conditions correctly.
//
//  4. On outcome=kept, ALSO updates autoresearch.best.value +
//     autoresearch.best.experiment_id on the run entity so the next
//     propose iteration compares against the new best.
//
// See configs/rules/autoresearch/03-propose-to-execute.json
// iteration 3 for the rule pack's persona contract — "you do not
// decide; the tool does."
//
// Discipline note (framework-alignment review per
// cmd/semteams/tools/README.md): no upstream "empirical reviewer
// tool" primitive exists. The compare-and-stamp pattern justifies
// itself on the rule-pack architectural test: a rule cannot run
// numeric comparison + conditional triple update; an LLM cannot
// run empirical compare without forfeiting the property. Migration
// target: graduate to upstream if future categories (e.g. a
// generalised "tournament" pack) need the same primitive.
package emitautoresearchmeasurement

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"time"

	"github.com/c360studio/semstreams/agentic"
	"github.com/c360studio/semstreams/message"
	agentictools "github.com/c360studio/semstreams/processor/agentic-tools"
	"github.com/c360studio/semstreams/types"
)

// ToolName is the LLM-facing tool name.
const ToolName = "emit_autoresearch_measurement"

// toolSource tags the triples this tool publishes.
const toolSource = "autoresearch-emit-measurement"

// Outcome vocabulary. Pinned in code; the execute persona's
// post-emit revert branch (configs/rules/autoresearch/03 iteration
// 4) branches on these literal values.
const (
	OutcomeKept     = "kept"
	OutcomeReverted = "reverted"
	OutcomeCrashed  = "crashed"
)

// runEntityRoleKey is the related_loops key the spawn rule pins to
// the run entity's 6-part ID.
const runEntityRoleKey = "run-loop-entity-id"

// Predicates on the calling execute loop entity. Rules 04a + 04b
// (clean-completion / loop-failed) key off these.
const (
	predicateMeasurementValue      = "autoresearch.measurement.value"
	predicateMeasurementPass       = "autoresearch.measurement.pass"
	predicateMeasurementOutcome    = "autoresearch.measurement.outcome"
	predicateMeasurementDelta      = "autoresearch.measurement.delta"
	predicateMeasurementStdoutTail = "autoresearch.measurement.stdout_tail"
	predicateMeasurementStderrTail = "autoresearch.measurement.stderr_tail"
	predicateMeasurementStampedAt  = "autoresearch.measurement.stamped_at"
	predicateMeasurementBestRefVal = "autoresearch.measurement.best_at_comparison"
)

// Predicates on the run entity (updated on outcome=kept).
const (
	runPredicateBestValue        = "autoresearch.best.value"
	runPredicateBestExperimentID = "autoresearch.best.experiment_id"
)

// BestValueReader reads the current autoresearch.best.value from
// the run entity. Production wiring supplies a chain.NATSEntityReader-
// backed adapter; tests inject a fake.
type BestValueReader interface {
	ReadBestValue(ctx context.Context, runEntityID string) (best float64, ok bool, err error)
}

// Executor implements agentic.ToolExecutor for
// emit_autoresearch_measurement.
type Executor struct {
	publisher agentictools.TriplePublisher
	reader    BestValueReader
	platform  types.PlatformMeta
	logger    *slog.Logger
}

// NewExecutor constructs an Executor. All deps must be non-nil.
func NewExecutor(publisher agentictools.TriplePublisher, reader BestValueReader, platform types.PlatformMeta, logger *slog.Logger) *Executor {
	if publisher == nil {
		panic("emitautoresearchmeasurement.NewExecutor: publisher must not be nil")
	}
	if reader == nil {
		panic("emitautoresearchmeasurement.NewExecutor: reader must not be nil")
	}
	if logger == nil {
		logger = slog.Default()
	}
	return &Executor{publisher: publisher, reader: reader, platform: platform, logger: logger}
}

// ListTools returns the LLM-facing schema.
func (e *Executor) ListTools() []agentic.ToolDefinition {
	return []agentic.ToolDefinition{{
		Name:        ToolName,
		Description: "Stamp the measurement value + pass on the execute loop entity. The tool's Go executor reads autoresearch.best.value from the run entity, compares numerically (lower-is-better), and stamps outcome=kept|reverted|crashed. On kept, ALSO updates autoresearch.best.value + best.experiment_id on the run entity. The persona does NOT decide keep/revert; the tool does. The post-call revert/keep bash branch keys off the stamped outcome.",
		Parameters: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"value":       map[string]any{"type": "number", "description": "The measurement extracted via metric_parser. Lower-is-better."},
				"pass":        map[string]any{"type": "boolean", "description": "True iff the measurement command exited 0."},
				"stdout_tail": map[string]any{"type": "string", "description": "Last ~200 chars of stdout for audit."},
				"stderr_tail": map[string]any{"type": "string", "description": "Last ~200 chars of stderr if any."},
			},
			"required": []string{"value", "pass"},
		},
	}}
}

// Execute parses args, reads best.value, compares, stamps.
func (e *Executor) Execute(ctx context.Context, call agentic.ToolCall) (agentic.ToolResult, error) {
	if call.Name != ToolName {
		return errResult(call, agentic.ToolErrorNotFound, "unknown tool: %s", call.Name)
	}
	if call.LoopID == "" {
		return errResult(call, agentic.ToolErrorInternal,
			"emit_autoresearch_measurement invoked without loop_id; cannot resolve execute loop entity")
	}

	args, err := parseArgs(call.Arguments)
	if err != nil {
		return errResult(call, agentic.ToolErrorInvalidArgs, "%v", err)
	}

	runEntityID, err := runEntityFromCall(call)
	if err != nil {
		return errResult(call, agentic.ToolErrorInternal, "%v", err)
	}

	executeEntityID, err := agentic.TryLoopExecutionEntityID(e.platform.Org, e.platform.Platform, call.LoopID)
	if err != nil {
		return errResult(call, agentic.ToolErrorInternal, "execute loop entity id: %v", err)
	}

	best, bestOK, readErr := e.reader.ReadBestValue(ctx, runEntityID)
	if readErr != nil {
		return errResult(call, agentic.ToolErrorNetwork, "read autoresearch.best.value from %s: %v", runEntityID, readErr)
	}
	if !bestOK {
		// Defensive: the run entity should have best.value seeded
		// by emit_autoresearch_baseline before any execute fires.
		// Absent → structural ordering bug; surface explicitly so
		// the chain wedges visibly rather than silently
		// miscomputing outcome.
		return errResult(call, agentic.ToolErrorInternal,
			"autoresearch.best.value absent on run entity %s; baseline must run before any execute (rule 02)", runEntityID)
	}

	outcome, delta := compareOutcome(args.Value, args.Pass, best)

	now := time.Now().UTC()
	executeTriples := args.executeTriples(executeEntityID, outcome, delta, best, now)
	if err := e.publisher.AddTriplesBatch(ctx, executeTriples); err != nil {
		return errResult(call, agentic.ToolErrorNetwork, "stamp measurement triples on %s: %v", executeEntityID, err)
	}

	// On outcome=kept also update best.* on the run entity. The
	// experiment_id is the calling loop's loop_id (the execute that
	// won the compare). This becomes the new reference for the
	// next propose iteration's compare.
	if outcome == OutcomeKept {
		runUpdate := []message.Triple{
			runBaseTriple(runEntityID, runPredicateBestValue, args.Value, now),
			runBaseTriple(runEntityID, runPredicateBestExperimentID, call.LoopID, now),
		}
		if err := e.publisher.AddTriplesBatch(ctx, runUpdate); err != nil {
			return errResult(call, agentic.ToolErrorNetwork, "update best on run entity %s: %v", runEntityID, err)
		}
	}

	body, _ := json.Marshal(map[string]any{
		"execute_entity_id":  executeEntityID,
		"run_entity_id":      runEntityID,
		"value":              args.Value,
		"pass":               args.Pass,
		"outcome":            outcome,
		"delta":              delta,
		"best_at_comparison": best,
		"new_best":           outcome == OutcomeKept,
	})

	e.logger.Info("emit_autoresearch_measurement",
		slog.String("execute_entity_id", executeEntityID),
		slog.String("run_entity_id", runEntityID),
		slog.Float64("value", args.Value),
		slog.Bool("pass", args.Pass),
		slog.String("outcome", outcome),
		slog.Float64("delta", delta),
		slog.Float64("best_at_comparison", best))

	return agentic.ToolResult{
		CallID:  call.ID,
		Name:    call.Name,
		Content: string(body),
		Metadata: map[string]any{
			"outcome":            outcome,
			"delta":              delta,
			"best_at_comparison": best,
			"new_best":           outcome == OutcomeKept,
		},
	}, nil
}

// compareOutcome implements the empirical compare. Pure function;
// the structural test of the autoresearch contract. Lower-is-better
// semantics:
//
//   - pass=false                      → crashed (value irrelevant)
//   - pass=true && value < best       → kept
//   - pass=true && value >= best      → reverted
//
// Delta is (value - best); negative deltas mean improvement.
// Returned even for crashed outcomes so the audit triple captures
// what the measurement WOULD have been compared to.
func compareOutcome(value float64, pass bool, best float64) (string, float64) {
	delta := value - best
	if !pass {
		return OutcomeCrashed, delta
	}
	if value < best {
		return OutcomeKept, delta
	}
	return OutcomeReverted, delta
}

func runEntityFromCall(call agentic.ToolCall) (string, error) {
	related, _ := call.Metadata[agentic.MetadataKeyRelatedLoops].(map[string]any)
	if v, ok := related[runEntityRoleKey].(string); ok && v != "" {
		return v, nil
	}
	return "", fmt.Errorf("emit_autoresearch_measurement: related_loops[%q] missing or empty; spawn rule must pin run-loop-entity-id at chain start", runEntityRoleKey)
}

type parsedArgs struct {
	Value      float64
	Pass       bool
	StdoutTail string
	StderrTail string
}

func parseArgs(raw map[string]any) (*parsedArgs, error) {
	p := &parsedArgs{}
	v, ok := raw["value"].(float64)
	if !ok {
		return nil, fmt.Errorf("value is required (numeric)")
	}
	p.Value = v
	pass, ok := raw["pass"].(bool)
	if !ok {
		return nil, fmt.Errorf("pass is required (boolean)")
	}
	p.Pass = pass
	if s, ok := raw["stdout_tail"].(string); ok {
		p.StdoutTail = s
	}
	if s, ok := raw["stderr_tail"].(string); ok {
		p.StderrTail = s
	}
	return p, nil
}

func (p *parsedArgs) executeTriples(executeEntityID, outcome string, delta, best float64, now time.Time) []message.Triple {
	base := func(pred string, obj any) message.Triple {
		return message.Triple{
			Subject:    executeEntityID,
			Predicate:  pred,
			Object:     obj,
			Source:     toolSource,
			Timestamp:  now,
			Confidence: 1.0,
		}
	}
	out := []message.Triple{
		base(predicateMeasurementValue, p.Value),
		base(predicateMeasurementPass, p.Pass),
		base(predicateMeasurementOutcome, outcome),
		base(predicateMeasurementDelta, delta),
		base(predicateMeasurementBestRefVal, best),
		base(predicateMeasurementStampedAt, now.Format(time.RFC3339Nano)),
	}
	if p.StdoutTail != "" {
		out = append(out, base(predicateMeasurementStdoutTail, p.StdoutTail))
	}
	if p.StderrTail != "" {
		out = append(out, base(predicateMeasurementStderrTail, p.StderrTail))
	}
	return out
}

func runBaseTriple(subject, predicate string, object any, now time.Time) message.Triple {
	return message.Triple{
		Subject:    subject,
		Predicate:  predicate,
		Object:     object,
		Source:     toolSource,
		Timestamp:  now,
		Confidence: 1.0,
	}
}

func errResult(call agentic.ToolCall, kind agentic.ToolErrorKind, format string, args ...any) (agentic.ToolResult, error) {
	return agentic.ToolResult{
		CallID:    call.ID,
		Name:      call.Name,
		Error:     fmt.Sprintf(format, args...),
		ErrorKind: kind,
	}, nil
}
