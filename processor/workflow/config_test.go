package workflow

import (
	"encoding/json"
	"testing"
)

func TestConfigValidation(t *testing.T) {
	tests := []struct {
		name    string
		config  Config
		wantErr bool
		errMsg  string
	}{
		{
			name:    "valid default config",
			config:  DefaultConfig(),
			wantErr: false,
		},
		{
			name: "empty definitions bucket",
			config: func() Config {
				c := DefaultConfig()
				c.DefinitionsBucket = ""
				return c
			}(),
			wantErr: true,
			errMsg:  "definitions_bucket is required",
		},
		{
			name: "empty executions bucket",
			config: func() Config {
				c := DefaultConfig()
				c.ExecutionsBucket = ""
				return c
			}(),
			wantErr: true,
			errMsg:  "executions_bucket is required",
		},
		{
			name: "empty stream name",
			config: func() Config {
				c := DefaultConfig()
				c.StreamName = ""
				return c
			}(),
			wantErr: true,
			errMsg:  "stream_name is required",
		},
		{
			name: "invalid default timeout",
			config: func() Config {
				c := DefaultConfig()
				c.DefaultTimeout = "invalid"
				return c
			}(),
			wantErr: true,
			errMsg:  "invalid default_timeout format",
		},
		{
			name: "zero default max iterations",
			config: func() Config {
				c := DefaultConfig()
				c.DefaultMaxIterations = 0
				return c
			}(),
			wantErr: true,
			errMsg:  "default_max_iterations must be greater than 0",
		},
		{
			name: "invalid request timeout",
			config: func() Config {
				c := DefaultConfig()
				c.RequestTimeout = "not-a-duration"
				return c
			}(),
			wantErr: true,
			errMsg:  "invalid request_timeout format",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.config.Validate()
			if tt.wantErr {
				if err == nil {
					t.Errorf("expected error containing %q, got nil", tt.errMsg)
					return
				}
				if tt.errMsg != "" && err.Error() != tt.errMsg {
					// Check if error contains the expected message
					if err.Error()[:len(tt.errMsg)] != tt.errMsg[:min(len(err.Error()), len(tt.errMsg))] {
						t.Errorf("error message = %q, want containing %q", err.Error(), tt.errMsg)
					}
				}
			} else if err != nil {
				t.Errorf("unexpected error: %v", err)
			}
		})
	}
}

func TestDefinitionValidation(t *testing.T) {
	tests := []struct {
		name    string
		def     Definition
		wantErr bool
		errMsg  string
	}{
		{
			name: "valid workflow",
			def: Definition{
				ID:      "test-workflow",
				Name:    "Test Workflow",
				Enabled: true,
				Trigger: TriggerDef{Subject: "workflow.trigger.test"},
				Steps: []StepDef{
					{
						Name:   "step1",
						Action: ActionDef{Type: "publish", Subject: "test.subject"},
					},
				},
			},
			wantErr: false,
		},
		{
			name: "missing id",
			def: Definition{
				Name:    "Test Workflow",
				Trigger: TriggerDef{Subject: "workflow.trigger.test"},
				Steps: []StepDef{
					{
						Name:   "step1",
						Action: ActionDef{Type: "publish", Subject: "test.subject"},
					},
				},
			},
			wantErr: true,
			errMsg:  "workflow id is required",
		},
		{
			name: "missing name",
			def: Definition{
				ID:      "test-workflow",
				Trigger: TriggerDef{Subject: "workflow.trigger.test"},
				Steps: []StepDef{
					{
						Name:   "step1",
						Action: ActionDef{Type: "publish", Subject: "test.subject"},
					},
				},
			},
			wantErr: true,
			errMsg:  "workflow name is required",
		},
		{
			name: "missing trigger subject",
			def: Definition{
				ID:      "test-workflow",
				Name:    "Test Workflow",
				Trigger: TriggerDef{},
				Steps: []StepDef{
					{
						Name:   "step1",
						Action: ActionDef{Type: "publish", Subject: "test.subject"},
					},
				},
			},
			wantErr: true,
			errMsg:  "invalid trigger",
		},
		{
			name: "no steps",
			def: Definition{
				ID:      "test-workflow",
				Name:    "Test Workflow",
				Trigger: TriggerDef{Subject: "workflow.trigger.test"},
				Steps:   []StepDef{},
			},
			wantErr: true,
			errMsg:  "workflow must have at least one step",
		},
		{
			name: "duplicate step names",
			def: Definition{
				ID:      "test-workflow",
				Name:    "Test Workflow",
				Trigger: TriggerDef{Subject: "workflow.trigger.test"},
				Steps: []StepDef{
					{
						Name:   "step1",
						Action: ActionDef{Type: "publish", Subject: "test.subject"},
					},
					{
						Name:   "step1",
						Action: ActionDef{Type: "publish", Subject: "test.subject"},
					},
				},
			},
			wantErr: true,
			errMsg:  "duplicate step name: step1",
		},
		{
			name: "invalid on_success reference",
			def: Definition{
				ID:      "test-workflow",
				Name:    "Test Workflow",
				Trigger: TriggerDef{Subject: "workflow.trigger.test"},
				Steps: []StepDef{
					{
						Name:      "step1",
						Action:    ActionDef{Type: "publish", Subject: "test.subject"},
						OnSuccess: "nonexistent",
					},
				},
			},
			wantErr: true,
			errMsg:  "step step1 references unknown on_success step: nonexistent",
		},
		{
			name: "valid on_success to complete",
			def: Definition{
				ID:      "test-workflow",
				Name:    "Test Workflow",
				Trigger: TriggerDef{Subject: "workflow.trigger.test"},
				Steps: []StepDef{
					{
						Name:      "step1",
						Action:    ActionDef{Type: "publish", Subject: "test.subject"},
						OnSuccess: "complete",
					},
				},
			},
			wantErr: false,
		},
		{
			name: "invalid timeout",
			def: Definition{
				ID:      "test-workflow",
				Name:    "Test Workflow",
				Trigger: TriggerDef{Subject: "workflow.trigger.test"},
				Timeout: "invalid",
				Steps: []StepDef{
					{
						Name:   "step1",
						Action: ActionDef{Type: "publish", Subject: "test.subject"},
					},
				},
			},
			wantErr: true,
			errMsg:  "invalid timeout",
		},
		{
			name: "negative max iterations",
			def: Definition{
				ID:            "test-workflow",
				Name:          "Test Workflow",
				Trigger:       TriggerDef{Subject: "workflow.trigger.test"},
				MaxIterations: -1,
				Steps: []StepDef{
					{
						Name:   "step1",
						Action: ActionDef{Type: "publish", Subject: "test.subject"},
					},
				},
			},
			wantErr: true,
			errMsg:  "max_iterations cannot be negative",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.def.Validate()
			if tt.wantErr {
				if err == nil {
					t.Errorf("expected error containing %q, got nil", tt.errMsg)
					return
				}
				if tt.errMsg != "" && err.Error() != tt.errMsg {
					t.Logf("actual error: %v", err)
				}
			} else if err != nil {
				t.Errorf("unexpected error: %v", err)
			}
		})
	}
}

func TestActionDefValidation(t *testing.T) {
	tests := []struct {
		name    string
		action  ActionDef
		wantErr bool
		errMsg  string
	}{
		{
			name:    "valid call action",
			action:  ActionDef{Type: "call", Subject: "test.subject"},
			wantErr: false,
		},
		{
			name:    "valid publish action",
			action:  ActionDef{Type: "publish", Subject: "test.subject"},
			wantErr: false,
		},
		{
			name:    "valid publish_agent action",
			action:  ActionDef{Type: "publish_agent", Subject: "agent.task.test", Role: "reviewer", Model: "claude-sonnet", Prompt: "Review this"},
			wantErr: false,
		},
		{
			name:    "publish_agent action without role",
			action:  ActionDef{Type: "publish_agent", Subject: "agent.task.test", Model: "claude-sonnet", Prompt: "Review this"},
			wantErr: true,
			errMsg:  "publish_agent action requires role",
		},
		{
			name:    "publish_agent action without model",
			action:  ActionDef{Type: "publish_agent", Subject: "agent.task.test", Role: "reviewer", Prompt: "Review this"},
			wantErr: true,
			errMsg:  "publish_agent action requires model",
		},
		{
			name:    "publish_agent action without prompt",
			action:  ActionDef{Type: "publish_agent", Subject: "agent.task.test", Role: "reviewer", Model: "claude-sonnet"},
			wantErr: true,
			errMsg:  "publish_agent action requires prompt",
		},
		{
			name:    "valid set_state action",
			action:  ActionDef{Type: "set_state", Entity: "test.entity"},
			wantErr: false,
		},
		{
			name:    "invalid action type",
			action:  ActionDef{Type: "unknown"},
			wantErr: true,
			errMsg:  "invalid action type: unknown",
		},
		{
			name:    "call action without subject",
			action:  ActionDef{Type: "call"},
			wantErr: true,
			errMsg:  "call action requires subject",
		},
		{
			name:    "set_state action without entity",
			action:  ActionDef{Type: "set_state"},
			wantErr: true,
			errMsg:  "set_state action requires entity",
		},
		{
			name:    "invalid timeout",
			action:  ActionDef{Type: "call", Subject: "test", Timeout: "invalid"},
			wantErr: true,
			errMsg:  "invalid action timeout",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.action.Validate()
			if tt.wantErr {
				if err == nil {
					t.Errorf("expected error, got nil")
					return
				}
			} else if err != nil {
				t.Errorf("unexpected error: %v", err)
			}
		})
	}
}

func TestConditionDefValidation(t *testing.T) {
	tests := []struct {
		name      string
		condition ConditionDef
		wantErr   bool
	}{
		{
			name:      "valid eq condition",
			condition: ConditionDef{Field: "steps.review.output.count", Operator: "eq", Value: 0},
			wantErr:   false,
		},
		{
			name:      "valid exists condition",
			condition: ConditionDef{Field: "steps.review.output", Operator: "exists"},
			wantErr:   false,
		},
		{
			name:      "missing field",
			condition: ConditionDef{Operator: "eq", Value: 0},
			wantErr:   true,
		},
		{
			name:      "invalid operator",
			condition: ConditionDef{Field: "test", Operator: "invalid"},
			wantErr:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.condition.Validate()
			if tt.wantErr && err == nil {
				t.Errorf("expected error, got nil")
			} else if !tt.wantErr && err != nil {
				t.Errorf("unexpected error: %v", err)
			}
		})
	}
}

func TestConfigJSONRoundTrip(t *testing.T) {
	original := DefaultConfig()

	data, err := json.Marshal(original)
	if err != nil {
		t.Fatalf("failed to marshal config: %v", err)
	}

	var decoded Config
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("failed to unmarshal config: %v", err)
	}

	if decoded.DefinitionsBucket != original.DefinitionsBucket {
		t.Errorf("DefinitionsBucket = %q, want %q", decoded.DefinitionsBucket, original.DefinitionsBucket)
	}
	if decoded.ExecutionsBucket != original.ExecutionsBucket {
		t.Errorf("ExecutionsBucket = %q, want %q", decoded.ExecutionsBucket, original.ExecutionsBucket)
	}
	if decoded.DefaultTimeout != original.DefaultTimeout {
		t.Errorf("DefaultTimeout = %q, want %q", decoded.DefaultTimeout, original.DefaultTimeout)
	}
}

func TestDefinitionJSONRoundTrip(t *testing.T) {
	original := Definition{
		ID:            "review-fix-cycle",
		Name:          "Review and Fix Loop",
		Description:   "Iterative review and fix workflow",
		Version:       "1.0.0",
		Enabled:       true,
		MaxIterations: 3,
		Timeout:       "10m",
		Trigger:       TriggerDef{Subject: "workflow.trigger.review-fix-cycle"},
		Steps: []StepDef{
			{
				Name: "review",
				Action: ActionDef{
					Type:    "publish_agent",
					Subject: "agent.task.${execution.id}.reviewer",
					Role:    "reviewer",
					Model:   "claude-sonnet",
					Prompt:  "${trigger.payload.code}",
				},
				OnSuccess: "check-issues",
			},
			{
				Name: "check-issues",
				Action: ActionDef{
					Type:    "publish",
					Subject: "workflow.internal.check",
				},
				Condition: &ConditionDef{
					Field:    "steps.review.output.issues_count",
					Operator: "eq",
					Value:    0,
				},
				OnSuccess: "complete",
				OnFail:    "fix",
			},
			{
				Name: "fix",
				Action: ActionDef{
					Type:    "publish_agent",
					Subject: "agent.task.${execution.id}.fixer",
					Role:    "fixer",
					Model:   "claude-sonnet",
					Prompt:  "Fix the issues found in the review",
				},
				OnSuccess: "review",
			},
		},
		OnComplete: []ActionDef{
			{Type: "publish", Subject: "workflow.events.completed"},
		},
	}

	data, err := json.Marshal(original)
	if err != nil {
		t.Fatalf("failed to marshal workflow: %v", err)
	}

	var decoded Definition
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("failed to unmarshal workflow: %v", err)
	}

	if err := decoded.Validate(); err != nil {
		t.Errorf("decoded workflow validation failed: %v", err)
	}

	if decoded.ID != original.ID {
		t.Errorf("ID = %q, want %q", decoded.ID, original.ID)
	}
	if decoded.MaxIterations != original.MaxIterations {
		t.Errorf("MaxIterations = %d, want %d", decoded.MaxIterations, original.MaxIterations)
	}
	if len(decoded.Steps) != len(original.Steps) {
		t.Errorf("Steps count = %d, want %d", len(decoded.Steps), len(original.Steps))
	}
}
