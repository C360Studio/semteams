package mock

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
)

func TestTrustGraphServer_Health(t *testing.T) {
	server := NewTrustGraphServer()

	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	w := httptest.NewRecorder()

	server.mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected status %d, got %d", http.StatusOK, w.Code)
	}

	var resp map[string]any
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	if status, ok := resp["status"].(string); !ok || status != "healthy" {
		t.Errorf("expected status 'healthy', got %v", resp["status"])
	}
}

func TestTrustGraphServer_TriplesQuery(t *testing.T) {
	testTriples := []TGTriple{
		{
			S: NewEntityValue("http://example.org/e/1"),
			P: NewEntityValue("http://www.w3.org/2000/01/rdf-schema#label"),
			O: NewLiteralValue("Test Entity"),
		},
		{
			S: NewEntityValue("http://example.org/e/2"),
			P: NewEntityValue("http://www.w3.org/2000/01/rdf-schema#label"),
			O: NewLiteralValue("Another Entity"),
		},
	}

	server := NewTrustGraphServer().WithImportTriples(testTriples)

	// Query all triples
	reqBody := `{"service": "triples-query", "request": {"limit": 100}}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/triples-query", bytes.NewBufferString(reqBody))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	server.mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected status %d, got %d", http.StatusOK, w.Code)
	}

	var resp map[string]any
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	respData, ok := resp["response"].(map[string]any)
	if !ok {
		t.Fatalf("expected response object, got %T", resp["response"])
	}

	triples, ok := respData["response"].([]any)
	if !ok {
		t.Fatalf("expected triples array, got %T", respData["response"])
	}

	if len(triples) != 2 {
		t.Errorf("expected 2 triples, got %d", len(triples))
	}

	if server.TriplesQueried() != 2 {
		t.Errorf("expected 2 triples queried, got %d", server.TriplesQueried())
	}
}

func TestTrustGraphServer_Knowledge(t *testing.T) {
	server := NewTrustGraphServer()

	// Store triples
	reqBody := `{
		"service": "knowledge",
		"request": {
			"operation": "put-kg-core-triples",
			"id": "test-core",
			"user": "test-user",
			"collection": "test-collection",
			"triples": [
				{"s": {"v": "http://example.org/e/1", "e": true}, "p": {"v": "http://example.org/prop", "e": true}, "o": {"v": "value", "e": false}}
			]
		}
	}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/knowledge", bytes.NewBufferString(reqBody))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	server.mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected status %d, got %d", http.StatusOK, w.Code)
	}

	if server.TriplesStored() != 1 {
		t.Errorf("expected 1 triple stored, got %d", server.TriplesStored())
	}

	// Verify stored triples
	stored := server.GetStoredTriples("test-core", "test-collection")
	if len(stored) != 1 {
		t.Errorf("expected 1 stored triple, got %d", len(stored))
	}
}

func TestTrustGraphServer_GraphRAG(t *testing.T) {
	server := NewTrustGraphServer().
		WithRAGResponse("threat", "Found 3 threat indicators").
		WithDefaultRAGResponse("No information found")

	tests := []struct {
		name     string
		query    string
		expected string
	}{
		{
			name:     "matching query",
			query:    "What are the threat indicators?",
			expected: "Found 3 threat indicators",
		},
		{
			name:     "non-matching query",
			query:    "What is the weather?",
			expected: "No information found",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			reqBody, _ := json.Marshal(map[string]any{
				"service": "graph-rag",
				"request": map[string]string{
					"query": tc.query,
				},
			})
			req := httptest.NewRequest(http.MethodPost, "/api/v1/graph-rag", bytes.NewBuffer(reqBody))
			req.Header.Set("Content-Type", "application/json")
			w := httptest.NewRecorder()

			server.mux.ServeHTTP(w, req)

			if w.Code != http.StatusOK {
				t.Errorf("expected status %d, got %d", http.StatusOK, w.Code)
			}

			var resp map[string]any
			if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
				t.Fatalf("failed to decode response: %v", err)
			}

			respData, ok := resp["response"].(map[string]any)
			if !ok {
				t.Fatalf("expected response map, got %T", resp["response"])
			}
			response, ok := respData["response"].(string)
			if !ok {
				t.Fatalf("expected response string, got %T", respData["response"])
			}

			if response != tc.expected {
				t.Errorf("expected %q, got %q", tc.expected, response)
			}
		})
	}

	if server.RAGQueries() != int64(len(tests)) {
		t.Errorf("expected %d RAG queries, got %d", len(tests), server.RAGQueries())
	}
}

func TestTrustGraphServer_Stats(t *testing.T) {
	server := NewTrustGraphServer().WithImportTriples([]TGTriple{
		{S: NewEntityValue("http://example.org/e/1"), P: NewEntityValue("http://example.org/p"), O: NewLiteralValue("v")},
	})

	req := httptest.NewRequest(http.MethodGet, "/stats", nil)
	w := httptest.NewRecorder()

	server.mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected status %d, got %d", http.StatusOK, w.Code)
	}

	var stats map[string]any
	if err := json.NewDecoder(w.Body).Decode(&stats); err != nil {
		t.Fatalf("failed to decode stats: %v", err)
	}

	if importTriples, ok := stats["import_triples"].(float64); !ok || int(importTriples) != 1 {
		t.Errorf("expected 1 import triple, got %v", stats["import_triples"])
	}
}

func TestTrustGraphServer_GetStored(t *testing.T) {
	server := NewTrustGraphServer()

	// First store some triples
	server.knowledgeCoresMu.Lock()
	server.knowledgeCores["mycore:mycoll"] = []TGTriple{
		{S: NewEntityValue("http://example.org/e/1"), P: NewEntityValue("http://example.org/p"), O: NewLiteralValue("v")},
	}
	server.knowledgeCoresMu.Unlock()

	req := httptest.NewRequest(http.MethodGet, "/stored/mycore/mycoll", nil)
	w := httptest.NewRecorder()

	server.mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected status %d, got %d", http.StatusOK, w.Code)
	}

	var resp map[string]any
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	if count := resp["count"].(float64); int(count) != 1 {
		t.Errorf("expected count 1, got %v", count)
	}
}

func TestTrustGraphServer_Reset(t *testing.T) {
	server := NewTrustGraphServer()

	// Store some data
	server.knowledgeCoresMu.Lock()
	server.knowledgeCores["test:test"] = []TGTriple{{}}
	server.knowledgeCoresMu.Unlock()
	atomic.StoreInt64(&server.triplesStored, 10)

	// Reset
	req := httptest.NewRequest(http.MethodPost, "/reset", nil)
	w := httptest.NewRecorder()

	server.mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected status %d, got %d", http.StatusOK, w.Code)
	}

	// Verify reset
	if server.TriplesStored() != 0 {
		t.Errorf("expected 0 triples stored after reset, got %d", server.TriplesStored())
	}

	server.knowledgeCoresMu.RLock()
	coreCount := len(server.knowledgeCores)
	server.knowledgeCoresMu.RUnlock()

	if coreCount != 0 {
		t.Errorf("expected 0 knowledge cores after reset, got %d", coreCount)
	}
}
