//go:build integration

package graphgateway

import (
	"testing"

	"github.com/c360studio/semstreams/component"
)

// createTestComponentForLifecycle creates a test instance for lifecycle testing.
func createTestComponentForLifecycle() component.LifecycleComponent {
	// Use the existing createTestComponent helper which creates a component
	// with unconnected NATS client - suitable for lifecycle testing
	comp := createTestComponent(&testing.T{})
	return comp
}

// TestGraphGateway_ComprehensiveLifecycle runs the complete lifecycle test suite
func TestGraphGateway_ComprehensiveLifecycle(t *testing.T) {
	component.StandardLifecycleTests(t, createTestComponentForLifecycle)
}
