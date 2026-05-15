package builderdecide

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
	agvocab "github.com/c360studio/semstreams/vocabulary/agentic"
)

// ToolName is the LLM-facing tool name. Listed in the builder
// role's allowed_tools instead of upstream `decide` — see package doc for
// why a sibling tool, not a wrapping replacement.
const ToolName = "builder_decide"

// toolSource is the Source field on triples this tool publishes. Lets
// operators distinguish builder-terminal triples from coordinator-decide
// or rule-driven triple mutations at a glance in graph queries.
const toolSource = "builder-decide"

// Action enum values per ADR-032 §15. Closed set: anything else is rejected
// at the executor boundary so invalid actions never reach downstream rules.
const (
	ActionTestsPassing       = "tests_passing"
	ActionTestsFailing       = "tests_failing"
	ActionNeedsClarification = "needs_clarification"
)

// Predicate names stamped on the loop entity for builder-specific evidence.
// coordinator.next_action and coordinator.decision_reason come from the
// upstream agentic vocabulary so downstream rules can route on this loop
// the same way they route on a regular `decide` call.
const (
	predicateTestsRun         = "dev_via_spec.builder.tests_run"
	predicateTestsPassed      = "dev_via_spec.builder.tests_passed"
	predicateTestsFailed      = "dev_via_spec.builder.tests_failed"
	predicateArtifactSummary  = "dev_via_spec.builder.artifact_summary"
	predicateFailureSummary   = "dev_via_spec.builder.failure_summary"
	predicateRetryHint        = "dev_via_spec.builder.retry_hint"
	predicateBlockingQuestion = "dev_via_spec.builder.blocking_question"
)

// Executor implements agentic.ToolExecutor for builder_decide. It validates
// per-action evidence fields, mints coordinator + builder-evidence triples
// on the calling loop entity, and returns StopLoop=true on success.
type Executor struct {
	publisher agentictools.TriplePublisher
	platform  types.PlatformMeta
	logger    *slog.Logger
}

// NewExecutor constructs an Executor. publisher is the same NATS-backed
// triple publisher decide uses (graph.mutation.triple.add); platform sets
// the loop entity ID's org/platform prefix.
func NewExecutor(publisher agentictools.TriplePublisher, platform types.PlatformMeta, logger *slog.Logger) *Executor {
	if logger == nil {
		logger = slog.Default()
	}
	return &Executor{
		publisher: publisher,
		platform:  platform,
		logger:    logger,
	}
}

// ListTools returns the LLM-facing schema. Keeps `action` enumerated so the
// model sees the closed set in its tool catalog; evidence fields are
// declared as optional in the JSON schema (per-action requirements are
// enforced by the executor, not by JSON schema, since the requirement set
// is conditional on action).
func (e *Executor) ListTools() []agentic.ToolDefinition {
	return []agentic.ToolDefinition{{
		Name:        ToolName,
		Description: "Terminal decision tool for the builder role. Call exactly once after iterating bash calls in your sandbox workspace. action must be one of tests_passing | tests_failing | needs_clarification. Per-action evidence fields: tests_passing requires tests_run (>0), tests_passed, tests_failed, artifact_summary; tests_failing requires tests_run, tests_failed, failure_summary, retry_hint; needs_clarification requires blocking_question (and reason carries the clarification context).",
		Parameters: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"action": map[string]any{
					"type":        "string",
					"enum":        []string{ActionTestsPassing, ActionTestsFailing, ActionNeedsClarification},
					"description": "Terminal action for this builder loop.",
				},
				"reason": map[string]any{
					"type":        "string",
					"description": "Short natural-language justification for the chosen action.",
				},
				"tests_run": map[string]any{
					"type":        "integer",
					"description": "Total tests executed in the most recent test invocation (required for tests_passing and tests_failing). Must be > 0 for tests_passing — the builder cannot terminate green without having run any tests.",
				},
				"tests_passed": map[string]any{
					"type":        "integer",
					"description": "Tests that passed (required for tests_passing).",
				},
				"tests_failed": map[string]any{
					"type":        "integer",
					"description": "Tests that failed (required for tests_passing and tests_failing).",
				},
				"artifact_summary": map[string]any{
					"type":        "string",
					"description": "Short description of the built artifacts (required for tests_passing). E.g. 'OSGi bundle target/osh-driver-1.0.0.jar with surefire harness; 5 tests, 5 passing'.",
				},
				"failure_summary": map[string]any{
					"type":        "string",
					"description": "Short description of why tests failed (required for tests_failing).",
				},
				"retry_hint": map[string]any{
					"type":        "string",
					"description": "Hint describing how a retry should differ from this attempt (required for tests_failing).",
				},
				"blocking_question": map[string]any{
					"type":        "string",
					"description": "Specific question that must be answered to proceed (required for needs_clarification).",
				},
			},
			"required": []string{"action", "reason"},
		},
	}}
}

// Execute parses + validates the arguments against the per-action contract,
// publishes triples on the calling loop entity, and returns StopLoop=true.
// LLM-facing arg errors land on Result.Error with ToolErrorInvalidArgs (so
// the framework retry policy can step in); transport errors use
// ToolErrorNetwork; preconditions (missing LoopID, marshal) use
// ToolErrorInternal.
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
			Error:     "builder_decide invoked without a loop_id; cannot resolve calling loop entity",
			ErrorKind: agentic.ToolErrorInternal,
		}, nil
	}

	args, err := parseArgs(call.Arguments)
	if err != nil {
		return agentic.ToolResult{
			CallID:    call.ID,
			Name:      call.Name,
			Error:     err.Error(),
			ErrorKind: agentic.ToolErrorInvalidArgs,
		}, nil
	}

	loopEntityID := agentic.LoopExecutionEntityID(e.platform.Org, e.platform.Platform, call.LoopID)
	now := time.Now().UTC()
	triples := buildTriples(loopEntityID, args, now)

	// Short-circuit on first triple failure — partial triple sets would
	// produce a half-fired rule that is hard to debug. Same posture as
	// emitspecartifact / decide.
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

	payload, err := json.Marshal(args)
	if err != nil {
		return agentic.ToolResult{
			CallID:    call.ID,
			Name:      call.Name,
			Error:     fmt.Sprintf("marshal builder-decide payload: %v", err),
			ErrorKind: agentic.ToolErrorInternal,
		}, nil
	}

	e.logger.Info("builder_decide published",
		slog.String("loop_id", call.LoopID),
		slog.String("action", args.Action),
		slog.Int("tests_run", args.TestsRun),
		slog.Int("tests_passed", args.TestsPassed),
		slog.Int("tests_failed", args.TestsFailed))

	return agentic.ToolResult{
		CallID:   call.ID,
		Name:     call.Name,
		Content:  string(payload),
		StopLoop: true,
		Metadata: map[string]any{
			"action":         args.Action,
			"reason":         args.Reason,
			"loop_entity_id": loopEntityID,
			"tests_run":      args.TestsRun,
			"tests_passed":   args.TestsPassed,
			"tests_failed":   args.TestsFailed,
		},
	}, nil
}

// builderDecideArgs is the parsed shape of the tool's arguments. JSON tags
// drive the marshal-to-Content payload that downstream consumers read via
// read_loop_result; per-action fields are omitempty so the payload only
// carries what the action requires.
type builderDecideArgs struct {
	Action           string `json:"action"`
	Reason           string `json:"reason"`
	TestsRun         int    `json:"tests_run,omitempty"`
	TestsPassed      int    `json:"tests_passed,omitempty"`
	TestsFailed      int    `json:"tests_failed,omitempty"`
	ArtifactSummary  string `json:"artifact_summary,omitempty"`
	FailureSummary   string `json:"failure_summary,omitempty"`
	RetryHint        string `json:"retry_hint,omitempty"`
	BlockingQuestion string `json:"blocking_question,omitempty"`
}

// parseArgs reads the untyped tool arguments into builderDecideArgs and
// enforces per-action evidence requirements. JSON-number values are
// accepted as both float64 (Go's default JSON decode) and int — the LLM
// can emit either. ADR-032 §15 closes the "exit 0 with no tests"
// loophole here: action=tests_passing with tests_run=0 is rejected.
func parseArgs(raw map[string]any) (builderDecideArgs, error) {
	action, ok := raw["action"].(string)
	if !ok || action == "" {
		return builderDecideArgs{}, fmt.Errorf("action is required and must be a non-empty string")
	}
	switch action {
	case ActionTestsPassing, ActionTestsFailing, ActionNeedsClarification:
	default:
		return builderDecideArgs{}, fmt.Errorf("action must be one of tests_passing|tests_failing|needs_clarification (got %q)", action)
	}

	reason, ok := raw["reason"].(string)
	if !ok || reason == "" {
		return builderDecideArgs{}, fmt.Errorf("reason is required and must be a non-empty string")
	}

	args := builderDecideArgs{Action: action, Reason: reason}

	switch action {
	case ActionTestsPassing:
		if err := requireInt(raw, "tests_run", action, &args.TestsRun, true); err != nil {
			return builderDecideArgs{}, err
		}
		if err := requireInt(raw, "tests_passed", action, &args.TestsPassed, false); err != nil {
			return builderDecideArgs{}, err
		}
		if err := requireInt(raw, "tests_failed", action, &args.TestsFailed, false); err != nil {
			return builderDecideArgs{}, err
		}
		if err := requireString(raw, "artifact_summary", action, &args.ArtifactSummary); err != nil {
			return builderDecideArgs{}, err
		}
	case ActionTestsFailing:
		if err := requireInt(raw, "tests_run", action, &args.TestsRun, false); err != nil {
			return builderDecideArgs{}, err
		}
		if err := requireInt(raw, "tests_failed", action, &args.TestsFailed, false); err != nil {
			return builderDecideArgs{}, err
		}
		if err := requireString(raw, "failure_summary", action, &args.FailureSummary); err != nil {
			return builderDecideArgs{}, err
		}
		if err := requireString(raw, "retry_hint", action, &args.RetryHint); err != nil {
			return builderDecideArgs{}, err
		}
		// tests_passed is not required for tests_failing but, if supplied, must
		// still be a valid non-negative integer — we don't silently accept
		// "five" here.
		if err := optionalInt(raw, "tests_passed", &args.TestsPassed); err != nil {
			return builderDecideArgs{}, err
		}
	case ActionNeedsClarification:
		if err := requireString(raw, "blocking_question", action, &args.BlockingQuestion); err != nil {
			return builderDecideArgs{}, err
		}
	}

	return args, nil
}

// requireInt extracts an integer-valued field. mustBePositive enforces the
// "tests_passing with tests_run=0 is invalid" gate from ADR-032 §15. JSON
// numbers come in as float64 from map[string]any decoding; we accept any
// numeric whose fractional part is zero. action is threaded through so the
// error messages name the contract the LLM violated — small models
// self-correct better when the message says "for action=tests_passing"
// than "for this action".
func requireInt(raw map[string]any, key, action string, dst *int, mustBePositive bool) error {
	v, present := raw[key]
	if !present || v == nil {
		return fmt.Errorf("%s is required for action=%s", key, action)
	}
	n, err := toInt(v)
	if err != nil {
		return fmt.Errorf("%s must be an integer for action=%s (got %v): %v", key, action, v, err)
	}
	if n < 0 {
		return fmt.Errorf("%s must be non-negative for action=%s (got %d)", key, action, n)
	}
	if mustBePositive && n == 0 {
		return fmt.Errorf("%s must be > 0 for action=%s (the builder cannot terminate green without having run any tests)", key, action)
	}
	*dst = n
	return nil
}

// optionalInt extracts an integer-valued field if present, ignoring absence.
// Type errors are reported (a present-but-non-numeric value should fail
// loudly so the LLM corrects the call).
func optionalInt(raw map[string]any, key string, dst *int) error {
	v, present := raw[key]
	if !present || v == nil {
		return nil
	}
	n, err := toInt(v)
	if err != nil {
		return fmt.Errorf("%s must be an integer (got %v): %v", key, v, err)
	}
	*dst = n
	return nil
}

// requireString extracts a non-empty string-valued field. action is threaded
// through so the error messages name the contract the LLM violated.
func requireString(raw map[string]any, key, action string, dst *string) error {
	v, present := raw[key]
	if !present || v == nil {
		return fmt.Errorf("%s is required for action=%s", key, action)
	}
	s, ok := v.(string)
	if !ok {
		return fmt.Errorf("%s must be a string for action=%s (got %T)", key, action, v)
	}
	if s == "" {
		return fmt.Errorf("%s must be a non-empty string for action=%s", key, action)
	}
	*dst = s
	return nil
}

// toInt coerces a JSON-decoded numeric to int. JSON numbers decode to
// float64 by default; we also accept int (for direct callers).
func toInt(v any) (int, error) {
	switch n := v.(type) {
	case int:
		return n, nil
	case int64:
		return int(n), nil
	case float64:
		if n != float64(int(n)) {
			return 0, fmt.Errorf("non-integer numeric value %v", v)
		}
		return int(n), nil
	default:
		return 0, fmt.Errorf("unsupported type %T", v)
	}
}

// buildTriples assembles the deterministic triple set in publish order.
// coordinator.next_action and coordinator.decision_reason go first (parity
// with decide); per-action evidence triples follow. Counts and references
// only — never free-text content (ADR-028 §Layer 2). Free-text fields like
// artifact_summary and failure_summary are LLM-authored summaries with a
// reasonable size cap; if abuse surfaces, add a length guard here rather
// than in the persona.
func buildTriples(loopEntityID string, args builderDecideArgs, now time.Time) []message.Triple {
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
	out := []message.Triple{
		base(agvocab.CoordinatorNextAction, args.Action),
		base(agvocab.CoordinatorDecisionReason, args.Reason),
	}
	switch args.Action {
	case ActionTestsPassing:
		out = append(out,
			base(predicateTestsRun, args.TestsRun),
			base(predicateTestsPassed, args.TestsPassed),
			base(predicateTestsFailed, args.TestsFailed),
			base(predicateArtifactSummary, args.ArtifactSummary),
		)
	case ActionTestsFailing:
		out = append(out,
			base(predicateTestsRun, args.TestsRun),
			base(predicateTestsFailed, args.TestsFailed),
			base(predicateFailureSummary, args.FailureSummary),
			base(predicateRetryHint, args.RetryHint),
		)
	case ActionNeedsClarification:
		out = append(out, base(predicateBlockingQuestion, args.BlockingQuestion))
	}
	return out
}
