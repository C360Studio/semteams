package graphindextemporal

import (
	"encoding/json"
	"testing"

	"github.com/c360studio/semstreams/component"
	"github.com/c360studio/semstreams/natsclient"
)

// createTestGraphIndexTemporalComponent creates a test instance for lifecycle testing.
func createTestGraphIndexTemporalComponent() component.LifecycleComponent {
	// Create unconnected NATS client (won't actually connect)
	natsClient, err := natsclient.NewClient("nats://localhost:4222")
	if err != nil {
		panic("failed to create NATS client: " + err.Error())
	}

	config := DefaultConfig()
	deps := component.Dependencies{
		NATSClient: natsClient,
	}

	configJSON, err := json.Marshal(config)
	if err != nil {
		panic("failed to marshal config: " + err.Error())
	}

	comp, err := CreateGraphIndexTemporal(configJSON, deps)
	if err != nil {
		panic("failed to create component: " + err.Error())
	}

	return comp.(component.LifecycleComponent)
}

// TestGraphIndexTemporal_ComprehensiveLifecycle runs the complete lifecycle test suite
func TestGraphIndexTemporal_ComprehensiveLifecycle(t *testing.T) {
	component.StandardLifecycleTests(t, createTestGraphIndexTemporalComponent)
}
