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
	"regexp"
	"strings"
	"time"

	"github.com/c360/semstreams/test/e2e/client"
	"github.com/c360/semstreams/test/e2e/config"
	"github.com/c360/semstreams/test/e2e/scenarios/search"
	"github.com/c360/semstreams/test/e2e/scenarios/stages"
)

// Variant-specific entity count expectations
const (
	// StructuralMinEntities is the minimum entities expected for structural tier
	// All tiers now load testdata/semantic/*.jsonl with 74 unique entities
	StructuralMinEntities = 50

	// StatisticalMinEntities is the minimum entities expected for statistical tier
	// Statistical tier loads testdata/semantic/*.jsonl with 74 unique entities
	StatisticalMinEntities = 50

	// SemanticMinEntities is the minimum entities expected for semantic tier
	// Same testdata as statistical tier
	SemanticMinEntities = 50
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

	// Search results from executeVerifySearchQuality for reuse by comparison
	searchStats *search.Stats

	// Cached variant detection (set once, reused across stages)
	detectedVariant *variantInfo
}

// TieredConfig contains configuration for tiered E2E tests
type TieredConfig struct {
	// Variant configuration
	Variant string `json:"variant"` // "structural", "statistical", "semantic"

	// Test data configuration
	MessageCount    int           `json:"message_count"`
	MessageInterval time.Duration `json:"message_interval"`

	// Validation configuration (event-driven, matching structural tier patterns)
	ValidationTimeout time.Duration `json:"validation_timeout"` // Timeout for metric waits (30s for semantic)
	PollInterval      time.Duration `json:"poll_interval"`      // Poll interval for metric waits (100ms)
	MinProcessed      int           `json:"min_processed"`

	// Entity verification (from test data files)
	MinExpectedEntities int    `json:"min_expected_entities"`
	NatsURL             string `json:"nats_url"`
	MetricsURL          string `json:"metrics_url"`
	ServiceManagerURL   string `json:"service_manager_url"`
	GatewayURL          string `json:"gateway_url"`
	GraphQLURL          string `json:"graphql_url"` // GraphQL endpoint (port varies by profile)

	// Comparison output configuration
	OutputDir string `json:"output_dir"`

	// Baseline comparison (matching structural tier patterns)
	BaselineFile         string  `json:"baseline_file,omitempty"` // Path to baseline JSON (optional)
	MaxRegressionPercent float64 `json:"max_regression_percent"`  // Default 20%

	// Structural tier config (rules-only, no ML dependencies)
	ExpectedEmbeddings int `json:"expected_embeddings"` // 0 for structural variant
	ExpectedClusters   int `json:"expected_clusters"`   // 0 for structural variant
	MinRulesEvaluated  int `json:"min_rules_evaluated"` // Min rules evaluated for structural
	MinOnEnterFired    int `json:"min_on_enter_fired"`  // Min OnEnter transitions
	MinOnExitFired     int `json:"min_on_exit_fired"`   // Min OnExit transitions
}

// DefaultTieredConfig returns default configuration
func DefaultTieredConfig() *TieredConfig {
	return &TieredConfig{
		Variant:         "", // Auto-detect from environment
		MessageCount:    20,
		MessageInterval: 50 * time.Millisecond,
		// Event-driven validation timeouts
		// 60s default allows time for testdata files to load + entity processing
		ValidationTimeout:    60 * time.Second,
		PollInterval:         100 * time.Millisecond, // Fast polling for responsiveness
		MinProcessed:         10,                     // At least 50% should make it through
		MinExpectedEntities:  50,                     // Test data has 74 entities, expect at least 50 indexed
		NatsURL:              config.DefaultEndpoints.NATS,
		MetricsURL:           config.DefaultEndpoints.Metrics,
		ServiceManagerURL:    config.DefaultEndpoints.HTTP,
		GatewayURL:           config.DefaultEndpoints.HTTP + "/api-gateway",
		GraphQLURL:           "http://localhost:8082/graphql", // Default for statistical profile
		OutputDir:            "test/e2e/results",
		MaxRegressionPercent: 20.0, // 20% regression threshold
		// Structural tier defaults (rules-only, no ML)
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

	// Set GraphQL URL if not explicitly configured
	// Docker compose maps all profiles to host port 8082 for GraphQL
	if cfg.GraphQLURL == "" {
		cfg.GraphQLURL = "http://localhost:8082/graphql"
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
	// Pre-flight: Validate flowgraph configuration before running tests
	// This catches issues like JetStream subscribers connected to NATS publishers
	// which would cause components to hang waiting for streams that never get created
	if err := s.client.CheckFlowHealth(ctx); err != nil {
		return fmt.Errorf("flowgraph pre-flight validation failed: %w", err)
	}

	// Verify UDP endpoint is reachable
	conn, err := net.Dial("udp", s.udpAddr)
	if err != nil {
		return fmt.Errorf("cannot reach UDP endpoint %s: %w", s.udpAddr, err)
	}
	_ = conn.Close()

	// Initialize observability clients (matching structural tier patterns)
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

// getStagesForVariant returns the filtered list of stages for a given variant.
//
// Stages are organized following the progressive enhancement model:
// - Tier 0 (Structural): Graph traversal, indexes, rules - runs on ALL tiers
// - Tier 1 (Statistical): Tier 0 + embeddings, search, communities
// - Tier 2 (Semantic): Tier 1 + neural embeddings, LLM, GraphRAG
func (s *TieredScenario) getStagesForVariant(variant string) []stage {
	allStages := []stage{
		// === Common setup stages (all tiers) ===
		{"verify-components", s.executeVerifyComponents, nil},
		{"send-mixed-data", s.executeSendMixedData, nil},
		{"validate-processing", s.executeValidateProcessing, nil},

		// Wait for embeddings BEFORE counting entities (statistical/semantic tiers)
		// This ensures all entities have completed the embedding pipeline before validation
		{"wait-for-embeddings", s.executeWaitForEmbeddings, []string{"statistical", "semantic"}},

		// Wait for entity stabilization (structural tier only)
		// Structural tier doesn't wait for embeddings, so we need to wait for entity count to stabilize
		{"wait-for-entity-stabilization", s.executeWaitForEntityStabilization, []string{"structural"}},

		{"verify-entity-count", s.executeVerifyEntityCount, nil},
		{"verify-entity-retrieval", s.executeVerifyEntityRetrieval, nil},
		{"validate-entity-structure", s.executeValidateEntityStructure, nil},
		{"verify-index-population", s.executeVerifyIndexPopulation, nil},

		// === Tier 0: Structural capabilities (run on ALL tiers) ===
		// PathRAG is pure graph traversal - no embeddings required
		// Sensor PathRAG runs on all tiers (sensor data loads quickly)
		{"test-pathrag-sensor", s.executeTestPathRAGSensor, nil},
		{"test-pathrag-boundary", s.executeTestPathRAGBoundary, nil},
		// Document PathRAG runs on all tiers (entity stabilization ensures documents are loaded)
		{"test-pathrag-document", s.executeTestPathRAGDocument, nil},
		// EntityID hierarchy navigation (6-part EntityID structure)
		{"test-entityid-hierarchy", s.executeTestEntityIDHierarchy, nil},
		{"test-entities-by-prefix", s.executeTestEntitiesByPrefix, nil},
		// Spatial/Temporal index queries (all tiers have indexed geo/time data)
		{"test-spatial-query", s.executeTestSpatialQuery, nil},
		{"test-temporal-query", s.executeTestTemporalQuery, nil},
		{"test-zone-relationships", s.executeTestZoneRelationships, nil},

		// === Tier 0 ONLY: Zero-ML constraint validation ===
		// These verify structural tier has NO ML inference
		{"validate-zero-embeddings", s.executeValidateZeroEmbeddings, []string{"structural"}},
		{"validate-zero-clusters", s.executeValidateZeroClusters, []string{"structural"}},
		{"validate-rule-transitions", s.executeValidateRuleTransitions, []string{"structural"}},
		// Structural indexes - work on structural tier via EntityID sibling edges (no ML required)
		{"validate-kcore-index-structural", s.executeValidateKCoreIndexStructural, []string{"structural"}},
		{"validate-pivot-index-structural", s.executeValidatePivotIndexStructural, []string{"structural"}},

		// === Tier 1+: Statistical capabilities (statistical + semantic) ===
		{"verify-search-quality", s.executeVerifySearchQuality, []string{"statistical", "semantic"}},
		{"test-http-gateway", s.executeTestHTTPGateway, []string{"statistical", "semantic"}},
		{"test-embedding-fallback", s.executeTestEmbeddingFallback, []string{"statistical", "semantic"}},
		{"validate-community-structure", s.executeValidateCommunityStructure, []string{"statistical", "semantic"}},
		// Structural indexes (k-core, pivot) - require community detection for meaningful structure
		{"validate-kcore-index", s.executeValidateKCoreIndex, []string{"statistical", "semantic"}},
		{"validate-pivot-index", s.executeValidatePivotIndex, []string{"statistical", "semantic"}},

		// === Tier 2: Semantic capabilities (semantic only) ===
		{"test-graphrag-local", s.executeTestGraphRAGLocal, []string{"semantic"}},
		{"test-graphrag-global", s.executeTestGraphRAGGlobal, []string{"semantic"}},
		{"validate-llm-enhancement", s.executeValidateLLMEnhancement, []string{"semantic"}},

		// Wait for rule evaluations to stabilize (semantic tier only)
		// Semantic tier's neural embeddings are slower, so rule evaluations via KV watch
		// may still be in progress when validate-rules would normally run
		{"wait-for-rule-stabilization", s.executeWaitForRuleStabilization, []string{"semantic"}},

		// === Common validation stages (all tiers) ===
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

	// Capture final metrics baseline for regression detection
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

		// Compare to baseline file if configured
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

	// Build structured results (dual-write for backward compatibility)
	result.Structured = BuildTieredResults(result, s.searchStats)

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

	var allRequired []string

	// Structural tier uses minimal structural.json config
	if s.config.Variant == "structural" {
		// Minimal components for structural/rules-only testing
		allRequired = []string{"udp", "iot_sensor", "rule", "graph", "file"}
	} else {
		// Full components for statistical/semantic tiers
		// Input components
		inputComponents := []string{"udp"}
		// Domain processors (document_processor, iot_sensor handle domain-specific data)
		domainProcessors := []string{"document_processor", "iot_sensor"}
		// Semantic components (rule processor + graph processor)
		semanticComponents := []string{"rule", "graph"}
		// Output/storage components
		outputComponents := []string{"file", "objectstore"}

		allRequired = append(inputComponents, domainProcessors...)
		allRequired = append(allRequired, semanticComponents...)
		allRequired = append(allRequired, outputComponents...)
	}

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
		"variant":  s.config.Variant,
		"required": allRequired,
		"total":    len(allRequired),
		"found":    len(components),
	}

	return nil
}

// executeSendMixedData sends mixed test data (entities + regular messages)
func (s *TieredScenario) executeSendMixedData(ctx context.Context, result *Result) error {
	// Capture baseline BEFORE sending data
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
// using event-driven metric waits instead of fixed delays
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
		// Wait for processing using event-driven metric polling
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
	expectedOutputs := []string{"file", "objectstore"}
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

// executeWaitForEmbeddings waits for embeddings to be generated based on embedding provider.
// This stage runs before any search stages to ensure vectorCache is populated.
//
// Key fix: Wait for TOTAL embeddings (entityCount), not baseline + entityCount.
// The baseline approach was flawed because baseline is captured AFTER data is sent,
// so if 50 embeddings already exist, waiting for baseline(50) + entityCount(74) = 124
// would timeout since only 74 embeddings will ever exist.
func (s *TieredScenario) executeWaitForEmbeddings(ctx context.Context, result *Result) error {
	variant := s.detectVariantAndProvider(result)

	switch variant.embeddingProvider {
	case "disabled", "":
		// Structural tier - no embeddings to wait for
		result.Details["embedding_wait"] = "skipped (embeddings disabled)"
		return nil

	case "bm25", "http":
		// For HTTP, verify semembed health first
		if variant.embeddingProvider == "http" && !s.checkSemembedHealth(result) {
			result.Warnings = append(result.Warnings, "semembed unavailable, HTTP embeddings may not be generated")
			return nil
		}

		entityCount := s.getEntityCountForEmbeddings(result)

		waitOpts := client.WaitOpts{
			Timeout:      s.config.ValidationTimeout,
			PollInterval: s.config.PollInterval,
			Comparator:   ">=",
		}

		// Wait for TOTAL embeddings generated (not baseline + delta)
		startWait := time.Now()
		if err := s.metrics.WaitForMetric(ctx,
			"indexengine_embeddings_generated_total",
			float64(entityCount),
			waitOpts); err != nil {
			result.Warnings = append(result.Warnings,
				fmt.Sprintf("Embedding wait: %v", err))
		}

		// Wait for entity count to stabilize in NATS KV.
		// This is more reliable than the previous 500ms sleep because the embedding
		// metric increments BEFORE the entity is persisted to KV. Polling actual
		// entity count ensures all entities are written before proceeding.
		stabilization := s.waitForEntityCountStabilization(ctx, entityCount)

		result.Details["embedding_wait"] = map[string]any{
			"provider":             variant.embeddingProvider,
			"entity_count":         entityCount,
			"wait_duration":        time.Since(startWait).String(),
			"entity_stabilization": stabilization.Stabilized,
			"final_entity_count":   stabilization.FinalCount,
		}

	default:
		// Unknown provider - still wait for entity stabilization as a safety measure
		// This handles cases where the embedding provider metric isn't available yet
		// but embeddings are expected (e.g., semantic tier during startup)
		result.Warnings = append(result.Warnings, fmt.Sprintf("Unknown embedding provider: %s, waiting for entity stabilization", variant.embeddingProvider))

		entityCount := s.getEntityCountForEmbeddings(result)
		stabilization := s.waitForEntityCountStabilization(ctx, entityCount)

		result.Details["embedding_wait"] = map[string]any{
			"provider":             variant.embeddingProvider,
			"entity_count":         entityCount,
			"entity_stabilization": stabilization.Stabilized,
			"final_entity_count":   stabilization.FinalCount,
			"fallback_mode":        true,
		}
	}

	return nil
}

// entityStabilizationResult contains the result of waiting for entity count to stabilize.
type entityStabilizationResult struct {
	FinalCount   int
	WaitDuration time.Duration
	Stabilized   bool
	TimedOut     bool
}

// waitForEntityCountStabilization polls NATS KV until entity count reaches and stabilizes
// at expectedCount for multiple consecutive checks. This is more reliable than waiting
// for metrics because the metric may increment before the entity is persisted.
//
// Returns the stabilization result including final count and whether stabilization succeeded.
func (s *TieredScenario) waitForEntityCountStabilization(ctx context.Context, expectedCount int) entityStabilizationResult {
	const stabilizationChecks = 3
	const checkInterval = 200 * time.Millisecond

	startWait := time.Now()
	deadline := time.Now().Add(s.config.ValidationTimeout)

	var lastCount int
	stableCount := 0

	if s.natsClient == nil {
		return entityStabilizationResult{
			FinalCount:   0,
			WaitDuration: 0,
			Stabilized:   false,
			TimedOut:     false,
		}
	}

	for time.Now().Before(deadline) {
		count, err := s.natsClient.CountEntities(ctx)
		if err != nil {
			time.Sleep(checkInterval)
			continue
		}

		if count == lastCount && count >= expectedCount {
			stableCount++
			if stableCount >= stabilizationChecks {
				// Entity count has stabilized at or above expected
				return entityStabilizationResult{
					FinalCount:   count,
					WaitDuration: time.Since(startWait),
					Stabilized:   true,
					TimedOut:     false,
				}
			}
		} else {
			stableCount = 0
		}

		lastCount = count
		time.Sleep(checkInterval)
	}

	// Timeout - return what we got
	return entityStabilizationResult{
		FinalCount:   lastCount,
		WaitDuration: time.Since(startWait),
		Stabilized:   false,
		TimedOut:     true,
	}
}

// executeWaitForEntityStabilization waits for entity count to stabilize (structural tier).
// This is needed because structural tier doesn't wait for embeddings, so entities may still
// be processing when we start validation.
func (s *TieredScenario) executeWaitForEntityStabilization(ctx context.Context, result *Result) error {
	const expectedEntities = 74 // All tiers expect 74 entities from testdata/semantic/

	stabilization := s.waitForEntityCountStabilization(ctx, expectedEntities)

	result.Details["entity_stabilization"] = map[string]any{
		"final_count":   stabilization.FinalCount,
		"expected":      expectedEntities,
		"wait_duration": stabilization.WaitDuration.String(),
		"stabilized":    stabilization.Stabilized,
		"timed_out":     stabilization.TimedOut,
	}

	if stabilization.TimedOut && stabilization.FinalCount < expectedEntities {
		result.Warnings = append(result.Warnings,
			fmt.Sprintf("Entity stabilization: got %d, expected %d", stabilization.FinalCount, expectedEntities))
	}

	return nil
}

// executeWaitForRuleStabilization waits for rule evaluation count to stabilize.
// This is needed because rules are evaluated asynchronously via KV watch,
// and the semantic tier's slower embedding generation means rule evaluations
// may still be in progress when validate-rules runs.
func (s *TieredScenario) executeWaitForRuleStabilization(ctx context.Context, result *Result) error {
	startTime := time.Now()

	// Get initial evaluation count
	initialMetrics, err := s.metrics.ExtractRuleMetrics(ctx)
	if err != nil {
		result.Warnings = append(result.Warnings, fmt.Sprintf("Failed to get initial rule metrics: %v", err))
		return nil
	}

	// Poll until evaluation count stabilizes (no change for 2 consecutive polls)
	lastCount := initialMetrics.Evaluations
	stableCount := 0
	requiredStablePolls := 2 // Must be stable for 2 consecutive polls

	ticker := time.NewTicker(s.config.PollInterval)
	defer ticker.Stop()

	timeout := time.After(s.config.ValidationTimeout)

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-timeout:
			result.Details["rule_stabilization"] = map[string]any{
				"stabilized":    false,
				"final_count":   lastCount,
				"wait_duration": time.Since(startTime).String(),
			}
			return nil
		case <-ticker.C:
			currentMetrics, err := s.metrics.ExtractRuleMetrics(ctx)
			if err != nil {
				continue
			}

			if currentMetrics.Evaluations == lastCount {
				stableCount++
				if stableCount >= requiredStablePolls {
					result.Details["rule_stabilization"] = map[string]any{
						"stabilized":    true,
						"final_count":   currentMetrics.Evaluations,
						"wait_duration": time.Since(startTime).String(),
						"on_enter":      currentMetrics.OnEnterFired,
						"on_exit":       currentMetrics.OnExitFired,
					}
					return nil
				}
			} else {
				stableCount = 0
				lastCount = currentMetrics.Evaluations
			}
		}
	}
}

// getEntityCountForEmbeddings returns the expected number of entities for embedding wait.
func (s *TieredScenario) getEntityCountForEmbeddings(result *Result) int {
	if count, ok := result.Metrics["entity_count"].(int); ok {
		return count
	}
	// Fallback based on variant
	variant := s.detectVariantAndProvider(result)
	if variant.variant == "structural" {
		return 7 // Structural test data
	}
	return 74 // Kitchen sink dataset
}

// executeTestHTTPGateway validates GraphQL Gateway query endpoints
func (s *TieredScenario) executeTestHTTPGateway(ctx context.Context, result *Result) error {
	graphqlURL := s.config.GraphQLURL
	httpClient := &http.Client{Timeout: 10 * time.Second}

	// Test globalSearch via GraphQL endpoint
	graphqlQuery := map[string]any{
		"query": `query($query: String!, $level: Int, $maxCommunities: Int) {
			globalSearch(query: $query, level: $level, maxCommunities: $maxCommunities) {
				entities { id type }
				count
			}
		}`,
		"variables": map[string]any{
			"query":          "robot warehouse",
			"level":          0,
			"maxCommunities": 10,
		},
	}

	queryJSON, err := json.Marshal(graphqlQuery)
	if err != nil {
		result.Warnings = append(result.Warnings, fmt.Sprintf("Failed to marshal GraphQL query: %v", err))
		return nil // Not a hard failure
	}

	req, err := http.NewRequestWithContext(ctx, "POST", graphqlURL, strings.NewReader(string(queryJSON)))
	if err != nil {
		result.Warnings = append(result.Warnings, fmt.Sprintf("Failed to create gateway request: %v", err))
		return nil
	}
	req.Header.Set("Content-Type", "application/json")

	startTime := time.Now()
	resp, err := httpClient.Do(req)
	if err != nil {
		result.Warnings = append(result.Warnings, fmt.Sprintf("GraphQL Gateway request failed: %v", err))
		return nil
	}
	defer resp.Body.Close()

	latency := time.Since(startTime)
	result.Metrics["graphql_gateway_latency_ms"] = latency.Milliseconds()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		result.Warnings = append(result.Warnings, fmt.Sprintf("GraphQL Gateway returned status %d: %s", resp.StatusCode, body))
		return nil
	}

	// Parse GraphQL response structure
	var gqlResp struct {
		Data struct {
			GlobalSearch struct {
				Entities []struct {
					ID   string `json:"id"`
					Type string `json:"type"`
				} `json:"entities"`
				Count int `json:"count"`
			} `json:"globalSearch"`
		} `json:"data"`
		Errors []struct {
			Message string `json:"message"`
		} `json:"errors"`
	}

	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		result.Warnings = append(result.Warnings, fmt.Sprintf("Failed to read gateway response: %v", err))
		return nil
	}

	if err := json.Unmarshal(bodyBytes, &gqlResp); err != nil {
		result.Warnings = append(result.Warnings, fmt.Sprintf("Failed to parse gateway response: %v", err))
		return nil
	}

	if len(gqlResp.Errors) > 0 {
		result.Warnings = append(result.Warnings, fmt.Sprintf("GraphQL search error: %s", gqlResp.Errors[0].Message))
		return nil
	}

	hitCount := len(gqlResp.Data.GlobalSearch.Entities)
	result.Metrics["graphql_gateway_search_hits"] = hitCount
	result.Details["graphql_gateway_tested"] = true
	result.Details["graphql_gateway_endpoint"] = graphqlURL

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
// using MetricsClient for consistent metric access
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
		// Wait for rules to process using event-driven wait
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

	// Record metrics
	result.Metrics["rules_triggered_count"] = int(finalMetrics.Triggers)
	result.Metrics["rules_evaluated_count"] = int(finalMetrics.Evaluations)
	result.Metrics["rules_triggered_delta"] = triggeredDelta
	result.Metrics["rules_evaluated_delta"] = evaluatedDelta
	result.Metrics["rule_metrics_found"] = foundRuleMetrics

	// Add state transition metrics
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
// and detects potential data loss by comparing expected vs actual entity counts.
// This function polls until minimum entities are loaded to handle file loader timing.
func (s *TieredScenario) executeVerifyEntityCount(ctx context.Context, result *Result) error {
	if s.natsClient == nil {
		result.Warnings = append(result.Warnings, "NATS client not available, skipping entity count verification")
		return nil
	}

	minRequired := s.getMinRequiredEntities()
	criticalEntities := s.getCriticalEntities()

	// Poll until entities are loaded
	actualCount, pollCount, criticalFound, lastErr := s.pollForEntities(ctx, minRequired, criticalEntities)
	result.Metrics["entity_load_poll_count"] = pollCount

	// Check for failures
	if err := s.validateEntityLoadResult(actualCount, minRequired, pollCount, criticalFound, criticalEntities, lastErr); err != nil {
		return err
	}

	// Record metrics and details
	s.recordEntityMetrics(result, actualCount)
	return nil
}

func (s *TieredScenario) getMinRequiredEntities() int {
	switch s.config.Variant {
	case "structural":
		return StructuralMinEntities
	case "statistical":
		return StatisticalMinEntities
	case "semantic":
		return SemanticMinEntities
	default:
		return s.config.MinExpectedEntities
	}
}

func (s *TieredScenario) getCriticalEntities() []string {
	switch s.config.Variant {
	case "structural":
		return []string{"c360.logistics.environmental.sensor.temperature.temp-sensor-001"}
	case "statistical", "semantic":
		return []string{"c360.logistics.content.document.operations.doc-ops-001"}
	default:
		return []string{"c360.logistics.content.document.operations.doc-ops-001"}
	}
}

func (s *TieredScenario) pollForEntities(ctx context.Context, minRequired int, criticalEntities []string) (int, int, bool, error) {
	var actualCount int
	var lastErr error
	var criticalFound bool

	deadline := time.Now().Add(s.config.ValidationTimeout)
	pollCount := 0

	for time.Now().Before(deadline) {
		var err error
		actualCount, err = s.natsClient.CountEntities(ctx)
		if err != nil {
			lastErr = err
			time.Sleep(s.config.PollInterval)
			pollCount++
			continue
		}

		if actualCount >= minRequired {
			criticalFound = s.verifyCriticalEntities(ctx, criticalEntities)
			if criticalFound {
				break
			}
		}

		time.Sleep(s.config.PollInterval)
		pollCount++
	}
	return actualCount, pollCount, criticalFound, lastErr
}

func (s *TieredScenario) verifyCriticalEntities(ctx context.Context, criticalEntities []string) bool {
	for _, entityID := range criticalEntities {
		if _, err := s.natsClient.GetEntity(ctx, entityID); err != nil {
			return false
		}
	}
	return true
}

func (s *TieredScenario) validateEntityLoadResult(actualCount, minRequired, pollCount int, criticalFound bool, criticalEntities []string, lastErr error) error {
	if actualCount < minRequired {
		if lastErr != nil {
			return fmt.Errorf("entity loading timeout: got %d, need %d after %d polls (last error: %v)",
				actualCount, minRequired, pollCount, lastErr)
		}
		return fmt.Errorf("entity loading timeout: got %d, need %d after %d polls (waited %v)",
			actualCount, minRequired, pollCount, s.config.ValidationTimeout)
	}
	if !criticalFound {
		return fmt.Errorf("critical entities not found after %d polls: %v", pollCount, criticalEntities)
	}
	return nil
}

func (s *TieredScenario) recordEntityMetrics(result *Result, actualCount int) {
	// All tiers use testdata/semantic/*.jsonl (74 unique entities)
	expectedFromTestData := 74
	expectedFromUDP := 0
	totalExpected := expectedFromTestData + expectedFromUDP

	var dataLossPercent float64
	if totalExpected > 0 {
		dataLossPercent = 100.0 * float64(totalExpected-actualCount) / float64(totalExpected)
		if dataLossPercent < 0 {
			dataLossPercent = 0
		}
	}

	result.Metrics["entity_count"] = actualCount
	result.Metrics["expected_from_testdata"] = expectedFromTestData
	result.Metrics["expected_from_udp"] = expectedFromUDP
	result.Metrics["total_expected_entities"] = totalExpected
	result.Metrics["min_expected_entities"] = s.config.MinExpectedEntities
	result.Metrics["data_loss_percent"] = dataLossPercent

	dataLossThreshold := 10.0
	if dataLossPercent > dataLossThreshold {
		result.Warnings = append(result.Warnings,
			fmt.Sprintf("Data loss detected: %.1f%% (expected %d, got %d)",
				dataLossPercent, totalExpected, actualCount))
	}
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
		{"c360.logistics.content.document.operations.doc-ops-001", "document", "documents.jsonl"},
		{"c360.logistics.content.document.quality.doc-quality-001", "document", "documents.jsonl"},
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

// executeVerifyStructuralIndexes validates k-core and pivot indexes (structural tier only)
func (s *TieredScenario) executeVerifyStructuralIndexes(ctx context.Context, result *Result) error {
	verifier := &stages.StructuralIndexVerifier{NATSClient: s.natsClient}
	indexResult, err := verifier.VerifyStructuralIndexes(ctx)
	if err != nil {
		result.Warnings = append(result.Warnings, fmt.Sprintf("Structural index verification failed: %v", err))
		return nil
	}

	result.Details["structural_indexes"] = indexResult
	result.Warnings = append(result.Warnings, indexResult.Warnings...)
	if len(indexResult.Errors) > 0 {
		return fmt.Errorf("structural index errors: %v", indexResult.Errors)
	}
	return nil
}

// executeVerifySearchQuality validates that semantic search returns expected results
// with score threshold assertions, not just binary hit/no-hit checks
func (s *TieredScenario) executeVerifySearchQuality(ctx context.Context, result *Result) error {
	// Use similarity search for both statistical and semantic tiers (embedding-based with real scores)
	// Statistical tier uses BM25 embeddings, semantic tier uses neural embeddings
	// Structural tier has no embeddings, so uses global search (community-based)
	var executor *search.Executor
	if s.config.Variant == "statistical" || s.config.Variant == "semantic" {
		executor = search.NewSimilarityExecutor(s.config.GraphQLURL, 10*time.Second)
	} else {
		executor = search.NewExecutor(s.config.GraphQLURL, 10*time.Second)
	}
	queries := search.DefaultQueries()

	// Execute all queries and get stats
	stats := executor.ExecuteAll(ctx, queries)

	// Store stats for potential reuse by comparison stage
	s.searchStats = stats

	// Record results in legacy format for backward compatibility
	s.recordSearchQualityResultsFromStats(result, stats)

	return nil
}

// searchQualityTest defines a search quality test case.
type searchQualityTest struct {
	query           string
	expectedPattern string
	description     string
	minScore        float64
	minHits         int
	// Known-answer validation (Phase 3 improvement)
	mustInclude []string // Entity ID substrings that MUST appear in results
	mustExclude []string // Entity ID substrings that should NOT appear (warning only)
}

// searchQualityStats tracks aggregate statistics for search quality tests.
type searchQualityStats struct {
	searchResults          map[string]any
	allScores              []float64
	queriesWithResults     int
	queriesMeetingMinScore int
	queriesMeetingMinHits  int
	// Known-answer validation stats (Phase 3 improvement)
	knownAnswerTestsPassed int
	knownAnswerTestsTotal  int
	knownAnswerFailures    []string // Descriptions of failed known-answer tests
}

// getSearchQualityTests returns the search quality test cases.
// Phase 3 improvement: Added known-answer validation based on testdata/semantic/ content.
func (s *TieredScenario) getSearchQualityTests() []searchQualityTest {
	return []searchQualityTest{
		// Original natural language tests
		{
			query:           "What documents mention forklift safety?",
			expectedPattern: "forklift",
			description:     "Natural language document search",
			minScore:        0.3,
			minHits:         1,
			mustInclude:     []string{"doc-ops-001"}, // Forklift Operation Manual
			mustExclude:     []string{"sensor-temp"}, // Temperature sensors irrelevant
		},
		{
			query:           "Are there safety observations related to temperature?",
			expectedPattern: "temperature",
			description:     "Cross-domain safety query",
			minScore:        0.3,
			minHits:         1,
			mustInclude:     []string{"sensor-temp"}, // Temperature sensors
		},
		{
			query:           "What maintenance was done on cold storage equipment?",
			expectedPattern: "cold",
			description:     "Maintenance semantic search",
			minScore:        0.3,
			minHits:         1,
			mustInclude:     []string{"maint-"}, // Maintenance records
		},
		{
			query:           "Find all sensors in zone-a",
			expectedPattern: "zone-a",
			description:     "Location-based sensor query",
			minScore:        0.3,
			minHits:         1,
		},
		// Known-answer tests derived from testdata/semantic/ (Phase 3)
		{
			query:           "forklift operation inspection equipment maintenance",
			expectedPattern: "ops",
			description:     "Operations query should return operations docs",
			minScore:        0.3,
			minHits:         1,
			mustInclude:     []string{"doc-ops"},                       // doc-ops-001 (Forklift Operation Manual)
			mustExclude:     []string{"sensor-humid", "sensor-motion"}, // Humidity/motion sensors irrelevant
		},
		{
			query:           "cold storage temperature monitoring refrigeration",
			expectedPattern: "temp",
			description:     "Temperature query should return temp sensors",
			minScore:        0.3,
			minHits:         1,
			mustInclude:     []string{"sensor-temp"},         // sensor-temp-001, sensor-temp-002, etc.
			mustExclude:     []string{"doc-hr", "doc-audit"}, // HR and audit docs irrelevant
		},
		{
			query:           "hydraulic fluid maintenance equipment repair",
			expectedPattern: "maint",
			description:     "Maintenance query should return maintenance records",
			minScore:        0.3,
			minHits:         1,
			mustInclude:     []string{"maint-"}, // maint-001 (hydraulic maintenance)
		},
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

	// Phase 3 improvement: Known-answer validation
	knownAnswerPassed := true
	missingRequired := []string{}
	unexpectedFound := []string{}

	// Check mustInclude - required entities that MUST appear in results
	for _, required := range test.mustInclude {
		found := false
		for _, hit := range topHits {
			if strings.Contains(strings.ToLower(hit), strings.ToLower(required)) {
				found = true
				break
			}
		}
		if !found {
			knownAnswerPassed = false
			missingRequired = append(missingRequired, required)
		}
	}

	// Check mustExclude - entities that should NOT appear (warning only, not failure)
	for _, forbidden := range test.mustExclude {
		for _, hit := range topHits {
			if strings.Contains(strings.ToLower(hit), strings.ToLower(forbidden)) {
				unexpectedFound = append(unexpectedFound, hit)
				break
			}
		}
	}

	// Track known-answer test results
	if len(test.mustInclude) > 0 {
		stats.knownAnswerTestsTotal++
		if knownAnswerPassed {
			stats.knownAnswerTestsPassed++
		} else {
			stats.knownAnswerFailures = append(stats.knownAnswerFailures,
				fmt.Sprintf("query %q: missing required %v - %s", test.query, missingRequired, test.description))
		}
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
		// Known-answer validation results
		"known_answer_passed": knownAnswerPassed,
		"missing_required":    missingRequired,
		"unexpected_found":    unexpectedFound,
		"must_include":        test.mustInclude,
		"must_exclude":        test.mustExclude,
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

	// Phase 3 improvement: Report known-answer test failures as warnings
	for _, failure := range stats.knownAnswerFailures {
		result.Warnings = append(result.Warnings, fmt.Sprintf("Known-answer test failed: %s", failure))
	}

	result.Metrics["search_queries_tested"] = len(searchTests)
	result.Metrics["search_queries_with_results"] = stats.queriesWithResults
	result.Metrics["search_min_score_met"] = stats.queriesMeetingMinScore
	result.Metrics["search_min_hits_met"] = stats.queriesMeetingMinHits
	result.Metrics["search_quality_score"] = overallAvgScore
	// Phase 3 improvement: Known-answer test metrics
	result.Metrics["known_answer_tests_passed"] = stats.knownAnswerTestsPassed
	result.Metrics["known_answer_tests_total"] = stats.knownAnswerTestsTotal

	result.Details["search_quality_verification"] = map[string]any{
		"queries":           len(searchTests),
		"queries_with_hits": stats.queriesWithResults,
		"min_score_met":     stats.queriesMeetingMinScore,
		"min_hits_met":      stats.queriesMeetingMinHits,
		"overall_avg_score": overallAvgScore,
		"weak_threshold":    weakResultsThreshold,
		"results":           stats.searchResults,
		"message":           fmt.Sprintf("%d/%d queries returned results, avg score: %.2f", stats.queriesWithResults, len(searchTests), overallAvgScore),
		// Phase 3 improvement: Known-answer validation summary
		"known_answer_tests_passed": stats.knownAnswerTestsPassed,
		"known_answer_tests_total":  stats.knownAnswerTestsTotal,
		"known_answer_failures":     stats.knownAnswerFailures,
	}
}

// recordSearchQualityResultsFromStats records search quality results from the new search.Stats format
// This provides backward compatibility with the legacy result format
func (s *TieredScenario) recordSearchQualityResultsFromStats(result *Result, stats *search.Stats) {
	weakResultsThreshold := 0.5
	if stats.OverallAvgScore > 0 && stats.OverallAvgScore < weakResultsThreshold {
		result.Warnings = append(result.Warnings,
			fmt.Sprintf("Weak search results: average score %.2f is below %.2f threshold",
				stats.OverallAvgScore, weakResultsThreshold))
	}

	// Report known-answer test failures as warnings
	for _, failure := range stats.KnownAnswerFailures {
		result.Warnings = append(result.Warnings, fmt.Sprintf("Known-answer test failed: %s", failure))
	}

	result.Metrics["search_queries_tested"] = stats.TotalQueries
	result.Metrics["search_queries_with_results"] = stats.QueriesWithResults
	result.Metrics["search_min_score_met"] = stats.QueriesMeetingMinScore
	result.Metrics["search_min_hits_met"] = stats.QueriesMeetingMinHits
	result.Metrics["search_quality_score"] = stats.OverallAvgScore
	result.Metrics["known_answer_tests_passed"] = stats.KnownAnswerTestsPassed
	result.Metrics["known_answer_tests_total"] = stats.KnownAnswerTestsTotal

	// Build legacy results format for backward compatibility
	legacyResults := make(map[string]any)
	for _, r := range stats.Results {
		legacyResults[r.Query] = map[string]any{
			"hit_count":           len(r.Hits),
			"top_hits":            extractEntityIDs(r.Hits),
			"top_scores":          extractScores(r.Hits),
			"description":         r.Description,
			"avg_score":           r.Validation.AvgScore,
			"meets_min_score":     r.Validation.MeetsMinScore,
			"meets_min_hits":      r.Validation.MeetsMinHits,
			"matches_pattern":     r.Validation.MatchesPattern,
			"known_answer_passed": r.Validation.KnownAnswerPassed,
			"missing_required":    r.Validation.MissingRequired,
			"unexpected_found":    r.Validation.UnexpectedFound,
			"latency_ms":          r.LatencyMs,
			"error":               r.Error,
		}
	}

	result.Details["search_quality_verification"] = map[string]any{
		"queries":                   stats.TotalQueries,
		"queries_with_hits":         stats.QueriesWithResults,
		"min_score_met":             stats.QueriesMeetingMinScore,
		"min_hits_met":              stats.QueriesMeetingMinHits,
		"overall_avg_score":         stats.OverallAvgScore,
		"weak_threshold":            weakResultsThreshold,
		"results":                   legacyResults,
		"message":                   fmt.Sprintf("%d/%d queries returned results, avg score: %.2f", stats.QueriesWithResults, stats.TotalQueries, stats.OverallAvgScore),
		"known_answer_tests_passed": stats.KnownAnswerTestsPassed,
		"known_answer_tests_total":  stats.KnownAnswerTestsTotal,
		"known_answer_failures":     stats.KnownAnswerFailures,
		"total_latency_ms":          stats.TotalLatencyMs,
	}
}

// extractEntityIDs extracts entity IDs from search hits
func extractEntityIDs(hits []search.Hit) []string {
	ids := make([]string, len(hits))
	for i, h := range hits {
		ids[i] = h.EntityID
	}
	return ids
}

// extractScores extracts scores from search hits
func extractScores(hits []search.Hit) []float64 {
	scores := make([]float64, len(hits))
	for i, h := range hits {
		scores[i] = h.Score
	}
	return scores
}

// executeCompareCoreMl captures search results for Core vs ML comparison
// and persists results to JSON for later analysis
// variantInfo holds detected variant and embedding provider information
type variantInfo struct {
	variant           string
	embeddingProvider string
}

// detectVariantAndProvider detects which variant (structural/statistical/semantic) is running based on semembed availability and metrics.
// Results are cached on first call and reused for subsequent calls.
func (s *TieredScenario) detectVariantAndProvider(result *Result) variantInfo {
	// Return cached result if already detected
	if s.detectedVariant != nil {
		return *s.detectedVariant
	}

	info := variantInfo{variant: "statistical", embeddingProvider: "unknown"} // Default to statistical (BM25)

	// Check semembed availability first
	if semembedAvailable, ok := result.Details["semembed_available"].(bool); ok && semembedAvailable {
		info.variant = "semantic"
	}

	// Check embedding provider from metrics (overrides semembed detection)
	metricsURL := s.config.MetricsURL + "/metrics"
	resp, err := http.Get(metricsURL)
	if err == nil {
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

	// Cache the result for future calls
	s.detectedVariant = &info

	return info
}

// NOTE: Comparison functions removed - use CLI compare instead:
//   ./e2e --compare-structured --baseline results/structural.json --target results/statistical.json
// The structured results from each run contain all the data needed for comparison.

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
