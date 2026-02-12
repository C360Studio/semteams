//go:build integration

package jsongeneric_test

import (
	"encoding/json"
	"testing"

	"github.com/c360studio/semstreams/component"
	jsongeneric "github.com/c360studio/semstreams/processor/json_generic"
)

// createTestJSONGenericComponent creates a test instance for lifecycle testing.
// Uses the shared NATS client from json_generic_integration_test.go TestMain.
func createTestJSONGenericComponent() component.LifecycleComponent {
	config := jsongeneric.DefaultConfig()
	deps := component.Dependencies{
		NATSClient: getSharedNATSClient(&testing.T{}),
	}

	configJSON, err := json.Marshal(config)
	if err != nil {
		panic("failed to marshal config: " + err.Error())
	}

	comp, err := jsongeneric.NewProcessor(configJSON, deps)
	if err != nil {
		panic("failed to create component: " + err.Error())
	}

	return comp.(component.LifecycleComponent)
}

// TestJSONGeneric_ComprehensiveLifecycle runs the complete lifecycle test suite
func TestJSONGeneric_ComprehensiveLifecycle(t *testing.T) {
	component.StandardLifecycleTests(t, createTestJSONGenericComponent)
}
