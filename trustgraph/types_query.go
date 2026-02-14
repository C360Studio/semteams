package trustgraph

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
