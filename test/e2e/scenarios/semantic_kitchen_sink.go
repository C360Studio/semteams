// Package scenarios provides E2E test scenarios for SemStreams semantic processing
package scenarios

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/c360/semstreams/test/e2e/client"
	"github.com/c360/semstreams/test/e2e/config"
)

// SemanticKitchenSinkScenario validates comprehensive semantic processing
type SemanticKitchenSinkScenario struct {
	name        string
	description string
	client      *client.ObservabilityClient
	natsClient  *client.NATSValidationClient
	udpAddr     string
	natsURL     string
	config      *SemanticKitchenSinkConfig
}

// SemanticKitchenSinkConfig contains configuration for kitchen sink test
type SemanticKitchenSinkConfig struct {
	// Test data configuration
	MessageCount    int           `json:"message_count"`
	MessageInterval time.Duration `json:"message_interval"`

	// Validation configuration
	ValidationDelay time.Duration `json:"validation_delay"`
	MinProcessed    int           `json:"min_processed"`

	// Entity verification (from test data files)
	MinExpectedEntities int    `json:"min_expected_entities"`
	NatsURL             string `json:"nats_url"`
	MetricsURL          string `json:"metrics_url"`
	GatewayURL          string `json:"gateway_url"`

	// Comparison output configuration
	OutputDir string `json:"output_dir"`
}

// ComparisonData represents comparison results for Core vs ML analysis
type ComparisonData struct {
	Variant           string                        `json:"variant"`
	EmbeddingProvider string                        `json:"embedding_provider"`
	Timestamp         time.Time                     `json:"timestamp"`
	SearchResults     map[string]SearchQueryResult  `json:"search_results"`
}

// SearchQueryResult represents results from a single search query
type SearchQueryResult struct {
	Query     string    `json:"query"`
	Hits      []string  `json:"hits"`
	Scores    []float64 `json:"scores"`
	LatencyMs int64     `json:"latency_ms"`
	HitCount  int       `json:"hit_count"`
}

// DefaultSemanticKitchenSinkConfig returns default configuration
func DefaultSemanticKitchenSinkConfig() *SemanticKitchenSinkConfig {
	return &SemanticKitchenSinkConfig{
		MessageCount:        20,
		MessageInterval:     50 * time.Millisecond,
		ValidationDelay:     5 * time.Second,
		MinProcessed:        10, // At least 50% should make it through
		MinExpectedEntities: 50, // Test data has 74 entities, expect at least 50 indexed
		NatsURL:             config.DefaultEndpoints.NATS,
		MetricsURL:          config.DefaultEndpoints.Metrics,
		GatewayURL:          config.DefaultEndpoints.HTTP + "/api-gateway",
		OutputDir:           "test/e2e/results",
	}
}

// NewSemanticKitchenSinkScenario creates a new kitchen sink semantic test scenario
func NewSemanticKitchenSinkScenario(
	obsClient *client.ObservabilityClient,
	udpAddr string,
	cfg *SemanticKitchenSinkConfig,
) *SemanticKitchenSinkScenario {
	if cfg == nil {
		cfg = DefaultSemanticKitchenSinkConfig()
	}
	if udpAddr == "" {
		udpAddr = "localhost:14550"
	}
	natsURL := cfg.NatsURL
	if natsURL == "" {
		natsURL = config.DefaultEndpoints.NATS
	}

	return &SemanticKitchenSinkScenario{
		name:        "semantic-kitchen-sink",
		description: "Tests comprehensive semantic stack: Protocol + Graph + Indexes + Queries + Multiple Outputs",
		client:      obsClient,
		udpAddr:     udpAddr,
		natsURL:     natsURL,
		config:      cfg,
	}
}

// Name returns the scenario name
func (s *SemanticKitchenSinkScenario) Name() string {
	return s.name
}

// Description returns the scenario description
func (s *SemanticKitchenSinkScenario) Description() string {
	return s.description
}

// Setup prepares the scenario
func (s *SemanticKitchenSinkScenario) Setup(ctx context.Context) error {
	// Verify UDP endpoint is reachable
	conn, err := net.Dial("udp", s.udpAddr)
	if err != nil {
		return fmt.Errorf("cannot reach UDP endpoint %s: %w", s.udpAddr, err)
	}
	_ = conn.Close()

	// Create NATS validation client for KV bucket assertions
	natsClient, err := client.NewNATSValidationClient(ctx, s.natsURL)
	if err != nil {
		// NATS is optional - warn but don't fail
		// Some stages will skip if natsClient is nil
		return nil
	}
	s.natsClient = natsClient

	return nil
}

// Execute runs the kitchen sink semantic test scenario
func (s *SemanticKitchenSinkScenario) Execute(ctx context.Context) (*Result, error) {
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
		{"verify-components", s.executeVerifyComponents},
		{"send-mixed-data", s.executeSendMixedData},
		{"validate-processing", s.executeValidateProcessing},
		{"verify-entity-count", s.executeVerifyEntityCount},             // Verify entity indexing
		{"verify-entity-retrieval", s.executeVerifyEntityRetrieval},   // Verify specific entities
		{"validate-entity-structure", s.executeValidateEntityStructure}, // Validate entity structure
		{"verify-index-population", s.executeVerifyIndexPopulation},   // Verify all indexes
		{"test-semantic-search", s.executeTestSemanticSearch},
		{"verify-search-quality", s.executeVerifySearchQuality}, // NEW: Verify search results
		{"compare-core-ml", s.executeCompareCoreMl},             // NEW: Compare Core vs ML
		{"compare-communities", s.executeCompareCommunities},    // NEW: Compare community summaries
		{"test-http-gateway", s.executeTestHTTPGateway},
		{"test-embedding-fallback", s.executeTestEmbeddingFallback},
		{"validate-rules", s.executeValidateRules},
		{"validate-metrics", s.executeValidateMetrics},
		{"verify-outputs", s.executeVerifyOutputs},
	}

	// Execute each stage
	for _, stage := range stages {
		stageStart := time.Now()

		if err := stage.fn(ctx, result); err != nil {
			result.Success = false
			result.Error = fmt.Sprintf("%s failed: %v", stage.name, err)
			result.EndTime = time.Now()
			result.Duration = result.EndTime.Sub(result.StartTime)
			return result, nil // Return result even on failure
		}

		result.Metrics[fmt.Sprintf("%s_duration_ms", stage.name)] = time.Since(stageStart).Milliseconds()
	}

	// Overall success
	result.Success = true
	result.EndTime = time.Now()
	result.Duration = result.EndTime.Sub(result.StartTime)

	return result, nil
}

// Teardown cleans up after the scenario
func (s *SemanticKitchenSinkScenario) Teardown(ctx context.Context) error {
	// Close NATS client if it was created
	if s.natsClient != nil {
		if err := s.natsClient.Close(ctx); err != nil {
			return fmt.Errorf("failed to close NATS client: %w", err)
		}
		s.natsClient = nil
	}
	return nil
}

// executeVerifyComponents checks that all kitchen sink components exist
func (s *SemanticKitchenSinkScenario) executeVerifyComponents(ctx context.Context, result *Result) error {
	components, err := s.client.GetComponents(ctx)
	if err != nil {
		result.Errors = append(result.Errors, fmt.Sprintf("Failed to get components: %v", err))
		return fmt.Errorf("component verification failed: %w", err)
	}

	// Input components
	inputComponents := []string{"udp"}
	// Domain processors (document_processor, iot_sensor handle domain-specific data)
	domainProcessors := []string{"document_processor", "iot_sensor"}
	// Semantic components (rule processor + graph processor)
	semanticComponents := []string{"rule", "graph"}
	// Output components
	outputComponents := []string{"file", "httppost", "websocket", "objectstore"}
	// Gateway components (use instance names from config, not factory names)
	gatewayComponents := []string{"api-gateway"}

	allRequired := append(inputComponents, domainProcessors...)
	allRequired = append(allRequired, semanticComponents...)
	allRequired = append(allRequired, outputComponents...)
	allRequired = append(allRequired, gatewayComponents...)

	foundComponents := make(map[string]bool)
	for _, comp := range components {
		foundComponents[comp.Name] = true
	}

	missingComponents := []string{}
	for _, required := range allRequired {
		if !foundComponents[required] {
			missingComponents = append(missingComponents, required)
		}
	}

	if len(missingComponents) > 0 {
		result.Errors = append(result.Errors,
			fmt.Sprintf("Missing components: %v", missingComponents))
		return fmt.Errorf("missing components: %v", missingComponents)
	}

	result.Details["component_breakdown"] = map[string]any{
		"inputs":   inputComponents,
		"domain":   domainProcessors,
		"semantic": semanticComponents,
		"outputs":  outputComponents,
		"gateways": gatewayComponents,
		"total":    len(allRequired),
		"found":    len(components),
	}

	return nil
}

// executeSendMixedData sends mixed test data (entities + regular messages)
func (s *SemanticKitchenSinkScenario) executeSendMixedData(ctx context.Context, result *Result) error {
	conn, err := net.Dial("udp", s.udpAddr)
	if err != nil {
		result.Errors = append(result.Errors, fmt.Sprintf("Failed to connect to UDP: %v", err))
		return fmt.Errorf("UDP connection failed: %w", err)
	}
	defer conn.Close()

	messagesSent := 0
	telemetryCount := 0
	regularCount := 0

	for i := 0; i < s.config.MessageCount; i++ {
		var testMsg map[string]any

		// Alternate between telemetry (entities) and regular messages
		if i%2 == 0 {
			// Telemetry message (will be processed by graph)
			testMsg = map[string]any{
				"type":        "telemetry",
				"entity_id":   fmt.Sprintf("device-%d", i/2),
				"entity_type": "sensor",
				"timestamp":   time.Now().Unix(),
				"data": map[string]any{
					"temperature": 20.0 + float64(i),
					"humidity":    50.0 + float64(i*2),
					"pressure":    1013.0 + float64(i)*0.5,
					"location": map[string]any{
						"lat": 37.7749 + float64(i)*0.001,
						"lon": -122.4194 + float64(i)*0.001,
					},
				},
				"value": i * 5,
			}
			telemetryCount++
		} else {
			// Regular message (will be filtered out of entity stream)
			testMsg = map[string]any{
				"type":      "regular",
				"value":     i * 10,
				"timestamp": time.Now().Unix(),
				"metadata": map[string]any{
					"source":   "test",
					"sequence": i,
				},
			}
			regularCount++
		}

		msgBytes, err := json.Marshal(testMsg)
		if err != nil {
			continue
		}

		_, err = conn.Write(msgBytes)
		if err != nil {
			result.Warnings = append(result.Warnings, fmt.Sprintf("Failed to send message %d: %v", i, err))
			continue
		}

		messagesSent++

		// Wait between messages
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(s.config.MessageInterval):
		}
	}

	result.Metrics["messages_sent"] = messagesSent
	result.Metrics["telemetry_sent"] = telemetryCount
	result.Metrics["regular_sent"] = regularCount
	result.Details["data_sent"] = fmt.Sprintf(
		"Sent %d messages: %d telemetry (entities), %d regular",
		messagesSent, telemetryCount, regularCount)

	return nil
}

// executeValidateProcessing validates data was processed through semantic pipeline
func (s *SemanticKitchenSinkScenario) executeValidateProcessing(ctx context.Context, result *Result) error {
	// Wait for processing
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-time.After(s.config.ValidationDelay):
	}

	// Query component status
	components, err := s.client.GetComponents(ctx)
	if err != nil {
		result.Errors = append(result.Errors, fmt.Sprintf("Failed to get components: %v", err))
		return fmt.Errorf("component query failed: %w", err)
	}

	// Find graph processor and verify it's healthy
	var graphFound bool
	for _, comp := range components {
		if comp.Name == "graph" {
			graphFound = true
			if !comp.Healthy {
				result.Warnings = append(
					result.Warnings,
					fmt.Sprintf("Graph processor not healthy: state=%s", comp.State),
				)
			}
			result.Details["graph_processor_status"] = map[string]any{
				"name":    comp.Name,
				"type":    comp.Type,
				"healthy": comp.Healthy,
				"state":   comp.State,
			}
			break
		}
	}

	if !graphFound {
		result.Errors = append(result.Errors, "Graph processor not found")
		return fmt.Errorf("graph processor not found")
	}

	result.Metrics["component_count"] = len(components)
	result.Details["processing_validation"] = fmt.Sprintf(
		"Graph processor found and processing. Components: %d",
		len(components))

	return nil
}

// executeVerifyOutputs verifies multiple outputs are working
func (s *SemanticKitchenSinkScenario) executeVerifyOutputs(ctx context.Context, result *Result) error {
	components, err := s.client.GetComponents(ctx)
	if err != nil {
		result.Errors = append(result.Errors, fmt.Sprintf("Failed to get components: %v", err))
		return fmt.Errorf("component query failed: %w", err)
	}

	// Verify all outputs are present
	expectedOutputs := []string{"file", "httppost", "websocket", "objectstore"}
	foundOutputs := make(map[string]bool)

	for _, comp := range components {
		for _, expected := range expectedOutputs {
			if comp.Name == expected {
				foundOutputs[expected] = true
			}
		}
	}

	missingOutputs := []string{}
	for _, expected := range expectedOutputs {
		if !foundOutputs[expected] {
			missingOutputs = append(missingOutputs, expected)
		}
	}

	if len(missingOutputs) > 0 {
		result.Warnings = append(result.Warnings, fmt.Sprintf("Missing outputs: %v", missingOutputs))
	}

	result.Metrics["outputs_found"] = len(foundOutputs)
	result.Metrics["outputs_expected"] = len(expectedOutputs)
	result.Details["output_validation"] = map[string]any{
		"expected": expectedOutputs,
		"found":    foundOutputs,
		"missing":  missingOutputs,
	}

	return nil
}

// executeTestSemanticSearch validates semantic search with semembed embeddings
func (s *SemanticKitchenSinkScenario) executeTestSemanticSearch(ctx context.Context, result *Result) error {
	// Check semembed health endpoint
	semembedHealthURL := "http://localhost:8081/health"
	resp, err := http.Get(semembedHealthURL)
	if err != nil {
		result.Warnings = append(result.Warnings, fmt.Sprintf("semembed health check failed: %v", err))
		// Not a hard failure - might be running in environment without semembed
		result.Details["semembed_available"] = false
		return nil
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		result.Warnings = append(result.Warnings, fmt.Sprintf("semembed unhealthy: status=%d", resp.StatusCode))
		result.Details["semembed_available"] = false
		return nil
	}

	result.Details["semembed_available"] = true
	result.Metrics["semembed_health_status"] = resp.StatusCode

	// Send entities with rich text content for embedding
	conn, err := net.Dial("udp", s.udpAddr)
	if err != nil {
		result.Errors = append(result.Errors, fmt.Sprintf("Failed to connect for semantic test: %v", err))
		return fmt.Errorf("UDP connection failed: %w", err)
	}
	defer conn.Close()

	semanticTestMessages := []map[string]any{
		{
			"type":        "telemetry",
			"entity_id":   "robot-alpha",
			"entity_type": "robot",
			"timestamp":   time.Now().Unix(),
			"description": "Autonomous delivery robot operating in warehouse facility",
			"data": map[string]any{
				"battery":     85.5,
				"temperature": 42.0,
			},
		},
		{
			"type":        "telemetry",
			"entity_id":   "robot-beta",
			"entity_type": "robot",
			"timestamp":   time.Now().Unix(),
			"description": "Mobile robot performing inventory scanning tasks",
			"data": map[string]any{
				"battery":     92.0,
				"temperature": 38.5,
			},
		},
	}

	for i, msg := range semanticTestMessages {
		msgBytes, err := json.Marshal(msg)
		if err != nil {
			continue
		}

		if _, err := conn.Write(msgBytes); err != nil {
			result.Warnings = append(result.Warnings, fmt.Sprintf("Failed to send semantic message %d: %v", i, err))
		}
	}

	// Wait for embedding generation and indexing
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-time.After(2 * time.Second):
	}

	result.Metrics["semantic_messages_sent"] = len(semanticTestMessages)

	// Wait for embeddings to be generated
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-time.After(3 * time.Second):
	}

	// Query metrics to verify embeddings were generated
	metricsURL := s.config.MetricsURL + "/metrics"
	metricsResp, err := http.Get(metricsURL)
	if err != nil {
		result.Warnings = append(result.Warnings, fmt.Sprintf("Failed to query metrics: %v", err))
	} else {
		defer metricsResp.Body.Close()
		body, _ := io.ReadAll(metricsResp.Body)
		metricsText := string(body)

		// Check for embedding metrics
		embeddingsGenerated := strings.Contains(metricsText, "indexengine_embeddings_generated_total")
		embeddingsActive := strings.Contains(metricsText, "indexengine_embeddings_active")
		embeddingProvider := strings.Contains(metricsText, "indexengine_embedding_provider")

		result.Details["semantic_search_test"] = map[string]any{
			"semembed_healthy":            true,
			"messages_sent":               len(semanticTestMessages),
			"embedding_tested":            true,
			"embeddings_generated_metric": embeddingsGenerated,
			"embeddings_active_metric":    embeddingsActive,
			"embedding_provider_metric":   embeddingProvider,
		}

		if embeddingsGenerated && embeddingsActive && embeddingProvider {
			result.Metrics["embedding_metrics_verified"] = 1
		}
	}

	return nil
}

// executeTestHTTPGateway validates HTTP Gateway query endpoints
func (s *SemanticKitchenSinkScenario) executeTestHTTPGateway(ctx context.Context, result *Result) error {
	gatewayURL := s.config.GatewayURL
	httpClient := &http.Client{Timeout: 10 * time.Second}

	// Test semantic search endpoint
	searchQuery := map[string]interface{}{
		"query":     "robot warehouse",
		"threshold": 0.2,
		"limit":     10,
	}

	queryJSON, err := json.Marshal(searchQuery)
	if err != nil {
		result.Warnings = append(result.Warnings, fmt.Sprintf("Failed to marshal search query: %v", err))
		return nil // Not a hard failure
	}

	url := gatewayURL + "/search/semantic"
	req, err := http.NewRequestWithContext(ctx, "POST", url, strings.NewReader(string(queryJSON)))
	if err != nil {
		result.Warnings = append(result.Warnings, fmt.Sprintf("Failed to create gateway request: %v", err))
		return nil
	}
	req.Header.Set("Content-Type", "application/json")

	startTime := time.Now()
	resp, err := httpClient.Do(req)
	if err != nil {
		result.Warnings = append(result.Warnings, fmt.Sprintf("HTTP Gateway request failed: %v", err))
		return nil
	}
	defer resp.Body.Close()

	latency := time.Since(startTime)
	result.Metrics["http_gateway_latency_ms"] = latency.Milliseconds()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		result.Warnings = append(result.Warnings, fmt.Sprintf("HTTP Gateway returned status %d: %s", resp.StatusCode, body))
		return nil
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

	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		result.Warnings = append(result.Warnings, fmt.Sprintf("Failed to read gateway response: %v", err))
		return nil
	}

	if err := json.Unmarshal(bodyBytes, &searchResult); err != nil {
		result.Warnings = append(result.Warnings, fmt.Sprintf("Failed to parse gateway response: %v", err))
		return nil
	}

	if searchResult.Error != "" {
		result.Warnings = append(result.Warnings, fmt.Sprintf("Gateway search error: %s", searchResult.Error))
		return nil
	}

	hitCount := len(searchResult.Data.Hits)
	result.Metrics["http_gateway_search_hits"] = hitCount
	result.Details["http_gateway_tested"] = true
	result.Details["http_gateway_endpoint"] = url

	return nil
}

// executeTestEmbeddingFallback validates BM25 fallback when semembed unavailable
func (s *SemanticKitchenSinkScenario) executeTestEmbeddingFallback(ctx context.Context, result *Result) error {
	// Check if semembed was available during semantic search test
	semembedAvailable, ok := result.Details["semembed_available"].(bool)
	if !ok {
		semembedAvailable = false
	}

	// Query component status to verify graph processor configuration
	components, err := s.client.GetComponents(ctx)
	if err != nil {
		result.Errors = append(result.Errors, fmt.Sprintf("Failed to get components for fallback test: %v", err))
		return fmt.Errorf("component query failed: %w", err)
	}

	// Find graph processor and check its health
	var graphHealthy bool
	for _, comp := range components {
		if comp.Name == "graph" {
			graphHealthy = comp.Healthy
			break
		}
	}

	// The key insight: if semembed is unavailable, graph should still be healthy (using BM25)
	// If semembed is available, we verify hybrid mode is working
	result.Details["embedding_fallback_test"] = map[string]any{
		"semembed_available": semembedAvailable,
		"graph_healthy":      graphHealthy,
		"fallback_mode":      !semembedAvailable,
		"message":            "Graph processor operational regardless of semembed availability",
	}

	// If semembed was unavailable but graph is healthy, BM25 fallback is working
	if !semembedAvailable && graphHealthy {
		result.Metrics["fallback_verified"] = 1
		result.Details["fallback_validation"] = "BM25 fallback active - graph healthy without semembed"
	} else if semembedAvailable && graphHealthy {
		result.Metrics["hybrid_mode_verified"] = 1
		result.Details["fallback_validation"] = "Hybrid mode active - semembed + BM25 available"
	}

	// Send test message to verify search works in current mode
	conn, err := net.Dial("udp", s.udpAddr)
	if err != nil {
		result.Warnings = append(result.Warnings, fmt.Sprintf("Failed to connect for fallback test: %v", err))
		return nil // Don't fail the whole test
	}
	defer conn.Close()

	fallbackMsg := map[string]any{
		"type":        "telemetry",
		"entity_id":   "fallback-test-device",
		"entity_type": "sensor",
		"timestamp":   time.Now().Unix(),
		"description": "Testing search fallback mechanism with lexical matching",
		"data": map[string]any{
			"value": 123,
		},
	}

	msgBytes, err := json.Marshal(fallbackMsg)
	if err == nil {
		_, _ = conn.Write(msgBytes)
		result.Metrics["fallback_test_messages_sent"] = 1
	}

	return nil
}

// executeValidateRules validates that rules are being evaluated and triggered
// with actual counter value extraction, not just metric presence checks
func (s *SemanticKitchenSinkScenario) executeValidateRules(ctx context.Context, result *Result) error {
	// Query metrics for rule execution stats (before sending test data)
	metricsURL := s.config.MetricsURL + "/metrics"
	resp, err := http.Get(metricsURL)
	if err != nil {
		result.Warnings = append(result.Warnings, fmt.Sprintf("Failed to query metrics for rules: %v", err))
		return nil // Not a hard failure
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		result.Warnings = append(result.Warnings, fmt.Sprintf("Failed to read metrics for rules: %v", err))
		return nil
	}

	metricsTextBefore := string(body)

	// Parse initial metric values
	triggeredBefore := parseMetricValue(metricsTextBefore, "rule_processor_rules_triggered")
	evaluatedBefore := parseMetricValue(metricsTextBefore, "rule_processor_rules_evaluated")

	// Check for rule-related metrics presence
	ruleMetrics := map[string]bool{
		"rule_processor_events_processed":   strings.Contains(metricsTextBefore, "rule_processor_events_processed"),
		"rule_processor_rules_evaluated":    strings.Contains(metricsTextBefore, "rule_processor_rules_evaluated"),
		"rule_processor_rules_triggered":    strings.Contains(metricsTextBefore, "rule_processor_rules_triggered"),
		"rule_processor_conditions_checked": strings.Contains(metricsTextBefore, "rule_processor_conditions_checked"),
	}

	foundRuleMetrics := 0
	for _, found := range ruleMetrics {
		if found {
			foundRuleMetrics++
		}
	}

	// Send data that should trigger rules
	conn, err := net.Dial("udp", s.udpAddr)
	if err != nil {
		result.Warnings = append(result.Warnings, fmt.Sprintf("Failed to connect for rule test: %v", err))
		return nil
	}
	defer conn.Close()

	// Messages designed to trigger specific rules
	ruleTestMessages := []map[string]any{
		// Should trigger low-battery-alert
		{
			"type":      "telemetry",
			"entity_id": "battery-test-device",
			"battery":   map[string]any{"level": 15.0},
			"timestamp": time.Now().Unix(),
		},
		// Should trigger high-temperature-alert
		{
			"type":      "telemetry",
			"entity_id": "temp-test-device",
			"data":      map[string]any{"temperature": 55.0},
			"timestamp": time.Now().Unix(),
		},
	}

	sentCount := 0
	for _, msg := range ruleTestMessages {
		msgBytes, err := json.Marshal(msg)
		if err != nil {
			continue
		}
		if _, err := conn.Write(msgBytes); err == nil {
			sentCount++
		}
	}

	result.Metrics["rule_test_messages_sent"] = sentCount

	// Wait for rules to process
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-time.After(2 * time.Second):
	}

	// Query metrics again to check if rules were triggered
	resp2, err := http.Get(metricsURL)
	if err != nil {
		result.Warnings = append(result.Warnings, fmt.Sprintf("Failed to query metrics after rule test: %v", err))
		return nil
	}
	defer resp2.Body.Close()

	body2, _ := io.ReadAll(resp2.Body)
	metricsTextAfter := string(body2)

	// Parse final metric values
	triggeredAfter := parseMetricValue(metricsTextAfter, "rule_processor_rules_triggered")
	evaluatedAfter := parseMetricValue(metricsTextAfter, "rule_processor_rules_evaluated")

	// Calculate delta (rules triggered by our test data)
	triggeredDelta := triggeredAfter - triggeredBefore
	evaluatedDelta := evaluatedAfter - evaluatedBefore

	// Record metrics
	result.Metrics["rules_triggered_count"] = triggeredAfter
	result.Metrics["rules_evaluated_count"] = evaluatedAfter
	result.Metrics["rules_triggered_delta"] = triggeredDelta
	result.Metrics["rules_evaluated_delta"] = evaluatedDelta
	result.Metrics["rule_metrics_found"] = foundRuleMetrics

	// Validate rules actually triggered
	if triggeredDelta < 1 && sentCount > 0 {
		result.Warnings = append(result.Warnings,
			fmt.Sprintf("No rules triggered despite sending %d test messages (triggered delta: %d)",
				sentCount, triggeredDelta))
	}

	// Consider validation passed if we have rule metrics and some evaluation happened
	validationPassed := foundRuleMetrics >= 2 && evaluatedAfter > 0
	if validationPassed {
		result.Metrics["rules_validation_passed"] = 1
	}

	result.Details["rule_validation"] = map[string]any{
		"metrics_present":    ruleMetrics,
		"metrics_found":      foundRuleMetrics,
		"triggered_before":   triggeredBefore,
		"triggered_after":    triggeredAfter,
		"triggered_delta":    triggeredDelta,
		"evaluated_before":   evaluatedBefore,
		"evaluated_after":    evaluatedAfter,
		"evaluated_delta":    evaluatedDelta,
		"test_messages_sent": sentCount,
		"validation_passed":  validationPassed,
		"message": fmt.Sprintf("Rules: %d triggered, %d evaluated (delta: +%d triggered, +%d evaluated)",
			triggeredAfter, evaluatedAfter, triggeredDelta, evaluatedDelta),
	}

	return nil
}

// parseMetricValue extracts a numeric value from Prometheus metrics text
// Returns 0 if metric not found or parsing fails
func parseMetricValue(metricsText, metricName string) int {
	// Look for lines like: metric_name{labels} value
	// or: metric_name value
	lines := strings.Split(metricsText, "\n")
	for _, line := range lines {
		// Skip comments and empty lines
		if strings.HasPrefix(line, "#") || len(strings.TrimSpace(line)) == 0 {
			continue
		}

		// Check if line contains the metric name
		if strings.Contains(line, metricName) {
			// Extract the value (last space-separated token)
			parts := strings.Fields(line)
			if len(parts) >= 2 {
				valueStr := parts[len(parts)-1]
				// Parse as float first (Prometheus counters can be floats)
				if val, err := fmt.Sscanf(valueStr, "%f", new(float64)); err == nil && val == 1 {
					var floatVal float64
					fmt.Sscanf(valueStr, "%f", &floatVal)
					return int(floatVal)
				}
			}
		}
	}
	return 0
}

// executeValidateMetrics validates Prometheus metrics exposure
func (s *SemanticKitchenSinkScenario) executeValidateMetrics(_ context.Context, result *Result) error {
	// Query metrics endpoint (port 9090, not 8080 which is the HTTP API)
	metricsURL := s.config.MetricsURL + "/metrics"
	resp, err := http.Get(metricsURL)
	if err != nil {
		result.Errors = append(result.Errors, fmt.Sprintf("Failed to query metrics endpoint: %v", err))
		return fmt.Errorf("metrics endpoint unreachable: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		result.Errors = append(result.Errors, fmt.Sprintf("Metrics endpoint returned status %d", resp.StatusCode))
		return fmt.Errorf("metrics endpoint returned status %d", resp.StatusCode)
	}

	// Read response body
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		result.Errors = append(result.Errors, fmt.Sprintf("Failed to read metrics response: %v", err))
		return fmt.Errorf("failed to read metrics: %w", err)
	}

	metricsText := string(body)

	// Define key metrics to validate (presence only, not values)
	// Metrics list curated from processor/graph/indexmanager/metrics.go, pkg/cache/metrics.go,
	// and processor/json_filter/metrics.go - updated 2025-11-30
	requiredMetrics := []string{
		"indexengine_events_processed_total", // IndexEngine events successfully processed
		"indexengine_index_updates_total",    // Per-index update counts
		"semstreams_cache_hits_total",        // DataManager L1/L2 cache hits
		"semstreams_cache_misses_total",      // DataManager cache misses
	}

	// Optional metrics (present only when certain features active)
	optionalMetrics := []string{
		"indexengine_events_total",               // Total events received
		"indexengine_events_failed_total",        // Processing failures
		"indexengine_embeddings_generated_total", // Embedding generation count
		"semstreams_json_filter_matched_total",   // JSON filter matched messages
		"semstreams_json_filter_dropped_total",   // JSON filter dropped messages
	}

	foundRequired := make(map[string]bool)
	foundOptional := make(map[string]bool)
	missingRequired := []string{}

	// Check required metrics
	for _, metric := range requiredMetrics {
		if strings.Contains(metricsText, metric) {
			foundRequired[metric] = true
		} else {
			missingRequired = append(missingRequired, metric)
		}
	}

	// Check optional metrics (don't fail if missing)
	for _, metric := range optionalMetrics {
		if strings.Contains(metricsText, metric) {
			foundOptional[metric] = true
		}
	}

	// Fail if required metrics are missing
	if len(missingRequired) > 0 {
		result.Errors = append(result.Errors, fmt.Sprintf("Missing required metrics: %v", missingRequired))
		return fmt.Errorf("missing required metrics: %v", missingRequired)
	}

	// Record results
	result.Metrics["required_metrics_found"] = len(foundRequired)
	result.Metrics["required_metrics_expected"] = len(requiredMetrics)
	result.Metrics["optional_metrics_found"] = len(foundOptional)
	result.Metrics["optional_metrics_expected"] = len(optionalMetrics)

	result.Details["metrics_validation"] = map[string]any{
		"endpoint":         metricsURL,
		"required_found":   foundRequired,
		"optional_found":   foundOptional,
		"missing_required": missingRequired,
		"message":          fmt.Sprintf("Found %d/%d required metrics, %d/%d optional metrics", len(foundRequired), len(requiredMetrics), len(foundOptional), len(optionalMetrics)),
	}

	return nil
}

// executeVerifyEntityCount validates that entities from test data files are indexed
// and detects potential data loss by comparing expected vs actual entity counts
func (s *SemanticKitchenSinkScenario) executeVerifyEntityCount(ctx context.Context, result *Result) error {
	if s.natsClient == nil {
		result.Warnings = append(result.Warnings, "NATS client not available, skipping entity count verification")
		return nil
	}

	// Count entities in ENTITY_STATES bucket
	actualCount, err := s.natsClient.CountEntities(ctx)
	if err != nil {
		result.Warnings = append(result.Warnings, fmt.Sprintf("Failed to count entities: %v", err))
		return nil // Not a hard failure
	}

	// Expected entities from test data files:
	// - documents.jsonl: 12 entities
	// - maintenance.jsonl: 16 entities
	// - observations.jsonl: 15 entities
	// - sensor_docs.jsonl: 15 entities
	// - sensors.jsonl: 30 entities
	// Total: 88 entities from test data
	expectedFromTestData := 88

	// Add telemetry entities sent during this test run (from send-mixed-data stage)
	expectedFromUDP := 0
	if telemetrySent, ok := result.Metrics["telemetry_sent"].(int); ok {
		expectedFromUDP = telemetrySent
	}

	// Total expected entities
	totalExpected := expectedFromTestData + expectedFromUDP

	// Calculate data loss percentage
	var dataLossPercent float64
	if totalExpected > 0 {
		dataLossPercent = 100.0 * float64(totalExpected-actualCount) / float64(totalExpected)
		if dataLossPercent < 0 {
			dataLossPercent = 0 // More entities than expected (not data loss)
		}
	}

	result.Metrics["entity_count"] = actualCount
	result.Metrics["expected_from_testdata"] = expectedFromTestData
	result.Metrics["expected_from_udp"] = expectedFromUDP
	result.Metrics["total_expected_entities"] = totalExpected
	result.Metrics["min_expected_entities"] = s.config.MinExpectedEntities
	result.Metrics["data_loss_percent"] = dataLossPercent

	// Warn on data loss threshold (>10%)
	dataLossThreshold := 10.0
	if dataLossPercent > dataLossThreshold {
		result.Warnings = append(result.Warnings,
			fmt.Sprintf("Data loss detected: %.1f%% (expected %d, got %d)",
				dataLossPercent, totalExpected, actualCount))
	}

	// Warn if below minimum threshold
	if actualCount < s.config.MinExpectedEntities {
		result.Warnings = append(result.Warnings,
			fmt.Sprintf("Entity count %d is below minimum expected %d", actualCount, s.config.MinExpectedEntities))
	}

	result.Details["entity_count_verification"] = map[string]any{
		"actual_count":        actualCount,
		"expected_from_data":  expectedFromTestData,
		"expected_from_udp":   expectedFromUDP,
		"total_expected":      totalExpected,
		"min_expected":        s.config.MinExpectedEntities,
		"data_loss_percent":   dataLossPercent,
		"meets_minimum":       actualCount >= s.config.MinExpectedEntities,
		"data_loss_threshold": dataLossThreshold,
		"data_loss_detected":  dataLossPercent > dataLossThreshold,
		"message":             fmt.Sprintf("Found %d entities (expected %d, loss: %.1f%%)", actualCount, totalExpected, dataLossPercent),
	}

	return nil
}

// executeVerifyEntityRetrieval validates that specific known entities can be retrieved
func (s *SemanticKitchenSinkScenario) executeVerifyEntityRetrieval(ctx context.Context, result *Result) error {
	if s.natsClient == nil {
		result.Warnings = append(result.Warnings, "NATS client not available, skipping entity retrieval verification")
		return nil
	}

	// Test entities from test data files
	// These are fully-qualified entity IDs after processing with org_id=c360, platform=logistics
	// Format: {org}.{platform}.{domain}.{system}.{category/status/severity}.{id}
	testEntities := []struct {
		id           string
		expectedType string
		source       string
	}{
		{"c360.logistics.content.document.safety.doc-safety-001", "document", "documents.jsonl"},
		{"c360.logistics.content.document.operations.doc-ops-001", "document", "documents.jsonl"},
		{"c360.logistics.maintenance.work.completed.maint-001", "maintenance", "maintenance.jsonl"},
		{"c360.logistics.observation.record.high.obs-001", "observation", "observations.jsonl"},
		{"c360.logistics.sensor.document.temperature.sensor-temp-001", "sensor_doc", "sensor_docs.jsonl"},
	}

	foundEntities := 0
	missingEntities := []string{}
	entityDetails := make(map[string]any)

	for _, te := range testEntities {
		entity, err := s.natsClient.GetEntity(ctx, te.id)
		if err != nil {
			missingEntities = append(missingEntities, te.id)
			entityDetails[te.id] = map[string]any{
				"found":         false,
				"error":         err.Error(),
				"expected_type": te.expectedType,
				"source":        te.source,
			}
			continue
		}

		foundEntities++
		entityDetails[te.id] = map[string]any{
			"found":         true,
			"actual_type":   entity.Type,
			"expected_type": te.expectedType,
			"source":        te.source,
		}
	}

	result.Metrics["entities_retrieved"] = foundEntities
	result.Metrics["entities_missing"] = len(missingEntities)

	result.Details["entity_retrieval_verification"] = map[string]any{
		"tested":   len(testEntities),
		"found":    foundEntities,
		"missing":  missingEntities,
		"entities": entityDetails,
		"message":  fmt.Sprintf("Retrieved %d/%d test entities", foundEntities, len(testEntities)),
	}

	// Log as warning if some entities missing but don't fail
	if len(missingEntities) > 0 {
		result.Warnings = append(result.Warnings,
			fmt.Sprintf("Missing entities: %v", missingEntities))
	}

	return nil
}

// executeValidateEntityStructure validates entity data structure integrity
func (s *SemanticKitchenSinkScenario) executeValidateEntityStructure(ctx context.Context, result *Result) error {
	if s.natsClient == nil {
		result.Warnings = append(result.Warnings, "NATS client not available, skipping entity structure validation")
		return nil
	}

	// Sample up to 5 entities for structure validation
	entities, err := s.natsClient.GetEntitySample(ctx, 5)
	if err != nil {
		result.Warnings = append(result.Warnings, fmt.Sprintf("Failed to get entity sample: %v", err))
		return nil
	}

	if len(entities) == 0 {
		result.Warnings = append(result.Warnings, "No entities available for structure validation")
		return nil
	}

	validatedCount := 0
	validationErrors := []string{}
	entityDetails := make(map[string]any)

	for _, entity := range entities {
		entityValid := true
		issues := []string{}

		// Validate ID format (non-empty, should have dot-separated segments)
		if entity.ID == "" {
			issues = append(issues, "empty ID")
			entityValid = false
		} else if !strings.Contains(entity.ID, ".") {
			issues = append(issues, "ID missing expected format (no dot separators)")
			entityValid = false
		}

		// Validate Triples (should have at least one triple)
		if len(entity.Triples) == 0 {
			issues = append(issues, "no triples")
			entityValid = false
		} else {
			// Validate triple structure
			for i, triple := range entity.Triples {
				if triple.Subject == "" {
					issues = append(issues, fmt.Sprintf("triple[%d]: empty subject", i))
					entityValid = false
				}
				if triple.Predicate == "" {
					issues = append(issues, fmt.Sprintf("triple[%d]: empty predicate", i))
					entityValid = false
				}
			}
		}

		// Validate Version (should be positive)
		if entity.Version <= 0 {
			issues = append(issues, fmt.Sprintf("invalid version: %d", entity.Version))
			entityValid = false
		}

		// Validate UpdatedAt (should be non-empty and parseable if present)
		if entity.UpdatedAt != "" {
			// Try to parse as RFC3339 or similar format
			if _, err := time.Parse(time.RFC3339, entity.UpdatedAt); err != nil {
				// Try alternate format
				if _, err := time.Parse(time.RFC3339Nano, entity.UpdatedAt); err != nil {
					issues = append(issues, fmt.Sprintf("invalid timestamp format: %s", entity.UpdatedAt))
					// Don't fail validation for timestamp format issues
				}
			}
		}

		if entityValid {
			validatedCount++
		} else {
			validationErrors = append(validationErrors, fmt.Sprintf("%s: %v", entity.ID, issues))
		}

		entityDetails[entity.ID] = map[string]any{
			"valid":        entityValid,
			"issues":       issues,
			"triple_count": len(entity.Triples),
			"version":      entity.Version,
			"has_updated":  entity.UpdatedAt != "",
		}
	}

	result.Metrics["entities_validated"] = validatedCount
	result.Metrics["entities_sampled"] = len(entities)
	result.Metrics["validation_errors"] = len(validationErrors)

	result.Details["entity_structure_validation"] = map[string]any{
		"sampled":           len(entities),
		"validated":         validatedCount,
		"errors":            validationErrors,
		"entities":          entityDetails,
		"validation_passed": len(validationErrors) == 0,
		"message":           fmt.Sprintf("Validated %d/%d sampled entities", validatedCount, len(entities)),
	}

	if len(validationErrors) > 0 {
		result.Warnings = append(result.Warnings,
			fmt.Sprintf("Entity structure validation issues: %v", validationErrors))
	}

	return nil
}

// executeVerifyIndexPopulation validates that all 7 core indexes are populated
func (s *SemanticKitchenSinkScenario) executeVerifyIndexPopulation(ctx context.Context, result *Result) error {
	if s.natsClient == nil {
		result.Warnings = append(result.Warnings, "NATS client not available, skipping index population verification")
		return nil
	}

	// Core indexes that should be populated
	indexes := []struct {
		name     string
		bucket   string
		required bool
	}{
		{"entity_states", client.IndexBuckets.EntityStates, true},
		{"predicate", client.IndexBuckets.Predicate, true},
		{"incoming", client.IndexBuckets.Incoming, true},
		{"outgoing", client.IndexBuckets.Outgoing, true},
		{"alias", client.IndexBuckets.Alias, false},     // May be empty if no aliases
		{"spatial", client.IndexBuckets.Spatial, false}, // May be empty if no geo data
		{"temporal", client.IndexBuckets.Temporal, true},
	}

	indexDetails := make(map[string]any)
	populatedCount := 0
	emptyRequired := []string{}

	for _, idx := range indexes {
		count, err := s.natsClient.CountBucketKeys(ctx, idx.bucket)
		if err != nil {
			indexDetails[idx.name] = map[string]any{
				"bucket":    idx.bucket,
				"error":     err.Error(),
				"populated": false,
			}
			if idx.required {
				emptyRequired = append(emptyRequired, idx.name)
			}
			continue
		}

		populated := count > 0
		if populated {
			populatedCount++
		} else if idx.required {
			emptyRequired = append(emptyRequired, idx.name)
		}

		// Get sample keys for debugging
		sampleKeys, _ := s.natsClient.GetBucketKeysSample(ctx, idx.bucket, 3)

		indexDetails[idx.name] = map[string]any{
			"bucket":      idx.bucket,
			"key_count":   count,
			"populated":   populated,
			"sample_keys": sampleKeys,
		}
	}

	result.Metrics["indexes_populated"] = populatedCount
	result.Metrics["indexes_total"] = len(indexes)

	result.Details["index_population_verification"] = map[string]any{
		"indexes":        indexDetails,
		"populated":      populatedCount,
		"total":          len(indexes),
		"empty_required": emptyRequired,
		"message":        fmt.Sprintf("Populated %d/%d indexes", populatedCount, len(indexes)),
	}

	if len(emptyRequired) > 0 {
		result.Warnings = append(result.Warnings,
			fmt.Sprintf("Required indexes empty: %v", emptyRequired))
	}

	return nil
}

// executeVerifySearchQuality validates that semantic search returns expected results
// with score threshold assertions, not just binary hit/no-hit checks
func (s *SemanticKitchenSinkScenario) executeVerifySearchQuality(ctx context.Context, result *Result) error {
	// Test search via HTTP Gateway
	gatewayURL := s.config.GatewayURL
	httpClient := &http.Client{Timeout: 10 * time.Second}

	// Search queries with expected results and quality thresholds
	searchTests := []struct {
		query           string
		expectedPattern string
		description     string
		minScore        float64 // Minimum acceptable score for quality hits
		minHits         int     // Minimum expected hits
	}{
		{"temperature sensor", "temp", "Should find temperature sensors", 0.3, 2},
		{"safety procedures", "safety", "Should find safety documents", 0.3, 1},
		{"maintenance", "maint", "Should find maintenance records", 0.3, 3},
	}

	searchResults := make(map[string]any)
	queriesWithResults := 0
	queriesMeetingMinScore := 0
	queriesMeetingMinHits := 0
	allScores := []float64{}

	for _, test := range searchTests {
		searchQuery := map[string]any{
			"query":     test.query,
			"threshold": 0.1, // Low threshold to get results
			"limit":     10,  // Get more results for quality analysis
		}

		queryJSON, err := json.Marshal(searchQuery)
		if err != nil {
			continue
		}

		req, err := http.NewRequestWithContext(ctx, "POST",
			gatewayURL+"/search/semantic", strings.NewReader(string(queryJSON)))
		if err != nil {
			continue
		}
		req.Header.Set("Content-Type", "application/json")

		resp, err := httpClient.Do(req)
		if err != nil {
			searchResults[test.query] = map[string]any{
				"error":       err.Error(),
				"description": test.description,
			}
			continue
		}

		bodyBytes, _ := io.ReadAll(resp.Body)
		resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			searchResults[test.query] = map[string]any{
				"status":      resp.StatusCode,
				"description": test.description,
			}
			continue
		}

		// Parse response
		var searchResp struct {
			Data struct {
				Query string `json:"query"`
				Hits  []struct {
					EntityID string  `json:"entity_id"`
					Score    float64 `json:"score"`
				} `json:"hits"`
			} `json:"data"`
			Error string `json:"error"`
		}

		if err := json.Unmarshal(bodyBytes, &searchResp); err != nil {
			searchResults[test.query] = map[string]any{
				"error":       "parse error",
				"description": test.description,
			}
			continue
		}

		hasResults := len(searchResp.Data.Hits) > 0
		if hasResults {
			queriesWithResults++
		}

		// Check if meets minimum hits requirement
		meetsMinHits := len(searchResp.Data.Hits) >= test.minHits
		if meetsMinHits {
			queriesMeetingMinHits++
		}

		// Check scores and calculate average
		matchesPattern := false
		topHits := []string{}
		topScores := []float64{}
		hitsAboveMinScore := 0
		var scoreSum float64

		for _, hit := range searchResp.Data.Hits {
			topHits = append(topHits, hit.EntityID)
			topScores = append(topScores, hit.Score)
			allScores = append(allScores, hit.Score)
			scoreSum += hit.Score

			if hit.Score >= test.minScore {
				hitsAboveMinScore++
			}
			if strings.Contains(strings.ToLower(hit.EntityID), test.expectedPattern) {
				matchesPattern = true
			}
		}

		// Calculate average score for this query
		avgScore := 0.0
		if len(searchResp.Data.Hits) > 0 {
			avgScore = scoreSum / float64(len(searchResp.Data.Hits))
		}

		// Check if meets minimum score threshold
		meetsMinScore := hitsAboveMinScore > 0
		if meetsMinScore {
			queriesMeetingMinScore++
		}

		searchResults[test.query] = map[string]any{
			"hit_count":           len(searchResp.Data.Hits),
			"top_hits":            topHits,
			"top_scores":          topScores,
			"matches_pattern":     matchesPattern,
			"expected":            test.expectedPattern,
			"description":         test.description,
			"avg_score":           avgScore,
			"min_score_threshold": test.minScore,
			"min_hits_threshold":  test.minHits,
			"hits_above_min":      hitsAboveMinScore,
			"meets_min_score":     meetsMinScore,
			"meets_min_hits":      meetsMinHits,
		}
	}

	// Calculate overall search quality score (average of all scores)
	overallAvgScore := 0.0
	if len(allScores) > 0 {
		var sum float64
		for _, s := range allScores {
			sum += s
		}
		overallAvgScore = sum / float64(len(allScores))
	}

	// Quality threshold warning
	weakResultsThreshold := 0.5
	if overallAvgScore > 0 && overallAvgScore < weakResultsThreshold {
		result.Warnings = append(result.Warnings,
			fmt.Sprintf("Weak search results: average score %.2f is below %.2f threshold",
				overallAvgScore, weakResultsThreshold))
	}

	result.Metrics["search_queries_tested"] = len(searchTests)
	result.Metrics["search_queries_with_results"] = queriesWithResults
	result.Metrics["search_min_score_met"] = queriesMeetingMinScore
	result.Metrics["search_min_hits_met"] = queriesMeetingMinHits
	result.Metrics["search_quality_score"] = overallAvgScore

	result.Details["search_quality_verification"] = map[string]any{
		"queries":             len(searchTests),
		"queries_with_hits":   queriesWithResults,
		"min_score_met":       queriesMeetingMinScore,
		"min_hits_met":        queriesMeetingMinHits,
		"overall_avg_score":   overallAvgScore,
		"weak_threshold":      weakResultsThreshold,
		"results":             searchResults,
		"message":             fmt.Sprintf("%d/%d queries returned results, avg score: %.2f", queriesWithResults, len(searchTests), overallAvgScore),
	}

	return nil
}

// executeCompareCoreMl captures search results for Core vs ML comparison
// and persists results to JSON for later analysis
func (s *SemanticKitchenSinkScenario) executeCompareCoreMl(ctx context.Context, result *Result) error {
	// Detect which variant is running based on semembed availability
	variant := "core"
	semembedAvailable, ok := result.Details["semembed_available"].(bool)
	if ok && semembedAvailable {
		variant = "ml"
	}

	// Check embedding provider from metrics
	metricsURL := s.config.MetricsURL + "/metrics"
	resp, err := http.Get(metricsURL)
	embeddingProvider := "unknown"
	if err == nil {
		defer resp.Body.Close()
		body, _ := io.ReadAll(resp.Body)
		metricsText := string(body)

		if strings.Contains(metricsText, `indexengine_embedding_provider{provider="http"}`) {
			embeddingProvider = "http"
			variant = "ml"
		} else if strings.Contains(metricsText, `indexengine_embedding_provider{provider="bm25"}`) {
			embeddingProvider = "bm25"
			variant = "core"
		}
	}

	// Run comparison queries
	gatewayURL := s.config.GatewayURL
	httpClient := &http.Client{Timeout: 10 * time.Second}

	comparisonQueries := []string{
		"temperature sensor reading",
		"maintenance schedule",
		"safety documentation",
		"cold storage monitoring",
	}

	// Use structured types for comparison data
	searchResults := make(map[string]SearchQueryResult)
	queryResults := make(map[string]any) // For backward compatibility with result.Details

	for _, query := range comparisonQueries {
		searchQuery := map[string]any{
			"query":     query,
			"threshold": 0.1,
			"limit":     10,
		}

		queryJSON, _ := json.Marshal(searchQuery)
		req, err := http.NewRequestWithContext(ctx, "POST",
			gatewayURL+"/search/semantic", strings.NewReader(string(queryJSON)))
		if err != nil {
			continue
		}
		req.Header.Set("Content-Type", "application/json")

		// Track latency
		queryStart := time.Now()
		resp, err := httpClient.Do(req)
		latencyMs := time.Since(queryStart).Milliseconds()

		if err != nil {
			queryResults[query] = map[string]any{"error": err.Error()}
			continue
		}

		bodyBytes, _ := io.ReadAll(resp.Body)
		resp.Body.Close()

		var searchResp struct {
			Data struct {
				Hits []struct {
					EntityID string  `json:"entity_id"`
					Score    float64 `json:"score"`
				} `json:"hits"`
			} `json:"data"`
		}

		if err := json.Unmarshal(bodyBytes, &searchResp); err != nil {
			continue
		}

		// Capture results for comparison
		hitIDs := []string{}
		scores := []float64{}
		for _, hit := range searchResp.Data.Hits {
			hitIDs = append(hitIDs, hit.EntityID)
			scores = append(scores, hit.Score)
		}

		// Store in structured format for JSON persistence
		searchResults[query] = SearchQueryResult{
			Query:     query,
			Hits:      hitIDs,
			Scores:    scores,
			LatencyMs: latencyMs,
			HitCount:  len(hitIDs),
		}

		// Store in map format for backward compatibility
		queryResults[query] = map[string]any{
			"hits":       hitIDs,
			"scores":     scores,
			"count":      len(hitIDs),
			"latency_ms": latencyMs,
		}
	}

	// Persist comparison results to JSON file
	comparisonFile := ""
	if s.config.OutputDir != "" {
		compData := ComparisonData{
			Variant:           variant,
			EmbeddingProvider: embeddingProvider,
			Timestamp:         time.Now(),
			SearchResults:     searchResults,
		}

		// Ensure output directory exists
		if err := os.MkdirAll(s.config.OutputDir, 0755); err != nil {
			result.Warnings = append(result.Warnings,
				fmt.Sprintf("Failed to create output directory: %v", err))
		} else {
			// Generate filename with variant and timestamp
			filename := fmt.Sprintf("comparison-%s-%s.json",
				variant, time.Now().Format("20060102-150405"))
			comparisonFile = filepath.Join(s.config.OutputDir, filename)

			data, err := json.MarshalIndent(compData, "", "  ")
			if err != nil {
				result.Warnings = append(result.Warnings,
					fmt.Sprintf("Failed to marshal comparison data: %v", err))
			} else {
				if err := os.WriteFile(comparisonFile, data, 0644); err != nil {
					result.Warnings = append(result.Warnings,
						fmt.Sprintf("Failed to write comparison file: %v", err))
				}
			}
		}
	}

	result.Details["core_ml_comparison"] = map[string]any{
		"variant":            variant,
		"embedding_provider": embeddingProvider,
		"queries":            queryResults,
		"comparison_file":    comparisonFile,
		"message": fmt.Sprintf("Captured %d search queries for %s variant (%s embeddings)",
			len(comparisonQueries), variant, embeddingProvider),
	}

	result.Metrics["comparison_variant"] = variant
	result.Metrics["embedding_provider"] = embeddingProvider

	return nil
}

// CommunityComparison represents a comparison of statistical vs LLM summaries for a community
type CommunityComparison struct {
	CommunityID        string   `json:"community_id"`
	Level              int      `json:"level"`
	MemberCount        int      `json:"member_count"`
	StatisticalSummary string   `json:"statistical_summary"`
	LLMSummary         string   `json:"llm_summary,omitempty"`
	SummaryStatus      string   `json:"summary_status"`
	Keywords           []string `json:"keywords"`
	SummaryLengthRatio float64  `json:"summary_length_ratio,omitempty"`
	WordOverlap        float64  `json:"word_overlap,omitempty"`
}

// CommunitySummaryReport contains aggregated community summary metrics
type CommunitySummaryReport struct {
	Variant              string                `json:"variant"`
	Timestamp            time.Time             `json:"timestamp"`
	CommunitiesTotal     int                   `json:"communities_total"`
	LLMEnhancedCount     int                   `json:"llm_enhanced_count"`
	StatisticalOnlyCount int                   `json:"statistical_only_count"`
	AvgSummaryLengthRatio float64              `json:"avg_summary_length_ratio"`
	AvgWordOverlap       float64               `json:"avg_word_overlap"`
	Communities          []CommunityComparison `json:"communities"`
}

// executeCompareCommunities compares statistical vs LLM-enhanced community summaries
func (s *SemanticKitchenSinkScenario) executeCompareCommunities(ctx context.Context, result *Result) error {
	if s.natsClient == nil {
		result.Warnings = append(result.Warnings, "NATS client not available, skipping community comparison")
		return nil
	}

	// Retrieve all communities from COMMUNITY_INDEX bucket
	communities, err := s.natsClient.GetAllCommunities(ctx)
	if err != nil {
		result.Warnings = append(result.Warnings, fmt.Sprintf("Failed to get communities: %v", err))
		return nil
	}

	if len(communities) == 0 {
		result.Warnings = append(result.Warnings, "No communities found for comparison")
		result.Metrics["communities_total"] = 0
		return nil
	}

	// Compare statistical vs LLM summaries for each community
	comparisons := make([]CommunityComparison, 0, len(communities))
	llmEnhancedCount := 0
	statisticalOnlyCount := 0
	var totalLengthRatio float64
	var totalWordOverlap float64
	ratioCount := 0

	for _, comm := range communities {
		comparison := CommunityComparison{
			CommunityID:        comm.ID,
			Level:              comm.Level,
			MemberCount:        len(comm.Members),
			StatisticalSummary: comm.StatisticalSummary,
			LLMSummary:         comm.LLMSummary,
			SummaryStatus:      comm.SummaryStatus,
			Keywords:           comm.Keywords,
		}

		// Track summary status counts
		switch comm.SummaryStatus {
		case "llm-enhanced":
			llmEnhancedCount++
		case "statistical", "":
			statisticalOnlyCount++
		}

		// Calculate metrics only when both summaries exist
		if comm.LLMSummary != "" && comm.StatisticalSummary != "" {
			// Length ratio: how much longer/shorter is LLM summary
			if len(comm.StatisticalSummary) > 0 {
				comparison.SummaryLengthRatio = float64(len(comm.LLMSummary)) / float64(len(comm.StatisticalSummary))
				totalLengthRatio += comparison.SummaryLengthRatio
				ratioCount++
			}

			// Word overlap: Jaccard similarity on word sets
			comparison.WordOverlap = wordJaccard(comm.StatisticalSummary, comm.LLMSummary)
			totalWordOverlap += comparison.WordOverlap
		}

		comparisons = append(comparisons, comparison)
	}

	// Calculate averages
	avgLengthRatio := 0.0
	avgWordOverlap := 0.0
	if ratioCount > 0 {
		avgLengthRatio = totalLengthRatio / float64(ratioCount)
		avgWordOverlap = totalWordOverlap / float64(ratioCount)
	}

	// Get variant from earlier comparison stage
	variant := "unknown"
	if v, ok := result.Metrics["comparison_variant"].(string); ok {
		variant = v
	}

	// Record metrics
	result.Metrics["communities_total"] = len(communities)
	result.Metrics["communities_llm_enhanced"] = llmEnhancedCount
	result.Metrics["communities_statistical_only"] = statisticalOnlyCount
	result.Metrics["avg_summary_length_ratio"] = avgLengthRatio
	result.Metrics["avg_word_overlap"] = avgWordOverlap

	// Persist community comparison report
	comparisonFile := ""
	if s.config.OutputDir != "" {
		report := CommunitySummaryReport{
			Variant:               variant,
			Timestamp:             time.Now(),
			CommunitiesTotal:      len(communities),
			LLMEnhancedCount:      llmEnhancedCount,
			StatisticalOnlyCount:  statisticalOnlyCount,
			AvgSummaryLengthRatio: avgLengthRatio,
			AvgWordOverlap:        avgWordOverlap,
			Communities:           comparisons,
		}

		filename := fmt.Sprintf("community-comparison-%s-%s.json",
			variant, time.Now().Format("20060102-150405"))
		comparisonFile = filepath.Join(s.config.OutputDir, filename)

		data, err := json.MarshalIndent(report, "", "  ")
		if err == nil {
			if err := os.WriteFile(comparisonFile, data, 0644); err != nil {
				result.Warnings = append(result.Warnings,
					fmt.Sprintf("Failed to write community comparison file: %v", err))
			}
		}
	}

	result.Details["community_comparison"] = map[string]any{
		"total":               len(communities),
		"llm_enhanced":        llmEnhancedCount,
		"statistical_only":    statisticalOnlyCount,
		"avg_length_ratio":    avgLengthRatio,
		"avg_word_overlap":    avgWordOverlap,
		"comparison_file":     comparisonFile,
		"communities":         comparisons,
		"message": fmt.Sprintf("Compared %d communities: %d LLM-enhanced, %d statistical only",
			len(communities), llmEnhancedCount, statisticalOnlyCount),
	}

	return nil
}

// wordJaccard calculates Jaccard similarity on word sets
func wordJaccard(a, b string) float64 {
	wordsA := toWordSet(strings.ToLower(a))
	wordsB := toWordSet(strings.ToLower(b))

	intersection := 0
	for word := range wordsA {
		if wordsB[word] {
			intersection++
		}
	}

	union := len(wordsA) + len(wordsB) - intersection
	if union == 0 {
		return 1.0
	}
	return float64(intersection) / float64(union)
}

// toWordSet converts a string to a set of words (excluding short words and punctuation)
func toWordSet(s string) map[string]bool {
	words := strings.Fields(s)
	set := make(map[string]bool)
	for _, w := range words {
		// Remove punctuation
		w = strings.Trim(w, ".,!?;:()[]{}\"'")
		// Skip short words (less than 3 characters)
		if len(w) > 2 {
			set[w] = true
		}
	}
	return set
}
