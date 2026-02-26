package reactive

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/c360studio/semstreams/message"
)

// BuilderTestState is a test state type that embeds ExecutionState.
type BuilderTestState struct {
	ExecutionState
	CustomField string `json:"custom_field"`
	Counter     int    `json:"counter"`
}

// GetExecutionState implements StateAccessor to avoid reflection.
func (s *BuilderTestState) GetExecutionState() *ExecutionState {
	return &s.ExecutionState
}

// BuilderTestPayload is a test message payload.
type BuilderTestPayload struct {
	Value string `json:"value"`
}

func (p *BuilderTestPayload) Schema() message.Type {
	return message.Type{Domain: "test", Category: "builder-payload", Version: "v1"}
}

func (p *BuilderTestPayload) Validate() error {
	return nil
}

func (p *BuilderTestPayload) MarshalJSON() ([]byte, error) {
	type Alias BuilderTestPayload
	return json.Marshal((*Alias)(p))
}

func (p *BuilderTestPayload) UnmarshalJSON(data []byte) error {
	type Alias BuilderTestPayload
	return json.Unmarshal(data, (*Alias)(p))
}

func TestWorkflowBuilder_Basic(t *testing.T) {
	def, err := NewWorkflow("test-workflow").
		WithDescription("Test workflow").
		WithStateBucket("TEST_BUCKET").
		WithStateFactory(func() any { return &BuilderTestState{} }).
		WithMaxIterations(10).
		WithTimeout(5 * time.Minute).
		AddRule(NewRule("rule-1").
			WatchKV("TEST_BUCKET", "test.*").
			When("always true", Always()).
			Mutate(func(_ *RuleContext, _ any) error { return nil }).
			MustBuild()).
		Build()

	if err != nil {
		t.Fatalf("Build failed: %v", err)
	}

	if def.ID != "test-workflow" {
		t.Errorf("Expected ID 'test-workflow', got %q", def.ID)
	}
	if def.Description != "Test workflow" {
		t.Errorf("Expected Description 'Test workflow', got %q", def.Description)
	}
	if def.StateBucket != "TEST_BUCKET" {
		t.Errorf("Expected StateBucket 'TEST_BUCKET', got %q", def.StateBucket)
	}
	if def.MaxIterations != 10 {
		t.Errorf("Expected MaxIterations 10, got %d", def.MaxIterations)
	}
	if def.Timeout != 5*time.Minute {
		t.Errorf("Expected Timeout 5m, got %v", def.Timeout)
	}
	if len(def.Rules) != 1 {
		t.Errorf("Expected 1 rule, got %d", len(def.Rules))
	}
}

func TestWorkflowBuilder_Events(t *testing.T) {
	def, err := NewWorkflow("event-workflow").
		WithStateBucket("BUCKET").
		WithStateFactory(func() any { return &BuilderTestState{} }).
		WithOnComplete("workflow.complete").
		WithOnFail("workflow.fail").
		WithOnEscalate("workflow.escalate").
		AddRule(NewRule("rule-1").
			WatchKV("BUCKET", "*").
			When("always", Always()).
			Complete().
			MustBuild()).
		Build()

	if err != nil {
		t.Fatalf("Build failed: %v", err)
	}

	if def.Events.OnComplete != "workflow.complete" {
		t.Errorf("Expected OnComplete 'workflow.complete', got %q", def.Events.OnComplete)
	}
	if def.Events.OnFail != "workflow.fail" {
		t.Errorf("Expected OnFail 'workflow.fail', got %q", def.Events.OnFail)
	}
	if def.Events.OnEscalate != "workflow.escalate" {
		t.Errorf("Expected OnEscalate 'workflow.escalate', got %q", def.Events.OnEscalate)
	}
}

func TestWorkflowBuilder_ValidationErrors(t *testing.T) {
	tests := []struct {
		name    string
		builder *WorkflowBuilder
		wantErr string
	}{
		{
			name:    "missing ID",
			builder: NewWorkflow("").WithStateBucket("BUCKET").WithStateFactory(func() any { return &BuilderTestState{} }),
			wantErr: "workflow.id",
		},
		{
			name:    "missing state bucket",
			builder: NewWorkflow("test").WithStateFactory(func() any { return &BuilderTestState{} }),
			wantErr: "workflow.state_bucket",
		},
		{
			name:    "missing state factory",
			builder: NewWorkflow("test").WithStateBucket("BUCKET"),
			wantErr: "workflow.state_factory",
		},
		{
			name: "no rules",
			builder: NewWorkflow("test").
				WithStateBucket("BUCKET").
				WithStateFactory(func() any { return &BuilderTestState{} }),
			wantErr: "workflow.rules",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := tt.builder.Build()
			if err == nil {
				t.Error("Expected error, got nil")
				return
			}
			if !contains(err.Error(), tt.wantErr) {
				t.Errorf("Expected error containing %q, got %q", tt.wantErr, err.Error())
			}
		})
	}
}

func TestRuleBuilder_WatchKV(t *testing.T) {
	rule, err := NewRule("kv-rule").
		WatchKV("TEST_BUCKET", "state.*").
		When("phase is pending", PhaseIs("pending")).
		Mutate(PhaseTransition("running")).
		Build()

	if err != nil {
		t.Fatalf("Build failed: %v", err)
	}

	if rule.ID != "kv-rule" {
		t.Errorf("Expected ID 'kv-rule', got %q", rule.ID)
	}
	if rule.Trigger.WatchBucket != "TEST_BUCKET" {
		t.Errorf("Expected WatchBucket 'TEST_BUCKET', got %q", rule.Trigger.WatchBucket)
	}
	if rule.Trigger.WatchPattern != "state.*" {
		t.Errorf("Expected WatchPattern 'state.*', got %q", rule.Trigger.WatchPattern)
	}
	if rule.Trigger.Mode() != TriggerStateOnly {
		t.Errorf("Expected TriggerStateOnly, got %v", rule.Trigger.Mode())
	}
}

func TestRuleBuilder_OnSubject(t *testing.T) {
	rule, err := NewRule("subject-rule").
		OnSubject("test.events.>", func() any { return &BuilderTestPayload{} }).
		When("has message", HasMessage()).
		Publish("test.output", func(_ *RuleContext) (message.Payload, error) {
			return &BuilderTestPayload{Value: "output"}, nil
		}).
		Build()

	if err != nil {
		t.Fatalf("Build failed: %v", err)
	}

	if rule.Trigger.Subject != "test.events.>" {
		t.Errorf("Expected Subject 'test.events.>', got %q", rule.Trigger.Subject)
	}
	if rule.Trigger.MessageFactory == nil {
		t.Error("Expected MessageFactory to be set")
	}
	if rule.Trigger.Mode() != TriggerMessageOnly {
		t.Errorf("Expected TriggerMessageOnly, got %v", rule.Trigger.Mode())
	}
}

func TestRuleBuilder_OnJetStreamSubject(t *testing.T) {
	rule, err := NewRule("js-rule").
		OnJetStreamSubject("EVENTS", "events.>", func() any { return &BuilderTestPayload{} }).
		When("always", Always()).
		Mutate(func(_ *RuleContext, _ any) error { return nil }).
		Build()

	if err != nil {
		t.Fatalf("Build failed: %v", err)
	}

	if rule.Trigger.StreamName != "EVENTS" {
		t.Errorf("Expected StreamName 'EVENTS', got %q", rule.Trigger.StreamName)
	}
	if rule.Trigger.Subject != "events.>" {
		t.Errorf("Expected Subject 'events.>', got %q", rule.Trigger.Subject)
	}
}

func TestRuleBuilder_CombinedTrigger(t *testing.T) {
	rule, err := NewRule("combined-rule").
		OnSubject("callback.>", func() any { return &BuilderTestPayload{} }).
		WithStateLookup("STATE_BUCKET", func(_ any) string {
			return "state-key"
		}).
		When("has both", And(HasState(), HasMessage())).
		Mutate(PhaseTransition("completed")).
		Build()

	if err != nil {
		t.Fatalf("Build failed: %v", err)
	}

	if rule.Trigger.Mode() != TriggerMessageAndState {
		t.Errorf("Expected TriggerMessageAndState, got %v", rule.Trigger.Mode())
	}
	if rule.Trigger.StateBucket != "STATE_BUCKET" {
		t.Errorf("Expected StateBucket 'STATE_BUCKET', got %q", rule.Trigger.StateBucket)
	}
	if rule.Trigger.StateKeyFunc == nil {
		t.Error("Expected StateKeyFunc to be set")
	}

	// Critical: WatchBucket should be empty when using StateLookup only.
	// The engine must NOT try to start a KV watch for this rule.
	// This prevents the "nats: invalid bucket name" error when WatchBucket is empty.
	if rule.Trigger.WatchBucket != "" {
		t.Errorf("Expected WatchBucket to be empty for StateLookup-only rule, got %q", rule.Trigger.WatchBucket)
	}
}

func TestRuleBuilder_PublishAsync(t *testing.T) {
	rule, err := NewRule("async-rule").
		WatchKV("BUCKET", "*").
		When("pending", PhaseIs("pending")).
		PublishAsync(
			"task.input",
			func(_ *RuleContext) (message.Payload, error) {
				return &BuilderTestPayload{Value: "task"}, nil
			},
			"test.result.v1",
			func(_ *RuleContext, _ any) error {
				// Apply result
				return nil
			},
		).
		Build()

	if err != nil {
		t.Fatalf("Build failed: %v", err)
	}

	if rule.Action.Type != ActionPublishAsync {
		t.Errorf("Expected ActionPublishAsync, got %v", rule.Action.Type)
	}
	if rule.Action.PublishSubject != "task.input" {
		t.Errorf("Expected PublishSubject 'task.input', got %q", rule.Action.PublishSubject)
	}
	if rule.Action.ExpectedResultType != "test.result.v1" {
		t.Errorf("Expected ExpectedResultType 'test.result.v1', got %q", rule.Action.ExpectedResultType)
	}
	if rule.Action.BuildPayload == nil {
		t.Error("Expected BuildPayload to be set")
	}
	if rule.Action.MutateState == nil {
		t.Error("Expected MutateState to be set")
	}
}

func TestRuleBuilder_Publish(t *testing.T) {
	rule, err := NewRule("publish-rule").
		WatchKV("BUCKET", "*").
		When("running", PhaseIs("running")).
		Publish("output.subject", SimplePayloadBuilder(&BuilderTestPayload{Value: "test"})).
		Build()

	if err != nil {
		t.Fatalf("Build failed: %v", err)
	}

	if rule.Action.Type != ActionPublish {
		t.Errorf("Expected ActionPublish, got %v", rule.Action.Type)
	}
}

func TestRuleBuilder_PublishWithMutation(t *testing.T) {
	rule, err := NewRule("publish-mutate-rule").
		WatchKV("BUCKET", "*").
		When("always", Always()).
		PublishWithMutation(
			"output.subject",
			SimplePayloadBuilder(&BuilderTestPayload{Value: "test"}),
			PhaseTransition("next"),
		).
		Build()

	if err != nil {
		t.Fatalf("Build failed: %v", err)
	}

	if rule.Action.Type != ActionPublish {
		t.Errorf("Expected ActionPublish, got %v", rule.Action.Type)
	}
	if rule.Action.MutateState == nil {
		t.Error("Expected MutateState to be set")
	}
}

func TestRuleBuilder_Mutate(t *testing.T) {
	rule, err := NewRule("mutate-rule").
		WatchKV("BUCKET", "*").
		When("always", Always()).
		Mutate(PhaseTransition("completed")).
		Build()

	if err != nil {
		t.Fatalf("Build failed: %v", err)
	}

	if rule.Action.Type != ActionMutate {
		t.Errorf("Expected ActionMutate, got %v", rule.Action.Type)
	}
}

func TestRuleBuilder_Complete(t *testing.T) {
	rule, err := NewRule("complete-rule").
		WatchKV("BUCKET", "*").
		When("done", PhaseIs("done")).
		Complete().
		Build()

	if err != nil {
		t.Fatalf("Build failed: %v", err)
	}

	if rule.Action.Type != ActionComplete {
		t.Errorf("Expected ActionComplete, got %v", rule.Action.Type)
	}
}

func TestRuleBuilder_CompleteWithMutation(t *testing.T) {
	rule, err := NewRule("complete-mutate-rule").
		WatchKV("BUCKET", "*").
		When("done", PhaseIs("done")).
		CompleteWithMutation(PhaseTransition("completed")).
		Build()

	if err != nil {
		t.Fatalf("Build failed: %v", err)
	}

	if rule.Action.Type != ActionComplete {
		t.Errorf("Expected ActionComplete, got %v", rule.Action.Type)
	}
	if rule.Action.MutateState == nil {
		t.Error("Expected MutateState to be set")
	}
}

func TestRuleBuilder_CooldownAndMaxFirings(t *testing.T) {
	rule, err := NewRule("limited-rule").
		WatchKV("BUCKET", "*").
		When("always", Always()).
		Mutate(func(_ *RuleContext, _ any) error { return nil }).
		WithCooldown(5 * time.Second).
		WithMaxFirings(3).
		Build()

	if err != nil {
		t.Fatalf("Build failed: %v", err)
	}

	if rule.Cooldown != 5*time.Second {
		t.Errorf("Expected Cooldown 5s, got %v", rule.Cooldown)
	}
	if rule.MaxFirings != 3 {
		t.Errorf("Expected MaxFirings 3, got %d", rule.MaxFirings)
	}
}

func TestRuleBuilder_Logic(t *testing.T) {
	t.Run("WhenAll (default)", func(t *testing.T) {
		rule := NewRule("and-rule").
			WatchKV("BUCKET", "*").
			WhenAll().
			When("cond1", Always()).
			When("cond2", Always()).
			Mutate(func(_ *RuleContext, _ any) error { return nil }).
			MustBuild()

		if rule.Logic != "and" {
			t.Errorf("Expected Logic 'and', got %q", rule.Logic)
		}
	})

	t.Run("WhenAny", func(t *testing.T) {
		rule := NewRule("or-rule").
			WatchKV("BUCKET", "*").
			WhenAny().
			When("cond1", Always()).
			When("cond2", Never()).
			Mutate(func(_ *RuleContext, _ any) error { return nil }).
			MustBuild()

		if rule.Logic != "or" {
			t.Errorf("Expected Logic 'or', got %q", rule.Logic)
		}
	})
}

func TestRuleBuilder_ValidationErrors(t *testing.T) {
	tests := []struct {
		name    string
		builder *RuleBuilder
		wantErr string
	}{
		{
			name:    "missing ID",
			builder: NewRule("").WatchKV("BUCKET", "*").Mutate(func(_ *RuleContext, _ any) error { return nil }),
			wantErr: "rule.id",
		},
		{
			name:    "missing trigger",
			builder: NewRule("test").Mutate(func(_ *RuleContext, _ any) error { return nil }),
			wantErr: "trigger",
		},
		{
			name:    "subject without message factory",
			builder: NewRule("test").OnSubject("events.>", nil).Mutate(func(_ *RuleContext, _ any) error { return nil }),
			wantErr: "message_factory",
		},
		{
			name:    "publish without subject",
			builder: NewRule("test").WatchKV("BUCKET", "*").Publish("", nil),
			wantErr: "publish_subject",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := tt.builder.Build()
			if err == nil {
				t.Error("Expected error, got nil")
				return
			}
			if !contains(err.Error(), tt.wantErr) {
				t.Errorf("Expected error containing %q, got %q", tt.wantErr, err.Error())
			}
		})
	}
}

func TestMustBuild_Panics(t *testing.T) {
	t.Run("workflow panic", func(t *testing.T) {
		defer func() {
			if r := recover(); r == nil {
				t.Error("Expected panic, got none")
			}
		}()
		NewWorkflow("").MustBuild()
	})

	t.Run("rule panic", func(t *testing.T) {
		defer func() {
			if r := recover(); r == nil {
				t.Error("Expected panic, got none")
			}
		}()
		NewRule("").MustBuild()
	})
}

func TestChainMutators(t *testing.T) {
	state := &BuilderTestState{
		ExecutionState: ExecutionState{
			Phase:     "initial",
			Iteration: 0,
		},
	}
	ctx := &RuleContext{State: state}

	chained := ChainMutators(
		PhaseTransition("phase1"),
		IncrementIterationMutator(),
		PhaseTransition("phase2"),
	)

	err := chained(ctx, nil)
	if err != nil {
		t.Fatalf("ChainMutators failed: %v", err)
	}

	if state.Phase != "phase2" {
		t.Errorf("Expected Phase 'phase2', got %q", state.Phase)
	}
	if state.Iteration != 1 {
		t.Errorf("Expected Iteration 1, got %d", state.Iteration)
	}
}

func TestSimplePayloadBuilder(t *testing.T) {
	payload := &BuilderTestPayload{Value: "test"}
	builder := SimplePayloadBuilder(payload)

	result, err := builder(&RuleContext{})
	if err != nil {
		t.Fatalf("SimplePayloadBuilder failed: %v", err)
	}

	if result != payload {
		t.Error("Expected same payload instance")
	}
}

func TestPhaseTransition(t *testing.T) {
	state := &BuilderTestState{
		ExecutionState: ExecutionState{Phase: "old"},
	}
	ctx := &RuleContext{State: state}

	mutator := PhaseTransition("new")
	if err := mutator(ctx, nil); err != nil {
		t.Fatalf("PhaseTransition failed: %v", err)
	}

	if state.Phase != "new" {
		t.Errorf("Expected Phase 'new', got %q", state.Phase)
	}
}

func TestSetErrorMutator(t *testing.T) {
	state := &BuilderTestState{
		ExecutionState: ExecutionState{
			Status: StatusRunning,
		},
	}
	ctx := &RuleContext{State: state}

	mutator := SetErrorMutator("something went wrong")
	if err := mutator(ctx, nil); err != nil {
		t.Fatalf("SetErrorMutator failed: %v", err)
	}

	if state.Error != "something went wrong" {
		t.Errorf("Expected Error 'something went wrong', got %q", state.Error)
	}
	if state.Status != StatusFailed {
		t.Errorf("Expected Status StatusFailed, got %v", state.Status)
	}
}

// Helper function using standard library
func contains(s, substr string) bool {
	return strings.Contains(s, substr)
}
