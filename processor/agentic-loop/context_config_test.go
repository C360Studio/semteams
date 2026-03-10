package agenticloop_test

import (
	"encoding/json"
	"testing"

	agenticloop "github.com/c360studio/semstreams/processor/agentic-loop"
)

func TestContextConfig_Validate(t *testing.T) {
	tests := []struct {
		name    string
		config  agenticloop.ContextConfig
		wantErr bool
		errMsg  string
	}{
		{
			name: "valid minimal config",
			config: agenticloop.ContextConfig{
				Enabled:          true,
				CompactThreshold: 0.60,
				ToolResultMaxAge: 3,
				HeadroomTokens:   6400,
			},
			wantErr: false,
		},
		{
			name: "valid disabled config",
			config: agenticloop.ContextConfig{
				Enabled: false,
			},
			wantErr: false,
		},
		{
			name: "threshold too low",
			config: agenticloop.ContextConfig{
				Enabled:          true,
				CompactThreshold: -0.1,
				ToolResultMaxAge: 3,
				HeadroomTokens:   6400,
			},
			wantErr: true,
			errMsg:  "compact_threshold",
		},
		{
			name: "threshold too high",
			config: agenticloop.ContextConfig{
				Enabled:          true,
				CompactThreshold: 1.5,
				ToolResultMaxAge: 3,
				HeadroomTokens:   6400,
			},
			wantErr: true,
			errMsg:  "compact_threshold",
		},
		{
			name: "zero threshold",
			config: agenticloop.ContextConfig{
				Enabled:          true,
				CompactThreshold: 0,
				ToolResultMaxAge: 3,
				HeadroomTokens:   6400,
			},
			wantErr: true,
			errMsg:  "compact_threshold",
		},
		{
			name: "negative tool result max age",
			config: agenticloop.ContextConfig{
				Enabled:          true,
				CompactThreshold: 0.60,
				ToolResultMaxAge: -1,
				HeadroomTokens:   6400,
			},
			wantErr: true,
			errMsg:  "tool_result_max_age",
		},
		{
			name: "zero tool result max age",
			config: agenticloop.ContextConfig{
				Enabled:          true,
				CompactThreshold: 0.60,
				ToolResultMaxAge: 0,
				HeadroomTokens:   6400,
			},
			wantErr: true,
			errMsg:  "tool_result_max_age",
		},
		{
			name: "negative headroom tokens",
			config: agenticloop.ContextConfig{
				Enabled:          true,
				CompactThreshold: 0.60,
				ToolResultMaxAge: 3,
				HeadroomTokens:   -100,
			},
			wantErr: true,
			errMsg:  "headroom_tokens",
		},
		{
			name: "boundary threshold 0.01",
			config: agenticloop.ContextConfig{
				Enabled:          true,
				CompactThreshold: 0.01,
				ToolResultMaxAge: 3,
				HeadroomTokens:   6400,
			},
			wantErr: false,
		},
		{
			name: "boundary threshold 1.0",
			config: agenticloop.ContextConfig{
				Enabled:          true,
				CompactThreshold: 1.0,
				ToolResultMaxAge: 3,
				HeadroomTokens:   6400,
			},
			wantErr: false,
		},
		{
			name: "boundary tool result age 1",
			config: agenticloop.ContextConfig{
				Enabled:          true,
				CompactThreshold: 0.60,
				ToolResultMaxAge: 1,
				HeadroomTokens:   6400,
			},
			wantErr: false,
		},
		{
			name: "zero headroom tokens allowed",
			config: agenticloop.ContextConfig{
				Enabled:          true,
				CompactThreshold: 0.60,
				ToolResultMaxAge: 3,
				HeadroomTokens:   0,
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.config.Validate()
			if (err != nil) != tt.wantErr {
				t.Errorf("Validate() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if tt.wantErr && err != nil && tt.errMsg != "" {
				if !containsIgnoreCase(err.Error(), tt.errMsg) {
					t.Errorf("Validate() error = %v, expected to contain %q", err, tt.errMsg)
				}
			}
		})
	}
}

func TestContextConfig_DefaultValues(t *testing.T) {
	cfg := agenticloop.DefaultContextConfig()

	// Verify enabled
	if !cfg.Enabled {
		t.Errorf("DefaultContextConfig() enabled = false, want true")
	}

	// Verify compact threshold
	if cfg.CompactThreshold != 0.60 {
		t.Errorf("DefaultContextConfig() compact_threshold = %f, want 0.60", cfg.CompactThreshold)
	}

	// Verify tool result max age
	if cfg.ToolResultMaxAge != 3 {
		t.Errorf("DefaultContextConfig() tool_result_max_age = %d, want 3", cfg.ToolResultMaxAge)
	}

	// Verify headroom tokens
	if cfg.HeadroomTokens != 6400 {
		t.Errorf("DefaultContextConfig() headroom_tokens = %d, want 6400", cfg.HeadroomTokens)
	}

	// Verify default config is valid
	if err := cfg.Validate(); err != nil {
		t.Errorf("DefaultContextConfig() should be valid, got error: %v", err)
	}
}

func TestContextConfig_JSONSerialization(t *testing.T) {
	tests := []struct {
		name   string
		config agenticloop.ContextConfig
	}{
		{
			name: "full config",
			config: agenticloop.ContextConfig{
				Enabled:          true,
				CompactThreshold: 0.60,
				ToolResultMaxAge: 3,
				HeadroomTokens:   6400,
			},
		},
		{
			name: "disabled config",
			config: agenticloop.ContextConfig{
				Enabled: false,
			},
		},
		{
			name: "custom thresholds",
			config: agenticloop.ContextConfig{
				Enabled:          true,
				CompactThreshold: 0.75,
				ToolResultMaxAge: 5,
				HeadroomTokens:   10000,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Marshal
			data, err := json.Marshal(tt.config)
			if err != nil {
				t.Fatalf("Marshal() error = %v", err)
			}

			// Unmarshal
			var decoded agenticloop.ContextConfig
			if err := json.Unmarshal(data, &decoded); err != nil {
				t.Fatalf("Unmarshal() error = %v", err)
			}

			// Verify round-trip
			if decoded.Enabled != tt.config.Enabled {
				t.Errorf("Enabled = %v, want %v", decoded.Enabled, tt.config.Enabled)
			}
			if decoded.CompactThreshold != tt.config.CompactThreshold {
				t.Errorf("CompactThreshold = %f, want %f", decoded.CompactThreshold, tt.config.CompactThreshold)
			}
			if decoded.ToolResultMaxAge != tt.config.ToolResultMaxAge {
				t.Errorf("ToolResultMaxAge = %d, want %d", decoded.ToolResultMaxAge, tt.config.ToolResultMaxAge)
			}
			if decoded.HeadroomTokens != tt.config.HeadroomTokens {
				t.Errorf("HeadroomTokens = %d, want %d", decoded.HeadroomTokens, tt.config.HeadroomTokens)
			}
		})
	}
}
