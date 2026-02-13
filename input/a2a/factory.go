package a2a

import (
	"fmt"

	"github.com/c360studio/semstreams/component"
	"github.com/c360studio/semstreams/pkg/errs"
)

// RegistryInterface defines the minimal interface needed for registration.
type RegistryInterface interface {
	RegisterWithConfig(component.RegistrationConfig) error
}

// Register registers the A2A adapter input component with the given registry.
func Register(registry RegistryInterface) error {
	if registry == nil {
		return errs.WrapInvalid(fmt.Errorf("registry cannot be nil"), "Factory", "Register", "validate registry")
	}
	return registry.RegisterWithConfig(component.RegistrationConfig{
		Name:        "a2a-adapter",
		Factory:     NewComponent,
		Schema:      componentSchema,
		Type:        "input",
		Protocol:    "a2a",
		Domain:      "agntcy",
		Description: "Receives A2A task requests from external agents",
		Version:     "1.0.0",
	})
}
