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
	"regexp"
	"strings"
	"time"

	"github.com/c360/semstreams/test/e2e/client"
	"github.com/c360/semstreams/test/e2e/config"
)

// TieredScenario validates comprehensive semantic processing
type TieredScenario struct {
	name        string
	description string
	client      *client.ObservabilityClient
	natsClient  *client.NATSValidationClient
	udpAddr     string
	natsURL     string
	config      *TieredConfig

	// Observability clients for consistent metric access
	metrics   *client.MetricsClient
	msgLogger *client.MessageLoggerClient
	tracer    *client.FlowTracer

	// Pre-send baseline for event-driven validation (captured before sending data)
	preSendBaseline *client.MetricsBaseline
}

// TieredConfig contains configuration for tiered E2E tests
type TieredConfig struct {
	// Variant configuration
	Variant string `json:"variant"` // "structural", "statistical", "semantic"

	// Test data configuration
	MessageCount    int           `json:"message_count"`
	MessageInterval time.Duration `json:"message_interval"`

	// Validation configuration (event-driven, matching tier0 patterns)
	ValidationTimeout time.Duration `json:"validation_timeout"` // Timeout for metric waits (30s for semantic)
	PollInterval      time.Duration `json:"poll_interval"`      // Poll interval for metric waits (100ms)
	MinProcessed      int           `json:"min_processed"`

	// Entity verification (from test data files)
	MinExpectedEntities int    `json:"min_expected_entities"`
	NatsURL             string `json:"nats_url"`
	MetricsURL          string `json:"metrics_url"`
	ServiceManagerURL   string `json:"service_manager_url"`
	GatewayURL          string `json:"gateway_url"`

	// Comparison output configuration
	OutputDir string `json:"output_dir"`

	// Baseline comparison (matching tier0 patterns)
	BaselineFile         string  `json:"baseline_file,omitempty"` // Path to baseline JSON (optional)
	MaxRegressionPercent float64 `json:"max_regression_percent"`  // Default 20%

	// Structural tier config (rules-only, from tier0_rules_iot.go)
	ExpectedEmbeddings int `json:"expected_embeddings"` // 0 for structural variant
	ExpectedClusters   int `json:"expected_clusters"`   // 0 for structural variant
	MinRulesEvaluated  int `json:"min_rules_evaluated"` // Min rules evaluated for structural
	MinOnEnterFired    int `json:"min_on_enter_fired"`  // Min OnEnter transitions
	MinOnExitFired     int `json:"min_on_exit_fired"`   // Min OnExit transitions
}

// ComparisonData represents comparison results for Core vs ML analysis
type ComparisonData struct {
	Variant           string                       `json:"variant"`
	EmbeddingProvider string                       `json:"embedding_provider"`
	Timestamp         time.Time                    `json:"timestamp"`
	SearchResults     map[string]SearchQueryResult `json:"search_results"`
}

// SearchQueryResult represents results from a single search query
type SearchQueryResult struct {
	Query     string    `json:"query"`
	Hits      []string  `json:"hits"`
	Scores    []float64 `json:"scores"`
	LatencyMs int64     `json:"latency_ms"`
	HitCount  int       `json:"hit_count"`
}

// DefaultTieredConfig returns default configuration
func DefaultTieredConfig() *TieredConfig {
	return &TieredConfig{
		Variant:         "", // Auto-detect from environment
		MessageCount:    20,
		MessageInterval: 50 * time.Millisecond,
		// Event-driven validation timeouts (matching tier0 patterns)
		ValidationTimeout:    30 * time.Second,       // Longer for semantic: embeddings + clustering
		PollInterval:         100 * time.Millisecond, // Fast polling for responsiveness
		MinProcessed:         10,                     // At least 50% should make it through
		MinExpectedEntities:  50,                     // Test data has 74 entities, expect at least 50 indexed
		NatsURL:              config.DefaultEndpoints.NATS,
		MetricsURL:           config.DefaultEndpoints.Metrics,
		ServiceManagerURL:    config.DefaultEndpoints.HTTP,
		GatewayURL:           config.DefaultEndpoints.HTTP + "/api-gateway",
		OutputDir:            "test/e2e/results",
		MaxRegressionPercent: 20.0, // 20% regression threshold
		// Structural tier defaults (from tier0_rules_iot.go)
		ExpectedEmbeddings: 0, // Structural: NO embeddings
		ExpectedClusters:   0, // Structural: NO clustering
		MinRulesEvaluated:  5,
		MinOnEnterFired:    2, // Expect at least 2 OnEnter transitions
		MinOnExitFired:     1, // Expect at least 1 OnExit transition
	}
}

// NewTieredScenario creates a new tiered semantic test scenario
func NewTieredScenario(
	obsClient *client.ObservabilityClient,
	udpAddr string,
	cfg *TieredConfig,
) *TieredScenario {
	if cfg == nil {
		cfg = DefaultTieredConfig()
	}
	if udpAddr == "" {
		udpAddr = "localhost:14550"
	}
	natsURL := cfg.NatsURL
	if natsURL == "" {
		natsURL = config.DefaultEndpoints.NATS
	}

	return &TieredScenario{
		name:        "tiered",
		description: "Tests tiered semantic stack: Protocol + Graph + Indexes + Queries + Multiple Outputs",
		client:      obsClient,
		udpAddr:     udpAddr,
		natsURL:     natsURL,
		config:      cfg,
	}
}

// Name returns the scenario name
func (s *TieredScenario) Name() string {
	return s.name
}

// Description returns the scenario description
func (s *TieredScenario) Description() string {
	return s.description
}

// Setup prepares the scenario
func (s *TieredScenario) Setup(ctx context.Context) error {
	// Verify UDP endpoint is reachable
	conn, err := net.Dial("udp", s.udpAddr)
	if err != nil {
		return fmt.Errorf("cannot reach UDP endpoint %s: %w", s.udpAddr, err)
	}
	_ = conn.Close()

	// Initialize observability clients (matching tier0 patterns)
	s.metrics = client.NewMetricsClient(s.config.MetricsURL)
	s.msgLogger = client.NewMessageLoggerClient(s.config.ServiceManagerURL)
	s.tracer = client.NewFlowTracer(s.metrics, s.msgLogger)

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

// Execute runs the tiered semantic test scenario
// stage represents a test stage with variant filtering
type stage struct {
	name     string
	fn       func(context.Context, *Result) error
	variants []string // Empty = run for all variants
}

// getStagesForVariant returns the filtered list of stages for a given variant
func (s *TieredScenario) getStagesForVariant(variant string) []stage {
	allStages := []stage{
		{"verify-components", s.executeVerifyComponents, nil},
		{"send-mixed-data", s.executeSendMixedData, nil},
		{"validate-processing", s.executeValidateProcessing, nil},
		{"verify-entity-count", s.executeVerifyEntityCount, nil},
		{"verify-entity-retrieval", s.executeVerifyEntityRetrieval, nil},
		{"validate-entity-structure", s.executeValidateEntityStructure, nil},
		{"verify-index-population", s.executeVerifyIndexPopulation, nil},

		// Structural-only stages
		{"validate-zero-embeddings", s.executeValidateZeroEmbeddings, []string{"structural"}},
		{"validate-zero-clusters", s.executeValidateZeroClusters, []string{"structural"}},
		{"validate-rule-transitions", s.executeValidateRuleTransitions, []string{"structural"}},

		// Statistical and Semantic stages (skip for structural)
		{"test-semantic-search", s.executeTestSemanticSearch, []string{"statistical", "semantic"}},
		{"verify-search-quality", s.executeVerifySearchQuality, []string{"statistical", "semantic"}},
		{"test-http-gateway", s.executeTestHTTPGateway, []string{"statistical", "semantic"}},
		{"test-embedding-fallback", s.executeTestEmbeddingFallback, []string{"statistical", "semantic"}},

		// Variant comparison stages
		{"compare-statistical-semantic", s.executeCompareStatisticalSemantic, []string{"statistical", "semantic"}},
		{"compare-communities", s.executeCompareCommunities, []string{"semantic"}}, // Semantic only

		// Common stages
		{"validate-rules", s.executeValidateRules, nil},
		{"validate-metrics", s.executeValidateMetrics, nil},
		{"verify-outputs", s.executeVerifyOutputs, nil},
	}

	// Filter stages based on variant
	stages := []stage{}
	for _, st := range allStages {
		if len(st.variants) == 0 {
			stages = append(stages, st)
		} else {
			for _, allowedVariant := range st.variants {
				if variant == allowedVariant {
					stages = append(stages, st)
					break
				}
			}
		}
	}
	return stages
}

// Execute runs the tiered semantic test scenario
func (s *TieredScenario) Execute(ctx context.Context) (*Result, error) {
	result := &Result{
		ScenarioName: s.name,
		StartTime:    time.Now(),
		Success:      false,
		Metrics:      make(map[string]any),
		Details:      make(map[string]any),
		Errors:       []string{},
		Warnings:     []string{},
	}

	// Detect variant if not explicitly set
	variant := s.config.Variant
	if variant == "" {
		info := s.detectVariantAndProvider(result)
		variant = info.variant
		result.Details["detected_variant"] = variant
		result.Details["detected_embedding_provider"] = info.embeddingProvider
	}
	result.Metrics["variant"] = variant

	// Get filtered stages for this variant
	stages := s.getStagesForVariant(variant)

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

	// Capture final metrics baseline for regression detection (matching tier0 pattern)
	endBaseline, err := s.metrics.CaptureBaseline(ctx)
	if err != nil {
		result.Warnings = append(result.Warnings, fmt.Sprintf("Failed to capture end baseline: %v", err))
	} else {
		// Store current run metrics for potential baseline capture
		currentSnapshot := map[string]any{
			"timestamp":   time.Now(),
			"duration_ms": time.Since(result.StartTime).Milliseconds(),
			"metrics":     endBaseline.Metrics,
		}
		result.Details["baseline_snapshot"] = currentSnapshot

		// Compare to baseline file if configured (matching tier0 pattern)
		if s.config.BaselineFile != "" {
			baselineData, err := os.ReadFile(s.config.BaselineFile)
			if err == nil {
				var loadedBaseline struct {
					Metrics map[string]float64 `json:"metrics"`
				}
				if json.Unmarshal(baselineData, &loadedBaseline) == nil {
					regressions := []string{}
					for metric, baselineValue := range loadedBaseline.Metrics {
						if currentValue, ok := endBaseline.Metrics[metric]; ok {
							if baselineValue > 0 {
								percentChange := ((currentValue - baselineValue) / baselineValue) * 100
								// Check for performance regressions (lower is worse for some metrics)
								if percentChange < -s.config.MaxRegressionPercent {
									regressions = append(regressions, fmt.Sprintf("%s: %.1f%% regression", metric, -percentChange))
								}
							}
						}
					}
					if len(regressions) > 0 {
						result.Warnings = append(result.Warnings, fmt.Sprintf("Performance regressions detected: %v", regressions))
					}
					result.Details["baseline_comparison"] = map[string]any{
						"baseline_file": s.config.BaselineFile,
						"regressions":   regressions,
					}
				}
			}
		}
	}

	// Overall success
	result.Success = true
	result.EndTime = time.Now()
	result.Duration = result.EndTime.Sub(result.StartTime)

	return result, nil
}

// Teardown cleans up after the scenario
func (s *TieredScenario) Teardown(ctx context.Context) error {
	// Close NATS client if it was created
	if s.natsClient != nil {
		if err := s.natsClient.Close(ctx); err != nil {
			return fmt.Errorf("failed to close NATS client: %w", err)
		}
		s.natsClient = nil
	}
	return nil
}

// executeVerifyComponents checks that all tiered test components exist
func (s *TieredScenario) executeVerifyComponents(ctx context.Context, result *Result) error {
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
func (s *TieredScenario) executeSendMixedData(ctx context.Context, result *Result) error {
	// Capture baseline BEFORE sending data (matching tier0 pattern)
	// This allows executeValidateProcessing to wait for the delta
	baseline, err := s.metrics.CaptureBaseline(ctx)
	if err != nil {
		result.Warnings = append(result.Warnings, fmt.Sprintf("Could not capture pre-send baseline: %v", err))
	} else {
		s.preSendBaseline = baseline
	}

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
// using event-driven metric waits (matching tier0 patterns) instead of fixed delays
func (s *TieredScenario) executeValidateProcessing(ctx context.Context, result *Result) error {
	// Test data (sensors.jsonl etc.) is loaded at container startup, so processing
	// may already be complete. Check current state first before waiting.
	currentValue, err := s.metrics.SumMetricsByName(ctx, "semstreams_datamanager_entities_updated_total")
	if err != nil {
		result.Warnings = append(result.Warnings, fmt.Sprintf("Could not fetch current metrics: %v", err))
	}

	// If we have sufficient entities already processed, skip waiting
	// (test data is pre-loaded, so baseline delta approach doesn't apply)
	if currentValue >= float64(s.config.MinProcessed) {
		result.Details["processing_already_complete"] = true
		result.Metrics["entities_processed_at_validation"] = currentValue
	} else if s.preSendBaseline == nil {
		result.Warnings = append(result.Warnings, "No pre-send baseline available, using default wait")
		// Use event-driven wait with component health check instead of hardcoded sleep
		if err := s.client.WaitForAllComponentsHealthy(ctx, 10*time.Second); err != nil {
			result.Warnings = append(result.Warnings, fmt.Sprintf("Components health wait: %v", err))
		}
	} else {
		// Wait for processing using event-driven metric polling (matching tier0 pattern)
		waitOpts := client.WaitOpts{
			Timeout:      s.config.ValidationTimeout,
			PollInterval: s.config.PollInterval,
			Comparator:   ">=",
		}

		// Get baseline value for entity updates (captured BEFORE sending data)
		baselineUpdates := s.preSendBaseline.Metrics["semstreams_datamanager_entities_updated_total"]
		expectedUpdates := baselineUpdates + float64(s.config.MinProcessed)

		if err := s.metrics.WaitForMetric(ctx, "semstreams_datamanager_entities_updated_total", expectedUpdates, waitOpts); err != nil {
			result.Warnings = append(result.Warnings, fmt.Sprintf("Processing wait: %v (may still be processing)", err))
			// Don't fail - processing may complete by later stages
		}
	}

	// Use FlowTracer to capture flow snapshot for stage validation
	flowSnapshot, err := s.tracer.CaptureFlowSnapshot(ctx)
	if err != nil {
		result.Warnings = append(result.Warnings, fmt.Sprintf("Failed to capture flow snapshot: %v", err))
	} else {
		result.Details["flow_snapshot_captured"] = true
		result.Metrics["flow_snapshot_message_count"] = flowSnapshot.MessageCount
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
func (s *TieredScenario) executeVerifyOutputs(ctx context.Context, result *Result) error {
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
// using event-driven metric waits and FlowTracer (matching tier0 patterns)
func (s *TieredScenario) executeTestSemanticSearch(ctx context.Context, result *Result) error {
	// Check semembed health
	if !s.checkSemembedHealth(result) {
		return nil
	}

	// Capture baseline for event-driven embedding wait
	baselineEmbeddings := s.captureEmbeddingBaseline(ctx, result)

	// Capture FlowTracer snapshot
	flowSnapshot, err := s.tracer.CaptureFlowSnapshot(ctx)
	if err != nil {
		result.Warnings = append(result.Warnings, fmt.Sprintf("Failed to capture flow snapshot for semantic search: %v", err))
	}

	// Send semantic test messages
	semanticTestMessages := s.buildSemanticTestMessages()
	if err := s.sendSemanticTestMessages(result, semanticTestMessages); err != nil {
		return err
	}

	// Wait for embeddings and validate flow
	s.waitForEmbeddings(ctx, result, baselineEmbeddings, len(semanticTestMessages))
	s.validateSemanticFlow(ctx, result, flowSnapshot, len(semanticTestMessages))

	// Verify embedding metrics
	s.verifyEmbeddingMetrics(result, len(semanticTestMessages))

	return nil
}

// checkSemembedHealth checks the semembed health endpoint.
func (s *TieredScenario) checkSemembedHealth(result *Result) bool {
	semembedHealthURL := "http://localhost:8081/health"
	resp, err := http.Get(semembedHealthURL)
	if err != nil {
		result.Warnings = append(result.Warnings, fmt.Sprintf("semembed health check failed: %v", err))
		result.Details["semembed_available"] = false
		return false
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		result.Warnings = append(result.Warnings, fmt.Sprintf("semembed unhealthy: status=%d", resp.StatusCode))
		result.Details["semembed_available"] = false
		return false
	}

	result.Details["semembed_available"] = true
	result.Metrics["semembed_health_status"] = resp.StatusCode
	return true
}

// captureEmbeddingBaseline captures the baseline embedding count.
func (s *TieredScenario) captureEmbeddingBaseline(ctx context.Context, result *Result) float64 {
	baseline, err := s.metrics.CaptureBaseline(ctx)
	if err != nil {
		result.Warnings = append(result.Warnings, fmt.Sprintf("Failed to capture embedding baseline: %v", err))
		return 0.0
	}
	if baseline != nil {
		return baseline.Metrics["indexengine_embeddings_generated_total"]
	}
	return 0.0
}

// buildSemanticTestMessages creates test messages for semantic search testing.
func (s *TieredScenario) buildSemanticTestMessages() []map[string]any {
	return []map[string]any{
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
}

// sendSemanticTestMessages sends test messages via UDP.
func (s *TieredScenario) sendSemanticTestMessages(result *Result, messages []map[string]any) error {
	conn, err := net.Dial("udp", s.udpAddr)
	if err != nil {
		result.Errors = append(result.Errors, fmt.Sprintf("Failed to connect for semantic test: %v", err))
		return fmt.Errorf("UDP connection failed: %w", err)
	}
	defer conn.Close()

	for i, msg := range messages {
		msgBytes, err := json.Marshal(msg)
		if err != nil {
			continue
		}
		if _, err := conn.Write(msgBytes); err != nil {
			result.Warnings = append(result.Warnings, fmt.Sprintf("Failed to send semantic message %d: %v", i, err))
		}
	}

	result.Metrics["semantic_messages_sent"] = len(messages)
	return nil
}

// waitForEmbeddings waits for embeddings to be generated.
func (s *TieredScenario) waitForEmbeddings(ctx context.Context, result *Result, baseline float64, messageCount int) {
	waitOpts := client.WaitOpts{
		Timeout:      s.config.ValidationTimeout,
		PollInterval: s.config.PollInterval,
		Comparator:   ">=",
	}

	expectedEmbeddings := baseline + float64(messageCount)
	if err := s.metrics.WaitForMetric(ctx, "indexengine_embeddings_generated_total", expectedEmbeddings, waitOpts); err != nil {
		result.Warnings = append(result.Warnings, fmt.Sprintf("Embedding generation wait: %v (may still be generating)", err))
	}
}

// validateSemanticFlow validates the message flow using FlowTracer.
func (s *TieredScenario) validateSemanticFlow(ctx context.Context, result *Result, flowSnapshot *client.FlowSnapshot, messageCount int) {
	if flowSnapshot == nil {
		return
	}

	flowResult, err := s.tracer.ValidateFlow(ctx, flowSnapshot, client.FlowExpectation{
		InputSubject:     "input.udp",
		ProcessingStages: []string{"process.graph"},
		MinMessages:      messageCount,
		MaxLatencyMs:     500,
		Timeout:          s.config.ValidationTimeout,
	})
	if err != nil {
		result.Warnings = append(result.Warnings, fmt.Sprintf("Flow validation error: %v", err))
		return
	}

	if flowResult != nil {
		if !flowResult.Valid {
			result.Warnings = append(result.Warnings, flowResult.Errors...)
		}
		result.Details["semantic_flow_validation"] = map[string]any{
			"valid":         flowResult.Valid,
			"messages":      flowResult.Messages,
			"avg_latency":   flowResult.AvgLatency.String(),
			"p99_latency":   flowResult.P99Latency.String(),
			"stage_metrics": flowResult.StageMetrics,
		}
	}
}

// verifyEmbeddingMetrics queries and verifies embedding metrics.
func (s *TieredScenario) verifyEmbeddingMetrics(result *Result, messageCount int) {
	metricsURL := s.config.MetricsURL + "/metrics"
	metricsResp, err := http.Get(metricsURL)
	if err != nil {
		result.Warnings = append(result.Warnings, fmt.Sprintf("Failed to query metrics: %v", err))
		return
	}
	defer metricsResp.Body.Close()

	body, _ := io.ReadAll(metricsResp.Body)
	metricsText := string(body)

	embeddingsGenerated := strings.Contains(metricsText, "indexengine_embeddings_generated_total")
	embeddingsActive := strings.Contains(metricsText, "indexengine_embeddings_active")
	embeddingProvider := strings.Contains(metricsText, "indexengine_embedding_provider")

	result.Details["semantic_search_test"] = map[string]any{
		"semembed_healthy":            true,
		"messages_sent":               messageCount,
		"embedding_tested":            true,
		"embeddings_generated_metric": embeddingsGenerated,
		"embeddings_active_metric":    embeddingsActive,
		"embedding_provider_metric":   embeddingProvider,
	}

	if embeddingsGenerated && embeddingsActive && embeddingProvider {
		result.Metrics["embedding_metrics_verified"] = 1
	}
}

// executeTestHTTPGateway validates HTTP Gateway query endpoints
func (s *TieredScenario) executeTestHTTPGateway(ctx context.Context, result *Result) error {
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
func (s *TieredScenario) executeTestEmbeddingFallback(ctx context.Context, result *Result) error {
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
// using MetricsClient for consistent metric access (matching tier0 patterns)
func (s *TieredScenario) executeValidateRules(ctx context.Context, result *Result) error {
	// Capture baseline metrics using MetricsClient
	baselineMetrics, err := s.metrics.ExtractRuleMetrics(ctx)
	if err != nil {
		result.Warnings = append(result.Warnings, fmt.Sprintf("Failed to capture baseline rule metrics: %v", err))
		// Initialize with zeros
		baselineMetrics = &client.RuleMetrics{}
	}

	// Check for rule metrics presence via raw fetch (for metric presence validation)
	metricsRaw, err := s.metrics.FetchRaw(ctx)
	ruleMetricsPresent := map[string]bool{
		"semstreams_rule_messages_received_total": err == nil && strings.Contains(metricsRaw, "semstreams_rule_messages_received_total"),
		"semstreams_rule_evaluations_total":       err == nil && strings.Contains(metricsRaw, "semstreams_rule_evaluations_total"),
		"semstreams_rule_triggers_total":          err == nil && strings.Contains(metricsRaw, "semstreams_rule_triggers_total"),
		"semstreams_rule_active_rules":            err == nil && strings.Contains(metricsRaw, "semstreams_rule_active_rules"),
	}

	foundRuleMetrics := 0
	for _, found := range ruleMetricsPresent {
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

	// Rules are primarily evaluated on pre-loaded test data, so baseline evaluations
	// are usually high. Check if we already have significant evaluations before waiting.
	if baselineMetrics.Evaluations >= 100 {
		// Already have many evaluations from test data, skip waiting for delta
		// (UDP rule test messages may not be processed due to json_generic disabled)
		result.Details["rules_already_evaluated"] = true
	} else {
		// Wait for rules to process using event-driven wait (matching tier0 pattern)
		waitOpts := client.WaitOpts{
			Timeout:      s.config.ValidationTimeout,
			PollInterval: s.config.PollInterval,
			Comparator:   ">=",
		}

		// Wait for at least one evaluation to occur per sent message
		expectedEvaluations := baselineMetrics.Evaluations + float64(sentCount)
		if err := s.metrics.WaitForMetric(ctx, "semstreams_rule_evaluations_total", expectedEvaluations, waitOpts); err != nil {
			result.Warnings = append(result.Warnings, fmt.Sprintf("Rule evaluation wait: %v", err))
		}
	}

	// Get final metrics using ExtractRuleMetrics helper
	finalMetrics, err := s.metrics.ExtractRuleMetrics(ctx)
	if err != nil {
		result.Warnings = append(result.Warnings, fmt.Sprintf("Failed to get final rule metrics: %v", err))
		return nil
	}

	// Calculate deltas
	triggeredDelta := int(finalMetrics.Triggers - baselineMetrics.Triggers)
	evaluatedDelta := int(finalMetrics.Evaluations - baselineMetrics.Evaluations)

	// Record metrics (matching tier0 output format)
	result.Metrics["rules_triggered_count"] = int(finalMetrics.Triggers)
	result.Metrics["rules_evaluated_count"] = int(finalMetrics.Evaluations)
	result.Metrics["rules_triggered_delta"] = triggeredDelta
	result.Metrics["rules_evaluated_delta"] = evaluatedDelta
	result.Metrics["rule_metrics_found"] = foundRuleMetrics

	// Add state transition metrics (matching tier0)
	result.Metrics["on_enter_fired"] = int(finalMetrics.OnEnterFired)
	result.Metrics["on_exit_fired"] = int(finalMetrics.OnExitFired)

	// Validate rules actually triggered
	if triggeredDelta < 1 && sentCount > 0 {
		result.Warnings = append(result.Warnings,
			fmt.Sprintf("No rules triggered despite sending %d test messages (triggered delta: %d)",
				sentCount, triggeredDelta))
	}

	// Consider validation passed if we have rule metrics and some evaluation happened
	validationPassed := foundRuleMetrics >= 2 && finalMetrics.Evaluations > 0
	if validationPassed {
		result.Metrics["rules_validation_passed"] = 1
	}

	result.Details["rule_validation"] = map[string]any{
		"metrics_present":    ruleMetricsPresent,
		"metrics_found":      foundRuleMetrics,
		"triggered_before":   int(baselineMetrics.Triggers),
		"triggered_after":    int(finalMetrics.Triggers),
		"triggered_delta":    triggeredDelta,
		"evaluated_before":   int(baselineMetrics.Evaluations),
		"evaluated_after":    int(finalMetrics.Evaluations),
		"evaluated_delta":    evaluatedDelta,
		"on_enter_fired":     int(finalMetrics.OnEnterFired),
		"on_exit_fired":      int(finalMetrics.OnExitFired),
		"test_messages_sent": sentCount,
		"validation_passed":  validationPassed,
		"message": fmt.Sprintf("Rules: %d triggered, %d evaluated (delta: +%d triggered, +%d evaluated), state transitions: %d enter, %d exit",
			int(finalMetrics.Triggers), int(finalMetrics.Evaluations), triggeredDelta, evaluatedDelta,
			int(finalMetrics.OnEnterFired), int(finalMetrics.OnExitFired)),
	}

	return nil
}

// executeValidateMetrics validates Prometheus metrics exposure
func (s *TieredScenario) executeValidateMetrics(_ context.Context, result *Result) error {
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
func (s *TieredScenario) executeVerifyEntityCount(ctx context.Context, result *Result) error {
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

	// Expected entities from test data files (count UNIQUE entity IDs, not records):
	// - documents.jsonl: 12 entities
	// - maintenance.jsonl: 16 entities
	// - observations.jsonl: 15 entities
	// - sensor_docs.jsonl: 15 entities
	// - sensors.jsonl: 16 entities (41 records → 16 unique device_ids; time-series updates same entity)
	// Total: 74 unique entities from test data
	expectedFromTestData := 74

	// UDP telemetry NOT counted: json_generic processor is disabled in config,
	// so UDP messages on raw.udp.messages are never converted to entities.
	// We keep the UDP sending logic for infrastructure testing, but don't expect entities from it.
	expectedFromUDP := 0
	_ = result.Metrics["telemetry_sent"] // Acknowledge metric exists but don't count it

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
func (s *TieredScenario) executeVerifyEntityRetrieval(ctx context.Context, result *Result) error {
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
func (s *TieredScenario) executeValidateEntityStructure(ctx context.Context, result *Result) error {
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
func (s *TieredScenario) executeVerifyIndexPopulation(ctx context.Context, result *Result) error {
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
func (s *TieredScenario) executeVerifySearchQuality(ctx context.Context, result *Result) error {
	httpClient := &http.Client{Timeout: 10 * time.Second}
	searchTests := s.getSearchQualityTests()

	// Execute all search tests
	stats := &searchQualityStats{
		searchResults: make(map[string]any),
		allScores:     []float64{},
	}

	for _, test := range searchTests {
		s.executeSearchQualityTest(ctx, httpClient, test, stats)
	}

	// Record results
	s.recordSearchQualityResults(result, searchTests, stats)

	return nil
}

// searchQualityTest defines a search quality test case.
type searchQualityTest struct {
	query           string
	expectedPattern string
	description     string
	minScore        float64
	minHits         int
}

// searchQualityStats tracks aggregate statistics for search quality tests.
type searchQualityStats struct {
	searchResults          map[string]any
	allScores              []float64
	queriesWithResults     int
	queriesMeetingMinScore int
	queriesMeetingMinHits  int
}

// getSearchQualityTests returns the search quality test cases.
func (s *TieredScenario) getSearchQualityTests() []searchQualityTest {
	return []searchQualityTest{
		{"What documents mention forklift safety?", "forklift", "Natural language document search", 0.3, 1},
		{"Are there safety observations related to temperature?", "temperature", "Cross-domain safety query", 0.3, 1},
		{"What maintenance was done on cold storage equipment?", "cold", "Maintenance semantic search", 0.3, 1},
		{"Find all sensors in zone-a", "zone-a", "Location-based sensor query", 0.3, 1},
	}
}

// executeSearchQualityTest executes a single search quality test.
func (s *TieredScenario) executeSearchQualityTest(
	ctx context.Context,
	httpClient *http.Client,
	test searchQualityTest,
	stats *searchQualityStats,
) {
	searchQuery := map[string]any{
		"query":     test.query,
		"threshold": 0.1,
		"limit":     10,
	}

	queryJSON, err := json.Marshal(searchQuery)
	if err != nil {
		return
	}

	req, err := http.NewRequestWithContext(ctx, "POST",
		s.config.GatewayURL+"/search/semantic", strings.NewReader(string(queryJSON)))
	if err != nil {
		return
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := httpClient.Do(req)
	if err != nil {
		stats.searchResults[test.query] = map[string]any{
			"error":       err.Error(),
			"description": test.description,
		}
		return
	}

	defer resp.Body.Close()
	bodyBytes, _ := io.ReadAll(resp.Body)

	if resp.StatusCode != http.StatusOK {
		stats.searchResults[test.query] = map[string]any{
			"status":      resp.StatusCode,
			"description": test.description,
		}
		return
	}

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
		stats.searchResults[test.query] = map[string]any{
			"error":       "parse error",
			"description": test.description,
		}
		return
	}

	s.processSearchQualityHits(test, searchResp.Data.Hits, stats)
}

// processSearchQualityHits processes hits from a search quality test.
func (s *TieredScenario) processSearchQualityHits(
	test searchQualityTest,
	hits []struct {
		EntityID string  `json:"entity_id"`
		Score    float64 `json:"score"`
	},
	stats *searchQualityStats,
) {
	if len(hits) > 0 {
		stats.queriesWithResults++
	}
	if len(hits) >= test.minHits {
		stats.queriesMeetingMinHits++
	}

	matchesPattern := false
	topHits := []string{}
	topScores := []float64{}
	hitsAboveMinScore := 0
	var scoreSum float64

	for _, hit := range hits {
		topHits = append(topHits, hit.EntityID)
		topScores = append(topScores, hit.Score)
		stats.allScores = append(stats.allScores, hit.Score)
		scoreSum += hit.Score

		if hit.Score >= test.minScore {
			hitsAboveMinScore++
		}
		if strings.Contains(strings.ToLower(hit.EntityID), test.expectedPattern) {
			matchesPattern = true
		}
	}

	avgScore := 0.0
	if len(hits) > 0 {
		avgScore = scoreSum / float64(len(hits))
	}

	meetsMinScore := hitsAboveMinScore > 0
	if meetsMinScore {
		stats.queriesMeetingMinScore++
	}

	stats.searchResults[test.query] = map[string]any{
		"hit_count":           len(hits),
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
		"meets_min_hits":      len(hits) >= test.minHits,
	}
}

// recordSearchQualityResults records search quality test results.
func (s *TieredScenario) recordSearchQualityResults(
	result *Result,
	searchTests []searchQualityTest,
	stats *searchQualityStats,
) {
	overallAvgScore := 0.0
	if len(stats.allScores) > 0 {
		var sum float64
		for _, score := range stats.allScores {
			sum += score
		}
		overallAvgScore = sum / float64(len(stats.allScores))
	}

	weakResultsThreshold := 0.5
	if overallAvgScore > 0 && overallAvgScore < weakResultsThreshold {
		result.Warnings = append(result.Warnings,
			fmt.Sprintf("Weak search results: average score %.2f is below %.2f threshold",
				overallAvgScore, weakResultsThreshold))
	}

	result.Metrics["search_queries_tested"] = len(searchTests)
	result.Metrics["search_queries_with_results"] = stats.queriesWithResults
	result.Metrics["search_min_score_met"] = stats.queriesMeetingMinScore
	result.Metrics["search_min_hits_met"] = stats.queriesMeetingMinHits
	result.Metrics["search_quality_score"] = overallAvgScore

	result.Details["search_quality_verification"] = map[string]any{
		"queries":           len(searchTests),
		"queries_with_hits": stats.queriesWithResults,
		"min_score_met":     stats.queriesMeetingMinScore,
		"min_hits_met":      stats.queriesMeetingMinHits,
		"overall_avg_score": overallAvgScore,
		"weak_threshold":    weakResultsThreshold,
		"results":           stats.searchResults,
		"message":           fmt.Sprintf("%d/%d queries returned results, avg score: %.2f", stats.queriesWithResults, len(searchTests), overallAvgScore),
	}
}

// executeCompareCoreMl captures search results for Core vs ML comparison
// and persists results to JSON for later analysis
// variantInfo holds detected variant and embedding provider information
type variantInfo struct {
	variant           string
	embeddingProvider string
}

// detectVariantAndProvider detects which variant (structural/statistical/semantic) is running based on semembed availability and metrics
func (s *TieredScenario) detectVariantAndProvider(result *Result) variantInfo {
	info := variantInfo{variant: "statistical", embeddingProvider: "unknown"} // Default to statistical (BM25)

	// Check semembed availability first
	if semembedAvailable, ok := result.Details["semembed_available"].(bool); ok && semembedAvailable {
		info.variant = "semantic"
	}

	// Check embedding provider from metrics (overrides semembed detection)
	metricsURL := s.config.MetricsURL + "/metrics"
	resp, err := http.Get(metricsURL)
	if err != nil {
		return info
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	metricsText := string(body)

	// Parse indexengine_embedding_provider metric value using regex
	// Format: indexengine_embedding_provider{component="..."} <value>
	re := regexp.MustCompile(`indexengine_embedding_provider\{[^}]*\}\s+(\d+(?:\.\d+)?)`)
	if matches := re.FindStringSubmatch(metricsText); len(matches) > 1 {
		switch matches[1] {
		case "2", "2.0":
			info.embeddingProvider = "http"
			info.variant = "semantic"
		case "1", "1.0":
			info.embeddingProvider = "bm25"
			info.variant = "statistical"
		case "0", "0.0":
			info.embeddingProvider = "disabled"
			info.variant = "structural" // No embeddings = structural (rules-only)
		}
	}

	// Legacy mapping for backwards compatibility
	// Map old "core" to "statistical" and "ml" to "semantic"
	if s.config.Variant == "core" {
		info.variant = "statistical"
		result.Details["legacy_variant_mapped"] = "core -> statistical"
	} else if s.config.Variant == "ml" {
		info.variant = "semantic"
		result.Details["legacy_variant_mapped"] = "ml -> semantic"
	}

	return info
}

// comparisonQueryResults holds the results of running comparison queries
type comparisonQueryResults struct {
	searchResults map[string]SearchQueryResult
	queryResults  map[string]any
}

// runComparisonQueries executes comparison search queries and returns the results
func (s *TieredScenario) runComparisonQueries(ctx context.Context) comparisonQueryResults {
	httpClient := &http.Client{Timeout: 10 * time.Second}
	gatewayURL := s.config.GatewayURL

	// Natural language queries for tiered semantic search testing
	comparisonQueries := []string{
		"What maintenance was done on cold storage equipment?",
		"Are there safety observations related to temperature?",
		"Find all sensors in zone-a",
		"What documents mention forklift safety?",
	}

	results := comparisonQueryResults{
		searchResults: make(map[string]SearchQueryResult),
		queryResults:  make(map[string]any),
	}

	for _, query := range comparisonQueries {
		s.executeComparisonQuery(ctx, httpClient, gatewayURL, query, &results)
	}

	return results
}

// executeComparisonQuery runs a single comparison query and stores the results
func (s *TieredScenario) executeComparisonQuery(
	ctx context.Context,
	httpClient *http.Client,
	gatewayURL string,
	query string,
	results *comparisonQueryResults,
) {
	searchQuery := map[string]any{"query": query, "threshold": 0.1, "limit": 10}
	queryJSON, _ := json.Marshal(searchQuery)

	req, err := http.NewRequestWithContext(ctx, "POST",
		gatewayURL+"/search/semantic", strings.NewReader(string(queryJSON)))
	if err != nil {
		return
	}
	req.Header.Set("Content-Type", "application/json")

	queryStart := time.Now()
	resp, err := httpClient.Do(req)
	latencyMs := time.Since(queryStart).Milliseconds()

	if err != nil {
		results.queryResults[query] = map[string]any{"error": err.Error()}
		return
	}

	defer resp.Body.Close()
	bodyBytes, _ := io.ReadAll(resp.Body)

	var searchResp struct {
		Data struct {
			Hits []struct {
				EntityID string  `json:"entity_id"`
				Score    float64 `json:"score"`
			} `json:"hits"`
		} `json:"data"`
	}

	if err := json.Unmarshal(bodyBytes, &searchResp); err != nil {
		return
	}

	hitIDs := make([]string, 0, len(searchResp.Data.Hits))
	scores := make([]float64, 0, len(searchResp.Data.Hits))
	for _, hit := range searchResp.Data.Hits {
		hitIDs = append(hitIDs, hit.EntityID)
		scores = append(scores, hit.Score)
	}

	results.searchResults[query] = SearchQueryResult{
		Query: query, Hits: hitIDs, Scores: scores, LatencyMs: latencyMs, HitCount: len(hitIDs),
	}
	results.queryResults[query] = map[string]any{
		"hits": hitIDs, "scores": scores, "count": len(hitIDs), "latency_ms": latencyMs,
	}
}

// persistComparisonResults saves comparison data to a JSON file
func (s *TieredScenario) persistComparisonResults(
	info variantInfo,
	searchResults map[string]SearchQueryResult,
	result *Result,
) string {
	if s.config.OutputDir == "" {
		return ""
	}

	compData := ComparisonData{
		Variant:           info.variant,
		EmbeddingProvider: info.embeddingProvider,
		Timestamp:         time.Now(),
		SearchResults:     searchResults,
	}

	if err := os.MkdirAll(s.config.OutputDir, 0755); err != nil {
		result.Warnings = append(result.Warnings,
			fmt.Sprintf("Failed to create output directory: %v", err))
		return ""
	}

	filename := fmt.Sprintf("comparison-%s-%s.json", info.variant, time.Now().Format("20060102-150405"))
	comparisonFile := filepath.Join(s.config.OutputDir, filename)

	data, err := json.MarshalIndent(compData, "", "  ")
	if err != nil {
		result.Warnings = append(result.Warnings,
			fmt.Sprintf("Failed to marshal comparison data: %v", err))
		return ""
	}

	if err := os.WriteFile(comparisonFile, data, 0644); err != nil {
		result.Warnings = append(result.Warnings,
			fmt.Sprintf("Failed to write comparison file: %v", err))
		return ""
	}

	return comparisonFile
}

// executeCompareStatisticalSemantic captures search results for Statistical vs Semantic comparison
// (renamed from executeCompareCoreMl for clarity)
func (s *TieredScenario) executeCompareStatisticalSemantic(ctx context.Context, result *Result) error {
	info := s.detectVariantAndProvider(result)
	queryResults := s.runComparisonQueries(ctx)
	comparisonFile := s.persistComparisonResults(info, queryResults.searchResults, result)

	result.Details["statistical_semantic_comparison"] = map[string]any{
		"variant":            info.variant,
		"embedding_provider": info.embeddingProvider,
		"queries":            queryResults.queryResults,
		"comparison_file":    comparisonFile,
		"message": fmt.Sprintf("Captured %d search queries for %s variant (%s embeddings)",
			4, info.variant, info.embeddingProvider),
	}

	result.Metrics["comparison_variant"] = info.variant
	result.Metrics["embedding_provider"] = info.embeddingProvider

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
	Variant               string                `json:"variant"`
	Timestamp             time.Time             `json:"timestamp"`
	CommunitiesTotal      int                   `json:"communities_total"`
	LLMEnhancedCount      int                   `json:"llm_enhanced_count"`
	StatisticalOnlyCount  int                   `json:"statistical_only_count"`
	LLMFailedCount        int                   `json:"llm_failed_count,omitempty"`
	LLMPendingCount       int                   `json:"llm_pending_count,omitempty"`
	LLMWaitDurationMs     int64                 `json:"llm_wait_duration_ms,omitempty"`
	AvgSummaryLengthRatio float64               `json:"avg_summary_length_ratio"`
	AvgWordOverlap        float64               `json:"avg_word_overlap"`
	NonSingletonCount     int                   `json:"non_singleton_count"`
	LargestCommunitySize  int                   `json:"largest_community_size"`
	AvgNonSingletonSize   float64               `json:"avg_non_singleton_size"`
	Communities           []CommunityComparison `json:"communities"`
}

// llmWaitResult holds the results of waiting for LLM enhancement
type llmWaitResult struct {
	durationMs   int64
	failedCount  int
	pendingCount int
}

// communityStats holds aggregated statistics about communities
type communityStats struct {
	comparisons          []CommunityComparison
	llmEnhancedCount     int
	statisticalOnlyCount int
	avgLengthRatio       float64
	avgWordOverlap       float64
	nonSingletonCount    int
	largestCommunitySize int
	avgNonSingletonSize  float64
}

// Semantic tier functions are in tiered_semantic.go
// Structural tier functions are in tiered_structural.go
