package graphclustering

import (
	"encoding/json"
	"testing"

	"github.com/c360studio/semstreams/component"
	"github.com/c360studio/semstreams/natsclient"
)

// createTestGraphClusteringComponent creates a test instance for lifecycle testing.
func createTestGraphClusteringComponent() component.LifecycleComponent {
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

	comp, err := CreateGraphClustering(configJSON, deps)
	if err != nil {
		panic("failed to create component: " + err.Error())
	}

	return comp.(component.LifecycleComponent)
}

// TestGraphClustering_ComprehensiveLifecycle runs the complete lifecycle test suite
func TestGraphClustering_ComprehensiveLifecycle(t *testing.T) {
	component.StandardLifecycleTests(t, createTestGraphClusteringComponent)
}
