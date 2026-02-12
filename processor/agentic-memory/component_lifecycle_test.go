//go:build integration

package agenticmemory_test

import (
	"encoding/json"
	"testing"

	"github.com/c360studio/semstreams/component"
	agenticmemory "github.com/c360studio/semstreams/processor/agentic-memory"
)

// createTestComponentForLifecycle creates a test instance for lifecycle testing.
func createTestComponentForLifecycle() component.LifecycleComponent {
	config := agenticmemory.DefaultConfig()

	rawConfig, err := json.Marshal(config)
	if err != nil {
		panic("failed to marshal config: " + err.Error())
	}

	deps := component.Dependencies{
		NATSClient: nil, // Lifecycle tests don't require real NATS
	}

	discoverable, err := agenticmemory.NewComponent(rawConfig, deps)
	if err != nil {
		panic("failed to create component: " + err.Error())
	}

	// Type assert to concrete Component type which implements LifecycleComponent
	comp, ok := discoverable.(component.LifecycleComponent)
	if !ok {
		panic("component does not implement LifecycleComponent")
	}

	return comp
}

// TestAgenticMemory_ComprehensiveLifecycle runs the complete lifecycle test suite
func TestAgenticMemory_ComprehensiveLifecycle(t *testing.T) {
	component.StandardLifecycleTests(t, createTestComponentForLifecycle)
}
