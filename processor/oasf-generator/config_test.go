package oasfgenerator

import (
	"testing"
	"time"
)

func TestConfig_Validate(t *testing.T) {
	tests := []struct {
		name    string
		config  Config
		wantErr bool
	}{
		{
			name:    "valid default config",
			config:  DefaultConfig(),
			wantErr: false,
		},
		{
			name: "missing ports",
			config: Config{
				EntityKVBucket: "ENTITY_STATES",
				OASFKVBucket:   "OASF_RECORDS",
			},
			wantErr: true,
		},
		{
			name: "missing entity bucket",
			config: Config{
				Ports:        DefaultConfig().Ports,
				OASFKVBucket: "OASF_RECORDS",
			},
			wantErr: true,
		},
		{
			name: "missing oasf bucket",
			config: Config{
				Ports:          DefaultConfig().Ports,
				EntityKVBucket: "ENTITY_STATES",
			},
			wantErr: true,
		},
		{
			name: "invalid debounce duration",
			config: Config{
				Ports:              DefaultConfig().Ports,
				EntityKVBucket:     "ENTITY_STATES",
				OASFKVBucket:       "OASF_RECORDS",
				GenerationDebounce: "invalid",
			},
			wantErr: true,
		},
		{
			name: "valid custom config",
			config: Config{
				Ports:              DefaultConfig().Ports,
				EntityKVBucket:     "CUSTOM_ENTITIES",
				OASFKVBucket:       "CUSTOM_OASF",
				GenerationDebounce: "2s",
				WatchPattern:       "*.agent.*",
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.config.Validate()
			if (err != nil) != tt.wantErr {
				t.Errorf("Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestConfig_GetGenerationDebounce(t *testing.T) {
	tests := []struct {
		name     string
		debounce string
		want     time.Duration
	}{
		{
			name:     "valid duration",
			debounce: "2s",
			want:     2 * time.Second,
		},
		{
			name:     "milliseconds",
			debounce: "500ms",
			want:     500 * time.Millisecond,
		},
		{
			name:     "empty defaults to 1s",
			debounce: "",
			want:     time.Second,
		},
		{
			name:     "invalid defaults to 1s",
			debounce: "invalid",
			want:     time.Second,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			config := Config{GenerationDebounce: tt.debounce}
			got := config.GetGenerationDebounce()
			if got != tt.want {
				t.Errorf("GetGenerationDebounce() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestDefaultConfig(t *testing.T) {
	config := DefaultConfig()

	// Check required fields are set
	if config.Ports == nil {
		t.Error("expected Ports to be set")
	}
	if config.EntityKVBucket == "" {
		t.Error("expected EntityKVBucket to be set")
	}
	if config.OASFKVBucket == "" {
		t.Error("expected OASFKVBucket to be set")
	}
	if config.DefaultAgentVersion == "" {
		t.Error("expected DefaultAgentVersion to be set")
	}

	// Check ports are configured
	if len(config.Ports.Inputs) == 0 {
		t.Error("expected input ports to be configured")
	}
	if len(config.Ports.Outputs) == 0 {
		t.Error("expected output ports to be configured")
	}
}
