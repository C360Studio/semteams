//go:build integration

package teamsloop_test

import (
	"encoding/json"
	"testing"

	"github.com/c360studio/semstreams/component"
	teamsloop "github.com/c360studio/semteams/processor/teams-loop"
)

// createTestComponentForLifecycle creates a test instance for lifecycle testing.
// Uses the shared NATS client from loop_integration_test.go TestMain.
func createTestComponentForLifecycle() component.LifecycleComponent {
	tc := sharedTestClient
	if tc == nil {
		panic("shared NATS client not initialized - TestMain from loop_integration_test.go must run first")
	}

	config := teamsloop.DefaultConfig()
	deps := component.Dependencies{
		NATSClient: tc.Client,
	}

	rawConfig, err := json.Marshal(config)
	if err != nil {
		panic("failed to marshal config: " + err.Error())
	}

	discoverable, err := teamsloop.NewComponent(rawConfig, deps)
	if err != nil {
		panic("failed to create component: " + err.Error())
	}

	comp, ok := discoverable.(component.LifecycleComponent)
	if !ok {
		panic("component does not implement LifecycleComponent")
	}

	return comp
}

// TestAgenticLoop_ComprehensiveLifecycle runs the complete lifecycle test suite
func TestAgenticLoop_ComprehensiveLifecycle(t *testing.T) {
	component.StandardLifecycleTests(t, createTestComponentForLifecycle)
}
