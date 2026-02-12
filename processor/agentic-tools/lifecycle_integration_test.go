//go:build integration

package agentictools_test

import (
	"encoding/json"
	"testing"

	"github.com/c360studio/semstreams/component"
	agentictools "github.com/c360studio/semstreams/processor/agentic-tools"
)

// createTestComponentForLifecycle creates a test instance for lifecycle testing.
// Uses the shared NATS client from tools_integration_test.go TestMain.
func createTestComponentForLifecycle() component.LifecycleComponent {
	if sharedNATSClient == nil {
		panic("shared NATS client not initialized")
	}

	config := agentictools.DefaultConfig()
	deps := component.Dependencies{
		NATSClient: sharedNATSClient,
	}

	rawConfig, err := json.Marshal(config)
	if err != nil {
		panic("failed to marshal config: " + err.Error())
	}

	discoverable, err := agentictools.NewComponent(rawConfig, deps)
	if err != nil {
		panic("failed to create component: " + err.Error())
	}

	comp, ok := discoverable.(component.LifecycleComponent)
	if !ok {
		panic("component does not implement LifecycleComponent")
	}

	return comp
}

// TestAgenticTools_ComprehensiveLifecycle runs the complete lifecycle test suite
func TestAgenticTools_ComprehensiveLifecycle(t *testing.T) {
	component.StandardLifecycleTests(t, createTestComponentForLifecycle)
}
