package client

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"
)

func TestClient_QueryTriples(t *testing.T) {
	// Create mock server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Verify request
		if r.Method != http.MethodPost {
			t.Errorf("Expected POST, got %s", r.Method)
		}
		if r.URL.Path != "/api/v1/triples-query" {
			t.Errorf("Expected /api/v1/triples-query, got %s", r.URL.Path)
		}
		if r.Header.Get("Content-Type") != "application/json" {
			t.Errorf("Expected Content-Type application/json, got %s", r.Header.Get("Content-Type"))
		}

		// Parse request body
		var req TriplesQueryRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Errorf("Failed to decode request: %v", err)
		}

		if req.Service != "triples-query" {
			t.Errorf("Expected service triples-query, got %s", req.Service)
		}

		// Send response
		resp := TriplesQueryResponse{
			Complete: true,
		}
		resp.Response.Response = []TGTriple{
			{
				S: TGValue{V: "http://example.org/entity1", E: true},
				P: TGValue{V: "http://www.w3.org/2000/01/rdf-schema#label", E: true},
				O: TGValue{V: "Entity One", E: false},
			},
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	// Create client
	client := New(Config{
		Endpoint: server.URL,
		Timeout:  5 * time.Second,
	})

	// Execute query
	triples, err := client.QueryTriples(context.Background(), TriplesQueryParams{
		S:     &TGValue{V: "http://example.org/entity1", E: true},
		Limit: 100,
	})

	if err != nil {
		t.Fatalf("QueryTriples failed: %v", err)
	}

	if len(triples) != 1 {
		t.Fatalf("Expected 1 triple, got %d", len(triples))
	}

	if triples[0].S.V != "http://example.org/entity1" {
		t.Errorf("Expected subject http://example.org/entity1, got %s", triples[0].S.V)
	}
	if triples[0].O.V != "Entity One" {
		t.Errorf("Expected object Entity One, got %s", triples[0].O.V)
	}
	if triples[0].O.E != false {
		t.Errorf("Expected object to be literal (E=false)")
	}
}

func TestClient_PutKGCoreTriples(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/knowledge" {
			t.Errorf("Expected /api/v1/knowledge, got %s", r.URL.Path)
		}

		var req KnowledgeRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Errorf("Failed to decode request: %v", err)
		}

		if req.Service != "knowledge" {
			t.Errorf("Expected service knowledge, got %s", req.Service)
		}
		if req.Request.Operation != "put-kg-core-triples" {
			t.Errorf("Expected operation put-kg-core-triples, got %s", req.Request.Operation)
		}
		if req.Request.ID != "test-core" {
			t.Errorf("Expected ID test-core, got %s", req.Request.ID)
		}
		if req.Request.User != "testuser" {
			t.Errorf("Expected user testuser, got %s", req.Request.User)
		}
		if len(req.Request.Triples) != 2 {
			t.Errorf("Expected 2 triples, got %d", len(req.Request.Triples))
		}

		resp := KnowledgeResponse{Complete: true}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	client := New(Config{Endpoint: server.URL})

	triples := []TGTriple{
		{
			S: TGValue{V: "http://example.org/entity1", E: true},
			P: TGValue{V: "http://www.w3.org/1999/02/22-rdf-syntax-ns#type", E: true},
			O: TGValue{V: "http://example.org/Type", E: true},
		},
		{
			S: TGValue{V: "http://example.org/entity1", E: true},
			P: TGValue{V: "http://www.w3.org/2000/01/rdf-schema#label", E: true},
			O: TGValue{V: "Entity One", E: false},
		},
	}

	err := client.PutKGCoreTriples(context.Background(), "test-core", "testuser", "test-collection", triples)
	if err != nil {
		t.Fatalf("PutKGCoreTriples failed: %v", err)
	}
}

func TestClient_GraphRAG(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/graph-rag" {
			t.Errorf("Expected /api/v1/graph-rag, got %s", r.URL.Path)
		}

		var req GraphRAGRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Errorf("Failed to decode request: %v", err)
		}

		if req.Service != "graph-rag" {
			t.Errorf("Expected service graph-rag, got %s", req.Service)
		}
		if req.Flow != "test-flow" {
			t.Errorf("Expected flow test-flow, got %s", req.Flow)
		}
		if req.Request.Query != "What is the answer?" {
			t.Errorf("Expected query 'What is the answer?', got %s", req.Request.Query)
		}

		resp := GraphRAGResponse{Complete: true}
		resp.Response.Response = "The answer is 42."
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	client := New(Config{Endpoint: server.URL})

	response, err := client.GraphRAG(context.Background(), "test-flow", "What is the answer?")
	if err != nil {
		t.Fatalf("GraphRAG failed: %v", err)
	}

	if response != "The answer is 42." {
		t.Errorf("Expected 'The answer is 42.', got %s", response)
	}
}

func TestClient_RetryOn5xx(t *testing.T) {
	var attempts int32

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		count := atomic.AddInt32(&attempts, 1)

		if count < 3 {
			// First two attempts fail with 500
			w.WriteHeader(http.StatusInternalServerError)
			w.Write([]byte("internal server error"))
			return
		}

		// Third attempt succeeds
		resp := TriplesQueryResponse{Complete: true}
		resp.Response.Response = []TGTriple{}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	client := New(Config{
		Endpoint:       server.URL,
		MaxRetries:     3,
		RetryBaseDelay: 10 * time.Millisecond, // Fast retry for tests
	})

	_, err := client.QueryTriples(context.Background(), TriplesQueryParams{Limit: 10})
	if err != nil {
		t.Fatalf("QueryTriples failed after retries: %v", err)
	}

	if atomic.LoadInt32(&attempts) != 3 {
		t.Errorf("Expected 3 attempts, got %d", atomic.LoadInt32(&attempts))
	}
}

func TestClient_NoRetryOn4xx(t *testing.T) {
	var attempts int32

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&attempts, 1)
		w.WriteHeader(http.StatusBadRequest)
		w.Write([]byte("bad request"))
	}))
	defer server.Close()

	client := New(Config{
		Endpoint:       server.URL,
		MaxRetries:     3,
		RetryBaseDelay: 10 * time.Millisecond,
	})

	_, err := client.QueryTriples(context.Background(), TriplesQueryParams{Limit: 10})
	if err == nil {
		t.Fatal("Expected error for 400 response")
	}

	// Should not retry 4xx errors
	if atomic.LoadInt32(&attempts) != 1 {
		t.Errorf("Expected 1 attempt (no retry for 4xx), got %d", atomic.LoadInt32(&attempts))
	}

	// Error message should contain status information
	if err.Error() == "" {
		t.Error("Expected non-empty error message")
	}
}

func TestClient_RetryAfter(t *testing.T) {
	var attempts int32
	var retryWaited bool

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		count := atomic.AddInt32(&attempts, 1)

		if count == 1 {
			w.Header().Set("Retry-After", "1") // 1 second
			w.WriteHeader(http.StatusTooManyRequests)
			w.Write([]byte("rate limited"))
			return
		}

		retryWaited = true
		resp := TriplesQueryResponse{Complete: true}
		resp.Response.Response = []TGTriple{}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	client := New(Config{
		Endpoint:       server.URL,
		MaxRetries:     3,
		RetryBaseDelay: 10 * time.Millisecond,
	})

	start := time.Now()
	_, err := client.QueryTriples(context.Background(), TriplesQueryParams{Limit: 10})
	elapsed := time.Since(start)

	if err != nil {
		t.Fatalf("QueryTriples failed: %v", err)
	}

	if !retryWaited {
		t.Error("Expected retry after rate limit")
	}

	// Should have waited at least 1 second for Retry-After
	if elapsed < 900*time.Millisecond {
		t.Errorf("Expected to wait at least 1 second for Retry-After, waited %v", elapsed)
	}
}

func TestClient_ContextCancellation(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Simulate slow response
		time.Sleep(5 * time.Second)
		resp := TriplesQueryResponse{Complete: true}
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	client := New(Config{
		Endpoint: server.URL,
		Timeout:  10 * time.Second,
	})

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	_, err := client.QueryTriples(ctx, TriplesQueryParams{Limit: 10})
	if err == nil {
		t.Fatal("Expected error due to context cancellation")
	}
}

func TestClient_APIKeyHeader(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		auth := r.Header.Get("Authorization")
		if auth != "Bearer test-api-key" {
			t.Errorf("Expected Authorization header 'Bearer test-api-key', got '%s'", auth)
		}

		resp := TriplesQueryResponse{Complete: true}
		resp.Response.Response = []TGTriple{}
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	client := New(Config{
		Endpoint: server.URL,
		APIKey:   "test-api-key",
	})

	_, err := client.QueryTriples(context.Background(), TriplesQueryParams{Limit: 10})
	if err != nil {
		t.Fatalf("QueryTriples failed: %v", err)
	}
}

func TestClient_APIErrorResponse(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := TriplesQueryResponse{
			Error: "invalid query parameters",
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	client := New(Config{Endpoint: server.URL})

	_, err := client.QueryTriples(context.Background(), TriplesQueryParams{})
	if err == nil {
		t.Fatal("Expected error for API error response")
	}

	apiErr, ok := err.(*APIError)
	if !ok {
		t.Fatalf("Expected *APIError, got %T", err)
	}
	if apiErr.Message != "invalid query parameters" {
		t.Errorf("Expected message 'invalid query parameters', got '%s'", apiErr.Message)
	}
}

func TestAPIError_IsRetryable(t *testing.T) {
	tests := []struct {
		statusCode int
		retryable  bool
	}{
		{200, false},
		{400, false},
		{401, false},
		{403, false},
		{404, false},
		{429, true}, // Rate limit
		{500, true}, // Server error
		{502, true}, // Bad gateway
		{503, true}, // Service unavailable
		{504, true}, // Gateway timeout
	}

	for _, tt := range tests {
		t.Run("", func(t *testing.T) {
			err := &APIError{StatusCode: tt.statusCode}
			if err.IsRetryable() != tt.retryable {
				t.Errorf("StatusCode %d: IsRetryable() = %v, want %v", tt.statusCode, err.IsRetryable(), tt.retryable)
			}
		})
	}
}

func TestNewEntityValue(t *testing.T) {
	v := NewEntityValue("http://example.org/entity")
	if v.V != "http://example.org/entity" {
		t.Errorf("Expected V = http://example.org/entity, got %s", v.V)
	}
	if !v.E {
		t.Error("Expected E = true for entity value")
	}
}

func TestNewLiteralValue(t *testing.T) {
	v := NewLiteralValue("test value")
	if v.V != "test value" {
		t.Errorf("Expected V = test value, got %s", v.V)
	}
	if v.E {
		t.Error("Expected E = false for literal value")
	}
}
