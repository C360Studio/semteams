//go:build integration

package agenticloop_test

import (
	"encoding/json"
	"os"
	"testing"
	"time"

	"github.com/c360studio/semstreams/component"
	"github.com/c360studio/semstreams/natsclient"
	agenticloop "github.com/c360studio/semstreams/processor/agentic-loop"
)

var sharedLifecycleNATSClient *natsclient.TestClient

func TestMain(m *testing.M) {
	t := &testing.T{}
	streams := []natsclient.TestStreamConfig{
		{Name: "AGENT", Subjects: []string{"agent.>", "tool.>"}},
	}
	sharedLifecycleNATSClient = natsclient.NewTestClient(t,
		natsclient.WithJetStream(),
		natsclient.WithKV(),
		natsclient.WithKVBuckets("AGENT_LOOPS", "AGENT_TRAJECTORIES"),
		natsclient.WithStreams(streams...),
	)
	code := m.Run()
	if sharedLifecycleNATSClient != nil {
		sharedLifecycleNATSClient.Terminate()
	}
	os.Exit(code)
}

// createTestComponentForLifecycle creates a test instance for lifecycle testing.
// Uses the shared NATS client from loop_integration_test.go TestMain.
func createTestComponentForLifecycle() component.LifecycleComponent {
	tc := sharedLifecycleNATSClient
	if tc == nil {
		panic("shared NATS client not initialized")
	}

	config := agenticloop.DefaultConfig()
	deps := component.Dependencies{
		NATSClient: tc.Client,
	}

	rawConfig, err := json.Marshal(config)
	if err != nil {
		panic("failed to marshal config: " + err.Error())
	}

	discoverable, err := agenticloop.NewComponent(rawConfig, deps)
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
