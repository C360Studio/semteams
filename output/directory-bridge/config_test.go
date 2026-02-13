package directorybridge

import (
	"strings"
	"testing"
	"time"
)

func TestDefaultConfig(t *testing.T) {
	cfg := DefaultConfig()

	if cfg.Ports == nil {
		t.Error("expected Ports to be set")
	}
	if len(cfg.Ports.Inputs) != 1 {
		t.Errorf("expected 1 input port, got %d", len(cfg.Ports.Inputs))
	}
	if len(cfg.Ports.Outputs) != 1 {
		t.Errorf("expected 1 output port, got %d", len(cfg.Ports.Outputs))
	}
	if cfg.HeartbeatInterval != "30s" {
		t.Errorf("expected heartbeat interval '30s', got %s", cfg.HeartbeatInterval)
	}
	if cfg.RegistrationTTL != "5m" {
		t.Errorf("expected registration TTL '5m', got %s", cfg.RegistrationTTL)
	}
	if cfg.IdentityProvider != "local" {
		t.Errorf("expected identity provider 'local', got %s", cfg.IdentityProvider)
	}
	if cfg.OASFKVBucket != "OASF_RECORDS" {
		t.Errorf("expected OASF KV bucket 'OASF_RECORDS', got %s", cfg.OASFKVBucket)
	}
	if cfg.RetryCount != 3 {
		t.Errorf("expected retry count 3, got %d", cfg.RetryCount)
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
			name:    "valid default config",
			config:  DefaultConfig(),
			wantErr: false,
		},
		{
			name: "missing ports",
			config: Config{
				OASFKVBucket: "OASF_RECORDS",
			},
			wantErr: true,
			errMsg:  "ports configuration is required",
		},
		{
			name: "missing oasf kv bucket",
			config: Config{
				Ports:        DefaultConfig().Ports,
				OASFKVBucket: "",
			},
			wantErr: true,
			errMsg:  "oasf_kv_bucket is required",
		},
		{
			name: "invalid heartbeat interval",
			config: Config{
				Ports:             DefaultConfig().Ports,
				OASFKVBucket:      "OASF_RECORDS",
				HeartbeatInterval: "invalid",
			},
			wantErr: true,
			errMsg:  "invalid heartbeat_interval",
		},
		{
			name: "invalid registration TTL",
			config: Config{
				Ports:           DefaultConfig().Ports,
				OASFKVBucket:    "OASF_RECORDS",
				RegistrationTTL: "invalid",
			},
			wantErr: true,
			errMsg:  "invalid registration_ttl",
		},
		{
			name: "invalid retry delay",
			config: Config{
				Ports:        DefaultConfig().Ports,
				OASFKVBucket: "OASF_RECORDS",
				RetryDelay:   "invalid",
			},
			wantErr: true,
			errMsg:  "invalid retry_delay",
		},
		{
			name: "empty directory URL allowed",
			config: Config{
				Ports:        DefaultConfig().Ports,
				OASFKVBucket: "OASF_RECORDS",
				DirectoryURL: "",
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
				} else if tt.errMsg != "" && err.Error() != tt.errMsg && !strings.Contains(err.Error(), tt.errMsg) {
					t.Errorf("expected error containing %q, got %q", tt.errMsg, err.Error())
				}
			} else if err != nil {
				t.Errorf("unexpected error: %v", err)
			}
		})
	}
}

func TestConfigGetHeartbeatInterval(t *testing.T) {
	tests := []struct {
		name     string
		interval string
		want     time.Duration
	}{
		{
			name:     "valid interval",
			interval: "30s",
			want:     30 * time.Second,
		},
		{
			name:     "empty uses default",
			interval: "",
			want:     30 * time.Second,
		},
		{
			name:     "invalid uses default",
			interval: "invalid",
			want:     30 * time.Second,
		},
		{
			name:     "minutes",
			interval: "2m",
			want:     2 * time.Minute,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := Config{HeartbeatInterval: tt.interval}
			got := cfg.GetHeartbeatInterval()
			if got != tt.want {
				t.Errorf("GetHeartbeatInterval() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestConfigGetRegistrationTTL(t *testing.T) {
	tests := []struct {
		name string
		ttl  string
		want time.Duration
	}{
		{
			name: "valid TTL",
			ttl:  "5m",
			want: 5 * time.Minute,
		},
		{
			name: "empty uses default",
			ttl:  "",
			want: 5 * time.Minute,
		},
		{
			name: "invalid uses default",
			ttl:  "invalid",
			want: 5 * time.Minute,
		},
		{
			name: "hours",
			ttl:  "1h",
			want: time.Hour,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := Config{RegistrationTTL: tt.ttl}
			got := cfg.GetRegistrationTTL()
			if got != tt.want {
				t.Errorf("GetRegistrationTTL() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestConfigGetRetryDelay(t *testing.T) {
	tests := []struct {
		name  string
		delay string
		want  time.Duration
	}{
		{
			name:  "valid delay",
			delay: "1s",
			want:  time.Second,
		},
		{
			name:  "empty uses default",
			delay: "",
			want:  time.Second,
		},
		{
			name:  "invalid uses default",
			delay: "invalid",
			want:  time.Second,
		},
		{
			name:  "milliseconds",
			delay: "500ms",
			want:  500 * time.Millisecond,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := Config{RetryDelay: tt.delay}
			got := cfg.GetRetryDelay()
			if got != tt.want {
				t.Errorf("GetRetryDelay() = %v, want %v", got, tt.want)
			}
		})
	}
}
