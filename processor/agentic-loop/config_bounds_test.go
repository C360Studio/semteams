package agenticloop_test

import (
	"bytes"
	"log/slog"
	"strings"
	"testing"

	agenticloop "github.com/c360studio/semstreams/processor/agentic-loop"
)

func TestContextConfig_Validate_ModelLimitBounds(t *testing.T) {
	tests := []struct {
		name        string
		modelLimits map[string]int
		wantErr     bool
		errContains string
	}{
		{
			name: "valid at MinContextLimit boundary",
			modelLimits: map[string]int{
				agenticloop.DefaultModelKey: agenticloop.MinContextLimit,
			},
			wantErr: false,
		},
		{
			name: "valid at MaxReasonableContextLimit boundary",
			modelLimits: map[string]int{
				agenticloop.DefaultModelKey: agenticloop.MaxReasonableContextLimit,
			},
			wantErr: false,
		},
		{
			name: "invalid below MinContextLimit",
			modelLimits: map[string]int{
				agenticloop.DefaultModelKey: agenticloop.MinContextLimit - 1,
			},
			wantErr:     true,
			errContains: "below minimum",
		},
		{
			name: "invalid above MaxReasonableContextLimit",
			modelLimits: map[string]int{
				agenticloop.DefaultModelKey: agenticloop.MaxReasonableContextLimit + 1,
			},
			wantErr:     true,
			errContains: "exceeds maximum",
		},
		{
			name: "one model below minimum with valid default",
			modelLimits: map[string]int{
				agenticloop.DefaultModelKey: 128000,
				"tiny-model":                500, // Below MinContextLimit
			},
			wantErr:     true,
			errContains: "below minimum",
		},
		{
			name: "one model above maximum with valid default",
			modelLimits: map[string]int{
				agenticloop.DefaultModelKey: 128000,
				"huge-model":                3_000_000, // Above MaxReasonableContextLimit
			},
			wantErr:     true,
			errContains: "exceeds maximum",
		},
		{
			name: "valid edge-first config with custom models",
			modelLimits: map[string]int{
				agenticloop.DefaultModelKey: 32000,
				"llama3.2:8b":               128000,
				"mistral-7b-instruct":       32000,
				"qwen2.5:32b":               128000,
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			config := agenticloop.ContextConfig{
				Enabled:            true,
				CompactThreshold:   0.60,
				ToolResultMaxAge:   3,
				HeadroomTokens:     6400,
				SummarizationModel: "fast",
				ModelLimits:        tt.modelLimits,
			}

			err := config.Validate()

			if (err != nil) != tt.wantErr {
				t.Errorf("Validate() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if tt.wantErr && err != nil && tt.errContains != "" {
				if !strings.Contains(err.Error(), tt.errContains) {
					t.Errorf("Validate() error = %q, expected to contain %q", err.Error(), tt.errContains)
				}
			}
		})
	}
}

func TestContextConfig_Constants(t *testing.T) {
	// Verify constants have sensible values
	if agenticloop.DefaultModelKey != "default" {
		t.Errorf("DefaultModelKey = %q, want %q", agenticloop.DefaultModelKey, "default")
	}

	if agenticloop.MinContextLimit < 1 {
		t.Errorf("MinContextLimit = %d, want >= 1", agenticloop.MinContextLimit)
	}

	if agenticloop.MaxReasonableContextLimit <= agenticloop.MinContextLimit {
		t.Errorf("MaxReasonableContextLimit (%d) should be > MinContextLimit (%d)",
			agenticloop.MaxReasonableContextLimit, agenticloop.MinContextLimit)
	}

	// Verify MaxReasonableContextLimit is 2M tokens (future-proof)
	if agenticloop.MaxReasonableContextLimit != 2_000_000 {
		t.Errorf("MaxReasonableContextLimit = %d, want 2000000", agenticloop.MaxReasonableContextLimit)
	}

	// Verify MinContextLimit is 1024 (sanity check)
	if agenticloop.MinContextLimit != 1024 {
		t.Errorf("MinContextLimit = %d, want 1024", agenticloop.MinContextLimit)
	}
}

func TestContextManager_WithLogger(t *testing.T) {
	var buf bytes.Buffer
	handler := slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelWarn})
	logger := slog.New(handler)

	config := agenticloop.ContextConfig{
		Enabled:            true,
		CompactThreshold:   0.60,
		ToolResultMaxAge:   3,
		HeadroomTokens:     6400,
		SummarizationModel: "fast",
		ModelLimits: map[string]int{
			agenticloop.DefaultModelKey: 128000,
		},
	}

	// Create context manager with custom logger and unknown model
	_ = agenticloop.NewContextManager("test-loop", "unknown-ollama-model", config,
		agenticloop.WithLogger(logger))

	// Verify warning was logged
	logOutput := buf.String()
	if !strings.Contains(logOutput, "model not in config") {
		t.Errorf("Expected warning about model not in config, got: %q", logOutput)
	}
	if !strings.Contains(logOutput, "unknown-ollama-model") {
		t.Errorf("Expected warning to contain model name, got: %q", logOutput)
	}
	if !strings.Contains(logOutput, "128000") {
		t.Errorf("Expected warning to contain default limit, got: %q", logOutput)
	}
}

func TestContextManager_WithLogger_KnownModel(t *testing.T) {
	var buf bytes.Buffer
	handler := slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelWarn})
	logger := slog.New(handler)

	config := agenticloop.DefaultContextConfig()

	// Create context manager with custom logger and known model
	_ = agenticloop.NewContextManager("test-loop", "gpt-4o", config,
		agenticloop.WithLogger(logger))

	// Verify NO warning was logged for known model
	logOutput := buf.String()
	if strings.Contains(logOutput, "model not in config") {
		t.Errorf("Unexpected warning for known model, got: %q", logOutput)
	}
}

func TestLoopManager_WithLogger(t *testing.T) {
	var buf bytes.Buffer
	handler := slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelWarn})
	logger := slog.New(handler)

	lm := agenticloop.NewLoopManager(agenticloop.WithLoopManagerLogger(logger))

	// Create loop with unknown model
	_, err := lm.CreateLoop("task-1", "architect", "unknown-local-model")
	if err != nil {
		t.Fatalf("CreateLoop() error = %v", err)
	}

	// Verify warning was logged
	logOutput := buf.String()
	if !strings.Contains(logOutput, "model not in config") {
		t.Errorf("Expected warning about model not in config, got: %q", logOutput)
	}
	if !strings.Contains(logOutput, "unknown-local-model") {
		t.Errorf("Expected warning to contain model name, got: %q", logOutput)
	}
}

func TestLoopManagerWithConfig_WithLogger(t *testing.T) {
	var buf bytes.Buffer
	handler := slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelWarn})
	logger := slog.New(handler)

	config := agenticloop.ContextConfig{
		Enabled:            true,
		CompactThreshold:   0.60,
		ToolResultMaxAge:   3,
		HeadroomTokens:     6400,
		SummarizationModel: "fast",
		ModelLimits: map[string]int{
			agenticloop.DefaultModelKey: 32000,
			"my-custom-model":           64000,
		},
	}

	lm := agenticloop.NewLoopManagerWithConfig(config, agenticloop.WithLoopManagerLogger(logger))

	// Create loop with known custom model - should NOT warn
	_, err := lm.CreateLoop("task-1", "architect", "my-custom-model")
	if err != nil {
		t.Fatalf("CreateLoop() error = %v", err)
	}

	logOutput := buf.String()
	if strings.Contains(logOutput, "model not in config") {
		t.Errorf("Unexpected warning for known custom model, got: %q", logOutput)
	}

	// Clear buffer
	buf.Reset()

	// Create loop with unknown model - should warn
	_, err = lm.CreateLoop("task-2", "architect", "some-other-model")
	if err != nil {
		t.Fatalf("CreateLoop() error = %v", err)
	}

	logOutput = buf.String()
	if !strings.Contains(logOutput, "model not in config") {
		t.Errorf("Expected warning for unknown model, got: %q", logOutput)
	}
	if !strings.Contains(logOutput, "32000") {
		t.Errorf("Expected warning to show custom default limit (32000), got: %q", logOutput)
	}
}
