package agenticdispatch

import (
	"github.com/c360/semstreams/component"
)

// Register registers the router component factory with the registry
func Register(registry *component.Registry) error {
	if registry == nil {
		panic("registry cannot be nil")
	}

	schema := buildConfigSchema()

	registration := &component.Registration{
		Name:        "router",
		Type:        "processor",
		Protocol:    "nats",
		Domain:      "agentic",
		Description: "Routes user messages to agentic loops with command parsing and permissions",
		Version:     "1.0.0",
		Schema:      schema,
		Factory:     NewComponent,
	}

	return registry.RegisterFactory("router", registration)
}

// buildConfigSchema builds the configuration schema for the router
func buildConfigSchema() component.ConfigSchema {
	return component.ConfigSchema{
		Properties: map[string]component.PropertySchema{
			"default_role": {
				Type:        "string",
				Description: "Default role for new tasks",
				Default:     "general",
			},
			"default_model": {
				Type:        "string",
				Description: "Default model for new tasks",
				Default:     "qwen2.5-coder:32b",
			},
			"auto_continue": {
				Type:        "boolean",
				Description: "Automatically continue last active loop",
				Default:     true,
			},
			"stream_name": {
				Type:        "string",
				Description: "NATS stream name for user messages",
				Default:     "USER",
			},
			"permissions": {
				Type:        "object",
				Description: "Permission configuration",
			},
		},
		Required: []string{},
	}
}

// buildDefaultInputPorts returns the default input ports
func buildDefaultInputPorts() []component.Port {
	defaultConfig := DefaultConfig()
	ports := make([]component.Port, 0, len(defaultConfig.Ports.Inputs))

	for _, def := range defaultConfig.Ports.Inputs {
		ports = append(ports, component.BuildPortFromDefinition(def, component.DirectionInput))
	}

	return ports
}

// buildDefaultOutputPorts returns the default output ports
func buildDefaultOutputPorts() []component.Port {
	defaultConfig := DefaultConfig()
	ports := make([]component.Port, 0, len(defaultConfig.Ports.Outputs))

	for _, def := range defaultConfig.Ports.Outputs {
		ports = append(ports, component.BuildPortFromDefinition(def, component.DirectionOutput))
	}

	return ports
}
