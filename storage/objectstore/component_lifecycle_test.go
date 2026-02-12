//go:build integration

package objectstore_test

import (
	"encoding/json"
	"testing"

	"github.com/c360studio/semstreams/component"
	"github.com/c360studio/semstreams/storage/objectstore"
	"github.com/stretchr/testify/require"
)

// createTestComponentForLifecycle creates a test instance for lifecycle testing.
// Uses the shared NATS client from store_integration_test.go TestMain.
func createTestComponentForLifecycle() component.LifecycleComponent {
	t := &testing.T{}

	// Use default config with unique bucket name for this test
	config := objectstore.DefaultConfig()
	config.BucketName = "LIFECYCLE_TEST_MESSAGES"
	configJSON, err := json.Marshal(config)
	require.NoError(t, err)

	// Use the shared NATS client from TestMain in store_integration_test.go
	deps := component.Dependencies{
		NATSClient: getSharedNATSClient(t),
	}

	comp, err := objectstore.NewComponent(configJSON, deps)
	require.NoError(t, err)

	return comp.(component.LifecycleComponent)
}

// TestObjectStore_ComprehensiveLifecycle runs the complete lifecycle test suite
func TestObjectStore_ComprehensiveLifecycle(t *testing.T) {
	component.StandardLifecycleTests(t, createTestComponentForLifecycle)
}
