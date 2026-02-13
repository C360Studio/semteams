package client

import (
	"context"
	"fmt"
)

// GraphRAG sends a natural language query to TrustGraph's GraphRAG API.
//
// GraphRAG combines knowledge graph retrieval with LLM generation to answer
// questions about the document-extracted knowledge.
//
// Parameters:
//   - flowID: The TrustGraph flow configuration to use (e.g., "graph-rag")
//   - query: The natural language question to answer
//
// Returns the LLM-generated response text.
//
// Example:
//
//	response, err := client.GraphRAG(ctx, "graph-rag", "What are the maintenance procedures for pump model X?")
func (c *Client) GraphRAG(ctx context.Context, flowID, query string) (string, error) {
	req := GraphRAGRequest{
		Service: "graph-rag",
		Flow:    flowID,
		Request: GraphRAGQuery{
			Query: query,
		},
	}

	var resp GraphRAGResponse
	if err := c.post(ctx, "/api/v1/graph-rag", req, &resp); err != nil {
		return "", fmt.Errorf("graph-rag query: %w", err)
	}

	if resp.Error != "" {
		return "", &APIError{
			StatusCode: 400,
			Message:    resp.Error,
		}
	}

	return resp.Response.Response, nil
}

// GraphRAGWithCollection sends a GraphRAG query targeting a specific collection.
//
// This is a convenience method that adds collection filtering to the query.
func (c *Client) GraphRAGWithCollection(ctx context.Context, flowID, query, collection string) (string, error) {
	req := GraphRAGRequest{
		Service: "graph-rag",
		Flow:    flowID,
		Request: GraphRAGQuery{
			Query:      query,
			Collection: collection,
		},
	}

	var resp GraphRAGResponse
	if err := c.post(ctx, "/api/v1/graph-rag", req, &resp); err != nil {
		return "", fmt.Errorf("graph-rag query: %w", err)
	}

	if resp.Error != "" {
		return "", &APIError{
			StatusCode: 400,
			Message:    resp.Error,
		}
	}

	return resp.Response.Response, nil
}
