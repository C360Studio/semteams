package reactive

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/c360studio/semstreams/message"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestExecutionStatus tests the ExecutionStatus constants.
func TestExecutionStatus(t *testing.T) {
	tests := []struct {
		status   ExecutionStatus
		expected string
	}{
		{StatusPending, "pending"},
		{StatusRunning, "running"},
		{StatusWaiting, "waiting"},
		{StatusCompleted, "completed"},
		{StatusFailed, "failed"},
		{StatusEscalated, "escalated"},
		{StatusTimedOut, "timed_out"},
	}

	for _, tt := range tests {
		t.Run(string(tt.status), func(t *testing.T) {
			assert.Equal(t, tt.expected, string(tt.status))
		})
	}
}

// TestTriggerModeString tests TriggerMode.String().
func TestTriggerModeString(t *testing.T) {
	tests := []struct {
		mode     TriggerMode
		expected string
	}{
		{TriggerInvalid, "invalid"},
		{TriggerStateOnly, "kv"},
		{TriggerMessageOnly, "message"},
		{TriggerMessageAndState, "message+state"},
	}

	for _, tt := range tests {
		t.Run(tt.expected, func(t *testing.T) {
			assert.Equal(t, tt.expected, tt.mode.String())
		})
	}
}

// TestTriggerSourceMode tests TriggerSource.Mode() for all trigger patterns.
func TestTriggerSourceMode(t *testing.T) {
	tests := []struct {
		name     string
		trigger  TriggerSource
		expected TriggerMode
	}{
		{
			name:     "invalid - no config",
			trigger:  TriggerSource{},
			expected: TriggerInvalid,
		},
		{
			name: "state only - KV watch",
			trigger: TriggerSource{
				WatchBucket:  "EXECUTIONS",
				WatchPattern: "plan-review.*",
			},
			expected: TriggerStateOnly,
		},
		{
			name: "message only - subject consumer",
			trigger: TriggerSource{
				Subject:        "workflow.trigger.plan-review",
				StreamName:     "WORKFLOW",
				MessageFactory: func() any { return &struct{}{} },
			},
			expected: TriggerMessageOnly,
		},
		{
			name: "message and state - combined trigger with StateBucket",
			trigger: TriggerSource{
				Subject:        "workflow.callback.plan-review.>",
				StreamName:     "WORKFLOW",
				MessageFactory: func() any { return &struct{}{} },
				StateBucket:    "EXECUTIONS",
				StateKeyFunc:   func(_ any) string { return "key" },
			},
			expected: TriggerMessageAndState,
		},
		{
			name: "message and state - combined trigger with WatchBucket",
			trigger: TriggerSource{
				Subject:        "workflow.callback.>",
				StreamName:     "WORKFLOW",
				MessageFactory: func() any { return &struct{}{} },
				WatchBucket:    "EXECUTIONS",
				WatchPattern:   "*",
			},
			expected: TriggerMessageAndState,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expected, tt.trigger.Mode())
		})
	}
}

// TestTriggerSourceValidate tests TriggerSource.Validate().
func TestTriggerSourceValidate(t *testing.T) {
	tests := []struct {
		name        string
		trigger     TriggerSource
		expectError bool
		errorField  string
	}{
		{
			name:        "invalid - no config",
			trigger:     TriggerSource{},
			expectError: true,
			errorField:  "trigger",
		},
		{
			name: "valid - state only",
			trigger: TriggerSource{
				WatchBucket:  "EXECUTIONS",
				WatchPattern: "plan-review.*",
			},
			expectError: false,
		},
		{
			name: "invalid - subject without message factory",
			trigger: TriggerSource{
				Subject:    "workflow.trigger",
				StreamName: "WORKFLOW",
			},
			expectError: true,
			errorField:  "trigger.message_factory",
		},
		{
			name: "invalid - state bucket without key func",
			trigger: TriggerSource{
				Subject:        "workflow.callback.>",
				StreamName:     "WORKFLOW",
				MessageFactory: func() any { return &struct{}{} },
				StateBucket:    "EXECUTIONS",
			},
			expectError: true,
			errorField:  "trigger.state_key_func",
		},
		{
			name: "valid - message only",
			trigger: TriggerSource{
				Subject:        "workflow.trigger",
				StreamName:     "WORKFLOW",
				MessageFactory: func() any { return &struct{}{} },
			},
			expectError: false,
		},
		{
			name: "valid - message and state",
			trigger: TriggerSource{
				Subject:        "workflow.callback.>",
				StreamName:     "WORKFLOW",
				MessageFactory: func() any { return &struct{}{} },
				StateBucket:    "EXECUTIONS",
				StateKeyFunc:   func(_ any) string { return "key" },
			},
			expectError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.trigger.Validate()
			if tt.expectError {
				require.Error(t, err)
				valErr, ok := err.(*ValidationError)
				require.True(t, ok, "expected ValidationError")
				assert.Equal(t, tt.errorField, valErr.Field)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

// TestActionTypeString tests ActionType.String().
func TestActionTypeString(t *testing.T) {
	tests := []struct {
		action   ActionType
		expected string
	}{
		{ActionPublishAsync, "publish_async"},
		{ActionPublish, "publish"},
		{ActionMutate, "mutate"},
		{ActionComplete, "complete"},
		{ActionType(99), "unknown"},
	}

	for _, tt := range tests {
		t.Run(tt.expected, func(t *testing.T) {
			assert.Equal(t, tt.expected, tt.action.String())
		})
	}
}

// TestActionValidate tests Action.Validate().
func TestActionValidate(t *testing.T) {
	dummyPayloadBuilder := func(_ *RuleContext) (message.Payload, error) { return nil, nil }
	dummyMutator := func(_ *RuleContext, _ any) error { return nil }

	tests := []struct {
		name        string
		action      Action
		expectError bool
		errorField  string
	}{
		{
			name: "publish_async - missing subject",
			action: Action{
				Type:               ActionPublishAsync,
				BuildPayload:       dummyPayloadBuilder,
				ExpectedResultType: "test.result.v1",
			},
			expectError: true,
			errorField:  "action.publish_subject",
		},
		{
			name: "publish_async - missing payload builder",
			action: Action{
				Type:               ActionPublishAsync,
				PublishSubject:     "workflow.async",
				ExpectedResultType: "test.result.v1",
			},
			expectError: true,
			errorField:  "action.build_payload",
		},
		{
			name: "publish_async - missing expected result type",
			action: Action{
				Type:           ActionPublishAsync,
				PublishSubject: "workflow.async",
				BuildPayload:   dummyPayloadBuilder,
			},
			expectError: true,
			errorField:  "action.expected_result_type",
		},
		{
			name: "publish_async - valid",
			action: Action{
				Type:               ActionPublishAsync,
				PublishSubject:     "workflow.async",
				BuildPayload:       dummyPayloadBuilder,
				ExpectedResultType: "test.result.v1",
			},
			expectError: false,
		},
		{
			name: "publish - missing subject",
			action: Action{
				Type:         ActionPublish,
				BuildPayload: dummyPayloadBuilder,
			},
			expectError: true,
			errorField:  "action.publish_subject",
		},
		{
			name: "publish - missing payload builder",
			action: Action{
				Type:           ActionPublish,
				PublishSubject: "workflow.events",
			},
			expectError: true,
			errorField:  "action.build_payload",
		},
		{
			name: "publish - valid",
			action: Action{
				Type:           ActionPublish,
				PublishSubject: "workflow.events",
				BuildPayload:   dummyPayloadBuilder,
			},
			expectError: false,
		},
		{
			name: "mutate - missing mutator",
			action: Action{
				Type: ActionMutate,
			},
			expectError: true,
			errorField:  "action.mutate_state",
		},
		{
			name: "mutate - valid",
			action: Action{
				Type:        ActionMutate,
				MutateState: dummyMutator,
			},
			expectError: false,
		},
		{
			name: "complete - valid without mutator",
			action: Action{
				Type: ActionComplete,
			},
			expectError: false,
		},
		{
			name: "complete - valid with mutator",
			action: Action{
				Type:        ActionComplete,
				MutateState: dummyMutator,
			},
			expectError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.action.Validate()
			if tt.expectError {
				require.Error(t, err)
				valErr, ok := err.(*ValidationError)
				require.True(t, ok, "expected ValidationError")
				assert.Equal(t, tt.errorField, valErr.Field)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

// TestRuleDefValidate tests RuleDef.Validate().
func TestRuleDefValidate(t *testing.T) {
	validTrigger := TriggerSource{
		WatchBucket:  "EXECUTIONS",
		WatchPattern: "*",
	}
	validAction := Action{
		Type:        ActionMutate,
		MutateState: func(_ *RuleContext, _ any) error { return nil },
	}

	tests := []struct {
		name        string
		rule        RuleDef
		expectError bool
		errorField  string
	}{
		{
			name: "missing ID",
			rule: RuleDef{
				Trigger: validTrigger,
				Action:  validAction,
			},
			expectError: true,
			errorField:  "rule.id",
		},
		{
			name: "invalid trigger",
			rule: RuleDef{
				ID:      "test-rule",
				Trigger: TriggerSource{},
				Action:  validAction,
			},
			expectError: true,
			errorField:  "trigger",
		},
		{
			name: "invalid action",
			rule: RuleDef{
				ID:      "test-rule",
				Trigger: validTrigger,
				Action:  Action{Type: ActionMutate}, // missing mutator
			},
			expectError: true,
			errorField:  "action.mutate_state",
		},
		{
			name: "invalid logic operator",
			rule: RuleDef{
				ID:      "test-rule",
				Trigger: validTrigger,
				Action:  validAction,
				Logic:   "xor",
			},
			expectError: true,
			errorField:  "rule.logic",
		},
		{
			name: "valid with and logic",
			rule: RuleDef{
				ID:      "test-rule",
				Trigger: validTrigger,
				Action:  validAction,
				Logic:   "and",
			},
			expectError: false,
		},
		{
			name: "valid with or logic",
			rule: RuleDef{
				ID:      "test-rule",
				Trigger: validTrigger,
				Action:  validAction,
				Logic:   "or",
			},
			expectError: false,
		},
		{
			name: "valid with empty logic (defaults to and)",
			rule: RuleDef{
				ID:      "test-rule",
				Trigger: validTrigger,
				Action:  validAction,
			},
			expectError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.rule.Validate()
			if tt.expectError {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.errorField)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

// TestDefinitionValidate tests Definition.Validate().
func TestDefinitionValidate(t *testing.T) {
	validRule := RuleDef{
		ID: "test-rule",
		Trigger: TriggerSource{
			WatchBucket:  "EXECUTIONS",
			WatchPattern: "*",
		},
		Action: Action{
			Type:        ActionMutate,
			MutateState: func(_ *RuleContext, _ any) error { return nil },
		},
	}

	tests := []struct {
		name        string
		def         Definition
		expectError bool
		errorField  string
	}{
		{
			name:        "missing ID",
			def:         Definition{},
			expectError: true,
			errorField:  "workflow.id",
		},
		{
			name: "missing state bucket",
			def: Definition{
				ID: "test-workflow",
			},
			expectError: true,
			errorField:  "workflow.state_bucket",
		},
		{
			name: "missing state factory",
			def: Definition{
				ID:          "test-workflow",
				StateBucket: "EXECUTIONS",
			},
			expectError: true,
			errorField:  "workflow.state_factory",
		},
		{
			name: "missing rules",
			def: Definition{
				ID:           "test-workflow",
				StateBucket:  "EXECUTIONS",
				StateFactory: func() any { return &ExecutionState{} },
			},
			expectError: true,
			errorField:  "workflow.rules",
		},
		{
			name: "valid definition",
			def: Definition{
				ID:           "test-workflow",
				StateBucket:  "EXECUTIONS",
				StateFactory: func() any { return &ExecutionState{} },
				Rules:        []RuleDef{validRule},
			},
			expectError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.def.Validate()
			if tt.expectError {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.errorField)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

// TestMatchesPattern tests the NATS-style wildcard pattern matching.
func TestMatchesPattern(t *testing.T) {
	tests := []struct {
		key      string
		pattern  string
		expected bool
	}{
		// Exact matches
		{"foo.bar", "foo.bar", true},
		{"foo.bar", "foo.baz", false},
		{"foo", "foo", true},
		{"foo", "bar", false},

		// Single token wildcard (*)
		{"foo.bar", "foo.*", true},
		{"foo.bar", "*.bar", true},
		{"foo.bar.baz", "foo.*.baz", true},
		{"foo.bar.baz", "foo.*", false}, // * only matches one token
		{"foo", "foo.*", false},         // * requires something after

		// Multi-token wildcard (>)
		{"foo.bar", "foo.>", true},
		{"foo.bar.baz", "foo.>", true},
		{"foo.bar.baz.qux", "foo.>", true},
		{"foo", "foo.>", false}, // > requires at least one more token
		{"bar.foo", "foo.>", false},

		// Edge cases
		{"plan-review.abc123", "plan-review.*", true},
		{"plan-review.abc123.extra", "plan-review.*", false},
		{"plan-review.abc123.extra", "plan-review.>", true},
		{"COMPLETE_exec123", "COMPLETE_*", true},
		{"COMPLETE_exec123_extra", "COMPLETE_*", true}, // * matches to end if no more dots

		// Empty cases
		{"", "", true},
		{"foo", "", false},
		{"", "foo", false},
	}

	for _, tt := range tests {
		t.Run(tt.key+"_"+tt.pattern, func(t *testing.T) {
			result := MatchesPattern(tt.key, tt.pattern)
			assert.Equal(t, tt.expected, result, "key=%q pattern=%q", tt.key, tt.pattern)
		})
	}
}

// TestBuildRuleContextFromKV tests building RuleContext from KV events.
func TestBuildRuleContextFromKV(t *testing.T) {
	type TestState struct {
		ExecutionState
		CustomField string `json:"custom_field"`
	}

	t.Run("successful unmarshal", func(t *testing.T) {
		state := TestState{
			ExecutionState: ExecutionState{
				ID:         "exec-123",
				WorkflowID: "test-workflow",
				Phase:      "pending",
				Status:     StatusRunning,
			},
			CustomField: "custom-value",
		}
		data, err := json.Marshal(state)
		require.NoError(t, err)

		event := KVWatchEvent{
			Bucket:    "EXECUTIONS",
			Key:       "exec-123",
			Value:     data,
			Revision:  5,
			Operation: KVOperationPut,
			Timestamp: time.Now(),
		}

		ctx, err := BuildRuleContextFromKV(event, func() any { return &TestState{} })
		require.NoError(t, err)
		require.NotNil(t, ctx)

		assert.Equal(t, uint64(5), ctx.KVRevision)
		assert.Equal(t, "exec-123", ctx.KVKey)
		assert.Nil(t, ctx.Message)
		assert.Empty(t, ctx.Subject)

		typedState, ok := ctx.State.(*TestState)
		require.True(t, ok)
		assert.Equal(t, "exec-123", typedState.ID)
		assert.Equal(t, "custom-value", typedState.CustomField)
	})

	t.Run("delete operation - no state", func(t *testing.T) {
		event := KVWatchEvent{
			Bucket:    "EXECUTIONS",
			Key:       "exec-123",
			Value:     nil,
			Revision:  6,
			Operation: KVOperationDelete,
			Timestamp: time.Now(),
		}

		ctx, err := BuildRuleContextFromKV(event, func() any { return &TestState{} })
		require.NoError(t, err)
		require.NotNil(t, ctx)

		assert.Nil(t, ctx.State)
		assert.Equal(t, uint64(6), ctx.KVRevision)
		assert.Equal(t, "exec-123", ctx.KVKey)
	})

	t.Run("unmarshal error", func(t *testing.T) {
		event := KVWatchEvent{
			Bucket:    "EXECUTIONS",
			Key:       "exec-123",
			Value:     []byte("invalid json"),
			Revision:  5,
			Operation: KVOperationPut,
			Timestamp: time.Now(),
		}

		ctx, err := BuildRuleContextFromKV(event, func() any { return &TestState{} })
		require.Error(t, err)
		assert.Nil(t, ctx)

		unmarshalErr, ok := err.(*UnmarshalError)
		require.True(t, ok)
		assert.Equal(t, "exec-123", unmarshalErr.Key)
	})
}

// TestBuildRuleContextFromMessage tests building RuleContext from messages.
func TestBuildRuleContextFromMessage(t *testing.T) {
	type TestMessage struct {
		TaskID    string `json:"task_id"`
		RequestID string `json:"request_id"`
	}

	t.Run("message only - no state loader", func(t *testing.T) {
		msg := TestMessage{
			TaskID:    "task-123",
			RequestID: "req-456",
		}
		data, err := json.Marshal(msg)
		require.NoError(t, err)

		event := SubjectMessageEvent{
			Subject:   "workflow.trigger.test",
			Data:      data,
			Timestamp: time.Now(),
		}

		ctx, err := BuildRuleContextFromMessage(
			event,
			func() any { return &TestMessage{} },
			nil, // no state loader
			nil, // no key func
		)
		require.NoError(t, err)
		require.NotNil(t, ctx)

		assert.Nil(t, ctx.State)
		assert.Equal(t, "workflow.trigger.test", ctx.Subject)

		typedMsg, ok := ctx.Message.(*TestMessage)
		require.True(t, ok)
		assert.Equal(t, "task-123", typedMsg.TaskID)
	})

	t.Run("message with state loader", func(t *testing.T) {
		type TestState struct {
			ExecutionState
		}

		msg := TestMessage{
			TaskID:    "task-123",
			RequestID: "req-456",
		}
		data, err := json.Marshal(msg)
		require.NoError(t, err)

		event := SubjectMessageEvent{
			Subject:   "workflow.callback.test",
			Data:      data,
			Timestamp: time.Now(),
		}

		state := &TestState{
			ExecutionState: ExecutionState{
				ID:     "exec-123",
				Status: StatusWaiting,
			},
		}

		stateLoader := func(key string) (any, uint64, error) {
			assert.Equal(t, "task-123", key)
			return state, uint64(10), nil
		}

		stateKeyFunc := func(msg any) string {
			return msg.(*TestMessage).TaskID
		}

		ctx, err := BuildRuleContextFromMessage(
			event,
			func() any { return &TestMessage{} },
			stateLoader,
			stateKeyFunc,
		)
		require.NoError(t, err)
		require.NotNil(t, ctx)

		assert.Equal(t, state, ctx.State)
		assert.Equal(t, uint64(10), ctx.KVRevision)
		assert.Equal(t, "task-123", ctx.KVKey)
		assert.Equal(t, "workflow.callback.test", ctx.Subject)
	})

	t.Run("message deserialize error", func(t *testing.T) {
		event := SubjectMessageEvent{
			Subject:   "workflow.trigger.test",
			Data:      []byte("not json"),
			Timestamp: time.Now(),
		}

		ctx, err := BuildRuleContextFromMessage(
			event,
			func() any { return &TestMessage{} },
			nil,
			nil,
		)
		require.Error(t, err)
		assert.Nil(t, ctx)

		deserErr, ok := err.(*MessageDeserializeError)
		require.True(t, ok)
		assert.Equal(t, "workflow.trigger.test", deserErr.Subject)
	})
}

// TestExecutionStateSerialization tests JSON round-trip for ExecutionState.
func TestExecutionStateSerialization(t *testing.T) {
	now := time.Now().Truncate(time.Second) // Truncate for JSON comparison
	completedAt := now.Add(5 * time.Minute)
	deadline := now.Add(30 * time.Minute)

	original := ExecutionState{
		ID:            "exec-123",
		WorkflowID:    "plan-review-loop",
		Phase:         "awaiting_review",
		Iteration:     2,
		Status:        StatusRunning,
		Error:         "",
		PendingTaskID: "task-456",
		PendingRuleID: "fire-reviewer",
		CreatedAt:     now,
		UpdatedAt:     now.Add(1 * time.Minute),
		CompletedAt:   &completedAt,
		Deadline:      &deadline,
		Timeline: []TimelineEntry{
			{
				Timestamp:   now,
				RuleID:      "fire-planner",
				TriggerMode: "kv",
				TriggerInfo: "exec-123",
				Action:      "publish_async",
				Phase:       "planning",
				Iteration:   1,
			},
		},
	}

	// Serialize
	data, err := json.Marshal(original)
	require.NoError(t, err)

	// Deserialize
	var restored ExecutionState
	err = json.Unmarshal(data, &restored)
	require.NoError(t, err)

	// Verify fields
	assert.Equal(t, original.ID, restored.ID)
	assert.Equal(t, original.WorkflowID, restored.WorkflowID)
	assert.Equal(t, original.Phase, restored.Phase)
	assert.Equal(t, original.Iteration, restored.Iteration)
	assert.Equal(t, original.Status, restored.Status)
	assert.Equal(t, original.PendingTaskID, restored.PendingTaskID)
	assert.Equal(t, original.PendingRuleID, restored.PendingRuleID)
	assert.Len(t, restored.Timeline, 1)
	assert.Equal(t, original.Timeline[0].RuleID, restored.Timeline[0].RuleID)
}

// TestKVOperationString tests KVOperation.String().
func TestKVOperationString(t *testing.T) {
	assert.Equal(t, "put", KVOperationPut.String())
	assert.Equal(t, "delete", KVOperationDelete.String())
	assert.Equal(t, "unknown", KVOperation(99).String())
}

// TestErrorTypes tests the custom error types.
func TestErrorTypes(t *testing.T) {
	t.Run("ValidationError", func(t *testing.T) {
		err := &ValidationError{Field: "test.field", Message: "is required"}
		assert.Equal(t, "test.field: is required", err.Error())
	})

	t.Run("WatchError", func(t *testing.T) {
		cause := assert.AnError
		err := &WatchError{Bucket: "TEST", Pattern: "*", Cause: cause}
		assert.Contains(t, err.Error(), "TEST:*")
		assert.Equal(t, cause, err.Unwrap())
	})

	t.Run("UnmarshalError", func(t *testing.T) {
		cause := assert.AnError
		err := &UnmarshalError{Key: "test-key", Cause: cause}
		assert.Contains(t, err.Error(), "test-key")
		assert.Equal(t, cause, err.Unwrap())
	})

	t.Run("ConsumerError", func(t *testing.T) {
		cause := assert.AnError
		err := &ConsumerError{Stream: "WORKFLOW", Consumer: "test", Op: "create", Cause: cause}
		assert.Contains(t, err.Error(), "WORKFLOW:test")
		assert.Contains(t, err.Error(), "create")
		assert.Equal(t, cause, err.Unwrap())
	})

	t.Run("MessageDeserializeError", func(t *testing.T) {
		cause := assert.AnError
		err := &MessageDeserializeError{Subject: "test.subject", Cause: cause}
		assert.Contains(t, err.Error(), "test.subject")
		assert.Equal(t, cause, err.Unwrap())
	})

	t.Run("StateLoadError", func(t *testing.T) {
		cause := assert.AnError
		err := &StateLoadError{Key: "test-key", Cause: cause}
		assert.Contains(t, err.Error(), "test-key")
		assert.Equal(t, cause, err.Unwrap())
	})

	t.Run("StateKeyError", func(t *testing.T) {
		err := &StateKeyError{Subject: "test.subject", Message: "empty key"}
		assert.Contains(t, err.Error(), "test.subject")
		assert.Contains(t, err.Error(), "empty key")
	})
}
