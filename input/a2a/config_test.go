package a2a

import (
	"testing"

	"github.com/c360studio/semstreams/component"
)

func TestDefaultConfig(t *testing.T) {
	cfg := DefaultConfig()

	// Should have default ports
	if cfg.Ports == nil {
		t.Fatal("expected default ports configuration")
	}

	// Should have input port
	if len(cfg.Ports.Inputs) == 0 {
		t.Fatal("expected at least one input port")
	}

	// Should have output ports
	if len(cfg.Ports.Outputs) == 0 {
		t.Fatal("expected at least one output port")
	}

	// Should have default values
	if cfg.Transport != "http" {
		t.Errorf("expected default transport 'http', got %q", cfg.Transport)
	}

	if cfg.ListenAddress != ":8080" {
		t.Errorf("expected default listen_address ':8080', got %q", cfg.ListenAddress)
	}

	if cfg.AgentCardPath != "/.well-known/agent.json" {
		t.Errorf("expected default agent_card_path, got %q", cfg.AgentCardPath)
	}

	if cfg.RequestTimeout != "30s" {
		t.Errorf("expected default request_timeout '30s', got %q", cfg.RequestTimeout)
	}

	if cfg.MaxConcurrentTasks != 10 {
		t.Errorf("expected default max_concurrent_tasks 10, got %d", cfg.MaxConcurrentTasks)
	}

	if !cfg.EnableAuthentication {
		t.Error("expected enable_authentication to be true by default")
	}

	if cfg.OASFBucket != "OASF_RECORDS" {
		t.Errorf("expected default oasf_bucket 'OASF_RECORDS', got %q", cfg.OASFBucket)
	}
}

func TestConfigValidate(t *testing.T) {
	tests := []struct {
		name    string
		config  Config
		wantErr bool
		errMsg  string
	}{
		{
			name: "valid http config",
			config: Config{
				Ports:     &component.PortConfig{},
				Transport: "http",
			},
			wantErr: false,
		},
		{
			name: "valid slim config",
			config: Config{
				Ports:       &component.PortConfig{},
				Transport:   "slim",
				SLIMGroupID: "did:agntcy:group:test",
			},
			wantErr: false,
		},
		{
			name:    "missing ports",
			config:  Config{},
			wantErr: true,
			errMsg:  "ports configuration is required",
		},
		{
			name: "invalid transport",
			config: Config{
				Ports:     &component.PortConfig{},
				Transport: "grpc",
			},
			wantErr: true,
			errMsg:  "invalid transport",
		},
		{
			name: "slim transport without group",
			config: Config{
				Ports:     &component.PortConfig{},
				Transport: "slim",
			},
			wantErr: true,
			errMsg:  "slim_group_id is required",
		},
		{
			name: "invalid request timeout",
			config: Config{
				Ports:          &component.PortConfig{},
				RequestTimeout: "notaduration",
			},
			wantErr: true,
			errMsg:  "invalid request_timeout",
		},
		{
			name: "negative max concurrent tasks",
			config: Config{
				Ports:              &component.PortConfig{},
				MaxConcurrentTasks: -1,
			},
			wantErr: true,
			errMsg:  "max_concurrent_tasks must be non-negative",
		},
		{
			name: "zero values allowed",
			config: Config{
				Ports:              &component.PortConfig{},
				MaxConcurrentTasks: 0,
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.config.Validate()
			if tt.wantErr {
				if err == nil {
					t.Error("expected error, got nil")
				} else if tt.errMsg != "" && !containsString(err.Error(), tt.errMsg) {
					t.Errorf("expected error containing %q, got %q", tt.errMsg, err.Error())
				}
			} else if err != nil {
				t.Errorf("unexpected error: %v", err)
			}
		})
	}
}

func TestConfigGetRequestTimeout(t *testing.T) {
	tests := []struct {
		name    string
		timeout string
		want    string
	}{
		{
			name:    "empty defaults to 30s",
			timeout: "",
			want:    "30s",
		},
		{
			name:    "1 minute",
			timeout: "1m",
			want:    "1m0s",
		},
		{
			name:    "10 seconds",
			timeout: "10s",
			want:    "10s",
		},
		{
			name:    "invalid defaults to 30s",
			timeout: "invalid",
			want:    "30s",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := Config{RequestTimeout: tt.timeout}
			got := cfg.GetRequestTimeout()
			if got.String() != tt.want {
				t.Errorf("GetRequestTimeout() = %v, want %v", got, tt.want)
			}
		})
	}
}

// containsString checks if s contains substr
func containsString(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
