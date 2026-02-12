package udp

import (
	"testing"

	"github.com/c360studio/semstreams/component"
	"github.com/c360studio/semstreams/natsclient"
)

// createTestComponent creates a test instance for lifecycle testing.
func createTestComponent() component.LifecycleComponent {
	// Find an available port for testing
	port := findAvailablePort(&testing.T{})

	mockClient := &natsclient.Client{}
	deps := InputDeps{
		Config:          testUDPConfig(port, "127.0.0.1", "test.subject"),
		NATSClient:      mockClient,
		MetricsRegistry: nil,
		Logger:          nil,
	}

	input, err := NewInput(deps)
	if err != nil {
		panic("failed to create test component: " + err.Error())
	}

	return input
}

// TestUDPInput_ComprehensiveLifecycle runs the complete lifecycle test suite
func TestUDPInput_ComprehensiveLifecycle(t *testing.T) {
	component.StandardLifecycleTests(t, createTestComponent)
}
