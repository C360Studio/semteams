package mcp

import (
	"github.com/c360/semstreams/component"
)

// Register registers the MCP gateway with the component registry.
func Register(registry *component.Registry) error {
	return registry.RegisterWithConfig(component.RegistrationConfig{
		Name:        "mcp",
		Factory:     NewMCPGateway,
		Schema:      mcpGatewaySchema,
		Type:        "gateway",
		Protocol:    "mcp",
		Domain:      "network",
		Description: "MCP gateway for AI agent integration via GraphQL",
		Version:     "0.1.0",
	})
}
