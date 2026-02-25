// Package reactive provides a reactive workflow engine built on rules engine primitives.
//
// The reactive workflow engine replaces imperative step sequencing with typed rules
// that react to KV state changes and NATS messages. Workflows are defined in Go code,
// enabling compile-time type checking of all data flows.
//
// Key concepts:
//   - TriggerSource: unified reactive primitive (KV watch, subject consumer, or both)
//   - RuleContext: provides typed access to execution state and triggering message
//   - ConditionFunc: typed condition evaluation against RuleContext
//   - StateMutatorFunc: typed state mutation after action completion
//
// See ADR-021 for architectural details.
package reactive

import (
	"strconv"
	"time"

	"github.com/c360studio/semstreams/message"
)

// ExecutionStatus represents the overall status of a workflow execution.
type ExecutionStatus string

const (
	// StatusPending indicates the execution has been created but not started.
	StatusPending ExecutionStatus = "pending"
	// StatusRunning indicates the execution is actively processing rules.
	StatusRunning ExecutionStatus = "running"
	// StatusWaiting indicates the execution is waiting for an async callback.
	StatusWaiting ExecutionStatus = "waiting"
	// StatusCompleted indicates the execution finished successfully.
	StatusCompleted ExecutionStatus = "completed"
	// StatusFailed indicates the execution failed with an error.
	StatusFailed ExecutionStatus = "failed"
	// StatusEscalated indicates the execution was escalated (e.g., max iterations exceeded).
	StatusEscalated ExecutionStatus = "escalated"
	// StatusTimedOut indicates the execution exceeded its timeout.
	StatusTimedOut ExecutionStatus = "timed_out"
)

// ExecutionState represents the typed state of a workflow execution.
// Each workflow definition declares its own concrete state type that embeds ExecutionState.
type ExecutionState struct {
	// ID is the unique execution identifier (KV key).
	ID string `json:"id"`

	// WorkflowID references the workflow definition.
	WorkflowID string `json:"workflow_id"`

	// Phase is the current execution phase (typed enum per workflow).
	Phase string `json:"phase"`

	// Iteration tracks retry/loop count.
	Iteration int `json:"iteration"`

	// Status is the overall execution status.
	Status ExecutionStatus `json:"status"`

	// Error holds the last error message if any.
	Error string `json:"error,omitempty"`

	// PendingTaskID is set when waiting for an async callback.
	PendingTaskID string `json:"pending_task_id,omitempty"`

	// PendingRuleID identifies which rule is awaiting a callback.
	PendingRuleID string `json:"pending_rule_id,omitempty"`

	// Timestamps
	CreatedAt   time.Time  `json:"created_at"`
	UpdatedAt   time.Time  `json:"updated_at"`
	CompletedAt *time.Time `json:"completed_at,omitempty"`

	// Deadline is the absolute time when this execution times out.
	Deadline *time.Time `json:"deadline,omitempty"`

	// Timeline records rule firings for debugging.
	Timeline []TimelineEntry `json:"timeline,omitempty"`
}

// TimelineEntry records a rule firing event.
type TimelineEntry struct {
	Timestamp   time.Time `json:"timestamp"`
	RuleID      string    `json:"rule_id"`
	TriggerMode string    `json:"trigger_mode"` // "kv", "message", "message+state"
	TriggerInfo string    `json:"trigger_info"` // KV key or NATS subject that triggered
	Action      string    `json:"action"`
	Phase       string    `json:"phase_after"`
	Iteration   int       `json:"iteration"`
}

// TriggerMode indicates which reactive primitive(s) a trigger uses.
type TriggerMode int

const (
	// TriggerInvalid indicates an invalid or unconfigured trigger.
	TriggerInvalid TriggerMode = iota
	// TriggerStateOnly uses KV watch only.
	TriggerStateOnly
	// TriggerMessageOnly uses subject consumer only.
	TriggerMessageOnly
	// TriggerMessageAndState uses subject consumer with KV state lookup.
	TriggerMessageAndState
)

// String returns a human-readable name for the trigger mode.
func (m TriggerMode) String() string {
	switch m {
	case TriggerStateOnly:
		return "kv"
	case TriggerMessageOnly:
		return "message"
	case TriggerMessageAndState:
		return "message+state"
	default:
		return "invalid"
	}
}

// TriggerSource defines what causes a rule to evaluate. A rule can watch
// KV state, consume stream messages, or both. When both are configured,
// the message arrival is the trigger and KV state is loaded for condition
// evaluation — this enables "event + state" patterns in a single rule.
type TriggerSource struct {
	// --- KV-based trigger ---

	// WatchBucket is the KV bucket to watch for state changes.
	// When set without Subject, KV changes trigger rule evaluation.
	WatchBucket string `json:"watch_bucket,omitempty"`

	// WatchPattern is the key pattern within the bucket (supports NATS wildcards).
	WatchPattern string `json:"watch_pattern,omitempty"`

	// --- Stream/subject-based trigger ---

	// Subject is the NATS subject to consume messages from.
	// When set, message arrival triggers rule evaluation.
	// Supports NATS wildcards (e.g., "workflow.callback.plan-review.>").
	Subject string `json:"subject,omitempty"`

	// StreamName is the JetStream stream to consume from.
	// Required when Subject is set and durable consumption is needed.
	// When empty with Subject set, uses Core NATS subscription (ephemeral).
	StreamName string `json:"stream_name,omitempty"`

	// MessageFactory creates a zero-value instance of the expected message type
	// for typed deserialization. Required when Subject is set.
	MessageFactory func() any `json:"-"`

	// --- Combined trigger: message + state ---

	// StateBucket is the KV bucket to load state from when a message triggers
	// the rule. This enables "event + state" patterns where the message arrival
	// is the trigger but conditions evaluate against both the message and the
	// accumulated KV state.
	//
	// When Subject is set AND StateBucket is set:
	//   - Message arrival triggers the rule
	//   - Engine loads state from StateBucket using StateKeyFunc
	//   - RuleContext contains both .Message and .State
	//
	// When only WatchBucket is set (no Subject):
	//   - KV change triggers the rule
	//   - RuleContext contains .State only (.Message is nil)
	//
	// When only Subject is set (no StateBucket or WatchBucket):
	//   - Message arrival triggers the rule
	//   - RuleContext contains .Message only (.State is nil)
	StateBucket string `json:"state_bucket,omitempty"`

	// StateKeyFunc extracts the KV key from the triggering message to load state.
	// Required when Subject + StateBucket are both set.
	// Example: func(msg any) string {
	//     return "plan-review." + msg.(*MyCallbackResult).TaskID
	// }
	StateKeyFunc func(msg any) string `json:"-"`
}

// Mode returns which reactive primitive(s) this trigger uses.
func (t *TriggerSource) Mode() TriggerMode {
	hasKV := t.WatchBucket != ""
	hasSubject := t.Subject != ""
	hasStateLookup := t.StateBucket != ""

	switch {
	case hasSubject && (hasStateLookup || hasKV):
		return TriggerMessageAndState
	case hasSubject:
		return TriggerMessageOnly
	case hasKV:
		return TriggerStateOnly
	default:
		return TriggerInvalid
	}
}

// Validate checks if the trigger source is properly configured.
func (t *TriggerSource) Validate() error {
	mode := t.Mode()
	if mode == TriggerInvalid {
		return &ValidationError{Field: "trigger", Message: "must specify either WatchBucket or Subject"}
	}
	if t.Subject != "" && t.MessageFactory == nil {
		return &ValidationError{Field: "trigger.message_factory", Message: "required when Subject is set"}
	}
	if t.StateBucket != "" && t.StateKeyFunc == nil {
		return &ValidationError{Field: "trigger.state_key_func", Message: "required when StateBucket is set"}
	}
	return nil
}

// RuleContext provides typed access to both the accumulated state and the
// triggering event. Which fields are populated depends on the TriggerMode:
//
//	TriggerStateOnly:       State is set, Message is nil
//	TriggerMessageOnly:     Message is set, State is nil
//	TriggerMessageAndState: Both State and Message are set
type RuleContext struct {
	// State is the typed execution entity from KV.
	// Nil when trigger mode is TriggerMessageOnly.
	// Type assertion to concrete state type: ctx.State.(*PlanReviewState)
	State any

	// Message is the typed triggering message from NATS.
	// Nil when trigger mode is TriggerStateOnly.
	// Type assertion to concrete message type: ctx.Message.(*PlanReviewResult)
	Message any

	// KVRevision is the KV entry revision (for optimistic concurrency).
	// Zero when no KV state is involved.
	KVRevision uint64

	// Subject is the NATS subject the message arrived on (for subject-triggered rules).
	// Empty when trigger mode is TriggerStateOnly.
	Subject string

	// KVKey is the KV key that triggered the rule (for KV-triggered rules).
	// Empty when trigger mode is TriggerMessageOnly without state lookup.
	KVKey string
}

// ConditionFunc evaluates a condition against a RuleContext.
// Returns true if the condition is met.
// Implementations use type assertions to access typed fields on State and/or Message.
type ConditionFunc func(ctx *RuleContext) bool

// Condition is evaluated against a RuleContext using a ConditionFunc.
// No more string-based "${steps.reviewer.verdict} == 'approved'" comparisons.
type Condition struct {
	// Description is human-readable (for logging/debugging).
	Description string

	// Evaluate is the typed condition function.
	Evaluate ConditionFunc
}

// ActionType determines the action behavior.
type ActionType int

const (
	// ActionPublishAsync publishes to NATS with callback tracking. The engine
	// parks the execution until the callback arrives, then calls MutateState.
	ActionPublishAsync ActionType = iota

	// ActionPublish publishes to NATS fire-and-forget, then calls MutateState immediately.
	ActionPublish

	// ActionMutate updates KV state without publishing. The state change may
	// trigger other rules via KV watch.
	ActionMutate

	// ActionComplete marks the execution as completed and publishes terminal events.
	ActionComplete
)

// String returns a human-readable name for the action type.
func (a ActionType) String() string {
	switch a {
	case ActionPublishAsync:
		return "publish_async"
	case ActionPublish:
		return "publish"
	case ActionMutate:
		return "mutate"
	case ActionComplete:
		return "complete"
	default:
		return "unknown"
	}
}

// PayloadBuilderFunc constructs a typed NATS message payload from rule context.
// Has access to both state and message for maximum flexibility.
// Output: a typed payload struct ready for json.Marshal + NATS publish.
type PayloadBuilderFunc func(ctx *RuleContext) (message.Payload, error)

// StateMutatorFunc updates the execution state after an action completes.
// For sync actions (Mutate, Complete, Publish), result is nil.
// For async actions (PublishAsync), result is the typed callback result.
// The mutated state is written back to KV, potentially triggering other rules.
type StateMutatorFunc func(ctx *RuleContext, result any) error

// Action defines what happens when a rule fires.
type Action struct {
	// Type determines the action behavior.
	Type ActionType

	// PublishSubject for publish/publish_async actions.
	PublishSubject string

	// ExpectedResultType is the registered payload type name for async results.
	// Required for ActionPublishAsync so the engine can deserialize the callback.
	// Example: "workflow.planner-result.v1"
	ExpectedResultType string

	// BuildPayload constructs the typed payload from the rule context.
	// This replaces string interpolation entirely — it's a Go function that reads
	// typed fields from state and/or message and produces a typed payload.
	// Required for ActionPublish and ActionPublishAsync.
	BuildPayload PayloadBuilderFunc

	// MutateState updates the execution entity after the action completes.
	// For sync actions, called immediately with result=nil.
	// For async actions, called when the callback arrives with the typed result.
	// The mutated state is written back to KV, which may trigger other rules.
	MutateState StateMutatorFunc
}

// Validate checks if the action is properly configured.
func (a *Action) Validate() error {
	switch a.Type {
	case ActionPublishAsync:
		if a.PublishSubject == "" {
			return &ValidationError{Field: "action.publish_subject", Message: "required for publish_async"}
		}
		if a.BuildPayload == nil {
			return &ValidationError{Field: "action.build_payload", Message: "required for publish_async"}
		}
		if a.ExpectedResultType == "" {
			return &ValidationError{Field: "action.expected_result_type", Message: "required for publish_async"}
		}
	case ActionPublish:
		if a.PublishSubject == "" {
			return &ValidationError{Field: "action.publish_subject", Message: "required for publish"}
		}
		if a.BuildPayload == nil {
			return &ValidationError{Field: "action.build_payload", Message: "required for publish"}
		}
	case ActionMutate:
		if a.MutateState == nil {
			return &ValidationError{Field: "action.mutate_state", Message: "required for mutate"}
		}
	case ActionComplete:
		// MutateState is optional for complete
	}
	return nil
}

// RuleDef defines a reactive rule. This is the Go-native equivalent of
// the current JSON workflow step definitions.
type RuleDef struct {
	// ID is the unique rule identifier within a workflow.
	ID string

	// Trigger defines what causes this rule to evaluate.
	Trigger TriggerSource

	// Conditions that must all be true for the rule to fire.
	// Evaluated against RuleContext which provides typed access to
	// both KV state and triggering message.
	Conditions []Condition

	// Logic determines how conditions are combined ("and" or "or").
	// Default is "and" (all conditions must be true).
	Logic string

	// Action to perform when conditions are met.
	Action Action

	// Cooldown prevents re-firing within this duration.
	Cooldown time.Duration

	// MaxFirings limits how many times this rule can fire per execution (0 = unlimited).
	MaxFirings int
}

// Validate checks if the rule definition is properly configured.
func (r *RuleDef) Validate() error {
	if r.ID == "" {
		return &ValidationError{Field: "rule.id", Message: "required"}
	}
	if err := r.Trigger.Validate(); err != nil {
		return err
	}
	if err := r.Action.Validate(); err != nil {
		return err
	}
	if r.Logic != "" && r.Logic != "and" && r.Logic != "or" {
		return &ValidationError{Field: "rule.logic", Message: "must be 'and' or 'or'"}
	}
	return nil
}

// EventConfig defines typed event subjects for external consumers.
type EventConfig struct {
	// OnComplete is the subject to publish when the workflow completes successfully.
	OnComplete string

	// OnFail is the subject to publish when the workflow fails.
	OnFail string

	// OnEscalate is the subject to publish when the workflow is escalated.
	OnEscalate string
}

// Definition defines a reactive workflow as Go code.
type Definition struct {
	// ID is the unique workflow identifier.
	ID string

	// Description is a human-readable description.
	Description string

	// StateBucket is the KV bucket for storing execution state.
	StateBucket string

	// StateFactory creates a zero-value instance of the workflow's concrete state type.
	// Must return a pointer to a struct that embeds ExecutionState.
	StateFactory func() any

	// MaxIterations limits loop iterations (0 = unlimited).
	MaxIterations int

	// Timeout is the maximum execution duration.
	Timeout time.Duration

	// Rules defines the reactive rules for this workflow.
	// Rules are evaluated in order; the first matching rule fires.
	Rules []RuleDef

	// Events defines typed event subjects for workflow lifecycle events.
	Events EventConfig
}

// Validate checks if the workflow definition is properly configured.
func (d *Definition) Validate() error {
	if d.ID == "" {
		return &ValidationError{Field: "workflow.id", Message: "required"}
	}
	if d.StateBucket == "" {
		return &ValidationError{Field: "workflow.state_bucket", Message: "required"}
	}
	if d.StateFactory == nil {
		return &ValidationError{Field: "workflow.state_factory", Message: "required"}
	}
	if len(d.Rules) == 0 {
		return &ValidationError{Field: "workflow.rules", Message: "at least one rule required"}
	}
	for i, rule := range d.Rules {
		if err := rule.Validate(); err != nil {
			return &ValidationError{Field: "workflow.rules[" + strconv.Itoa(i) + "]", Message: err.Error()}
		}
	}
	return nil
}

// ValidationError represents a validation error for a specific field.
type ValidationError struct {
	Field   string
	Message string
}

// Error implements the error interface.
func (e *ValidationError) Error() string {
	return e.Field + ": " + e.Message
}

// CallbackFields are injected into async task payloads for callback correlation.
// Used by the reactive workflow engine to track async step execution.
type CallbackFields struct {
	// TaskID uniquely identifies this async task.
	TaskID string `json:"task_id"`

	// CallbackSubject is where the executor should publish the result.
	CallbackSubject string `json:"callback_subject"`

	// ExecutionID is the workflow execution ID for direct lookup.
	ExecutionID string `json:"execution_id"`
}
