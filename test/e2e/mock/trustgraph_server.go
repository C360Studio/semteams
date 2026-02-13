// Package mock provides mock servers for E2E testing.
package mock

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

// TGTriple is TrustGraph's compact triple representation.
// This matches the wire format from bridge/trustgraph/client/types.go.
type TGTriple struct {
	S TGValue `json:"s"`
	P TGValue `json:"p"`
	O TGValue `json:"o"`
}

// TGValue is a compact value with entity flag.
type TGValue struct {
	V string `json:"v"` // Value (URI or literal)
	E bool   `json:"e"` // Is entity (true = URI, false = literal)
}

// NewEntityValue creates a TGValue representing an entity (URI).
func NewEntityValue(uri string) TGValue {
	return TGValue{V: uri, E: true}
}

// NewLiteralValue creates a TGValue representing a literal.
func NewLiteralValue(value string) TGValue {
	return TGValue{V: value, E: false}
}

// TrustGraphServer provides mock endpoints for TrustGraph integration testing.
// It implements the three main REST API endpoints:
//   - POST /api/v1/triples-query - Query triples (for import)
//   - POST /api/v1/knowledge - Store triples (for export)
//   - POST /api/v1/graph-rag - GraphRAG queries (for agentic tools)
//
// Plus debug/testing endpoints:
//   - GET /health - Health check
//   - GET /stats - Server statistics
//   - GET /stored/{core}/{collection} - Get stored triples
//   - POST /reset - Reset all state
type TrustGraphServer struct {
	mux    *http.ServeMux
	server *http.Server

	// Import data (seeded at startup, returned by triples-query)
	importTriples   []TGTriple
	importTriplesMu sync.RWMutex

	// Export data (stored by output component via knowledge API)
	knowledgeCores   map[string][]TGTriple // key: "coreID:collection"
	knowledgeCoresMu sync.RWMutex

	// GraphRAG responses (query substring -> response)
	ragResponses   map[string]string
	ragResponsesMu sync.RWMutex
	defaultRAGResp string

	// Stats for validation
	triplesQueried int64
	triplesStored  int64
	ragQueries     int64
	requestCount   int64
}

// NewTrustGraphServer creates a new mock TrustGraph server.
func NewTrustGraphServer() *TrustGraphServer {
	s := &TrustGraphServer{
		mux:            http.NewServeMux(),
		knowledgeCores: make(map[string][]TGTriple),
		ragResponses:   make(map[string]string),
		defaultRAGResp: "No relevant information found in the knowledge graph.",
	}
	s.setupRoutes()
	return s
}

// WithImportTriples configures the triples returned by the triples-query endpoint.
func (s *TrustGraphServer) WithImportTriples(triples []TGTriple) *TrustGraphServer {
	s.importTriplesMu.Lock()
	s.importTriples = triples
	s.importTriplesMu.Unlock()
	return s
}

// WithRAGResponse configures a GraphRAG response for queries containing the given substring.
func (s *TrustGraphServer) WithRAGResponse(queryContains, response string) *TrustGraphServer {
	s.ragResponsesMu.Lock()
	s.ragResponses[queryContains] = response
	s.ragResponsesMu.Unlock()
	return s
}

// WithDefaultRAGResponse sets the default response for GraphRAG queries with no match.
func (s *TrustGraphServer) WithDefaultRAGResponse(response string) *TrustGraphServer {
	s.ragResponsesMu.Lock()
	s.defaultRAGResp = response
	s.ragResponsesMu.Unlock()
	return s
}

func (s *TrustGraphServer) setupRoutes() {
	// Health endpoint
	s.mux.HandleFunc("/health", s.handleHealth)

	// TrustGraph API endpoints
	s.mux.HandleFunc("/api/v1/triples-query", s.handleTriplesQuery)
	s.mux.HandleFunc("/api/v1/knowledge", s.handleKnowledge)
	s.mux.HandleFunc("/api/v1/graph-rag", s.handleGraphRAG)

	// Debug/testing endpoints
	s.mux.HandleFunc("/stats", s.handleStats)
	s.mux.HandleFunc("/stored/", s.handleGetStored)
	s.mux.HandleFunc("/reset", s.handleReset)
}

// Start starts the server on the given address.
func (s *TrustGraphServer) Start(addr string) error {
	s.server = &http.Server{
		Addr:    addr,
		Handler: s.mux,
	}
	go func() {
		if err := s.server.ListenAndServe(); err != http.ErrServerClosed {
			log.Printf("TrustGraph mock server error: %v", err)
		}
	}()
	// Give server time to start
	time.Sleep(50 * time.Millisecond)
	return nil
}

// Stop stops the server.
func (s *TrustGraphServer) Stop() error {
	if s.server != nil {
		return s.server.Close()
	}
	return nil
}

// URL returns the server URL.
func (s *TrustGraphServer) URL() string {
	if s.server == nil {
		return ""
	}
	addr := s.server.Addr
	if strings.HasPrefix(addr, ":") {
		return "http://localhost" + addr
	}
	// Handle host:port format
	host, port, err := net.SplitHostPort(addr)
	if err != nil {
		return "http://" + addr
	}
	if host == "" || host == "0.0.0.0" {
		return fmt.Sprintf("http://localhost:%s", port)
	}
	return "http://" + addr
}

// Health handler
func (s *TrustGraphServer) handleHealth(w http.ResponseWriter, _ *http.Request) {
	atomic.AddInt64(&s.requestCount, 1)
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{
		"status": "healthy",
		"services": map[string]string{
			"triples-query": "healthy",
			"knowledge":     "healthy",
			"graph-rag":     "healthy",
		},
	})
}

// triplesQueryRequest matches the TrustGraph triples-query request format.
type triplesQueryRequest struct {
	ID      string `json:"id,omitempty"`
	Service string `json:"service"`
	Flow    string `json:"flow,omitempty"`
	Request struct {
		S     *TGValue `json:"s,omitempty"`
		P     *TGValue `json:"p,omitempty"`
		O     *TGValue `json:"o,omitempty"`
		Limit int      `json:"limit,omitempty"`
	} `json:"request"`
}

// Triples query handler - returns configured import triples
func (s *TrustGraphServer) handleTriplesQuery(w http.ResponseWriter, r *http.Request) {
	atomic.AddInt64(&s.requestCount, 1)

	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	body, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, "failed to read body", http.StatusBadRequest)
		return
	}

	var req triplesQueryRequest
	if err := json.Unmarshal(body, &req); err != nil {
		http.Error(w, "invalid JSON", http.StatusBadRequest)
		return
	}

	log.Printf("[TrustGraph Mock] Triples query received: service=%s, limit=%d", req.Service, req.Request.Limit)

	// Get import triples (may be filtered by request params)
	s.importTriplesMu.RLock()
	triples := s.filterTriples(s.importTriples, &req)
	s.importTriplesMu.RUnlock()

	atomic.AddInt64(&s.triplesQueried, int64(len(triples)))

	// Apply limit if specified
	limit := req.Request.Limit
	if limit > 0 && len(triples) > limit {
		triples = triples[:limit]
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{
		"id": req.ID,
		"response": map[string]any{
			"response": triples,
		},
		"complete": true,
	})
}

// filterTriples applies any filters from the query request.
func (s *TrustGraphServer) filterTriples(triples []TGTriple, req *triplesQueryRequest) []TGTriple {
	// If no filters, return all
	if req.Request.S == nil && req.Request.P == nil && req.Request.O == nil {
		return triples
	}

	var result []TGTriple
	for _, t := range triples {
		if req.Request.S != nil && t.S.V != req.Request.S.V {
			continue
		}
		if req.Request.P != nil && t.P.V != req.Request.P.V {
			continue
		}
		if req.Request.O != nil && t.O.V != req.Request.O.V {
			continue
		}
		result = append(result, t)
	}
	return result
}

// knowledgeRequest matches the TrustGraph knowledge API request format.
type knowledgeRequest struct {
	ID      string `json:"id,omitempty"`
	Service string `json:"service"`
	Request struct {
		Operation  string     `json:"operation"`
		ID         string     `json:"id"`         // Knowledge core ID
		User       string     `json:"user"`       // User identifier
		Collection string     `json:"collection"` // Collection name
		Triples    []TGTriple `json:"triples,omitempty"`
	} `json:"request"`
}

// Knowledge handler - stores triples from output component
func (s *TrustGraphServer) handleKnowledge(w http.ResponseWriter, r *http.Request) {
	atomic.AddInt64(&s.requestCount, 1)

	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	body, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, "failed to read body", http.StatusBadRequest)
		return
	}

	var req knowledgeRequest
	if err := json.Unmarshal(body, &req); err != nil {
		http.Error(w, "invalid JSON", http.StatusBadRequest)
		return
	}

	log.Printf("[TrustGraph Mock] Knowledge request: operation=%s, core=%s, collection=%s, triples=%d",
		req.Request.Operation, req.Request.ID, req.Request.Collection, len(req.Request.Triples))

	// Handle put-kg-core-triples operation
	if req.Request.Operation == "put-kg-core-triples" {
		key := fmt.Sprintf("%s:%s", req.Request.ID, req.Request.Collection)

		s.knowledgeCoresMu.Lock()
		existing := s.knowledgeCores[key]
		s.knowledgeCores[key] = append(existing, req.Request.Triples...)
		s.knowledgeCoresMu.Unlock()

		atomic.AddInt64(&s.triplesStored, int64(len(req.Request.Triples)))

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{
			"id": req.ID,
			"response": map[string]any{
				"response": map[string]any{
					"stored": len(req.Request.Triples),
				},
			},
			"complete": true,
		})
		return
	}

	// Handle other operations minimally
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{
		"id": req.ID,
		"response": map[string]any{
			"response": nil,
		},
		"complete": true,
	})
}

// graphRAGRequest matches the TrustGraph graph-rag request format.
type graphRAGRequest struct {
	ID      string `json:"id,omitempty"`
	Service string `json:"service"`
	Flow    string `json:"flow,omitempty"`
	Request struct {
		Query      string `json:"query"`
		Collection string `json:"collection,omitempty"`
	} `json:"request"`
}

// GraphRAG handler - returns configured responses
func (s *TrustGraphServer) handleGraphRAG(w http.ResponseWriter, r *http.Request) {
	atomic.AddInt64(&s.requestCount, 1)

	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	body, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, "failed to read body", http.StatusBadRequest)
		return
	}

	var req graphRAGRequest
	if err := json.Unmarshal(body, &req); err != nil {
		http.Error(w, "invalid JSON", http.StatusBadRequest)
		return
	}

	atomic.AddInt64(&s.ragQueries, 1)
	log.Printf("[TrustGraph Mock] GraphRAG query: %s", req.Request.Query)

	// Find matching response
	s.ragResponsesMu.RLock()
	response := s.defaultRAGResp
	for contains, resp := range s.ragResponses {
		if strings.Contains(strings.ToLower(req.Request.Query), strings.ToLower(contains)) {
			response = resp
			break
		}
	}
	s.ragResponsesMu.RUnlock()

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{
		"id": req.ID,
		"response": map[string]any{
			"response": response,
		},
		"complete": true,
	})
}

// Stats handler - returns server statistics for E2E validation
func (s *TrustGraphServer) handleStats(w http.ResponseWriter, _ *http.Request) {
	atomic.AddInt64(&s.requestCount, 1)

	s.knowledgeCoresMu.RLock()
	coreCount := len(s.knowledgeCores)
	totalStored := 0
	for _, triples := range s.knowledgeCores {
		totalStored += len(triples)
	}
	s.knowledgeCoresMu.RUnlock()

	s.importTriplesMu.RLock()
	importCount := len(s.importTriples)
	s.importTriplesMu.RUnlock()

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{
		"triples_queried": atomic.LoadInt64(&s.triplesQueried),
		"triples_stored":  atomic.LoadInt64(&s.triplesStored),
		"rag_queries":     atomic.LoadInt64(&s.ragQueries),
		"request_count":   atomic.LoadInt64(&s.requestCount),
		"import_triples":  importCount,
		"knowledge_cores": coreCount,
		"stored_total":    totalStored,
	})
}

// GetStored handler - returns stored triples for a specific core/collection
func (s *TrustGraphServer) handleGetStored(w http.ResponseWriter, r *http.Request) {
	atomic.AddInt64(&s.requestCount, 1)

	// Parse path: /stored/{core}/{collection}
	path := strings.TrimPrefix(r.URL.Path, "/stored/")
	parts := strings.SplitN(path, "/", 2)
	if len(parts) != 2 {
		http.Error(w, "invalid path: expected /stored/{core}/{collection}", http.StatusBadRequest)
		return
	}

	coreID, collection := parts[0], parts[1]
	key := fmt.Sprintf("%s:%s", coreID, collection)

	s.knowledgeCoresMu.RLock()
	triples := s.knowledgeCores[key]
	s.knowledgeCoresMu.RUnlock()

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{
		"core":       coreID,
		"collection": collection,
		"triples":    triples,
		"count":      len(triples),
	})
}

// Reset handler - resets all server state (useful for test isolation)
func (s *TrustGraphServer) handleReset(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	s.knowledgeCoresMu.Lock()
	s.knowledgeCores = make(map[string][]TGTriple)
	s.knowledgeCoresMu.Unlock()

	atomic.StoreInt64(&s.triplesQueried, 0)
	atomic.StoreInt64(&s.triplesStored, 0)
	atomic.StoreInt64(&s.ragQueries, 0)
	atomic.StoreInt64(&s.requestCount, 1) // Count this request

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{
		"status": "reset",
	})
}

// Stats returns server statistics for programmatic access.
func (s *TrustGraphServer) Stats() map[string]any {
	s.knowledgeCoresMu.RLock()
	coreCount := len(s.knowledgeCores)
	s.knowledgeCoresMu.RUnlock()

	return map[string]any{
		"triples_queried": atomic.LoadInt64(&s.triplesQueried),
		"triples_stored":  atomic.LoadInt64(&s.triplesStored),
		"rag_queries":     atomic.LoadInt64(&s.ragQueries),
		"request_count":   atomic.LoadInt64(&s.requestCount),
		"knowledge_cores": coreCount,
	}
}

// GetStoredTriples returns stored triples for a core/collection (for programmatic access).
// Returns a copy to avoid concurrent modification issues.
func (s *TrustGraphServer) GetStoredTriples(coreID, collection string) []TGTriple {
	key := fmt.Sprintf("%s:%s", coreID, collection)
	s.knowledgeCoresMu.RLock()
	defer s.knowledgeCoresMu.RUnlock()
	src := s.knowledgeCores[key]
	if src == nil {
		return nil
	}
	result := make([]TGTriple, len(src))
	copy(result, src)
	return result
}

// TriplesStored returns the total number of triples stored.
func (s *TrustGraphServer) TriplesStored() int64 {
	return atomic.LoadInt64(&s.triplesStored)
}

// TriplesQueried returns the total number of triples returned by queries.
func (s *TrustGraphServer) TriplesQueried() int64 {
	return atomic.LoadInt64(&s.triplesQueried)
}

// RAGQueries returns the number of GraphRAG queries received.
func (s *TrustGraphServer) RAGQueries() int64 {
	return atomic.LoadInt64(&s.ragQueries)
}
