package client

import (
	"context"
	"fmt"
)

// PutKGCoreTriples stores triples into a TrustGraph knowledge core.
//
// Parameters:
//   - coreID: The knowledge core identifier
//   - user: The user identifier for the operation
//   - collection: The collection name within the knowledge core
//   - triples: The triples to store
//
// Example:
//
//	err := client.PutKGCoreTriples(ctx, "operational-data", "semstreams", "sensors", triples)
func (c *Client) PutKGCoreTriples(ctx context.Context, coreID, user, collection string, triples []TGTriple) error {
	req := KnowledgeRequest{
		Service: "knowledge",
		Request: KnowledgeRequestBody{
			Operation:  "put-kg-core-triples",
			ID:         coreID,
			User:       user,
			Collection: collection,
			Triples:    triples,
		},
	}

	var resp KnowledgeResponse
	if err := c.post(ctx, "/api/v1/knowledge", req, &resp); err != nil {
		return fmt.Errorf("put kg core triples: %w", err)
	}

	if resp.Error != "" {
		return &APIError{
			StatusCode: 400,
			Message:    resp.Error,
		}
	}

	return nil
}

// DeleteKGCore deletes a knowledge core.
func (c *Client) DeleteKGCore(ctx context.Context, coreID, user string) error {
	req := KnowledgeRequest{
		Service: "knowledge",
		Request: KnowledgeRequestBody{
			Operation: "delete-kg-core",
			ID:        coreID,
			User:      user,
		},
	}

	var resp KnowledgeResponse
	if err := c.post(ctx, "/api/v1/knowledge", req, &resp); err != nil {
		return fmt.Errorf("delete kg core: %w", err)
	}

	if resp.Error != "" {
		return &APIError{
			StatusCode: 400,
			Message:    resp.Error,
		}
	}

	return nil
}

// ListKGCores lists knowledge cores for a user.
func (c *Client) ListKGCores(ctx context.Context, user string) ([]string, error) {
	req := KnowledgeRequest{
		Service: "knowledge",
		Request: KnowledgeRequestBody{
			Operation: "list-kg-cores",
			User:      user,
		},
	}

	var resp KnowledgeResponse
	if err := c.post(ctx, "/api/v1/knowledge", req, &resp); err != nil {
		return nil, fmt.Errorf("list kg cores: %w", err)
	}

	if resp.Error != "" {
		return nil, &APIError{
			StatusCode: 400,
			Message:    resp.Error,
		}
	}

	// Parse response - expected to be a list of strings
	if cores, ok := resp.Response.Response.([]any); ok {
		result := make([]string, 0, len(cores))
		for _, core := range cores {
			if s, ok := core.(string); ok {
				result = append(result, s)
			}
		}
		return result, nil
	}

	return nil, nil
}
