package graphindex

import (
	"testing"

	"github.com/c360studio/semstreams/component"
)

// createTestComponentForLifecycle creates a test instance for lifecycle testing.
func createTestComponentForLifecycle() component.LifecycleComponent {
	// Use the existing createTestComponent helper which creates a component
	// with unconnected NATS client - suitable for lifecycle testing.
	// Note: This component requires NATS connectivity to start successfully.
	// The StandardLifecycleTests will reveal context handling and nil-safety issues
	// that should be addressed in the component implementation.
	comp := createTestComponent(&testing.T{})
	return comp
}

// TestGraphIndex_ComprehensiveLifecycle runs the complete lifecycle test suite.
// NOTE: Some tests may fail because graph-index requires actual NATS connectivity
// to start. This reveals legitimate issues:
// - The component should handle context cancellation more gracefully
// - The component should check for nil context before calling context.WithCancel
// - The component should validate prerequisites before attempting NATS connection
func TestGraphIndex_ComprehensiveLifecycle(t *testing.T) {
	component.StandardLifecycleTests(t, createTestComponentForLifecycle)
}
