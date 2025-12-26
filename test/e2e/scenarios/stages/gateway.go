package stages

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// GatewayTester handles HTTP gateway testing
type GatewayTester struct {
	GatewayURL string
	Timeout    time.Duration
}

// GatewayTestResult contains results from gateway testing
type GatewayTestResult struct {
	Endpoint    string `json:"endpoint"`
	StatusCode  int    `json:"status_code"`
	LatencyMs   int64  `json:"latency_ms"`
	HitCount    int    `json:"hit_count"`
	Success     bool   `json:"success"`
	Error       string `json:"error,omitempty"`
	RawResponse string `json:"raw_response,omitempty"`
}

// SearchRequest represents a gateway search request
type SearchRequest struct {
	Query     string  `json:"query"`
	Threshold float64 `json:"threshold"`
	Limit     int     `json:"limit"`
}

// TestSemanticSearch tests the semantic search endpoint
func (g *GatewayTester) TestSemanticSearch(ctx context.Context, req SearchRequest) (*GatewayTestResult, error) {
	if g.Timeout == 0 {
		g.Timeout = 10 * time.Second
	}

	httpClient := &http.Client{Timeout: g.Timeout}
	url := g.GatewayURL + "/search/semantic"

	result := &GatewayTestResult{
		Endpoint: url,
	}

	queryJSON, err := json.Marshal(req)
	if err != nil {
		result.Error = fmt.Sprintf("failed to marshal search query: %v", err)
		return result, nil
	}

	httpReq, err := http.NewRequestWithContext(ctx, "POST", url, strings.NewReader(string(queryJSON)))
	if err != nil {
		result.Error = fmt.Sprintf("failed to create request: %v", err)
		return result, nil
	}
	httpReq.Header.Set("Content-Type", "application/json")

	startTime := time.Now()
	resp, err := httpClient.Do(httpReq)
	if err != nil {
		result.Error = fmt.Sprintf("request failed: %v", err)
		return result, nil
	}
	defer resp.Body.Close()

	result.LatencyMs = time.Since(startTime).Milliseconds()
	result.StatusCode = resp.StatusCode

	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		result.Error = fmt.Sprintf("failed to read response: %v", err)
		return result, nil
	}

	if resp.StatusCode != http.StatusOK {
		result.Error = fmt.Sprintf("status %d: %s", resp.StatusCode, string(bodyBytes))
		return result, nil
	}

	// Parse response structure
	var searchResult struct {
		Data struct {
			Query string `json:"query"`
			Hits  []struct {
				EntityID string  `json:"entity_id"`
				Score    float64 `json:"score"`
			} `json:"hits"`
		} `json:"data"`
		Error string `json:"error"`
	}

	if err := json.Unmarshal(bodyBytes, &searchResult); err != nil {
		result.Error = fmt.Sprintf("failed to parse response: %v", err)
		result.RawResponse = string(bodyBytes)
		return result, nil
	}

	if searchResult.Error != "" {
		result.Error = fmt.Sprintf("search error: %s", searchResult.Error)
		return result, nil
	}

	result.HitCount = len(searchResult.Data.Hits)
	result.Success = true

	return result, nil
}

// EmbeddingFallbackResult contains results from embedding fallback testing
type EmbeddingFallbackResult struct {
	SemembedAvailable bool   `json:"semembed_available"`
	GraphHealthy      bool   `json:"graph_healthy"`
	FallbackMode      bool   `json:"fallback_mode"`
	HybridMode        bool   `json:"hybrid_mode"`
	Message           string `json:"message"`
}

// MetricsTestResult contains results from metrics endpoint testing
type MetricsTestResult struct {
	Endpoint      string          `json:"endpoint"`
	StatusCode    int             `json:"status_code"`
	Success       bool            `json:"success"`
	MetricsFound  map[string]bool `json:"metrics_found"`
	FoundCount    int             `json:"found_count"`
	ExpectedCount int             `json:"expected_count"`
	Error         string          `json:"error,omitempty"`
}

// MetricsTester handles metrics endpoint validation
type MetricsTester struct {
	MetricsURL string
}

// ExpectedMetrics returns the standard metrics to verify
func ExpectedMetrics() []string {
	return []string{
		"semstreams_datamanager_entities_updated_total",
		"semstreams_rule_evaluations_total",
		"semstreams_rule_triggers_total",
		"semstreams_messages_total",
	}
}

// TestMetricsEndpoint validates the Prometheus metrics endpoint
func (m *MetricsTester) TestMetricsEndpoint(ctx context.Context, expected []string) (*MetricsTestResult, error) {
	url := m.MetricsURL + "/metrics"
	result := &MetricsTestResult{
		Endpoint:      url,
		ExpectedCount: len(expected),
		MetricsFound:  make(map[string]bool),
	}

	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		result.Error = fmt.Sprintf("failed to create request: %v", err)
		return result, nil
	}

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		result.Error = fmt.Sprintf("request failed: %v", err)
		return result, nil
	}
	defer resp.Body.Close()

	result.StatusCode = resp.StatusCode

	if resp.StatusCode != http.StatusOK {
		result.Error = fmt.Sprintf("unexpected status: %d", resp.StatusCode)
		return result, nil
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		result.Error = fmt.Sprintf("failed to read response: %v", err)
		return result, nil
	}

	metricsText := string(body)
	for _, metric := range expected {
		found := strings.Contains(metricsText, metric)
		result.MetricsFound[metric] = found
		if found {
			result.FoundCount++
		}
	}

	result.Success = result.FoundCount >= len(expected)/2 // At least half expected metrics

	return result, nil
}
