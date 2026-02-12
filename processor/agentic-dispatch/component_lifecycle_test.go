//go:build integration

package agenticdispatch

import (
	"encoding/json"
	"testing"

	"github.com/c360studio/semstreams/component"
)

// createTestComponentForLifecycle creates a test instance for lifecycle testing.
func createTestComponentForLifecycle() component.LifecycleComponent {
	config := DefaultConfig()

	rawConfig, err := json.Marshal(config)
	if err != nil {
		panic("failed to marshal config: " + err.Error())
	}

	deps := component.Dependencies{
		NATSClient: nil, // Lifecycle tests don't require real NATS
	}

	discoverable, err := NewComponent(rawConfig, deps)
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

// TestAgenticDispatch_ComprehensiveLifecycle runs the complete lifecycle test suite
func TestAgenticDispatch_ComprehensiveLifecycle(t *testing.T) {
	component.StandardLifecycleTests(t, createTestComponentForLifecycle)
}
