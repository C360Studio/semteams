package agentictools

import (
	"fmt"
	"strings"
	"sync"
	"time"
)

// ExternalToolRegistration represents an external tool registration request
type ExternalToolRegistration struct {
	Name        string                 `json:"name"`
	Description string                 `json:"description"`
	Parameters  map[string]interface{} `json:"parameters"`
	Provider    string                 `json:"provider"`
	Timestamp   time.Time              `json:"timestamp"`
}

// ToolUnregister represents a tool unregistration request
type ToolUnregister struct {
	Name     string `json:"name"`
	Provider string `json:"provider"`
}

// ToolHeartbeat represents a provider heartbeat message
type ToolHeartbeat struct {
	Provider  string    `json:"provider"`
	Tools     []string  `json:"tools"`
	Timestamp time.Time `json:"timestamp"`
}

// ToolDefinition represents an external tool definition
type ToolDefinition struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	Provider    string `json:"provider"`
	Available   bool   `json:"available"`
}

// ToolListResponse represents the response to a tool.list request
type ToolListResponse struct {
	Tools []ToolDefinition `json:"tools"`
}

// externalTool represents internal storage of an external tool
type externalTool struct {
	name        string
	description string
	provider    string
	available   bool
	parameters  map[string]interface{}
	timestamp   time.Time
}

// ExternalToolRegistry manages external tool registrations
type ExternalToolRegistry struct {
	tools      map[string]*externalTool // Key: "provider:name"
	heartbeats map[string]time.Time     // Key: provider name
	mu         sync.RWMutex
}

// NewExternalToolRegistry creates a new external tool registry
func NewExternalToolRegistry() *ExternalToolRegistry {
	return &ExternalToolRegistry{
		tools:      make(map[string]*externalTool),
		heartbeats: make(map[string]time.Time),
	}
}

// RegisterExternalTool registers an external tool
// Returns error if same tool+provider already registered
func (r *ExternalToolRegistry) RegisterExternalTool(reg ExternalToolRegistration) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	key := r.toolKey(reg.Provider, reg.Name)

	// Check if tool already registered
	if _, exists := r.tools[key]; exists {
		return fmt.Errorf("tool %q from provider %q is already registered", reg.Name, reg.Provider)
	}

	// Register the tool with available=true initially
	r.tools[key] = &externalTool{
		name:        reg.Name,
		description: reg.Description,
		provider:    reg.Provider,
		available:   true,
		parameters:  reg.Parameters,
		timestamp:   reg.Timestamp,
	}

	return nil
}

// UnregisterTool removes a tool by name+provider
// Idempotent - no error if tool doesn't exist
func (r *ExternalToolRegistry) UnregisterTool(unreg ToolUnregister) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	key := r.toolKey(unreg.Provider, unreg.Name)
	delete(r.tools, key)

	return nil
}

// ListExternalTools returns all external tools
// Returns empty slice (not nil) when empty
func (r *ExternalToolRegistry) ListExternalTools() []ToolDefinition {
	r.mu.RLock()
	defer r.mu.RUnlock()

	// Initialize with empty slice to ensure non-nil return
	tools := []ToolDefinition{}

	for _, tool := range r.tools {
		tools = append(tools, ToolDefinition{
			Name:        tool.name,
			Description: tool.description,
			Provider:    tool.provider,
			Available:   tool.available,
		})
	}

	return tools
}

// RecordHeartbeat updates provider's last heartbeat timestamp
// Also restores availability for provider's tools
func (r *ExternalToolRegistry) RecordHeartbeat(hb ToolHeartbeat) {
	r.mu.Lock()
	defer r.mu.Unlock()

	// Update heartbeat timestamp
	r.heartbeats[hb.Provider] = hb.Timestamp

	// Restore availability for all tools from this provider
	for key, tool := range r.tools {
		if tool.provider == hb.Provider {
			r.tools[key].available = true
		}
	}
}

// IsProviderHealthy returns true if provider's last heartbeat is within threshold
func (r *ExternalToolRegistry) IsProviderHealthy(provider string, threshold time.Duration) bool {
	r.mu.RLock()
	defer r.mu.RUnlock()

	lastHeartbeat, exists := r.heartbeats[provider]
	if !exists {
		return false
	}

	return time.Since(lastHeartbeat) < threshold
}

// MarkProviderUnavailable marks all tools from provider as unavailable
func (r *ExternalToolRegistry) MarkProviderUnavailable(provider string) {
	r.mu.Lock()
	defer r.mu.Unlock()

	for key, tool := range r.tools {
		if tool.provider == provider {
			r.tools[key].available = false
		}
	}
}

// toolKey generates a unique key for a tool (provider:name)
func (r *ExternalToolRegistry) toolKey(provider, name string) string {
	return provider + ":" + name
}

// ConsumerNameForTool generates a JetStream consumer name for a tool
// Sanitizes dots and underscores to dashes, adds "tool-exec-" prefix
// Examples:
//
//	file_read → tool-exec-file-read
//	graph.query → tool-exec-graph-query
func ConsumerNameForTool(toolName string) string {
	// Replace dots and underscores with dashes
	sanitized := strings.ReplaceAll(toolName, ".", "-")
	sanitized = strings.ReplaceAll(sanitized, "_", "-")

	// Add prefix
	return "tool-exec-" + sanitized
}
