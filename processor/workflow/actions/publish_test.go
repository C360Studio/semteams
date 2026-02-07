package actions

import (
	"context"
	"strings"
	"testing"
)

func TestPublishAgentAction_Validation(t *testing.T) {
	tests := []struct {
		name        string
		role        string
		model       string
		prompt      string
		taskID      string
		wantSuccess bool
		wantError   string
	}{
		{
			name:        "valid fields with taskID",
			role:        "general",
			model:       "claude-sonnet",
			prompt:      "hello",
			taskID:      "t1",
			wantSuccess: false, // No NATS client, but validation passes
			wantError:   "NATS client not available",
		},
		{
			name:        "valid fields without taskID (auto-generated)",
			role:        "general",
			model:       "claude-sonnet",
			prompt:      "hello",
			taskID:      "",
			wantSuccess: false, // No NATS client, but validation passes
			wantError:   "NATS client not available",
		},
		{
			name:        "missing model",
			role:        "general",
			model:       "",
			prompt:      "hello",
			taskID:      "t1",
			wantSuccess: false,
			wantError:   "model required",
		},
		{
			name:        "missing role",
			role:        "",
			model:       "claude-sonnet",
			prompt:      "hello",
			taskID:      "t1",
			wantSuccess: false,
			wantError:   "role required",
		},
		{
			name:        "missing prompt",
			role:        "general",
			model:       "claude-sonnet",
			prompt:      "",
			taskID:      "t1",
			wantSuccess: false,
			wantError:   "prompt required",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			action := NewPublishAgentAction("agent.task.test", tt.role, tt.model, tt.prompt, tt.taskID)
			result := action.Execute(context.Background(), &Context{})

			if result.Success != tt.wantSuccess {
				t.Errorf("Execute() success = %v, want %v", result.Success, tt.wantSuccess)
			}

			if tt.wantError != "" && result.Error == "" {
				t.Errorf("Execute() expected error containing %q, got no error", tt.wantError)
			}

			if tt.wantError != "" && result.Error != "" {
				if !strings.Contains(result.Error, tt.wantError) {
					t.Errorf("Execute() error = %q, want containing %q", result.Error, tt.wantError)
				}
			}
		})
	}
}
