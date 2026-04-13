//go:build integration

package teamsmodel_test

import (
	"encoding/json"
	"testing"

	"github.com/c360studio/semstreams/component"
	"github.com/c360studio/semstreams/model"
	teamsmodel "github.com/c360studio/semteams/processor/teams-model"
)

// createTestComponentForLifecycle creates a test instance for lifecycle testing.
// Uses the shared NATS client from model_integration_test.go TestMain.
func createTestComponentForLifecycle() component.LifecycleComponent {
	if sharedNATSClient == nil {
		panic("shared NATS client not initialized")
	}

	config := teamsmodel.DefaultConfig()
	// Use unique consumer suffix and delete on stop for test isolation
	config.ConsumerNameSuffix = "lifecycle"
	config.DeleteConsumerOnStop = true

	registry := &model.Registry{
		Endpoints: map[string]*model.EndpointConfig{
			"default": {
				URL:       "http://localhost:8080/v1",
				Model:     "gpt-4",
				MaxTokens: 128000,
			},
		},
	}

	deps := component.Dependencies{
		NATSClient:    sharedNATSClient,
		ModelRegistry: registry,
	}

	rawConfig, err := json.Marshal(config)
	if err != nil {
		panic("failed to marshal config: " + err.Error())
	}

	discoverable, err := teamsmodel.NewComponent(rawConfig, deps)
	if err != nil {
		panic("failed to create component: " + err.Error())
	}

	comp, ok := discoverable.(component.LifecycleComponent)
	if !ok {
		panic("component does not implement LifecycleComponent")
	}

	return comp
}

// TestAgenticModel_ComprehensiveLifecycle runs the complete lifecycle test suite
func TestAgenticModel_ComprehensiveLifecycle(t *testing.T) {
	component.StandardLifecycleTests(t, createTestComponentForLifecycle)
}
