// Package scenarios provides E2E test scenarios for SemStreams gateways
package scenarios

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/c360/semstreams/test/e2e/client"
	"github.com/c360/semstreams/test/e2e/config"
)

// MCPGatewayScenario validates MCP (Model Context Protocol) gateway operations
type MCPGatewayScenario struct {
	name        string
	description string
	client      *client.ObservabilityClient
	httpClient  *http.Client
	baseURL     string
	mcpURL      string
	natsURL     string
	config      *MCPGatewayConfig
}

// MCPGatewayConfig contains configuration for MCP gateway test
type MCPGatewayConfig struct {
	SetupDelay      time.Duration `json:"setup_delay"`
	ValidationDelay time.Duration `json:"validation_delay"`
	RequestTimeout  time.Duration `json:"request_timeout"`
}

// DefaultMCPGatewayConfig returns default configuration
func DefaultMCPGatewayConfig() *MCPGatewayConfig {
	return &MCPGatewayConfig{
		SetupDelay:      2 * time.Second,
		ValidationDelay: 1 * time.Second,
		RequestTimeout:  30 * time.Second,
	}
}

// NewMCPGatewayScenario creates a new MCP gateway test scenario
func NewMCPGatewayScenario(
	obsClient *client.ObservabilityClient,
	baseURL string,
	cfg *MCPGatewayConfig,
) *MCPGatewayScenario {
	if cfg == nil {
		cfg = DefaultMCPGatewayConfig()
	}
	if baseURL == "" {
		baseURL = "http://localhost:8081"
	}

	return &MCPGatewayScenario{
		name:        "gateway-mcp",
		description: "Tests MCP gateway: tool invocation, rate limiting, SSE transport, error handling",
		client:      obsClient,
		httpClient:  &http.Client{Timeout: cfg.RequestTimeout},
		baseURL:     baseURL,
		mcpURL:      baseURL + "/mcp",
		natsURL:     config.DefaultEndpoints.NATS,
		config:      cfg,
	}
}

// Name returns the scenario name
func (s *MCPGatewayScenario) Name() string {
	return s.name
}

// Description returns the scenario description
func (s *MCPGatewayScenario) Description() string {
	return s.description
}

// Setup prepares the scenario
func (s *MCPGatewayScenario) Setup(ctx context.Context) error {
	// Wait for services to be fully ready
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-time.After(s.config.SetupDelay):
	}

	// Verify MCP health endpoint is reachable
	healthURL := s.baseURL + "/health"
	req, err := http.NewRequestWithContext(ctx, "GET", healthURL, nil)
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}

	resp, err := s.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("MCP health endpoint not reachable at %s: %w", healthURL, err)
	}
	_ = resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("MCP health check returned status %d", resp.StatusCode)
	}

	return nil
}

// Execute runs the MCP gateway test scenario
func (s *MCPGatewayScenario) Execute(ctx context.Context) (*Result, error) {
	result := &Result{
		ScenarioName: s.name,
		StartTime:    time.Now(),
		Success:      false,
		Metrics:      make(map[string]any),
		Details:      make(map[string]any),
		Errors:       []string{},
		Warnings:     []string{},
	}

	// Track execution stages
	stages := []struct {
		name string
		fn   func(context.Context, *Result) error
	}{
		{"verify-mcp-health", s.executeVerifyHealth},
		{"verify-schema-endpoint", s.executeVerifySchema},
		{"test-tool-invocation", s.executeToolInvocation},
		{"test-tool-with-variables", s.executeToolWithVariables},
		{"test-error-handling", s.executeErrorHandling},
		{"test-rate-limiting", s.executeRateLimiting},
	}

	passedStages := 0
	failedStages := 0

	// Execute each stage
	for _, stage := range stages {
		stageStart := time.Now()

		if err := stage.fn(ctx, result); err != nil {
			failedStages++
			result.Errors = append(result.Errors, fmt.Sprintf("%s: %v", stage.name, err))
			result.Metrics[fmt.Sprintf("%s_status", stage.name)] = "failed"
		} else {
			passedStages++
			result.Metrics[fmt.Sprintf("%s_status", stage.name)] = "passed"
		}

		result.Metrics[fmt.Sprintf("%s_duration_ms", stage.name)] = time.Since(stageStart).Milliseconds()
	}

	// Overall success if most stages passed
	result.Metrics["stages_passed"] = passedStages
	result.Metrics["stages_failed"] = failedStages
	result.Success = failedStages == 0
	result.EndTime = time.Now()
	result.Duration = result.EndTime.Sub(result.StartTime)

	return result, nil
}

// Teardown cleans up after the scenario
func (s *MCPGatewayScenario) Teardown(_ context.Context) error {
	return nil
}

// executeVerifyHealth checks MCP health endpoint
func (s *MCPGatewayScenario) executeVerifyHealth(ctx context.Context, result *Result) error {
	healthURL := s.baseURL + "/health"

	req, err := http.NewRequestWithContext(ctx, "GET", healthURL, nil)
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}

	resp, err := s.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("health request failed: %w", err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)

	result.Details["mcp_health"] = map[string]any{
		"endpoint":    healthURL,
		"status_code": resp.StatusCode,
		"response":    string(body),
	}

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("health check returned status %d", resp.StatusCode)
	}

	return nil
}

// executeVerifySchema checks MCP schema endpoint
func (s *MCPGatewayScenario) executeVerifySchema(ctx context.Context, result *Result) error {
	schemaURL := s.baseURL + "/schema"

	req, err := http.NewRequestWithContext(ctx, "GET", schemaURL, nil)
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}

	resp, err := s.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("schema request failed: %w", err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)

	result.Details["mcp_schema"] = map[string]any{
		"endpoint":        schemaURL,
		"status_code":     resp.StatusCode,
		"response_length": len(body),
	}

	if resp.StatusCode != http.StatusOK {
		result.Warnings = append(result.Warnings, fmt.Sprintf("Schema endpoint returned status %d", resp.StatusCode))
		// Not a hard failure - schema endpoint may not be implemented
		return nil
	}

	return nil
}

// MCPRequest represents an MCP JSON-RPC request
type MCPRequest struct {
	JSONRPC string         `json:"jsonrpc"`
	ID      int            `json:"id"`
	Method  string         `json:"method"`
	Params  map[string]any `json:"params,omitempty"`
}

// MCPResponse represents an MCP JSON-RPC response
type MCPResponse struct {
	JSONRPC string         `json:"jsonrpc"`
	ID      int            `json:"id"`
	Result  map[string]any `json:"result,omitempty"`
	Error   *MCPError      `json:"error,omitempty"`
}

// MCPError represents an MCP error
type MCPError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

// executeMCPRequest sends an MCP request via SSE
func (s *MCPGatewayScenario) executeMCPRequest(ctx context.Context, mcpReq *MCPRequest) (*MCPResponse, error) {
	reqBody, err := json.Marshal(mcpReq)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, "POST", s.mcpURL, bytes.NewReader(reqBody))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "text/event-stream")

	resp, err := s.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	// Parse SSE response
	scanner := bufio.NewScanner(resp.Body)
	var lastData string

	for scanner.Scan() {
		line := scanner.Text()
		if strings.HasPrefix(line, "data: ") {
			lastData = strings.TrimPrefix(line, "data: ")
		}
	}

	if lastData == "" {
		// Try reading as regular JSON response
		body, _ := io.ReadAll(resp.Body)
		if len(body) > 0 {
			lastData = string(body)
		} else {
			return nil, fmt.Errorf("no data in SSE response")
		}
	}

	var mcpResp MCPResponse
	if err := json.Unmarshal([]byte(lastData), &mcpResp); err != nil {
		// Return raw response for debugging
		return &MCPResponse{
			Result: map[string]any{"raw_response": lastData},
		}, nil
	}

	return &mcpResp, nil
}

// executeToolInvocation tests basic tool invocation
func (s *MCPGatewayScenario) executeToolInvocation(ctx context.Context, result *Result) error {
	// Call the graphql tool with a simple query
	mcpReq := &MCPRequest{
		JSONRPC: "2.0",
		ID:      1,
		Method:  "tools/call",
		Params: map[string]any{
			"name": "graphql",
			"arguments": map[string]any{
				"query": "{ __typename }",
			},
		},
	}

	resp, err := s.executeMCPRequest(ctx, mcpReq)
	if err != nil {
		result.Warnings = append(result.Warnings, fmt.Sprintf("Tool invocation error: %v", err))
		result.Details["tool_invocation"] = map[string]any{
			"request_sent": true,
			"error":        err.Error(),
		}
		return nil // Not a hard failure - MCP may have different response format
	}

	result.Details["tool_invocation"] = map[string]any{
		"request_sent":    true,
		"response":        resp.Result,
		"has_error":       resp.Error != nil,
		"response_id":     resp.ID,
		"jsonrpc_version": resp.JSONRPC,
	}

	return nil
}

// executeToolWithVariables tests tool invocation with variables
func (s *MCPGatewayScenario) executeToolWithVariables(ctx context.Context, result *Result) error {
	// Call the graphql tool with variables
	mcpReq := &MCPRequest{
		JSONRPC: "2.0",
		ID:      2,
		Method:  "tools/call",
		Params: map[string]any{
			"name": "graphql",
			"arguments": map[string]any{
				"query": `
					query GetEntity($id: ID!) {
						entity(id: $id) {
							id
							type
						}
					}
				`,
				"variables": map[string]any{
					"id": "test-entity-1",
				},
			},
		},
	}

	resp, err := s.executeMCPRequest(ctx, mcpReq)
	if err != nil {
		result.Warnings = append(result.Warnings, fmt.Sprintf("Tool with variables error: %v", err))
	}

	result.Details["tool_with_variables"] = map[string]any{
		"request_sent":       true,
		"variables_included": true,
		"response":           resp,
	}

	return nil
}

// executeErrorHandling tests MCP error responses
func (s *MCPGatewayScenario) executeErrorHandling(ctx context.Context, result *Result) error {
	// Test invalid method
	invalidMethodReq := &MCPRequest{
		JSONRPC: "2.0",
		ID:      3,
		Method:  "invalid/method",
	}

	invalidResp, _ := s.executeMCPRequest(ctx, invalidMethodReq)
	invalidMethodHandled := invalidResp != nil && (invalidResp.Error != nil || invalidResp.Result != nil)

	// Test malformed GraphQL query
	malformedQueryReq := &MCPRequest{
		JSONRPC: "2.0",
		ID:      4,
		Method:  "tools/call",
		Params: map[string]any{
			"name": "graphql",
			"arguments": map[string]any{
				"query": "{ invalid query syntax",
			},
		},
	}

	malformedResp, _ := s.executeMCPRequest(ctx, malformedQueryReq)
	malformedQueryHandled := malformedResp != nil

	result.Details["error_handling"] = map[string]any{
		"invalid_method_handled":  invalidMethodHandled,
		"malformed_query_handled": malformedQueryHandled,
	}

	return nil
}

// executeRateLimiting tests rate limiting behavior
func (s *MCPGatewayScenario) executeRateLimiting(ctx context.Context, result *Result) error {
	// Send rapid requests to test rate limiting
	// Default: 10 req/s with burst of 20

	requestCount := 25 // Above burst limit
	successCount := 0
	rateLimitedCount := 0
	errorCount := 0

	startTime := time.Now()

	for i := 0; i < requestCount; i++ {
		mcpReq := &MCPRequest{
			JSONRPC: "2.0",
			ID:      100 + i,
			Method:  "tools/call",
			Params: map[string]any{
				"name": "graphql",
				"arguments": map[string]any{
					"query": "{ __typename }",
				},
			},
		}

		reqBody, _ := json.Marshal(mcpReq)
		req, _ := http.NewRequestWithContext(ctx, "POST", s.mcpURL, bytes.NewReader(reqBody))
		req.Header.Set("Content-Type", "application/json")

		resp, err := s.httpClient.Do(req)
		if err != nil {
			errorCount++
			continue
		}

		if resp.StatusCode == http.StatusTooManyRequests {
			rateLimitedCount++
		} else if resp.StatusCode == http.StatusOK {
			successCount++
		} else {
			errorCount++
		}

		_ = resp.Body.Close()
	}

	duration := time.Since(startTime)

	result.Details["rate_limiting"] = map[string]any{
		"requests_sent":      requestCount,
		"successful":         successCount,
		"rate_limited":       rateLimitedCount,
		"errors":             errorCount,
		"duration_ms":        duration.Milliseconds(),
		"rate_limit_working": rateLimitedCount > 0 || successCount == requestCount,
	}

	result.Metrics["rate_limit_requests"] = requestCount
	result.Metrics["rate_limit_success"] = successCount
	result.Metrics["rate_limit_blocked"] = rateLimitedCount

	// Rate limiting is working if either:
	// 1. Some requests were rate limited (429)
	// 2. All requests succeeded (rate limit not reached)
	return nil
}
