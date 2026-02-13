package client

import (
	"context"
	"fmt"
)

// QueryTriples queries triples from the TrustGraph triples-query API.
//
// The params can filter by subject, predicate, and/or object. At least one
// filter should typically be provided for efficiency.
//
// Example:
//
//	triples, err := client.QueryTriples(ctx, TriplesQueryParams{
//	    Subject: &TGValue{V: "http://example.org/entity", E: true},
//	    Limit:   100,
//	})
func (c *Client) QueryTriples(ctx context.Context, params TriplesQueryParams) ([]TGTriple, error) {
	req := TriplesQueryRequest{
		Service: "triples-query",
		Request: params,
	}

	var resp TriplesQueryResponse
	if err := c.post(ctx, "/api/v1/triples-query", req, &resp); err != nil {
		return nil, fmt.Errorf("triples query: %w", err)
	}

	if resp.Error != "" {
		return nil, &APIError{
			StatusCode: 400,
			Message:    resp.Error,
		}
	}

	return resp.Response.Response, nil
}

// QueryTriplesBySubject is a convenience method to query all triples for a subject.
func (c *Client) QueryTriplesBySubject(ctx context.Context, subjectURI string, limit int) ([]TGTriple, error) {
	return c.QueryTriples(ctx, TriplesQueryParams{
		S:     &TGValue{V: subjectURI, E: true},
		Limit: limit,
	})
}

// QueryTriplesByPredicate is a convenience method to query all triples with a predicate.
func (c *Client) QueryTriplesByPredicate(ctx context.Context, predicateURI string, limit int) ([]TGTriple, error) {
	return c.QueryTriples(ctx, TriplesQueryParams{
		P:     &TGValue{V: predicateURI, E: true},
		Limit: limit,
	})
}

// QueryAllTriples queries all triples with an optional limit.
// Use with caution on large knowledge graphs.
func (c *Client) QueryAllTriples(ctx context.Context, limit int) ([]TGTriple, error) {
	return c.QueryTriples(ctx, TriplesQueryParams{
		Limit: limit,
	})
}
