package model

import (
	"encoding/json"
	"testing"
)

func testRegistry() *Registry {
	return &Registry{
		Capabilities: map[string]*CapabilityConfig{
			"planning": {
				Description: "High-level reasoning",
				Preferred:   []string{"claude-sonnet", "qwen"},
				Fallback:    []string{"qwen-fast"},
			},
			"coding": {
				Description:   "Code generation with tool use",
				Preferred:     []string{"claude-sonnet"},
				Fallback:      []string{"qwen"},
				RequiresTools: true,
			},
			"fast": {
				Description: "Quick tasks",
				Preferred:   []string{"qwen-fast"},
			},
		},
		Endpoints: map[string]*EndpointConfig{
			"claude-sonnet": {
				Provider:      "anthropic",
				Model:         "claude-sonnet-4-20250514",
				MaxTokens:     200000,
				SupportsTools: true,
				ToolFormat:    "anthropic",
				APIKeyEnv:     "ANTHROPIC_API_KEY",
			},
			"qwen": {
				Provider:      "ollama",
				URL:           "http://localhost:11434/v1",
				Model:         "qwen3-coder:30b",
				MaxTokens:     131072,
				SupportsTools: true,
				ToolFormat:    "openai",
			},
			"qwen-fast": {
				Provider:  "ollama",
				URL:       "http://localhost:11434/v1",
				Model:     "qwen3:1.7b",
				MaxTokens: 32768,
			},
		},
		Defaults: DefaultsConfig{
			Model:      "qwen",
			Capability: "planning",
		},
	}
}

func TestValidate(t *testing.T) {
	tests := []struct {
		name    string
		modify  func(*Registry)
		wantErr string
	}{
		{
			name:   "valid registry",
			modify: func(_ *Registry) {},
		},
		{
			name: "no endpoints",
			modify: func(r *Registry) {
				r.Endpoints = nil
			},
			wantErr: "at least one endpoint is required",
		},
		{
			name: "endpoint missing model",
			modify: func(r *Registry) {
				r.Endpoints["bad"] = &EndpointConfig{MaxTokens: 1000}
			},
			wantErr: "endpoint \"bad\": model is required",
		},
		{
			name: "endpoint zero max_tokens",
			modify: func(r *Registry) {
				r.Endpoints["bad"] = &EndpointConfig{Model: "test", MaxTokens: 0}
			},
			wantErr: "endpoint \"bad\": max_tokens must be positive",
		},
		{
			name: "endpoint unknown provider",
			modify: func(r *Registry) {
				r.Endpoints["bad"] = &EndpointConfig{
					Provider: "unknown", Model: "test", MaxTokens: 1000,
				}
			},
			wantErr: "endpoint \"bad\": unknown provider \"unknown\"",
		},
		{
			name: "endpoint invalid tool_format",
			modify: func(r *Registry) {
				r.Endpoints["bad"] = &EndpointConfig{
					Model: "test", MaxTokens: 1000, ToolFormat: "bad",
				}
			},
			wantErr: "endpoint \"bad\": tool_format must be",
		},
		{
			name: "nil endpoint",
			modify: func(r *Registry) {
				r.Endpoints["bad"] = nil
			},
			wantErr: "endpoint \"bad\" is nil",
		},
		{
			name: "capability references non-existent preferred",
			modify: func(r *Registry) {
				r.Capabilities["bad"] = &CapabilityConfig{
					Preferred: []string{"nonexistent"},
				}
			},
			wantErr: "capability \"bad\": preferred endpoint \"nonexistent\" does not exist",
		},
		{
			name: "capability references non-existent fallback",
			modify: func(r *Registry) {
				r.Capabilities["bad"] = &CapabilityConfig{
					Preferred: []string{"qwen"},
					Fallback:  []string{"nonexistent"},
				}
			},
			wantErr: "capability \"bad\": fallback endpoint \"nonexistent\" does not exist",
		},
		{
			name: "capability empty preferred",
			modify: func(r *Registry) {
				r.Capabilities["bad"] = &CapabilityConfig{
					Preferred: []string{},
				}
			},
			wantErr: "capability \"bad\": at least one preferred endpoint is required",
		},
		{
			name: "nil capability",
			modify: func(r *Registry) {
				r.Capabilities["bad"] = nil
			},
			wantErr: "capability \"bad\" is nil",
		},
		{
			name: "requires_tools but no tool-capable endpoints",
			modify: func(r *Registry) {
				r.Capabilities["bad"] = &CapabilityConfig{
					Preferred:     []string{"qwen-fast"},
					RequiresTools: true,
				}
			},
			wantErr: "requires_tools is set but no endpoint in the chain supports tools",
		},
		{
			name: "default model references non-existent endpoint",
			modify: func(r *Registry) {
				r.Defaults.Model = "nonexistent"
			},
			wantErr: "defaults.model \"nonexistent\" references non-existent endpoint",
		},
		{
			name: "default capability references non-existent capability",
			modify: func(r *Registry) {
				r.Defaults.Capability = "nonexistent"
			},
			wantErr: "defaults.capability \"nonexistent\" references non-existent capability",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := testRegistry()
			tt.modify(r)
			err := r.Validate()
			if tt.wantErr == "" {
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				return
			}
			if err == nil {
				t.Fatal("expected error, got nil")
			}
			if got := err.Error(); !contains(got, tt.wantErr) {
				t.Fatalf("error %q does not contain %q", got, tt.wantErr)
			}
		})
	}
}

func TestResolve(t *testing.T) {
	r := testRegistry()

	tests := []struct {
		capability string
		want       string
	}{
		{"planning", "claude-sonnet"},
		{"coding", "claude-sonnet"}, // requires_tools filters, claude-sonnet supports tools
		{"fast", "qwen-fast"},
		{"unknown", "qwen"}, // falls back to default model
	}

	for _, tt := range tests {
		t.Run(tt.capability, func(t *testing.T) {
			got := r.Resolve(tt.capability)
			if got != tt.want {
				t.Fatalf("Resolve(%q) = %q, want %q", tt.capability, got, tt.want)
			}
		})
	}
}

func TestResolve_RequiresToolsFiltering(t *testing.T) {
	r := &Registry{
		Capabilities: map[string]*CapabilityConfig{
			"tools": {
				Preferred:     []string{"no-tools", "has-tools"},
				RequiresTools: true,
			},
		},
		Endpoints: map[string]*EndpointConfig{
			"no-tools": {
				Model: "basic", MaxTokens: 32768,
			},
			"has-tools": {
				Model: "advanced", MaxTokens: 128000, SupportsTools: true,
			},
		},
		Defaults: DefaultsConfig{Model: "no-tools"},
	}

	// Should skip "no-tools" and return "has-tools"
	got := r.Resolve("tools")
	if got != "has-tools" {
		t.Fatalf("Resolve(\"tools\") = %q, want \"has-tools\"", got)
	}
}

func TestGetFallbackChain(t *testing.T) {
	r := testRegistry()

	tests := []struct {
		capability string
		want       []string
	}{
		{"planning", []string{"claude-sonnet", "qwen", "qwen-fast"}},
		{"coding", []string{"claude-sonnet", "qwen"}}, // requires_tools filters out qwen-fast
		{"fast", []string{"qwen-fast"}},
		{"unknown", nil},
	}

	for _, tt := range tests {
		t.Run(tt.capability, func(t *testing.T) {
			got := r.GetFallbackChain(tt.capability)
			if !slicesEqual(got, tt.want) {
				t.Fatalf("GetFallbackChain(%q) = %v, want %v", tt.capability, got, tt.want)
			}
		})
	}
}

func TestGetEndpoint(t *testing.T) {
	r := testRegistry()

	t.Run("existing", func(t *testing.T) {
		ep := r.GetEndpoint("claude-sonnet")
		if ep == nil {
			t.Fatal("expected non-nil endpoint")
		}
		if ep.Model != "claude-sonnet-4-20250514" {
			t.Fatalf("got model %q, want %q", ep.Model, "claude-sonnet-4-20250514")
		}
		if ep.MaxTokens != 200000 {
			t.Fatalf("got max_tokens %d, want %d", ep.MaxTokens, 200000)
		}
		if !ep.SupportsTools {
			t.Fatal("expected supports_tools=true")
		}
	})

	t.Run("non-existent", func(t *testing.T) {
		ep := r.GetEndpoint("nonexistent")
		if ep != nil {
			t.Fatalf("expected nil, got %+v", ep)
		}
	})
}

func TestGetMaxTokens(t *testing.T) {
	r := testRegistry()

	tests := []struct {
		name string
		want int
	}{
		{"claude-sonnet", 200000},
		{"qwen", 131072},
		{"qwen-fast", 32768},
		{"nonexistent", 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := r.GetMaxTokens(tt.name)
			if got != tt.want {
				t.Fatalf("GetMaxTokens(%q) = %d, want %d", tt.name, got, tt.want)
			}
		})
	}
}

func TestGetDefault(t *testing.T) {
	r := testRegistry()
	if got := r.GetDefault(); got != "qwen" {
		t.Fatalf("GetDefault() = %q, want %q", got, "qwen")
	}
}

func TestListCapabilities(t *testing.T) {
	r := testRegistry()
	got := r.ListCapabilities()
	want := []string{"coding", "fast", "planning"}
	if !slicesEqual(got, want) {
		t.Fatalf("ListCapabilities() = %v, want %v", got, want)
	}
}

func TestListEndpoints(t *testing.T) {
	r := testRegistry()
	got := r.ListEndpoints()
	want := []string{"claude-sonnet", "qwen", "qwen-fast"}
	if !slicesEqual(got, want) {
		t.Fatalf("ListEndpoints() = %v, want %v", got, want)
	}
}

func TestJSONRoundTrip(t *testing.T) {
	r := testRegistry()
	data, err := json.Marshal(r)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	var got Registry
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if err := got.Validate(); err != nil {
		t.Fatalf("round-tripped registry is invalid: %v", err)
	}

	if got.Defaults.Model != r.Defaults.Model {
		t.Fatalf("defaults.model: got %q, want %q", got.Defaults.Model, r.Defaults.Model)
	}
	if len(got.Endpoints) != len(r.Endpoints) {
		t.Fatalf("endpoints count: got %d, want %d", len(got.Endpoints), len(r.Endpoints))
	}
	if len(got.Capabilities) != len(r.Capabilities) {
		t.Fatalf("capabilities count: got %d, want %d", len(got.Capabilities), len(r.Capabilities))
	}
}

func TestMinimalRegistry(t *testing.T) {
	r := &Registry{
		Endpoints: map[string]*EndpointConfig{
			"default": {
				Provider:  "ollama",
				URL:       "http://localhost:11434/v1",
				Model:     "llama3.2",
				MaxTokens: 128000,
			},
		},
		Defaults: DefaultsConfig{Model: "default"},
	}

	if err := r.Validate(); err != nil {
		t.Fatalf("minimal registry should be valid: %v", err)
	}

	if got := r.GetDefault(); got != "default" {
		t.Fatalf("GetDefault() = %q, want %q", got, "default")
	}

	if got := r.GetMaxTokens("default"); got != 128000 {
		t.Fatalf("GetMaxTokens(\"default\") = %d, want %d", got, 128000)
	}

	// Unknown capability falls back to default
	if got := r.Resolve("unknown"); got != "default" {
		t.Fatalf("Resolve(\"unknown\") = %q, want %q", got, "default")
	}
}

// helpers

func contains(s, substr string) bool {
	return len(s) >= len(substr) && searchString(s, substr)
}

func searchString(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

func slicesEqual(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
