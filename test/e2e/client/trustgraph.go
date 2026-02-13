// Package client provides test utilities for SemStreams E2E tests.
package client

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// TrustGraphMockClient provides HTTP client for testing the TrustGraph mock server.
type TrustGraphMockClient struct {
	baseURL    string
	httpClient *http.Client
}

// NewTrustGraphMockClient creates a new client for the TrustGraph mock server.
func NewTrustGraphMockClient(baseURL string) *TrustGraphMockClient {
	return &TrustGraphMockClient{
		baseURL: strings.TrimSuffix(baseURL, "/"),
		httpClient: &http.Client{
			Timeout: 10 * time.Second,
		},
	}
}

// TrustGraphStats contains server statistics from the mock.
type TrustGraphStats struct {
	TriplesQueried int64 `json:"triples_queried"`
	TriplesStored  int64 `json:"triples_stored"`
	RAGQueries     int64 `json:"rag_queries"`
	RequestCount   int64 `json:"request_count"`
	ImportTriples  int   `json:"import_triples"`
	KnowledgeCores int   `json:"knowledge_cores"`
	StoredTotal    int   `json:"stored_total"`
}

// TrustGraphTriple represents a triple in the mock server.
type TrustGraphTriple struct {
	S TrustGraphValue `json:"s"`
	P TrustGraphValue `json:"p"`
	O TrustGraphValue `json:"o"`
}

// TrustGraphValue represents a value in a triple.
type TrustGraphValue struct {
	V string `json:"v"` // Value (URI or literal)
	E bool   `json:"e"` // Is entity (true = URI, false = literal)
}

// StoredTriplesResponse is the response from /stored/{core}/{collection}.
type StoredTriplesResponse struct {
	Core       string             `json:"core"`
	Collection string             `json:"collection"`
	Triples    []TrustGraphTriple `json:"triples"`
	Count      int                `json:"count"`
}

// Health checks if the TrustGraph mock server is healthy.
func (c *TrustGraphMockClient) Health(ctx context.Context) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+"/health", nil)
	if err != nil {
		return err
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("health check failed: %s", resp.Status)
	}

	return nil
}

// GetStats retrieves statistics from the mock server.
func (c *TrustGraphMockClient) GetStats(ctx context.Context) (*TrustGraphStats, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+"/stats", nil)
	if err != nil {
		return nil, err
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("stats request failed: %s - %s", resp.Status, string(body))
	}

	var stats TrustGraphStats
	if err := json.NewDecoder(resp.Body).Decode(&stats); err != nil {
		return nil, fmt.Errorf("failed to decode stats: %w", err)
	}

	return &stats, nil
}

// GetStoredTriples retrieves stored triples for a specific core and collection.
func (c *TrustGraphMockClient) GetStoredTriples(ctx context.Context, coreID, collection string) (*StoredTriplesResponse, error) {
	url := fmt.Sprintf("%s/stored/%s/%s", c.baseURL, coreID, collection)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("get stored triples failed: %s - %s", resp.Status, string(body))
	}

	var result StoredTriplesResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("failed to decode stored triples: %w", err)
	}

	return &result, nil
}

// Reset resets all state in the mock server.
func (c *TrustGraphMockClient) Reset(ctx context.Context) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/reset", nil)
	if err != nil {
		return err
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("reset failed: %s - %s", resp.Status, string(body))
	}

	return nil
}

// WaitForStored waits until at least minCount triples are stored in the specified core/collection.
func (c *TrustGraphMockClient) WaitForStored(
	ctx context.Context,
	coreID, collection string,
	minCount int,
	timeout time.Duration,
) (*StoredTriplesResponse, error) {
	const pollInterval = 500 * time.Millisecond
	deadline := time.Now().Add(timeout)

	for time.Now().Before(deadline) {
		stored, err := c.GetStoredTriples(ctx, coreID, collection)
		if err != nil {
			return nil, err
		}
		if stored.Count >= minCount {
			return stored, nil
		}

		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(pollInterval):
		}
	}

	// Return final state even if count not reached
	return c.GetStoredTriples(ctx, coreID, collection)
}

// WaitForQueried waits until at least minCount triples have been queried.
func (c *TrustGraphMockClient) WaitForQueried(
	ctx context.Context,
	minCount int64,
	timeout time.Duration,
) (*TrustGraphStats, error) {
	const pollInterval = 500 * time.Millisecond
	deadline := time.Now().Add(timeout)

	for time.Now().Before(deadline) {
		stats, err := c.GetStats(ctx)
		if err != nil {
			return nil, err
		}
		if stats.TriplesQueried >= minCount {
			return stats, nil
		}

		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(pollInterval):
		}
	}

	// Return final stats even if count not reached
	return c.GetStats(ctx)
}

// ContainsTrustGraphURI checks if any triple in the list contains a TrustGraph URI.
// This is used to verify loop prevention (imported entities shouldn't be re-exported).
func ContainsTrustGraphURI(triples []TrustGraphTriple) bool {
	for _, t := range triples {
		if strings.Contains(t.S.V, "trustgraph.ai") {
			return true
		}
		if strings.Contains(t.P.V, "trustgraph.ai") {
			return true
		}
		if t.O.E && strings.Contains(t.O.V, "trustgraph.ai") {
			return true
		}
	}
	return false
}
