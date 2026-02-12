//go:build integration

package graphgateway

import (
	"encoding/json"
	"os"
	"testing"

	"github.com/c360studio/semstreams/component"
	"github.com/c360studio/semstreams/natsclient"
)

var sharedLifecycleNATSClient *natsclient.TestClient

func TestMain(m *testing.M) {
	t := &testing.T{}
	sharedLifecycleNATSClient = natsclient.NewTestClient(t, natsclient.WithKV())
	code := m.Run()
	if sharedLifecycleNATSClient != nil {
		sharedLifecycleNATSClient.Terminate()
	}
	os.Exit(code)
}

// createTestComponentForLifecycle creates a test instance for lifecycle testing.
func createTestComponentForLifecycle() component.LifecycleComponent {
	tc := sharedLifecycleNATSClient
	if tc == nil {
		panic("shared NATS client not initialized")
	}

	config := DefaultConfig()
	deps := component.Dependencies{
		NATSClient: tc.Client,
	}

	configJSON, err := json.Marshal(config)
	if err != nil {
		panic("failed to marshal config: " + err.Error())
	}

	comp, err := CreateGraphGateway(configJSON, deps)
	if err != nil {
		panic("failed to create component: " + err.Error())
	}

	return comp.(component.LifecycleComponent)
}

// TestGraphGateway_ComprehensiveLifecycle runs the complete lifecycle test suite
func TestGraphGateway_ComprehensiveLifecycle(t *testing.T) {
	component.StandardLifecycleTests(t, createTestComponentForLifecycle)
}
