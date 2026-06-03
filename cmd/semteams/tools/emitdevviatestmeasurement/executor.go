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
//   - dev_via_test.measurement.value         (float — 0.0..1.0;
//     defaults to 1.0 if pass,
//     0.0 if not pass; explicit
//     fractional values supported
//     for v2 per-test breakdown)
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
// coordinator wake-up (Slice 3 walker).
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

// ListTools returns the LLM-facing schema. pass is required; value
// is optional (defaults from pass — 1.0 if pass, 0.0 if not). The
// optional value field exists for v2 fractional convergence support
// (e.g. `go test -json` reports 7/10 tests passing → value=0.7,
// pass=false) without breaking the binary v1 contract.
func (e *Executor) ListTools() []agentic.ToolDefinition {
	return []agentic.ToolDefinition{{
		Name:        ToolName,
		Description: "Stamp Ralph's per-iteration test-run result on the loop entity. pass=true (test_command exit 0) ⇒ rule 04a routes to coordinator wake-up with task.status=done; pass=false ⇒ Ralph keeps iterating (or hits framework max_iterations and rule 04b routes loop-failed). NO empirical-reviewer logic (unlike emit_autoresearch_measurement) — binary v1 semantics, deferred kept/reverted machinery for v2 fractional convergence.",
		Parameters: map[string]any{
			"type":                 "object",
			"additionalProperties": false,
			"properties": map[string]any{
				"pass":        map[string]any{"type": "boolean", "description": "True iff the task's test_command exited 0. The single load-bearing signal — 04a/04b route on this."},
				"value":       map[string]any{"type": "number", "minimum": 0.0, "maximum": 1.0, "description": "Optional fractional convergence value 0.0..1.0 (e.g. 7/10 tests passing → 0.7). Defaults to 1.0 if pass else 0.0. v1 binary semantics don't read this — present for v2 fractional support without breaking the schema."},
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
	triples := args.triples(executeEntityID, now)
	if err := e.publisher.AddTriplesBatch(ctx, triples); err != nil {
		return errResult(call, agentic.ToolErrorNetwork, "stamp measurement triples on %s: %v", executeEntityID, err)
	}

	body, _ := json.Marshal(map[string]any{
		"execute_entity_id": executeEntityID,
		"pass":              args.Pass,
		"value":             args.effectiveValue(),
	})

	e.logger.Info("emit_dev_via_test_measurement",
		slog.String("execute_entity_id", executeEntityID),
		slog.Bool("pass", args.Pass),
		slog.Float64("value", args.effectiveValue()))

	return agentic.ToolResult{
		CallID:  call.ID,
		Name:    call.Name,
		Content: string(body),
		Metadata: map[string]any{
			"pass":  args.Pass,
			"value": args.effectiveValue(),
		},
	}, nil
}

type parsedArgs struct {
	Pass       bool
	Value      *float64 // pointer to distinguish absent (use default) from explicit 0.0
	StdoutTail string
	StderrTail string
}

// effectiveValue applies the "defaults from pass" policy. If
// Value was supplied explicitly, use it. Else 1.0 if Pass else 0.0.
func (p *parsedArgs) effectiveValue() float64 {
	if p.Value != nil {
		return *p.Value
	}
	if p.Pass {
		return 1.0
	}
	return 0.0
}

func parseArgs(raw map[string]any) (*parsedArgs, error) {
	if raw == nil {
		return nil, fmt.Errorf("arguments are required")
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

	if vRaw, present := raw["value"]; present {
		v, ok := vRaw.(float64)
		if !ok {
			return nil, fmt.Errorf("value must be numeric if supplied, got %T", vRaw)
		}
		if v < 0.0 || v > 1.0 {
			return nil, fmt.Errorf("value must be in [0.0, 1.0] if supplied, got %f", v)
		}
		p.Value = &v
	}

	if s, ok := raw["stdout_tail"].(string); ok {
		p.StdoutTail = s
	}
	if s, ok := raw["stderr_tail"].(string); ok {
		p.StderrTail = s
	}

	// Validate unknown fields. emit_dev_via_test_plan uses
	// DisallowUnknownFields via the JSON decoder; here the parse
	// is hand-walked, so we whitelist explicitly. Keeps drift loud.
	allowed := map[string]struct{}{
		"pass": {}, "value": {}, "stdout_tail": {}, "stderr_tail": {},
	}
	for k := range raw {
		if _, ok := allowed[k]; !ok {
			return nil, fmt.Errorf("unknown field %q (allowed: pass, value, stdout_tail, stderr_tail)", k)
		}
	}
	return p, nil
}

func (p *parsedArgs) triples(executeEntityID string, now time.Time) []message.Triple {
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
		base(predicateMeasurementValue, p.effectiveValue()),
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
