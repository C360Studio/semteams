// Package model provides a unified model registry for centralized endpoint
// configuration, capability-based routing, and tool capability metadata.
package model

import (
	"fmt"
	"sort"
)

// EndpointConfig defines an available model endpoint.
type EndpointConfig struct {
	// Provider identifies the API type: "anthropic", "ollama", "openai", "openrouter".
	Provider string `json:"provider"`
	// URL is the API endpoint. Required for ollama/openai/openrouter, optional for anthropic.
	URL string `json:"url,omitempty"`
	// Model is the model identifier sent to the provider.
	Model string `json:"model"`
	// MaxTokens is the context window size in tokens.
	MaxTokens int `json:"max_tokens"`
	// SupportsTools indicates whether this endpoint supports function/tool calling.
	SupportsTools bool `json:"supports_tools,omitempty"`
	// ToolFormat specifies the tool calling format: "anthropic" or "openai".
	// Empty means auto-detect from provider.
	ToolFormat string `json:"tool_format,omitempty"`
	// APIKeyEnv is the environment variable containing the API key.
	// Required for anthropic/openai/openrouter, ignored for ollama.
	APIKeyEnv string `json:"api_key_env,omitempty"`
	// Options holds provider-specific template parameters passed to the API.
	// For vLLM/ollama with thinking models, set "enable_thinking" and
	// "thinking_budget" here — they are forwarded as chat_template_kwargs.
	// Do not use for inference parameters (temperature, top_k, etc.) which
	// have dedicated fields in AgentRequest.
	Options map[string]any `json:"options,omitempty"`
}

// CapabilityConfig defines model preferences for a capability.
type CapabilityConfig struct {
	// Description explains what this capability is for.
	Description string `json:"description,omitempty"`
	// Preferred lists endpoint names in order of preference.
	Preferred []string `json:"preferred"`
	// Fallback lists backup endpoint names if all preferred are unavailable.
	Fallback []string `json:"fallback,omitempty"`
	// RequiresTools filters the chain to only tool-capable endpoints.
	RequiresTools bool `json:"requires_tools,omitempty"`
}

// DefaultsConfig holds default model settings.
type DefaultsConfig struct {
	// Model is the default endpoint name when no capability matches.
	Model string `json:"model"`
	// Capability is the default capability when none specified.
	Capability string `json:"capability,omitempty"`
}

// Registry holds all model endpoint definitions and capability routing.
// It is JSON-serializable for config loading and implements RegistryReader.
type Registry struct {
	Capabilities map[string]*CapabilityConfig `json:"capabilities,omitempty"`
	Endpoints    map[string]*EndpointConfig   `json:"endpoints"`
	Defaults     DefaultsConfig               `json:"defaults"`
}

// RegistryReader provides read-only access to the model registry.
// Components receive this interface via Dependencies.
type RegistryReader interface {
	// Resolve returns the preferred endpoint name for a capability.
	// Returns the first endpoint in the preferred list.
	// If RequiresTools is set, filters to tool-capable endpoints.
	Resolve(capability string) string

	// GetFallbackChain returns all endpoint names for a capability in preference order.
	// Includes both preferred and fallback endpoints.
	GetFallbackChain(capability string) []string

	// GetEndpoint returns the full endpoint configuration for an endpoint name.
	// Returns nil if the endpoint is not configured.
	GetEndpoint(name string) *EndpointConfig

	// GetMaxTokens returns the context window size for an endpoint name.
	// Returns 0 if the endpoint is not configured.
	GetMaxTokens(name string) int

	// GetDefault returns the default endpoint name.
	GetDefault() string

	// ListCapabilities returns all configured capability names sorted alphabetically.
	ListCapabilities() []string

	// ListEndpoints returns all configured endpoint names sorted alphabetically.
	ListEndpoints() []string
}

// Validate checks the registry configuration for consistency.
func (r *Registry) Validate() error {
	if len(r.Endpoints) == 0 {
		return fmt.Errorf("at least one endpoint is required")
	}

	for name, ep := range r.Endpoints {
		if err := validateEndpoint(name, ep); err != nil {
			return err
		}
	}

	for name, cap := range r.Capabilities {
		if err := r.validateCapability(name, cap); err != nil {
			return err
		}
	}

	if r.Defaults.Model != "" {
		if _, ok := r.Endpoints[r.Defaults.Model]; !ok {
			return fmt.Errorf("defaults.model %q references non-existent endpoint", r.Defaults.Model)
		}
	}

	if r.Defaults.Capability != "" {
		if _, ok := r.Capabilities[r.Defaults.Capability]; !ok {
			return fmt.Errorf("defaults.capability %q references non-existent capability", r.Defaults.Capability)
		}
	}

	return nil
}

func validateEndpoint(name string, ep *EndpointConfig) error {
	if ep == nil {
		return fmt.Errorf("endpoint %q is nil", name)
	}
	if ep.Model == "" {
		return fmt.Errorf("endpoint %q: model is required", name)
	}
	if ep.MaxTokens < 1 {
		return fmt.Errorf("endpoint %q: max_tokens must be positive", name)
	}

	validProviders := map[string]bool{
		"anthropic": true, "ollama": true, "openai": true, "openrouter": true,
	}
	if ep.Provider != "" && !validProviders[ep.Provider] {
		return fmt.Errorf("endpoint %q: unknown provider %q", name, ep.Provider)
	}

	if ep.ToolFormat != "" && ep.ToolFormat != "anthropic" && ep.ToolFormat != "openai" {
		return fmt.Errorf("endpoint %q: tool_format must be \"anthropic\" or \"openai\"", name)
	}

	return nil
}

func (r *Registry) validateCapability(name string, cap *CapabilityConfig) error {
	if cap == nil {
		return fmt.Errorf("capability %q is nil", name)
	}
	if len(cap.Preferred) == 0 {
		return fmt.Errorf("capability %q: at least one preferred endpoint is required", name)
	}

	// All referenced endpoints must exist
	for _, epName := range cap.Preferred {
		if _, ok := r.Endpoints[epName]; !ok {
			return fmt.Errorf("capability %q: preferred endpoint %q does not exist", name, epName)
		}
	}
	for _, epName := range cap.Fallback {
		if _, ok := r.Endpoints[epName]; !ok {
			return fmt.Errorf("capability %q: fallback endpoint %q does not exist", name, epName)
		}
	}

	// If RequiresTools, at least one endpoint in the chain must support tools
	if cap.RequiresTools {
		chain := append(cap.Preferred, cap.Fallback...)
		hasToolCapable := false
		for _, epName := range chain {
			if ep, ok := r.Endpoints[epName]; ok && ep.SupportsTools {
				hasToolCapable = true
				break
			}
		}
		if !hasToolCapable {
			return fmt.Errorf("capability %q: requires_tools is set but no endpoint in the chain supports tools", name)
		}
	}

	return nil
}

// Resolve returns the preferred endpoint name for a capability.
func (r *Registry) Resolve(capability string) string {
	capCfg, ok := r.Capabilities[capability]
	if !ok {
		return r.Defaults.Model
	}

	chain := r.buildChain(capCfg)
	if len(chain) == 0 {
		return r.Defaults.Model
	}
	return chain[0]
}

// GetFallbackChain returns all endpoint names for a capability in preference order.
func (r *Registry) GetFallbackChain(capability string) []string {
	capCfg, ok := r.Capabilities[capability]
	if !ok {
		return nil
	}
	return r.buildChain(capCfg)
}

// GetEndpoint returns the endpoint configuration for a name, or nil if not found.
func (r *Registry) GetEndpoint(name string) *EndpointConfig {
	ep, ok := r.Endpoints[name]
	if !ok {
		return nil
	}
	return ep
}

// GetMaxTokens returns the context window size for an endpoint name.
func (r *Registry) GetMaxTokens(name string) int {
	ep, ok := r.Endpoints[name]
	if !ok {
		return 0
	}
	return ep.MaxTokens
}

// GetDefault returns the default endpoint name.
func (r *Registry) GetDefault() string {
	return r.Defaults.Model
}

// ListCapabilities returns all configured capability names sorted alphabetically.
func (r *Registry) ListCapabilities() []string {
	names := make([]string, 0, len(r.Capabilities))
	for name := range r.Capabilities {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

// ListEndpoints returns all configured endpoint names sorted alphabetically.
func (r *Registry) ListEndpoints() []string {
	names := make([]string, 0, len(r.Endpoints))
	for name := range r.Endpoints {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

// buildChain constructs the full endpoint chain for a capability,
// filtering by tool support if RequiresTools is set.
func (r *Registry) buildChain(cap *CapabilityConfig) []string {
	all := make([]string, 0, len(cap.Preferred)+len(cap.Fallback))
	all = append(all, cap.Preferred...)
	all = append(all, cap.Fallback...)

	if !cap.RequiresTools {
		return all
	}

	// Filter to tool-capable endpoints only
	filtered := make([]string, 0, len(all))
	for _, name := range all {
		if ep, ok := r.Endpoints[name]; ok && ep.SupportsTools {
			filtered = append(filtered, name)
		}
	}
	return filtered
}
