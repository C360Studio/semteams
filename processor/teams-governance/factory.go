package teamsgovernance

import (
	"github.com/c360studio/semstreams/component"
)

// RegistryInterface defines the minimal interface needed for registration
type RegistryInterface interface {
	RegisterWithConfig(component.RegistrationConfig) error
}

// Register registers the agentic-governance processor component with the given registry
func Register(registry RegistryInterface) error {
	return registry.RegisterWithConfig(component.RegistrationConfig{
		Name:        "teams-governance",
		Factory:     NewComponent,
		Schema:      agenticGovernanceSchema,
		Type:        "processor",
		Protocol:    "agentic",
		Domain:      "governance",
		Description: "Content governance layer for agentic systems with PII redaction, injection detection, and rate limiting",
		Version:     "0.1.0",
	})
}
