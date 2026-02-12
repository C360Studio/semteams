package httppost

import (
	"encoding/json"
	"testing"

	"github.com/c360studio/semstreams/component"
	"github.com/c360studio/semstreams/natsclient"
)

// createTestComponent creates a test instance for lifecycle testing.
func createTestComponent() component.LifecycleComponent {
	mockClient := &natsclient.Client{}

	config := Config{
		URL:         "http://localhost:8080/test",
		Headers:     map[string]string{"X-Test": "value"},
		Timeout:     30,
		RetryCount:  3,
		ContentType: "application/json",
		Ports: &component.PortConfig{
			Inputs: []component.PortDefinition{
				{
					Name:     "nats_input",
					Type:     "nats",
					Subject:  "test.httppost.output",
					Required: true,
				},
			},
		},
	}

	rawConfig, err := json.Marshal(config)
	if err != nil {
		panic("failed to marshal config: " + err.Error())
	}

	deps := component.Dependencies{
		NATSClient: mockClient,
		Platform: component.PlatformMeta{
			Org:      "test",
			Platform: "test-platform",
		},
	}

	output, err := NewOutput(rawConfig, deps)
	if err != nil {
		panic("failed to create test component: " + err.Error())
	}

	lifecycleComp, ok := output.(component.LifecycleComponent)
	if !ok {
		panic("component does not implement LifecycleComponent")
	}

	return lifecycleComp
}

// TestHTTPPostOutput_ComprehensiveLifecycle runs the complete lifecycle test suite
func TestHTTPPostOutput_ComprehensiveLifecycle(t *testing.T) {
	component.StandardLifecycleTests(t, createTestComponent)
}
