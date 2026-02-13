//go:build integration

package trustgraph

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/c360studio/semstreams/bridge/trustgraph/client"
	"github.com/c360studio/semstreams/bridge/trustgraph/vocab"
	"github.com/c360studio/semstreams/message"
)

// MockTrustGraphServer provides a mock TrustGraph REST API for integration tests.
type MockTrustGraphServer struct {
	triples        []client.TGTriple
	knowledgeCores map[string][]client.TGTriple
	server         *httptest.Server
}

// NewMockTrustGraphServer creates a new mock server.
func NewMockTrustGraphServer() *MockTrustGraphServer {
	m := &MockTrustGraphServer{
		triples:        make([]client.TGTriple, 0),
		knowledgeCores: make(map[string][]client.TGTriple),
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/api/v1/triples-query", m.handleTriplesQuery)
	mux.HandleFunc("/api/v1/knowledge", m.handleKnowledge)
	mux.HandleFunc("/api/v1/graph-rag", m.handleGraphRAG)

	m.server = httptest.NewServer(mux)
	return m
}

// Close shuts down the mock server.
func (m *MockTrustGraphServer) Close() {
	m.server.Close()
}

// URL returns the mock server URL.
func (m *MockTrustGraphServer) URL() string {
	return m.server.URL
}

// AddTriples adds triples to the mock server.
func (m *MockTrustGraphServer) AddTriples(triples ...client.TGTriple) {
	m.triples = append(m.triples, triples...)
}

func (m *MockTrustGraphServer) handleTriplesQuery(w http.ResponseWriter, _ *http.Request) {
	resp := client.TriplesQueryResponse{
		Complete: true,
	}
	resp.Response.Response = m.triples

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp)
}

func (m *MockTrustGraphServer) handleKnowledge(w http.ResponseWriter, r *http.Request) {
	var req client.KnowledgeRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	// Store triples in knowledge core
	key := req.Request.ID + ":" + req.Request.Collection
	m.knowledgeCores[key] = append(m.knowledgeCores[key], req.Request.Triples...)

	resp := client.KnowledgeResponse{Complete: true}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp)
}

func (m *MockTrustGraphServer) handleGraphRAG(w http.ResponseWriter, _ *http.Request) {
	resp := client.GraphRAGResponse{
		Complete: true,
	}
	resp.Response.Response = "This is a mock GraphRAG response based on the knowledge graph."

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp)
}

// GetStoredTriples returns triples stored in a knowledge core.
func (m *MockTrustGraphServer) GetStoredTriples(coreID, collection string) []client.TGTriple {
	return m.knowledgeCores[coreID+":"+collection]
}

func TestIntegration_VocabTranslation_RoundTrip(t *testing.T) {
	translator := vocab.NewTranslator(vocab.TranslatorConfig{
		OrgMappings: map[string]string{
			"acme": "https://data.acme-corp.com/",
		},
		URIMappings: map[string]vocab.URIMapping{
			"data.acme-corp.com": {
				Org: "acme",
			},
		},
	})

	// SemStreams Triple
	ssTriple := message.Triple{
		Subject:    "acme.ops.robotics.gcs.drone.001",
		Predicate:  "entity.metadata.label",
		Object:     "Drone 001",
		Source:     "operator",
		Timestamp:  time.Now(),
		Confidence: 1.0,
	}

	// Convert to TrustGraph format
	tgTriple := translator.TripleToRDF(ssTriple)

	// Verify TrustGraph format
	if tgTriple.S.V != "https://data.acme-corp.com/ops/robotics/gcs/drone/001" {
		t.Errorf("Subject URI = %q, want https://data.acme-corp.com/ops/robotics/gcs/drone/001", tgTriple.S.V)
	}
	if !tgTriple.S.E {
		t.Error("Subject should be an entity (E=true)")
	}
	if tgTriple.O.E {
		t.Error("Object should be a literal (E=false)")
	}
	if tgTriple.O.V != "Drone 001" {
		t.Errorf("Object value = %q, want 'Drone 001'", tgTriple.O.V)
	}

	// Convert back to SemStreams format
	roundTripped := translator.RDFToTriple(vocab.TGTriple{
		S: vocab.TGValue{V: tgTriple.S.V, E: tgTriple.S.E},
		P: vocab.TGValue{V: tgTriple.P.V, E: tgTriple.P.E},
		O: vocab.TGValue{V: tgTriple.O.V, E: tgTriple.O.E},
	}, "trustgraph")

	// Verify round-trip
	if roundTripped.Subject != ssTriple.Subject {
		t.Errorf("Round-trip subject = %q, want %q", roundTripped.Subject, ssTriple.Subject)
	}
	if roundTripped.Object != ssTriple.Object {
		t.Errorf("Round-trip object = %v, want %v", roundTripped.Object, ssTriple.Object)
	}
}

func TestIntegration_Client_QueryTriples(t *testing.T) {
	mock := NewMockTrustGraphServer()
	defer mock.Close()

	// Add test triples
	mock.AddTriples(
		client.TGTriple{
			S: client.TGValue{V: "http://example.org/entity1", E: true},
			P: client.TGValue{V: "http://www.w3.org/2000/01/rdf-schema#label", E: true},
			O: client.TGValue{V: "Entity One", E: false},
		},
		client.TGTriple{
			S: client.TGValue{V: "http://example.org/entity1", E: true},
			P: client.TGValue{V: "http://www.w3.org/1999/02/22-rdf-syntax-ns#type", E: true},
			O: client.TGValue{V: "http://example.org/Type", E: true},
		},
	)

	// Create client
	c := client.New(client.Config{
		Endpoint: mock.URL(),
		Timeout:  5 * time.Second,
	})

	// Query triples
	triples, err := c.QueryTriples(context.Background(), client.TriplesQueryParams{
		Limit: 100,
	})

	if err != nil {
		t.Fatalf("QueryTriples failed: %v", err)
	}

	if len(triples) != 2 {
		t.Errorf("Expected 2 triples, got %d", len(triples))
	}
}

func TestIntegration_Client_PutKGCoreTriples(t *testing.T) {
	mock := NewMockTrustGraphServer()
	defer mock.Close()

	c := client.New(client.Config{
		Endpoint: mock.URL(),
		Timeout:  5 * time.Second,
	})

	triples := []client.TGTriple{
		{
			S: client.TGValue{V: "http://semstreams.io/e/sensor/001", E: true},
			P: client.TGValue{V: "http://www.w3.org/ns/sosa/hasSimpleResult", E: true},
			O: client.TGValue{V: "42.5", E: false},
		},
	}

	err := c.PutKGCoreTriples(context.Background(), "test-core", "semstreams", "operational", triples)
	if err != nil {
		t.Fatalf("PutKGCoreTriples failed: %v", err)
	}

	// Verify triples were stored
	stored := mock.GetStoredTriples("test-core", "operational")
	if len(stored) != 1 {
		t.Errorf("Expected 1 stored triple, got %d", len(stored))
	}
}

func TestIntegration_Client_GraphRAG(t *testing.T) {
	mock := NewMockTrustGraphServer()
	defer mock.Close()

	c := client.New(client.Config{
		Endpoint: mock.URL(),
		Timeout:  5 * time.Second,
	})

	response, err := c.GraphRAG(context.Background(), "test-flow", "What sensors are in zone 7?")
	if err != nil {
		t.Fatalf("GraphRAG failed: %v", err)
	}

	if response == "" {
		t.Error("Expected non-empty response")
	}
}

func TestIntegration_FullImportExportCycle(t *testing.T) {
	// This test demonstrates the full import/export cycle:
	// 1. Create mock TrustGraph with some triples
	// 2. Import triples via translation
	// 3. Export translated triples back
	// 4. Verify round-trip consistency

	mock := NewMockTrustGraphServer()
	defer mock.Close()

	// Add TrustGraph triples to import
	mock.AddTriples(
		client.TGTriple{
			S: client.TGValue{V: "http://trustgraph.ai/e/threat-report-001", E: true},
			P: client.TGValue{V: "http://www.w3.org/2000/01/rdf-schema#label", E: true},
			O: client.TGValue{V: "Supply Chain Threat Report", E: false},
		},
		client.TGTriple{
			S: client.TGValue{V: "http://trustgraph.ai/e/threat-report-001", E: true},
			P: client.TGValue{V: "http://www.w3.org/1999/02/22-rdf-syntax-ns#type", E: true},
			O: client.TGValue{V: "http://trustgraph.ai/e/ThreatReport", E: true},
		},
	)

	// Create translator with mappings
	translator := vocab.NewTranslator(vocab.TranslatorConfig{
		URIMappings: map[string]vocab.URIMapping{
			"trustgraph.ai": {
				Org:      "intel",
				Platform: "trustgraph",
				Domain:   "knowledge",
				System:   "document",
				Type:     "report",
			},
		},
	})

	// Create client
	c := client.New(client.Config{
		Endpoint: mock.URL(),
		Timeout:  5 * time.Second,
	})

	// Import: Query triples from TrustGraph
	importedTriples, err := c.QueryTriples(context.Background(), client.TriplesQueryParams{Limit: 100})
	if err != nil {
		t.Fatalf("Import query failed: %v", err)
	}

	// Translate imported triples to SemStreams format
	var ssTriples []message.Triple
	for _, tg := range importedTriples {
		vocabTG := vocab.TGTriple{
			S: vocab.TGValue{V: tg.S.V, E: tg.S.E},
			P: vocab.TGValue{V: tg.P.V, E: tg.P.E},
			O: vocab.TGValue{V: tg.O.V, E: tg.O.E},
		}
		ssTriples = append(ssTriples, translator.RDFToTriple(vocabTG, "trustgraph"))
	}

	// Verify translation
	if len(ssTriples) != 2 {
		t.Fatalf("Expected 2 translated triples, got %d", len(ssTriples))
	}

	// Check that entity ID was translated correctly
	expectedPrefix := "intel.trustgraph.knowledge.document.report"
	for _, triple := range ssTriples {
		if triple.Subject[:len(expectedPrefix)] != expectedPrefix {
			t.Errorf("Expected subject prefix %q, got %q", expectedPrefix, triple.Subject[:len(expectedPrefix)])
		}
		if triple.Source != "trustgraph" {
			t.Errorf("Expected source 'trustgraph', got %q", triple.Source)
		}
	}

	// Export: Translate back and send to TrustGraph
	var exportTriples []client.TGTriple
	for _, ss := range ssTriples {
		vocabTG := translator.TripleToRDF(ss)
		exportTriples = append(exportTriples, client.TGTriple{
			S: client.TGValue{V: vocabTG.S.V, E: vocabTG.S.E},
			P: client.TGValue{V: vocabTG.P.V, E: vocabTG.P.E},
			O: client.TGValue{V: vocabTG.O.V, E: vocabTG.O.E},
		})
	}

	err = c.PutKGCoreTriples(context.Background(), "semstreams-export", "test", "round-trip", exportTriples)
	if err != nil {
		t.Fatalf("Export failed: %v", err)
	}

	// Verify export
	exported := mock.GetStoredTriples("semstreams-export", "round-trip")
	if len(exported) != 2 {
		t.Errorf("Expected 2 exported triples, got %d", len(exported))
	}
}
