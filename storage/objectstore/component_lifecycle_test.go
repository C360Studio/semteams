package objectstore

import (
	"encoding/json"
	"testing"

	"github.com/c360studio/semstreams/component"
	"github.com/c360studio/semstreams/natsclient"
	"github.com/stretchr/testify/require"
)

// createTestComponentForLifecycle creates a test instance for lifecycle testing.
func createTestComponentForLifecycle() component.LifecycleComponent {
	t := &testing.T{}

	// Use default config with standard ports
	config := DefaultConfig()
	configJSON, err := json.Marshal(config)
	require.NoError(t, err)

	// Create unconnected NATS client (won't actually connect)
	natsClient, err := natsclient.NewClient("nats://localhost:4222")
	require.NoError(t, err)

	deps := component.Dependencies{
		NATSClient: natsClient,
	}

	comp, err := NewComponent(configJSON, deps)
	require.NoError(t, err)

	return comp.(component.LifecycleComponent)
}

// TestObjectStore_ComprehensiveLifecycle runs the complete lifecycle test suite
func TestObjectStore_ComprehensiveLifecycle(t *testing.T) {
	component.StandardLifecycleTests(t, createTestComponentForLifecycle)
}
