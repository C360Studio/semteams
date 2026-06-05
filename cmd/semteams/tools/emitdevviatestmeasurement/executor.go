// Package emitdevviatestmeasurement implements the SemTeams-local
// emit_dev_via_test_measurement tool — Ralph's per-iteration audit
// stamp inside the dev-via-test inner convergence loop (ADR-044).
//
// Ralph iterates within a single agentic loop: bash edit →
// bash test_command → emit_dev_via_test_measurement(pass, ...).
// The tool is a **simple stamp**, not an empirical reviewer — v1
// dev-via-test uses binary pass/fail semantics, so there is no
// kept/reverted choice to compute (unlike emit_autoresearch_measurement).
// Either tests pass and Ralph terminates with decide(measured),
// or tests fail and Ralph iterates again.
//
// Stamps on Ralph's loop entity (the entity for call.LoopID):
//
//   - dev_via_test.measurement.pass          (bool — exit code 0?)
//   - dev_via_test.measurement.value         (float — derived: 1.0
//     if pass, 0.0 if not.
//     Stamped for audit
//     symmetry with v2
//     fractional support; NOT
//     an LLM-visible arg in
//     v1 to avoid the
//     pass-vs-value conflict
//     foot-gun per Slice 2
//     reviewer R3 + N6.)
//   - dev_via_test.measurement.stdout_tail   (string — audit, optional)
//   - dev_via_test.measurement.stderr_tail   (string — audit, optional)
//   - dev_via_test.measurement.stamped_at    (RFC3339Nano)
//
// Iteration counter is not stamped — agentic.ToolCall does not
// expose it to tool executors in beta.96, and the rules don't
// need it (they react on the most recent pass=true). The framework's
// LoopEntity tracks iteration internally for trajectory audit.
//
// Rules 04a (converged) + 04b (failed) condition on
// dev_via_test.measurement.pass + agent.loop.outcome to route to the
// coordinator wake-up (Slice 3 coordinator).
//
// Discipline note (framework-alignment review): see ADR-044
// §addendum 2026-06-03 Slice 2. No upstream equivalent — same
// migration target as emit_autoresearch_measurement. The binary
// v1 semantics are deliberately simpler than autoresearch's
// compare-and-stamp pattern; if v2 needs fractional convergence
// with kept/reverted machinery, evaluate consolidating both tools
// at that point. Same posture as emitautoresearchmeasurement,
// emitautoresearchbaseline.
package emitdevviatestmeasurement

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
const ToolName = "emit_dev_via_test_measurement"

// toolSource tags the triples this tool publishes.
const toolSource = "dev-via-test-emit-measurement"

// Predicate constants stamped on Ralph's loop entity. Rules 04a +
// 04b key off these for the converged / failed routing.
const (
	predicateMeasurementPass       = "dev_via_test.measurement.pass"
	predicateMeasurementValue      = "dev_via_test.measurement.value"
	predicateMeasurementStdoutTail = "dev_via_test.measurement.stdout_tail"
	predicateMeasurementStderrTail = "dev_via_test.measurement.stderr_tail"
	predicateMeasurementStampedAt  = "dev_via_test.measurement.stamped_at"
)

// Executor implements agentic.ToolExecutor for
// emit_dev_via_test_measurement.
type Executor struct {
	publisher agentictools.TriplePublisher
	platform  types.PlatformMeta
	logger    *slog.Logger
}

// NewExecutor constructs an Executor. Publisher must be non-nil.
func NewExecutor(publisher agentictools.TriplePublisher, platform types.PlatformMeta, logger *slog.Logger) *Executor {
	if publisher == nil {
		panic("emitdevviatestmeasurement.NewExecutor: publisher must not be nil")
	}
	if logger == nil {
		logger = slog.Default()
	}
	return &Executor{publisher: publisher, platform: platform, logger: logger}
}

// ListTools returns the LLM-facing schema. pass is the only
// load-bearing signal; value is NOT an arg in v1 (it's derived
// stamping for audit symmetry — see Slice 2 reviewer R3 + N6).
// v2 fractional convergence will re-introduce value as an arg
// when richer test-result payloads (e.g. `go test -json`) drive
// the kept/reverted machinery.
func (e *Executor) ListTools() []agentic.ToolDefinition {
	return []agentic.ToolDefinition{{
		Name:        ToolName,
		Description: "Stamp Ralph's per-iteration test-run result on the loop entity. pass=true (test_command exit 0) ⇒ rule 04a routes to coordinator wake-up with task.status=done; pass=false ⇒ Ralph keeps iterating (or hits the framework's runaway-safety ceiling and rule 04b routes loop-failed). NO empirical-reviewer logic (unlike emit_autoresearch_measurement) — binary v1 semantics, deferred kept/reverted machinery for v2 fractional convergence.",
		Parameters: map[string]any{
			"type":                 "object",
			"additionalProperties": false,
			"properties": map[string]any{
				"pass":        map[string]any{"type": "boolean", "description": "True iff the task's test_command exited 0. The single load-bearing signal — 04a/04b route on this."},
				"stdout_tail": map[string]any{"type": "string", "description": "Last ~200 chars of stdout for audit + Ralph's next-iteration context if pass=false."},
				"stderr_tail": map[string]any{"type": "string", "description": "Last ~200 chars of stderr if any. The 04b loop-failed handler reads this so the coordinator's ask_user can quote the actual error."},
			},
			"required": []string{"pass"},
		},
	}}
}

// Execute parses args, stamps measurement triples on Ralph's loop
// entity. No empirical comparison (deliberately — binary v1).
func (e *Executor) Execute(ctx context.Context, call agentic.ToolCall) (agentic.ToolResult, error) {
	if call.Name != ToolName {
		return errResult(call, agentic.ToolErrorNotFound, "unknown tool: %s", call.Name)
	}
	if call.LoopID == "" {
		return errResult(call, agentic.ToolErrorInternal,
			"emit_dev_via_test_measurement invoked without loop_id; cannot resolve Ralph's loop entity")
	}

	args, err := parseArgs(call.Arguments)
	if err != nil {
		return errResult(call, agentic.ToolErrorInvalidArgs, "%v", err)
	}

	executeEntityID, err := agentic.TryLoopExecutionEntityID(e.platform.Org, e.platform.Platform, call.LoopID)
	if err != nil {
		return errResult(call, agentic.ToolErrorInternal, "execute loop entity id: %v", err)
	}

	now := time.Now().UTC()
	derivedValue := args.derivedValue()
	triples := args.triples(executeEntityID, derivedValue, now)
	if err := e.publisher.AddTriplesBatch(ctx, triples); err != nil {
		return errResult(call, agentic.ToolErrorNetwork, "stamp measurement triples on %s: %v", executeEntityID, err)
	}

	body, _ := json.Marshal(map[string]any{
		"execute_entity_id": executeEntityID,
		"pass":              args.Pass,
		"value":             derivedValue,
	})

	e.logger.Info("emit_dev_via_test_measurement",
		slog.String("execute_entity_id", executeEntityID),
		slog.Bool("pass", args.Pass),
		slog.Float64("value", derivedValue))

	return agentic.ToolResult{
		CallID:  call.ID,
		Name:    call.Name,
		Content: string(body),
		Metadata: map[string]any{
			"pass":  args.Pass,
			"value": derivedValue,
		},
	}, nil
}

type parsedArgs struct {
	Pass       bool
	StdoutTail string
	StderrTail string
}

// derivedValue returns the stamped value triple's content. Pure
// derivation from Pass in v1 — no LLM input. v2 fractional support
// will re-introduce an LLM-supplied value (with cross-field
// validation against pass per [[schema-shape-for-cross-field-constraints]]).
func (p *parsedArgs) derivedValue() float64 {
	if p.Pass {
		return 1.0
	}
	return 0.0
}

func parseArgs(raw map[string]any) (*parsedArgs, error) {
	if raw == nil {
		return nil, fmt.Errorf("arguments are required")
	}
	// Per Slice 2 reviewer N4: validate unknown fields FIRST so the
	// error surfaces before we consume known fields. Schema-thin
	// posture per [[schema-shape-for-cross-field-constraints]].
	allowed := map[string]struct{}{
		"pass": {}, "stdout_tail": {}, "stderr_tail": {},
	}
	for k := range raw {
		if _, ok := allowed[k]; !ok {
			return nil, fmt.Errorf("unknown field %q (allowed: pass, stdout_tail, stderr_tail)", k)
		}
	}

	p := &parsedArgs{}
	passRaw, ok := raw["pass"]
	if !ok {
		return nil, fmt.Errorf("pass is required (boolean)")
	}
	pass, ok := passRaw.(bool)
	if !ok {
		return nil, fmt.Errorf("pass must be boolean, got %T", passRaw)
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

func (p *parsedArgs) triples(executeEntityID string, derivedValue float64, now time.Time) []message.Triple {
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
		base(predicateMeasurementPass, p.Pass),
		base(predicateMeasurementValue, derivedValue),
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

func errResult(call agentic.ToolCall, kind agentic.ToolErrorKind, format string, args ...any) (agentic.ToolResult, error) {
	return agentic.ToolResult{
		CallID:    call.ID,
		Name:      call.Name,
		Error:     fmt.Sprintf(format, args...),
		ErrorKind: kind,
	}, nil
}
