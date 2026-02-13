package slim

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
	if cfg.KeyRatchetInterval != "1h" {
		t.Errorf("expected default key_ratchet_interval '1h', got %q", cfg.KeyRatchetInterval)
	}

	if cfg.ReconnectInterval != "5s" {
		t.Errorf("expected default reconnect_interval '5s', got %q", cfg.ReconnectInterval)
	}

	if cfg.MaxReconnectAttempts != 10 {
		t.Errorf("expected default max_reconnect_attempts 10, got %d", cfg.MaxReconnectAttempts)
	}

	if cfg.MessageBufferSize != 1000 {
		t.Errorf("expected default message_buffer_size 1000, got %d", cfg.MessageBufferSize)
	}

	if cfg.IdentityProvider != "local" {
		t.Errorf("expected default identity_provider 'local', got %q", cfg.IdentityProvider)
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
			name: "valid config",
			config: Config{
				Ports:              &component.PortConfig{},
				KeyRatchetInterval: "30m",
				ReconnectInterval:  "10s",
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
			name: "invalid key ratchet interval",
			config: Config{
				Ports:              &component.PortConfig{},
				KeyRatchetInterval: "invalid",
			},
			wantErr: true,
			errMsg:  "invalid key_ratchet_interval",
		},
		{
			name: "invalid reconnect interval",
			config: Config{
				Ports:             &component.PortConfig{},
				ReconnectInterval: "notaduration",
			},
			wantErr: true,
			errMsg:  "invalid reconnect_interval",
		},
		{
			name: "negative max reconnect attempts",
			config: Config{
				Ports:                &component.PortConfig{},
				MaxReconnectAttempts: -1,
			},
			wantErr: true,
			errMsg:  "max_reconnect_attempts must be non-negative",
		},
		{
			name: "negative message buffer size",
			config: Config{
				Ports:             &component.PortConfig{},
				MessageBufferSize: -1,
			},
			wantErr: true,
			errMsg:  "message_buffer_size must be non-negative",
		},
		{
			name: "zero values allowed",
			config: Config{
				Ports:                &component.PortConfig{},
				MaxReconnectAttempts: 0,
				MessageBufferSize:    0,
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

func TestConfigGetKeyRatchetInterval(t *testing.T) {
	tests := []struct {
		name     string
		interval string
		want     string
	}{
		{
			name:     "empty defaults to 1h",
			interval: "",
			want:     "1h0m0s",
		},
		{
			name:     "30 minutes",
			interval: "30m",
			want:     "30m0s",
		},
		{
			name:     "2 hours",
			interval: "2h",
			want:     "2h0m0s",
		},
		{
			name:     "invalid defaults to 1h",
			interval: "invalid",
			want:     "1h0m0s",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := Config{KeyRatchetInterval: tt.interval}
			got := cfg.GetKeyRatchetInterval()
			if got.String() != tt.want {
				t.Errorf("GetKeyRatchetInterval() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestConfigGetReconnectInterval(t *testing.T) {
	tests := []struct {
		name     string
		interval string
		want     string
	}{
		{
			name:     "empty defaults to 5s",
			interval: "",
			want:     "5s",
		},
		{
			name:     "10 seconds",
			interval: "10s",
			want:     "10s",
		},
		{
			name:     "1 minute",
			interval: "1m",
			want:     "1m0s",
		},
		{
			name:     "invalid defaults to 5s",
			interval: "invalid",
			want:     "5s",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := Config{ReconnectInterval: tt.interval}
			got := cfg.GetReconnectInterval()
			if got.String() != tt.want {
				t.Errorf("GetReconnectInterval() = %v, want %v", got, tt.want)
			}
		})
	}
}

// containsString checks if s contains substr
func containsString(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(substr) == 0 ||
		(len(s) > 0 && len(substr) > 0 && findSubstring(s, substr)))
}

func findSubstring(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
