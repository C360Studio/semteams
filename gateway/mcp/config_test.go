package mcp

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestConfig_Validate_Defaults(t *testing.T) {
	cfg := &Config{}
	err := cfg.Validate()
	require.NoError(t, err)

	assert.Equal(t, ":8081", cfg.BindAddress)
	assert.Equal(t, "30s", cfg.TimeoutStr)
	assert.Equal(t, "/mcp", cfg.Path)
	assert.Equal(t, "semstreams", cfg.ServerName)
	assert.Equal(t, "1.0.0", cfg.ServerVersion)
	assert.Equal(t, int64(1<<20), cfg.MaxRequestSize)
	assert.Equal(t, 30*time.Second, cfg.timeout)
}

func TestConfig_Validate_CustomValues(t *testing.T) {
	cfg := &Config{
		BindAddress:    "0.0.0.0:9000",
		TimeoutStr:     "2m",
		Path:           "/api/mcp",
		ServerName:     "custom-server",
		ServerVersion:  "2.0.0",
		MaxRequestSize: 5 << 20, // 5MB
	}
	err := cfg.Validate()
	require.NoError(t, err)

	assert.Equal(t, "0.0.0.0:9000", cfg.BindAddress)
	assert.Equal(t, 2*time.Minute, cfg.timeout)
	assert.Equal(t, "/api/mcp", cfg.Path)
	assert.Equal(t, "custom-server", cfg.ServerName)
	assert.Equal(t, "2.0.0", cfg.ServerVersion)
	assert.Equal(t, int64(5<<20), cfg.MaxRequestSize)
}

func TestConfig_Validate_Timeout(t *testing.T) {
	tests := []struct {
		name      string
		timeout   string
		wantErr   bool
		errSubstr string
		expected  time.Duration
	}{
		{
			name:     "valid 1 second",
			timeout:  "1s",
			wantErr:  false,
			expected: time.Second,
		},
		{
			name:     "valid 5 minutes",
			timeout:  "5m",
			wantErr:  false,
			expected: 5 * time.Minute,
		},
		{
			name:     "valid 90 seconds",
			timeout:  "90s",
			wantErr:  false,
			expected: 90 * time.Second,
		},
		{
			name:      "too short 500ms",
			timeout:   "500ms",
			wantErr:   true,
			errSubstr: "at least 1s",
		},
		{
			name:      "too long 10 minutes",
			timeout:   "10m",
			wantErr:   true,
			errSubstr: "not exceed 5m",
		},
		{
			name:      "invalid format",
			timeout:   "invalid",
			wantErr:   true,
			errSubstr: "invalid timeout duration",
		},
		{
			name:     "empty becomes default",
			timeout:  "",
			wantErr:  false,
			expected: 30 * time.Second,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := &Config{TimeoutStr: tt.timeout}
			err := cfg.Validate()

			if tt.wantErr {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.errSubstr)
			} else {
				require.NoError(t, err)
				assert.Equal(t, tt.expected, cfg.timeout)
			}
		})
	}
}

func TestConfig_Validate_BindAddress(t *testing.T) {
	tests := []struct {
		name      string
		address   string
		wantErr   bool
		errSubstr string
	}{
		{
			name:    "port only",
			address: ":8081",
			wantErr: false,
		},
		{
			name:    "localhost with port",
			address: "localhost:8080",
			wantErr: false,
		},
		{
			name:    "IP with port",
			address: "127.0.0.1:9000",
			wantErr: false,
		},
		{
			name:    "all interfaces",
			address: "0.0.0.0:8081",
			wantErr: false,
		},
		{
			name:    "port 1 minimum",
			address: ":1",
			wantErr: false,
		},
		{
			name:    "port 65535 maximum",
			address: ":65535",
			wantErr: false,
		},
		{
			name:      "port 0 invalid",
			address:   ":0",
			wantErr:   true,
			errSubstr: "port must be 1-65535",
		},
		{
			name:      "port 65536 invalid",
			address:   ":65536",
			wantErr:   true,
			errSubstr: "port must be 1-65535",
		},
		{
			name:      "no port",
			address:   "localhost",
			wantErr:   true,
			errSubstr: "invalid bind address",
		},
		{
			name:      "invalid format",
			address:   "not-an-address",
			wantErr:   true,
			errSubstr: "invalid bind address",
		},
		{
			name:      "non-numeric port",
			address:   ":abc",
			wantErr:   true,
			errSubstr: "port must be 1-65535",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := &Config{BindAddress: tt.address}
			err := cfg.Validate()

			if tt.wantErr {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.errSubstr)
			} else {
				require.NoError(t, err)
			}
		})
	}
}

func TestConfig_Validate_Path(t *testing.T) {
	tests := []struct {
		name      string
		path      string
		wantErr   bool
		errSubstr string
	}{
		{
			name:    "simple path",
			path:    "/mcp",
			wantErr: false,
		},
		{
			name:    "nested path",
			path:    "/api/v1/mcp",
			wantErr: false,
		},
		{
			name:    "root path",
			path:    "/",
			wantErr: false,
		},
		{
			name:      "no leading slash",
			path:      "mcp",
			wantErr:   true,
			errSubstr: "must start with /",
		},
		{
			name:      "relative path",
			path:      "api/mcp",
			wantErr:   true,
			errSubstr: "must start with /",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := &Config{Path: tt.path}
			err := cfg.Validate()

			if tt.wantErr {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.errSubstr)
			} else {
				require.NoError(t, err)
			}
		})
	}
}

func TestConfig_Validate_MaxRequestSize(t *testing.T) {
	tests := []struct {
		name      string
		size      int64
		wantErr   bool
		errSubstr string
	}{
		{
			name:    "1KB minimum",
			size:    1024,
			wantErr: false,
		},
		{
			name:    "1MB default equivalent",
			size:    1 << 20,
			wantErr: false,
		},
		{
			name:    "10MB",
			size:    10 << 20,
			wantErr: false,
		},
		{
			name:      "too small 512 bytes",
			size:      512,
			wantErr:   true,
			errSubstr: "at least 1KB",
		},
		{
			name:      "too small 1023 bytes",
			size:      1023,
			wantErr:   true,
			errSubstr: "at least 1KB",
		},
		{
			name:    "zero uses default",
			size:    0,
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := &Config{MaxRequestSize: tt.size}
			err := cfg.Validate()

			if tt.wantErr {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.errSubstr)
			} else {
				require.NoError(t, err)
			}
		})
	}
}

func TestConfig_Timeout_WithoutValidate(t *testing.T) {
	cfg := &Config{}
	// Without calling Validate(), should return default
	assert.Equal(t, 30*time.Second, cfg.Timeout())
}

func TestConfig_Timeout_AfterValidate(t *testing.T) {
	cfg := &Config{TimeoutStr: "2m"}
	err := cfg.Validate()
	require.NoError(t, err)

	assert.Equal(t, 2*time.Minute, cfg.Timeout())
}
