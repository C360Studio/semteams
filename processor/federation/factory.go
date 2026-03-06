package federation

import (
	"fmt"

	"github.com/c360studio/semstreams/component"
)

// RegistryInterface is the minimal interface required for component registration.
// The concrete *component.Registry satisfies this interface.
type RegistryInterface interface {
	RegisterWithConfig(component.RegistrationConfig) error
}

// Register registers the federation processor component with the given registry.
// Returns an error if registry is nil or registration fails.
func Register(registry RegistryInterface) error {
	if registry == nil {
		return fmt.Errorf("federation: registry cannot be nil")
	}
	return registry.RegisterWithConfig(component.RegistrationConfig{
		Name:        "federation-processor",
		Factory:     NewComponent,
		Schema:      federationSchema,
		Type:        "processor",
		Protocol:    "nats-jetstream",
		Domain:      "federation",
		Description: "Applies federation merge policy to incoming graph event payloads",
		Version:     "1.0.0",
	})
}
