//go:build integration

package component

import (
	"log"
	"testing"

	"github.com/c360studio/semstreams/natsclient"
	"github.com/nats-io/nats.go"
)

var (
	sharedNATSClient *nats.Conn
)

func TestMain(m *testing.M) {
	// Build tag ensures this only runs with -tags=integration
	testClient, err := natsclient.NewSharedTestClient(
		natsclient.WithJetStream(),
	)
	if err != nil {
		log.Fatalf("Failed to create shared test client: %v", err)
	}

	sharedNATSClient = testClient.Client.GetConnection()

	// Run tests
	exitCode := m.Run()

	// Cleanup
	testClient.Terminate()

	if exitCode != 0 {
		log.Fatal("tests failed")
	}
}

// getSharedNATSClient returns the shared NATS connection for integration tests
func getSharedNATSClient(t *testing.T) *nats.Conn {
	if sharedNATSClient == nil {
		t.Fatal("Shared NATS client not initialized - TestMain should have created it")
	}
	return sharedNATSClient
}
