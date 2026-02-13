package oasfgenerator

import (
	"fmt"

	"github.com/c360studio/semstreams/component"
	"github.com/c360studio/semstreams/pkg/errs"
)

// RegistryInterface defines the minimal interface needed for registration.
type RegistryInterface interface {
	RegisterWithConfig(component.RegistrationConfig) error
}

// Register registers the OASF generator processor component with the given registry.
func Register(registry RegistryInterface) error {
	if registry == nil {
		return errs.WrapInvalid(fmt.Errorf("registry cannot be nil"), "Factory", "Register", "validate registry")
	}
	return registry.RegisterWithConfig(component.RegistrationConfig{
		Name:        "oasf-generator",
		Factory:     NewComponent,
		Schema:      componentSchema,
		Type:        "processor",
		Protocol:    "oasf",
		Domain:      "agntcy",
		Description: "Generates OASF records from agent entity capabilities for AGNTCY directory registration",
		Version:     "1.0.0",
	})
}
