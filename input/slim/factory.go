package slim

import (
	"fmt"

	"github.com/c360studio/semstreams/component"
	"github.com/c360studio/semstreams/pkg/errs"
)

// RegistryInterface defines the minimal interface needed for registration.
type RegistryInterface interface {
	RegisterWithConfig(component.RegistrationConfig) error
}

// Register registers the SLIM bridge input component with the given registry.
func Register(registry RegistryInterface) error {
	if registry == nil {
		return errs.WrapInvalid(fmt.Errorf("registry cannot be nil"), "Factory", "Register", "validate registry")
	}
	return registry.RegisterWithConfig(component.RegistrationConfig{
		Name:        "slim-bridge",
		Factory:     NewComponent,
		Schema:      componentSchema,
		Type:        "input",
		Protocol:    "slim",
		Domain:      "agntcy",
		Description: "Receives messages from SLIM groups using MLS encryption",
		Version:     "1.0.0",
	})
}
