package websocket

import (
	"testing"

	"github.com/c360studio/semstreams/component"
	"github.com/c360studio/semstreams/natsclient"
	"github.com/c360studio/semstreams/pkg/security"
)

// createTestComponent creates a test instance for lifecycle testing.
func createTestComponent() component.LifecycleComponent {
	mockClient := &natsclient.Client{}
	config := DefaultConfig()
	config.Mode = ModeServer

	input, err := NewInput(
		"websocket-input-test",
		mockClient,
		config,
		nil, // metrics registry
		security.Config{},
	)
	if err != nil {
		panic("failed to create test component: " + err.Error())
	}

	return input
}

// TestWebSocketInput_ComprehensiveLifecycle runs the complete lifecycle test suite
func TestWebSocketInput_ComprehensiveLifecycle(t *testing.T) {
	component.StandardLifecycleTests(t, createTestComponent)
}
