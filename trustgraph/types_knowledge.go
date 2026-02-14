package trustgraph

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
