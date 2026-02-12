package http

import (
	"encoding/json"
	"testing"

	"github.com/c360studio/semstreams/component"
	"github.com/c360studio/semstreams/gateway"
	"github.com/c360studio/semstreams/natsclient"
	"github.com/stretchr/testify/require"
)

// createTestComponentForLifecycle creates a test instance for lifecycle testing.
func createTestComponentForLifecycle() component.LifecycleComponent {
	t := &testing.T{}

	// Create a minimal valid configuration
	config := gateway.Config{
		Routes: []gateway.RouteMapping{
			{
				Path:        "/test",
				Method:      "POST",
				NATSSubject: "test.subject",
			},
		},
		EnableCORS:     false,
		CORSOrigins:    []string{},
		MaxRequestSize: 1024 * 1024,
	}

	configJSON, err := json.Marshal(config)
	require.NoError(t, err)

	// Create unconnected NATS client (won't actually connect)
	natsClient, err := natsclient.NewClient("nats://localhost:4222")
	require.NoError(t, err)

	deps := component.Dependencies{
		NATSClient: natsClient,
	}

	comp, err := NewGateway(configJSON, deps)
	require.NoError(t, err)

	return comp.(component.LifecycleComponent)
}

// TestHTTPGateway_ComprehensiveLifecycle runs the complete lifecycle test suite
func TestHTTPGateway_ComprehensiveLifecycle(t *testing.T) {
	component.StandardLifecycleTests(t, createTestComponentForLifecycle)
}
