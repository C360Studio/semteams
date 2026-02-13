package directorybridge

import (
	"fmt"

	"github.com/c360studio/semstreams/component"
	"github.com/c360studio/semstreams/pkg/errs"
)

// RegistryInterface defines the minimal interface needed for registration.
type RegistryInterface interface {
	RegisterWithConfig(component.RegistrationConfig) error
}

// Register registers the directory bridge output component with the given registry.
func Register(registry RegistryInterface) error {
	if registry == nil {
		return errs.WrapInvalid(fmt.Errorf("registry cannot be nil"), "Factory", "Register", "validate registry")
	}
	return registry.RegisterWithConfig(component.RegistrationConfig{
		Name:        "directory-bridge",
		Factory:     NewComponent,
		Schema:      componentSchema,
		Type:        "output",
		Protocol:    "agntcy",
		Domain:      "agntcy",
		Description: "Registers agents with AGNTCY directories using OASF records",
		Version:     "1.0.0",
	})
}
