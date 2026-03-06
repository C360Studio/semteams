//go:build integration

package federation_test

import (
	"encoding/json"
	"testing"

	"github.com/c360studio/semstreams/component"
	proc "github.com/c360studio/semstreams/processor/federation"
)

// createTestComponentForLifecycle creates a test instance for lifecycle testing.
// Uses the shared NATS client from federation_integration_test.go TestMain.
func createTestComponentForLifecycle() component.LifecycleComponent {
	tc := sharedTestClient
	if tc == nil {
		panic("shared NATS client not initialized - TestMain from federation_integration_test.go must run first")
	}

	config := proc.DefaultConfig()
	deps := component.Dependencies{
		NATSClient: tc.Client,
	}

	rawConfig, err := json.Marshal(config)
	if err != nil {
		panic("failed to marshal config: " + err.Error())
	}

	discoverable, err := proc.NewComponent(rawConfig, deps)
	if err != nil {
		panic("failed to create component: " + err.Error())
	}

	comp, ok := discoverable.(component.LifecycleComponent)
	if !ok {
		panic("component does not implement LifecycleComponent")
	}

	return comp
}

// TestFederationProcessor_ComprehensiveLifecycle runs the complete lifecycle test suite.
func TestFederationProcessor_ComprehensiveLifecycle(t *testing.T) {
	component.StandardLifecycleTests(t, createTestComponentForLifecycle)
}
