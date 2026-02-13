// Package trustgraph provides an HTTP client for TrustGraph REST APIs.
//
// The client handles communication with TrustGraph's three main API endpoints:
//   - Triples Query API: Query RDF triples from the knowledge graph
//   - Knowledge API: Store triples into knowledge cores
//   - GraphRAG API: Natural language queries over the knowledge graph
//
// # Features
//
//   - Automatic retry with exponential backoff for 5xx errors
//   - Respect for Retry-After headers on rate limiting (429)
//   - Configurable timeouts per request
//   - Context cancellation support
//   - Optional API key authentication
//
// # Usage
//
//	client := trustgraph.New(trustgraph.Config{
//	    Endpoint: "http://localhost:8088",
//	    Timeout:  30 * time.Second,
//	})
//
//	// Query triples
//	triples, err := client.QueryTriples(ctx, trustgraph.TriplesQueryParams{
//	    Subject: &trustgraph.TGValue{V: "http://example.org/entity", E: true},
//	    Limit:   100,
//	})
//
//	// Store triples
//	err := client.PutKGCoreTriples(ctx, "core-id", "user", "collection", triples)
//
//	// GraphRAG query
//	response, err := client.GraphRAG(ctx, "flow-id", "What sensors are in zone 7?")
package trustgraph
