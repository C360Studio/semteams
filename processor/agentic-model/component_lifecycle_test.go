package agenticmodel_test

import (
	"encoding/json"
	"testing"

	"github.com/c360studio/semstreams/component"
	agenticmodel "github.com/c360studio/semstreams/processor/agentic-model"
)

// createTestComponentForLifecycle creates a test instance for lifecycle testing.
func createTestComponentForLifecycle() component.LifecycleComponent {
	config := agenticmodel.DefaultConfig()
	// Add required endpoint configuration
	config.Endpoints = map[string]agenticmodel.Endpoint{
		"default": {
			URL:   "http://localhost:8080/v1",
			Model: "gpt-4",
		},
	}

	rawConfig, err := json.Marshal(config)
	if err != nil {
		panic("failed to marshal config: " + err.Error())
	}

	deps := component.Dependencies{
		NATSClient: nil, // Lifecycle tests don't require real NATS
	}

	discoverable, err := agenticmodel.NewComponent(rawConfig, deps)
	if err != nil {
		panic("failed to create component: " + err.Error())
	}

	// Type assert to concrete Component type which implements LifecycleComponent
	comp, ok := discoverable.(component.LifecycleComponent)
	if !ok {
		panic("component does not implement LifecycleComponent")
	}

	return comp
}

// TestAgenticModel_ComprehensiveLifecycle runs the complete lifecycle test suite
func TestAgenticModel_ComprehensiveLifecycle(t *testing.T) {
	component.StandardLifecycleTests(t, createTestComponentForLifecycle)
}
