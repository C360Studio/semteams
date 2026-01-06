//go:build integration

package indexmanager

import (
	"log"
	"testing"

	"github.com/c360/semstreams/natsclient"
)

// Package-level shared test client to avoid Docker resource exhaustion
var (
	sharedTestClient *natsclient.TestClient
	sharedNATSClient *natsclient.Client
)

// TestMain sets up a single shared NATS container for all indexmanager tests
func TestMain(m *testing.M) {
	// Build tag ensures this only runs with -tags=integration
	testClient, err := natsclient.NewSharedTestClient(
		natsclient.WithJetStream(),
		natsclient.WithKV(),
		natsclient.WithKVBuckets(
			// Core entity and index buckets
			"ENTITY_STATES",
			"ALIAS_INDEX",
			"PREDICATE_INDEX",
			"INCOMING_INDEX",
			"OUTGOING_INDEX",
			"SPATIAL_INDEX",
			"TEMPORAL_INDEX",
			// Embedding buckets (required for semantic search features)
			"EMBEDDING_INDEX",
			"EMBEDDING_DEDUP",
			"EMBEDDINGS_CACHE",
		),
	)
	if err != nil {
		log.Fatalf("Failed to create shared test client: %v", err)
	}

	sharedTestClient = testClient
	sharedNATSClient = testClient.Client

	// Run tests
	code := m.Run()

	// Cleanup
	sharedTestClient.Terminate()

	if code != 0 {
		log.Fatal("tests failed")
	}
}

// getSharedNATSClient returns the shared NATS client for integration tests
func getSharedNATSClient(t *testing.T) *natsclient.Client {
	if sharedNATSClient == nil {
		t.Fatal("Shared NATS client not initialized - TestMain should have created it")
	}
	return sharedNATSClient
}
