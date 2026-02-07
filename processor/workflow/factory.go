package workflow

import (
	"github.com/c360studio/semstreams/component"
)

// Register registers the workflow processor component factory with the registry
func Register(registry *component.Registry) error {
	if registry == nil {
		panic("registry cannot be nil")
	}

	registration := &component.Registration{
		Name:        "workflow-processor",
		Type:        "processor",
		Protocol:    "nats",
		Domain:      "agentic",
		Description: "Orchestrates multi-step agentic workflows with loops, limits, and timeouts",
		Version:     "1.0.0",
		Schema:      schema,
		Factory:     NewComponent,
	}

	return registry.RegisterFactory("workflow-processor", registration)
}
