package cli

import (
	"github.com/c360/semstreams/component"
)

// Register registers the CLI input component factory with the registry
func Register(registry *component.Registry) error {
	if registry == nil {
		panic("registry cannot be nil")
	}

	schema := buildConfigSchema()

	registration := &component.Registration{
		Name:        "cli-input",
		Type:        "input",
		Protocol:    "nats",
		Domain:      "agentic",
		Description: "CLI input for interactive user sessions with Ctrl+C signal support",
		Version:     "1.0.0",
		Schema:      schema,
		Factory:     NewComponent,
	}

	return registry.RegisterFactory("cli-input", registration)
}

// buildConfigSchema builds the configuration schema
func buildConfigSchema() component.ConfigSchema {
	return component.ConfigSchema{
		Properties: map[string]component.PropertySchema{
			"user_id": {
				Type:        "string",
				Description: "User ID for the CLI session",
				Default:     "cli-user",
			},
			"session_id": {
				Type:        "string",
				Description: "Session ID for the CLI session",
				Default:     "cli-session",
			},
			"prompt": {
				Type:        "string",
				Description: "Command line prompt string",
				Default:     "> ",
			},
			"stream_name": {
				Type:        "string",
				Description: "NATS stream name for user messages",
				Default:     "USER",
			},
		},
		Required: []string{},
	}
}

// buildDefaultInputPorts returns the default input ports
func buildDefaultInputPorts() []component.Port {
	defaults := DefaultConfig()
	ports := make([]component.Port, 0, len(defaults.Ports.Inputs))

	for _, def := range defaults.Ports.Inputs {
		ports = append(ports, component.BuildPortFromDefinition(def, component.DirectionInput))
	}

	return ports
}

// buildDefaultOutputPorts returns the default output ports
func buildDefaultOutputPorts() []component.Port {
	defaults := DefaultConfig()
	ports := make([]component.Port, 0, len(defaults.Ports.Outputs))

	for _, def := range defaults.Ports.Outputs {
		ports = append(ports, component.BuildPortFromDefinition(def, component.DirectionOutput))
	}

	return ports
}
