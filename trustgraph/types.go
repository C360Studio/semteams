package trustgraph

// TGTriple is TrustGraph's compact triple representation.
// This is the wire format used in TrustGraph REST APIs.
type TGTriple struct {
	S TGValue `json:"s"`
	P TGValue `json:"p"`
	O TGValue `json:"o"`
}

// TGValue is a compact value with entity flag.
// When E is true, V contains a URI (entity reference).
// When E is false, V contains a literal value.
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

// TriplesQueryRequest is the request body for the triples-query API.
type TriplesQueryRequest struct {
	ID      string             `json:"id,omitempty"`
	Service string             `json:"service"`
	Flow    string             `json:"flow,omitempty"`
	Request TriplesQueryParams `json:"request"`
}

// TriplesQueryParams contains the parameters for a triples query.
type TriplesQueryParams struct {
	S     *TGValue `json:"s,omitempty"`     // Subject filter
	P     *TGValue `json:"p,omitempty"`     // Predicate filter
	O     *TGValue `json:"o,omitempty"`     // Object filter
	Limit int      `json:"limit,omitempty"` // Max triples to return
}

// TriplesQueryResponse is the response from the triples-query API.
type TriplesQueryResponse struct {
	ID       string `json:"id,omitempty"`
	Error    string `json:"error,omitempty"`
	Response struct {
		Response []TGTriple `json:"response"`
	} `json:"response"`
	Complete bool `json:"complete"`
}

// GraphRAGRequest is the request body for the graph-rag API.
type GraphRAGRequest struct {
	ID      string        `json:"id,omitempty"`
	Service string        `json:"service"`
	Flow    string        `json:"flow,omitempty"`
	Request GraphRAGQuery `json:"request"`
}

// GraphRAGQuery contains the parameters for a GraphRAG query.
type GraphRAGQuery struct {
	Query      string `json:"query"`
	Collection string `json:"collection,omitempty"`
}

// GraphRAGResponse is the response from the graph-rag API.
type GraphRAGResponse struct {
	ID       string `json:"id,omitempty"`
	Error    string `json:"error,omitempty"`
	Response struct {
		Response string `json:"response"`
	} `json:"response"`
	Complete bool `json:"complete"`
}

// KnowledgeRequest is the request body for the knowledge API.
type KnowledgeRequest struct {
	ID      string               `json:"id,omitempty"`
	Service string               `json:"service"`
	Request KnowledgeRequestBody `json:"request"`
}

// KnowledgeRequestBody contains the parameters for a knowledge API request.
type KnowledgeRequestBody struct {
	Operation  string     `json:"operation"`         // e.g., "put-kg-core-triples"
	ID         string     `json:"id"`                // Knowledge core ID
	User       string     `json:"user"`              // User identifier
	Collection string     `json:"collection"`        // Collection name
	Triples    []TGTriple `json:"triples,omitempty"` // Triples to store
	Query      string     `json:"query,omitempty"`   // For query operations
	Limit      int        `json:"limit,omitempty"`   // For query operations
}

// KnowledgeResponse is the response from the knowledge API.
type KnowledgeResponse struct {
	ID       string `json:"id,omitempty"`
	Error    string `json:"error,omitempty"`
	Response struct {
		Response any `json:"response"` // Response varies by operation
	} `json:"response"`
	Complete bool `json:"complete"`
}

// APIError represents an error from the TrustGraph API.
type APIError struct {
	StatusCode int
	Message    string
	RetryAfter int // Seconds to wait before retrying (for 429 responses)
}

// Error implements the error interface.
func (e *APIError) Error() string {
	if e.Message != "" {
		return e.Message
	}
	return "TrustGraph API error"
}

// IsRetryable returns true if the error suggests the request should be retried.
func (e *APIError) IsRetryable() bool {
	return e.StatusCode >= 500 || e.StatusCode == 429
}
