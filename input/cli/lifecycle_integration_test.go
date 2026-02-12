//go:build integration

package cli

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

// createTestComponent creates a test instance for lifecycle testing.
func createTestComponent() component.LifecycleComponent {
	tc := sharedLifecycleNATSClient
	if tc == nil {
		panic("shared NATS client not initialized")
	}

	config := DefaultConfig()
	deps := component.Dependencies{
		NATSClient: tc.Client,
		Platform: component.PlatformMeta{
			Org:      "test",
			Platform: "test-platform",
		},
	}

	rawConfig, err := json.Marshal(config)
	if err != nil {
		panic("failed to marshal config: " + err.Error())
	}

	comp, err := NewComponent(rawConfig, deps)
	if err != nil {
		panic("failed to create test component: " + err.Error())
	}

	lifecycleComp, ok := comp.(component.LifecycleComponent)
	if !ok {
		panic("component does not implement LifecycleComponent")
	}

	return lifecycleComp
}

// TestCLIInput_ComprehensiveLifecycle runs the complete lifecycle test suite
func TestCLIInput_ComprehensiveLifecycle(t *testing.T) {
	component.StandardLifecycleTests(t, createTestComponent)
}
