//go:build integration

package jsonmapprocessor_test

import (
	"encoding/json"
	"testing"

	"github.com/c360studio/semstreams/component"
	jsonmapprocessor "github.com/c360studio/semstreams/processor/json_map"
)

// createTestJSONMapComponent creates a test instance for lifecycle testing.
// Uses the shared NATS client from json_map_integration_test.go TestMain.
func createTestJSONMapComponent() component.LifecycleComponent {
	config := jsonmapprocessor.DefaultConfig()
	deps := component.Dependencies{
		NATSClient: getSharedNATSClient(&testing.T{}),
	}

	configJSON, err := json.Marshal(config)
	if err != nil {
		panic("failed to marshal config: " + err.Error())
	}

	comp, err := jsonmapprocessor.NewProcessor(configJSON, deps)
	if err != nil {
		panic("failed to create component: " + err.Error())
	}

	return comp.(component.LifecycleComponent)
}

// TestJSONMap_ComprehensiveLifecycle runs the complete lifecycle test suite
func TestJSONMap_ComprehensiveLifecycle(t *testing.T) {
	component.StandardLifecycleTests(t, createTestJSONMapComponent)
}
