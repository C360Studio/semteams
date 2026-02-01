package agenticmemory

import (
	"github.com/c360/semstreams/component"
)

// RegistryInterface defines the minimal interface needed for registration
type RegistryInterface interface {
	RegisterWithConfig(component.RegistrationConfig) error
}

// Register registers the agentic-memory processor component with the given registry
func Register(registry RegistryInterface) error {
	return registry.RegisterWithConfig(component.RegistrationConfig{
		Name:        "agentic-memory",
		Factory:     NewComponent,
		Schema:      agenticMemorySchema,
		Type:        "processor",
		Protocol:    "graph",
		Domain:      "agentic",
		Description: "Graph-backed agent memory with context hydration and fact extraction",
		Version:     "0.1.0",
	})
}
