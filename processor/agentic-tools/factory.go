package agentictools

import (
	"fmt"

	"github.com/c360studio/semstreams/component"
)

// RegistryInterface defines the minimal interface needed for registration
type RegistryInterface interface {
	RegisterWithConfig(component.RegistrationConfig) error
}

// Register registers the agentic-tools processor component with the given registry
func Register(registry RegistryInterface) error {
	if registry == nil {
		return fmt.Errorf("registry cannot be nil")
	}
	return registry.RegisterWithConfig(component.RegistrationConfig{
		Name:        "agentic-tools",
		Factory:     NewComponent,
		Schema:      agenticToolsSchema,
		Type:        "processor",
		Protocol:    "tools",
		Domain:      "semantic",
		Description: "Tool executor processor with filtering and timeout support",
		Version:     "0.1.0",
	})
}
