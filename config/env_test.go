package config

import (
	"os"
	"testing"
)

func TestExpandEnvWithDefaults(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		envVars  map[string]string
		expected string
	}{
		{
			name:     "braced var set",
			input:    "${MY_VAR}",
			envVars:  map[string]string{"MY_VAR": "hello"},
			expected: "hello",
		},
		{
			name:     "braced var unset",
			input:    "${MY_VAR}",
			envVars:  nil,
			expected: "",
		},
		{
			name:     "braced var with default, var set",
			input:    "${MY_VAR:-fallback}",
			envVars:  map[string]string{"MY_VAR": "hello"},
			expected: "hello",
		},
		{
			name:     "braced var with default, var unset",
			input:    "${MY_VAR:-fallback}",
			envVars:  nil,
			expected: "fallback",
		},
		{
			name:     "simple var set",
			input:    "$MY_VAR",
			envVars:  map[string]string{"MY_VAR": "hello"},
			expected: "hello",
		},
		{
			name:     "simple var unset",
			input:    "$MY_VAR",
			envVars:  nil,
			expected: "",
		},
		{
			name:     "multiple vars in string",
			input:    "${HOST:-localhost}:${PORT:-8080}",
			envVars:  map[string]string{"HOST": "example.com"},
			expected: "example.com:8080",
		},
		{
			name:     "empty default",
			input:    "${MY_VAR:-}",
			envVars:  nil,
			expected: "",
		},
		{
			name:     "URL with default",
			input:    "${LLM_API_URL:-http://localhost:11434}/v1",
			envVars:  nil,
			expected: "http://localhost:11434/v1",
		},
		{
			name:     "URL with default, var set",
			input:    "${LLM_API_URL:-http://localhost:11434}/v1",
			envVars:  map[string]string{"LLM_API_URL": "https://api.example.com"},
			expected: "https://api.example.com/v1",
		},
		{
			name:     "default with colons",
			input:    "${ADDR:-127.0.0.1:8080}",
			envVars:  nil,
			expected: "127.0.0.1:8080",
		},
		{
			name:     "default with path",
			input:    "${CONFIG_PATH:-/etc/app/config.json}",
			envVars:  nil,
			expected: "/etc/app/config.json",
		},
		{
			name:     "nested looking pattern is literal",
			input:    "${VAR:-${OTHER}}",
			envVars:  nil,
			expected: "${OTHER}",
		},
		{
			name:     "mixed braced and simple vars",
			input:    "prefix_${VAR1:-default1}_${VAR2}_suffix",
			envVars:  map[string]string{"VAR2": "value2"},
			expected: "prefix_default1_value2_suffix",
		},
		{
			name:     "no vars",
			input:    "plain text without variables",
			envVars:  nil,
			expected: "plain text without variables",
		},
		{
			name:     "var with underscore and numbers",
			input:    "${MY_VAR_123:-default}",
			envVars:  nil,
			expected: "default",
		},
		{
			name:     "simple var with underscore",
			input:    "$MY_VAR_123",
			envVars:  map[string]string{"MY_VAR_123": "test"},
			expected: "test",
		},
		{
			name:     "empty string",
			input:    "",
			envVars:  nil,
			expected: "",
		},
		{
			name:     "var set to empty string uses default",
			input:    "${MY_VAR:-default}",
			envVars:  map[string]string{"MY_VAR": ""},
			expected: "default",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Clear and set environment variables for this test
			for key := range tt.envVars {
				os.Unsetenv(key)
			}
			// Also unset common test vars
			os.Unsetenv("MY_VAR")
			os.Unsetenv("MY_VAR_123")
			os.Unsetenv("HOST")
			os.Unsetenv("PORT")
			os.Unsetenv("LLM_API_URL")
			os.Unsetenv("ADDR")
			os.Unsetenv("CONFIG_PATH")
			os.Unsetenv("VAR")
			os.Unsetenv("VAR1")
			os.Unsetenv("VAR2")

			for key, value := range tt.envVars {
				os.Setenv(key, value)
			}

			result := ExpandEnvWithDefaults(tt.input)
			if result != tt.expected {
				t.Errorf("ExpandEnvWithDefaults(%q) = %q, want %q", tt.input, result, tt.expected)
			}
		})
	}
}
