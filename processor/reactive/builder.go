package reactive

import (
	"time"

	"github.com/c360studio/semstreams/message"
)

// WorkflowBuilder provides a fluent API for building workflow definitions.
type WorkflowBuilder struct {
	def    Definition
	errors []error
}

// NewWorkflow starts building a new workflow definition.
func NewWorkflow(id string) *WorkflowBuilder {
	return &WorkflowBuilder{
		def: Definition{
			ID: id,
		},
	}
}

// WithDescription sets the workflow description.
func (b *WorkflowBuilder) WithDescription(desc string) *WorkflowBuilder {
	b.def.Description = desc
	return b
}

// WithStateBucket sets the KV bucket for storing execution state.
func (b *WorkflowBuilder) WithStateBucket(bucket string) *WorkflowBuilder {
	b.def.StateBucket = bucket
	return b
}

// WithStateFactory sets the factory function for creating typed state instances.
// The factory must return a pointer to a struct that embeds ExecutionState.
func (b *WorkflowBuilder) WithStateFactory(factory func() any) *WorkflowBuilder {
	b.def.StateFactory = factory
	return b
}

// WithMaxIterations sets the maximum loop iterations for this workflow.
func (b *WorkflowBuilder) WithMaxIterations(n int) *WorkflowBuilder {
	b.def.MaxIterations = n
	return b
}

// WithTimeout sets the maximum execution duration for this workflow.
func (b *WorkflowBuilder) WithTimeout(d time.Duration) *WorkflowBuilder {
	b.def.Timeout = d
	return b
}

// WithEvents sets the event configuration for workflow lifecycle events.
func (b *WorkflowBuilder) WithEvents(events EventConfig) *WorkflowBuilder {
	b.def.Events = events
	return b
}

// WithOnComplete sets the subject to publish when the workflow completes successfully.
func (b *WorkflowBuilder) WithOnComplete(subject string) *WorkflowBuilder {
	b.def.Events.OnComplete = subject
	return b
}

// WithOnFail sets the subject to publish when the workflow fails.
func (b *WorkflowBuilder) WithOnFail(subject string) *WorkflowBuilder {
	b.def.Events.OnFail = subject
	return b
}

// WithOnEscalate sets the subject to publish when the workflow is escalated.
func (b *WorkflowBuilder) WithOnEscalate(subject string) *WorkflowBuilder {
	b.def.Events.OnEscalate = subject
	return b
}

// AddRule adds a rule to the workflow.
func (b *WorkflowBuilder) AddRule(rule RuleDef) *WorkflowBuilder {
	b.def.Rules = append(b.def.Rules, rule)
	return b
}

// AddRuleFromBuilder adds a rule built using RuleBuilder.
func (b *WorkflowBuilder) AddRuleFromBuilder(rb *RuleBuilder) *WorkflowBuilder {
	rule, err := rb.Build()
	if err != nil {
		b.errors = append(b.errors, err)
		return b
	}
	b.def.Rules = append(b.def.Rules, rule)
	return b
}

// Build validates and returns the workflow definition.
func (b *WorkflowBuilder) Build() (*Definition, error) {
	// Check for accumulated errors
	if len(b.errors) > 0 {
		return nil, b.errors[0]
	}

	// Validate the definition
	if err := b.def.Validate(); err != nil {
		return nil, err
	}

	return &b.def, nil
}

// MustBuild validates and returns the workflow definition, panicking on error.
// Use this only in tests or during application initialization.
func (b *WorkflowBuilder) MustBuild() *Definition {
	def, err := b.Build()
	if err != nil {
		panic("workflow builder: " + err.Error())
	}
	return def
}

// RuleBuilder provides a fluent API for building rule definitions.
type RuleBuilder struct {
	rule   RuleDef
	errors []error
}

// NewRule starts building a new rule definition.
func NewRule(id string) *RuleBuilder {
	return &RuleBuilder{
		rule: RuleDef{
			ID:    id,
			Logic: "and", // Default logic
		},
	}
}

// WatchKV sets the rule to trigger on KV state changes.
func (b *RuleBuilder) WatchKV(bucket, pattern string) *RuleBuilder {
	b.rule.Trigger.WatchBucket = bucket
	b.rule.Trigger.WatchPattern = pattern
	return b
}

// OnSubject sets the rule to trigger on NATS subject messages.
func (b *RuleBuilder) OnSubject(subject string, messageFactory func() any) *RuleBuilder {
	b.rule.Trigger.Subject = subject
	b.rule.Trigger.MessageFactory = messageFactory
	return b
}

// OnJetStreamSubject sets the rule to trigger on JetStream messages.
func (b *RuleBuilder) OnJetStreamSubject(streamName, subject string, messageFactory func() any) *RuleBuilder {
	b.rule.Trigger.StreamName = streamName
	b.rule.Trigger.Subject = subject
	b.rule.Trigger.MessageFactory = messageFactory
	return b
}

// WithStateLookup configures state loading for message-triggered rules.
// This enables "event + state" patterns where the message arrival triggers
// the rule but conditions evaluate against both message and KV state.
func (b *RuleBuilder) WithStateLookup(bucket string, keyFunc func(msg any) string) *RuleBuilder {
	b.rule.Trigger.StateBucket = bucket
	b.rule.Trigger.StateKeyFunc = keyFunc
	return b
}

// When adds a condition that must be true for the rule to fire.
func (b *RuleBuilder) When(description string, condition ConditionFunc) *RuleBuilder {
	b.rule.Conditions = append(b.rule.Conditions, Condition{
		Description: description,
		Evaluate:    condition,
	})
	return b
}

// WhenAll sets the rule to require all conditions to be true (AND logic).
// This is the default behavior.
func (b *RuleBuilder) WhenAll() *RuleBuilder {
	b.rule.Logic = "and"
	return b
}

// WhenAny sets the rule to require any condition to be true (OR logic).
func (b *RuleBuilder) WhenAny() *RuleBuilder {
	b.rule.Logic = "or"
	return b
}

// PublishAsync sets the action to publish a message and wait for a callback.
// Note: Only one action method should be called per rule. If multiple action methods
// are called, only the last one takes effect (actions are mutually exclusive).
func (b *RuleBuilder) PublishAsync(
	subject string,
	buildPayload PayloadBuilderFunc,
	expectedResultType string,
	mutateState StateMutatorFunc,
) *RuleBuilder {
	b.rule.Action = Action{
		Type:               ActionPublishAsync,
		PublishSubject:     subject,
		BuildPayload:       buildPayload,
		ExpectedResultType: expectedResultType,
		MutateState:        mutateState,
	}
	return b
}

// Publish sets the action to publish a message fire-and-forget.
func (b *RuleBuilder) Publish(subject string, buildPayload PayloadBuilderFunc) *RuleBuilder {
	b.rule.Action = Action{
		Type:           ActionPublish,
		PublishSubject: subject,
		BuildPayload:   buildPayload,
	}
	return b
}

// PublishWithMutation sets the action to publish a message and mutate state.
func (b *RuleBuilder) PublishWithMutation(
	subject string,
	buildPayload PayloadBuilderFunc,
	mutateState StateMutatorFunc,
) *RuleBuilder {
	b.rule.Action = Action{
		Type:           ActionPublish,
		PublishSubject: subject,
		BuildPayload:   buildPayload,
		MutateState:    mutateState,
	}
	return b
}

// Mutate sets the action to update state without publishing.
func (b *RuleBuilder) Mutate(mutateState StateMutatorFunc) *RuleBuilder {
	b.rule.Action = Action{
		Type:        ActionMutate,
		MutateState: mutateState,
	}
	return b
}

// Complete sets the action to mark the execution as completed.
func (b *RuleBuilder) Complete() *RuleBuilder {
	b.rule.Action = Action{
		Type: ActionComplete,
	}
	return b
}

// CompleteWithMutation sets the action to mark the execution as completed
// after applying a final state mutation.
func (b *RuleBuilder) CompleteWithMutation(mutateState StateMutatorFunc) *RuleBuilder {
	b.rule.Action = Action{
		Type:        ActionComplete,
		MutateState: mutateState,
	}
	return b
}

// CompleteWithEvent sets the action to mark the execution as completed
// and publish a completion event.
func (b *RuleBuilder) CompleteWithEvent(subject string, buildPayload PayloadBuilderFunc) *RuleBuilder {
	b.rule.Action = Action{
		Type:           ActionComplete,
		PublishSubject: subject,
		BuildPayload:   buildPayload,
	}
	return b
}

// WithCooldown sets the cooldown period to prevent rapid re-firing.
func (b *RuleBuilder) WithCooldown(d time.Duration) *RuleBuilder {
	b.rule.Cooldown = d
	return b
}

// WithMaxFirings limits how many times this rule can fire per execution.
func (b *RuleBuilder) WithMaxFirings(n int) *RuleBuilder {
	b.rule.MaxFirings = n
	return b
}

// Build validates and returns the rule definition.
func (b *RuleBuilder) Build() (RuleDef, error) {
	if len(b.errors) > 0 {
		return RuleDef{}, b.errors[0]
	}

	if err := b.rule.Validate(); err != nil {
		return RuleDef{}, err
	}

	return b.rule, nil
}

// MustBuild validates and returns the rule definition, panicking on error.
// Use this only in tests or during application initialization.
func (b *RuleBuilder) MustBuild() RuleDef {
	rule, err := b.Build()
	if err != nil {
		panic("rule builder: " + err.Error())
	}
	return rule
}

// Convenience factory functions for common patterns

// SimplePayloadBuilder creates a PayloadBuilderFunc that returns a fixed payload.
// Useful for simple cases where the payload doesn't depend on context.
func SimplePayloadBuilder(payload message.Payload) PayloadBuilderFunc {
	return func(_ *RuleContext) (message.Payload, error) {
		return payload, nil
	}
}

// PhaseTransition creates a StateMutatorFunc that updates the execution phase.
// This is a common pattern for workflows that progress through phases.
func PhaseTransition(newPhase string) StateMutatorFunc {
	return func(ctx *RuleContext, _ any) error {
		if ctx.State == nil {
			return nil
		}
		es := ExtractExecutionState(ctx.State)
		if es != nil {
			es.Phase = newPhase
		}
		return nil
	}
}

// IncrementIterationMutator creates a StateMutatorFunc that increments the iteration counter.
func IncrementIterationMutator() StateMutatorFunc {
	return func(ctx *RuleContext, _ any) error {
		if ctx.State == nil {
			return nil
		}
		IncrementIteration(ctx.State)
		return nil
	}
}

// SetErrorMutator creates a StateMutatorFunc that sets an error on the execution state.
func SetErrorMutator(errorMsg string) StateMutatorFunc {
	return func(ctx *RuleContext, _ any) error {
		if ctx.State == nil {
			return nil
		}
		SetError(ctx.State, errorMsg)
		SetStatus(ctx.State, StatusFailed)
		return nil
	}
}

// ChainMutators combines multiple state mutators into a single mutator.
// They are executed in order.
func ChainMutators(mutators ...StateMutatorFunc) StateMutatorFunc {
	return func(ctx *RuleContext, result any) error {
		for _, m := range mutators {
			if m != nil {
				if err := m(ctx, result); err != nil {
					return err
				}
			}
		}
		return nil
	}
}
