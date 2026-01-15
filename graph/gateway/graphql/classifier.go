package graphql

import "context"

// QueryClassifier analyzes natural language queries and populates SearchOptions.
// Implementations may use various techniques (regex, embeddings, LLM) to extract
// temporal, spatial, and intent information from query text.
type QueryClassifier interface {
	// ClassifyQuery analyzes a query string and returns populated SearchOptions.
	// The original query text is preserved in SearchOptions.Query.
	// Returns non-nil SearchOptions even for empty queries.
	ClassifyQuery(ctx context.Context, query string) *SearchOptions
}
