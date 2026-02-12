//go:build integration

package agenticmodel_test

import (
	"encoding/json"
	"testing"

	"github.com/c360studio/semstreams/component"
	agenticmodel "github.com/c360studio/semstreams/processor/agentic-model"
)

// createTestComponentForLifecycle creates a test instance for lifecycle testing.
// Uses the shared NATS client from model_integration_test.go TestMain.
func createTestComponentForLifecycle() component.LifecycleComponent {
	if sharedNATSClient == nil {
		panic("shared NATS client not initialized")
	}

	config := agenticmodel.DefaultConfig()
	// Add required endpoint configuration
	config.Endpoints = map[string]agenticmodel.Endpoint{
		"default": {
			URL:   "http://localhost:8080/v1",
			Model: "gpt-4",
		},
	}
	deps := component.Dependencies{
		NATSClient: sharedNATSClient,
	}

	rawConfig, err := json.Marshal(config)
	if err != nil {
		panic("failed to marshal config: " + err.Error())
	}

	discoverable, err := agenticmodel.NewComponent(rawConfig, deps)
	if err != nil {
		panic("failed to create component: " + err.Error())
	}

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
