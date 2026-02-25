package reactive

import (
	"github.com/c360studio/semstreams/component"
)

// Register registers the reactive workflow engine component factory with the registry.
func Register(registry *component.Registry) error {
	if registry == nil {
		panic("registry cannot be nil")
	}

	registration := &component.Registration{
		Name:        "reactive-workflow",
		Type:        "processor",
		Protocol:    "nats",
		Domain:      "agentic",
		Description: "Reactive workflow engine using KV watch and subject triggers with typed Go conditions and actions",
		Version:     "1.0.0",
		Schema:      schema,
		Factory:     NewComponent,
	}

	return registry.RegisterFactory("reactive-workflow", registration)
}
