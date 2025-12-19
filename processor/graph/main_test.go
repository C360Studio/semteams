//go:build integration

package graph

import (
	"encoding/json"
	"log"
	"testing"
	"time"

	"github.com/c360/semstreams/message"
	"github.com/c360/semstreams/natsclient"
)

// Package-level shared test client to avoid Docker resource exhaustion
var (
	sharedTestClient *natsclient.TestClient
	sharedNATSClient *natsclient.Client
)

// TestMain sets up a single shared NATS container for all graph processor tests
func TestMain(m *testing.M) {
	// Build tag ensures this only runs with -tags=integration
	testClient, err := natsclient.NewSharedTestClient(
		natsclient.WithJetStream(),
		natsclient.WithKV(),
		natsclient.WithKVBuckets(
			"ENTITY_STATES",
			"ALIAS_INDEX",
			"PREDICATE_INDEX",
			"INCOMING_INDEX",
			"OUTGOING_INDEX",
			"SPATIAL_INDEX",
			"TEMPORAL_INDEX",
		),
		natsclient.WithTestTimeout(5*time.Second),
		natsclient.WithStartTimeout(30*time.Second),
	)
	if err != nil {
		log.Fatalf("Failed to create shared test client: %v", err)
	}

	sharedTestClient = testClient
	sharedNATSClient = testClient.Client

	// Run all tests
	exitCode := m.Run()

	// Cleanup integration test resources
	sharedTestClient.Terminate()

	if exitCode != 0 {
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

// TestGraphablePayload implements the Graphable interface for testing
type TestGraphablePayload struct {
	ID         string                   `json:"entity_id"`
	Properties map[string]interface{}   `json:"properties"`
	TripleData []map[string]interface{} `json:"triples"`
}

func (t *TestGraphablePayload) EntityID() string {
	return t.ID
}

func (t *TestGraphablePayload) Triples() []message.Triple {
	var triples []message.Triple
	for _, triple := range t.TripleData {
		triples = append(triples, message.Triple{
			Subject:   triple["subject"].(string),
			Predicate: triple["predicate"].(string),
			Object:    triple["object"],
		})
	}
	return triples
}

func (t *TestGraphablePayload) Schema() message.Type {
	return message.Type{
		Domain:   "test",
		Category: "graphable",
		Version:  "v1",
	}
}

func (t *TestGraphablePayload) Validate() error {
	return nil
}

func (t *TestGraphablePayload) MarshalJSON() ([]byte, error) {
	return json.Marshal(struct {
		EntityID   string                   `json:"entity_id"`
		Properties map[string]interface{}   `json:"properties"`
		TripleData []map[string]interface{} `json:"triples"`
	}{
		EntityID:   t.ID,
		Properties: t.Properties,
		TripleData: t.TripleData,
	})
}

func (t *TestGraphablePayload) UnmarshalJSON(data []byte) error {
	var tmp struct {
		EntityID   string                   `json:"entity_id"`
		Properties map[string]interface{}   `json:"properties"`
		TripleData []map[string]interface{} `json:"triples"`
	}

	if err := json.Unmarshal(data, &tmp); err != nil {
		return err
	}

	t.ID = tmp.EntityID
	t.Properties = tmp.Properties
	t.TripleData = tmp.TripleData
	return nil
}
