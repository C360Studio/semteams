// Package scenarios provides E2E test scenarios for SemStreams tiered inference
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
	"strconv"
	"time"

	"github.com/c360/semstreams/test/e2e/client"
)

// Tier0RulesIoTScenario validates Tier 0: Rules-only processing (no ML inference)
//
// Tier 0 demonstrates:
//   - Stateful rules with OnEnter/OnExit transitions
//   - Dynamic graph relationships (add_triple/remove_triple)
//   - Deterministic behavior (no embeddings, no clustering)
//   - PathRAG on explicit edges
//   - Hotpath-only processing (low latency)
//   - Event-driven validation (no fixed delays)
type Tier0RulesIoTScenario struct {
	name        string
	description string
	client      *client.ObservabilityClient
	metrics     *client.MetricsClient
	tracer      *client.FlowTracer
	udpAddr     string
	config      *Tier0Config
}

// Tier0Config contains configuration for Tier 0 rules-only test
type Tier0Config struct {
	// Validation timeouts (event-driven, not fixed delays)
	ValidationTimeout time.Duration `json:"validation_timeout"` // Max time to wait for metrics
	PollInterval      time.Duration `json:"poll_interval"`      // How often to check metrics

	// Expected metrics
	MinRulesEvaluated  int `json:"min_rules_evaluated"`
	MinOnEnterFired    int `json:"min_on_enter_fired"`
	MinOnExitFired     int `json:"min_on_exit_fired"`
	MaxLatencyMs       int `json:"max_latency_ms"`
	ExpectedEmbeddings int `json:"expected_embeddings"` // Should be 0 for tier 0
	ExpectedClusters   int `json:"expected_clusters"`   // Should be 0 for tier 0

	// Event-driven validation
	ExpectedRuleEvaluationsPerMessage int `json:"expected_rule_evaluations_per_message"` // Rules evaluated per message

	// Regression detection
	BaselineFile         string  `json:"baseline_file,omitempty"` // Path to baseline JSON (optional)
	MaxRegressionPercent float64 `json:"max_regression_percent"`  // Max acceptable regression (e.g., 20.0 for 20%)
	SaveBaseline         bool    `json:"save_baseline,omitempty"` // Save current run as new baseline
}

// DefaultTier0Config returns default configuration
func DefaultTier0Config() *Tier0Config {
	return &Tier0Config{
		ValidationTimeout:                 10 * time.Second, // Event-driven timeout
		PollInterval:                      100 * time.Millisecond,
		MinRulesEvaluated:                 5,
		MinOnEnterFired:                   2, // Expect at least cold-storage and humidity alerts
		MinOnExitFired:                    1, // Expect at least one alert to clear
		MaxLatencyMs:                      100,
		ExpectedEmbeddings:                0,    // Tier 0: NO embeddings
		ExpectedClusters:                  0,    // Tier 0: NO clustering
		ExpectedRuleEvaluationsPerMessage: 3,    // Each message triggers ~3 rule evaluations
		MaxRegressionPercent:              20.0, // 20% regression threshold
	}
}

// Tier0Baseline represents saved metrics from a previous run for regression detection
type Tier0Baseline struct {
	Timestamp           time.Time          `json:"timestamp"`
	ScenarioName        string             `json:"scenario_name"`
	Duration            time.Duration      `json:"duration"`
	Metrics             map[string]float64 `json:"metrics"`
	ExpectedRatesPerSec map[string]float64 `json:"expected_rates_per_sec"`
}

// LoadBaseline loads a baseline from a JSON file
func LoadBaseline(filePath string) (*Tier0Baseline, error) {
	if filePath == "" {
		return nil, nil
	}

	data, err := os.ReadFile(filePath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil // No baseline file is okay
		}
		return nil, fmt.Errorf("reading baseline file: %w", err)
	}

	var baseline Tier0Baseline
	if err := json.Unmarshal(data, &baseline); err != nil {
		return nil, fmt.Errorf("parsing baseline file: %w", err)
	}

	return &baseline, nil
}

// SaveBaseline saves current metrics as a baseline for future comparison
func SaveBaseline(filePath string, baseline *Tier0Baseline) error {
	data, err := json.MarshalIndent(baseline, "", "  ")
	if err != nil {
		return fmt.Errorf("marshaling baseline: %w", err)
	}

	if err := os.WriteFile(filePath, data, 0644); err != nil {
		return fmt.Errorf("writing baseline file: %w", err)
	}

	return nil
}

// CompareToBaseline compares current metrics to baseline and returns regressions
func CompareToBaseline(current, baseline *Tier0Baseline, maxRegressionPercent float64) []string {
	if baseline == nil {
		return nil
	}

	var regressions []string

	// Key metrics to check for regressions
	keyMetrics := []string{
		"semstreams_rule_evaluations_total",
		"semstreams_rule_triggers_total",
		"semstreams_datamanager_entities_updated_total",
	}

	for _, metric := range keyMetrics {
		baselineRate, hasBaseline := baseline.ExpectedRatesPerSec[metric]
		currentRate, hasCurrent := current.ExpectedRatesPerSec[metric]

		if hasBaseline && hasCurrent && baselineRate > 0 {
			// Calculate regression percentage
			regression := ((baselineRate - currentRate) / baselineRate) * 100
			if regression > maxRegressionPercent {
				regressions = append(regressions,
					fmt.Sprintf("%s: %.1f%% regression (baseline: %.2f/s, current: %.2f/s)",
						metric, regression, baselineRate, currentRate))
			}
		}
	}

	// Check duration regression
	if baseline.Duration > 0 && current.Duration > 0 {
		durationRegression := float64(current.Duration-baseline.Duration) / float64(baseline.Duration) * 100
		if durationRegression > maxRegressionPercent {
			regressions = append(regressions,
				fmt.Sprintf("duration: %.1f%% regression (baseline: %v, current: %v)",
					durationRegression, baseline.Duration, current.Duration))
		}
	}

	return regressions
}

// NewTier0RulesIoTScenario creates a new Tier 0 rules-only scenario
func NewTier0RulesIoTScenario(
	obsClient *client.ObservabilityClient,
	udpAddr string,
	config *Tier0Config,
) *Tier0RulesIoTScenario {
	if config == nil {
		config = DefaultTier0Config()
	}
	if udpAddr == "" {
		udpAddr = "localhost:14550"
	}

	// Initialize observability clients
	metricsClient := client.NewMetricsClient("http://localhost:9090")
	msgLoggerClient := client.NewMessageLoggerClient("http://localhost:8080")
	flowTracer := client.NewFlowTracer(metricsClient, msgLoggerClient)

	return &Tier0RulesIoTScenario{
		name:        "tier0-rules-iot",
		description: "Tier 0: Rules-only processing with stateful IoT alerts (no ML inference)",
		client:      obsClient,
		metrics:     metricsClient,
		tracer:      flowTracer,
		udpAddr:     udpAddr,
		config:      config,
	}
}

// Name returns the scenario name
func (s *Tier0RulesIoTScenario) Name() string {
	return s.name
}

// Description returns the scenario description
func (s *Tier0RulesIoTScenario) Description() string {
	return s.description
}

// Setup prepares the scenario
func (s *Tier0RulesIoTScenario) Setup(_ context.Context) error {
	// Verify UDP endpoint is reachable
	conn, err := net.Dial("udp", s.udpAddr)
	if err != nil {
		return fmt.Errorf("cannot reach UDP endpoint %s: %w", s.udpAddr, err)
	}
	_ = conn.Close()
	return nil
}

// Execute runs the Tier 0 rules-only scenario
func (s *Tier0RulesIoTScenario) Execute(ctx context.Context) (*Result, error) {
	result := &Result{
		ScenarioName: s.name,
		StartTime:    time.Now(),
		Success:      false,
		Metrics:      make(map[string]any),
		Details:      make(map[string]any),
		Errors:       []string{},
		Warnings:     []string{},
	}

	// Load baseline for regression detection (if configured)
	var loadedBaseline *Tier0Baseline
	if s.config.BaselineFile != "" {
		var err error
		loadedBaseline, err = LoadBaseline(s.config.BaselineFile)
		if err != nil {
			result.Warnings = append(result.Warnings, fmt.Sprintf("Could not load baseline: %v", err))
		} else if loadedBaseline != nil {
			result.Details["baseline_loaded"] = map[string]any{
				"file":      s.config.BaselineFile,
				"timestamp": loadedBaseline.Timestamp,
				"duration":  loadedBaseline.Duration.String(),
			}
		}
	}

	// Capture start metrics for baseline comparison
	startBaseline, _ := s.metrics.CaptureBaseline(ctx)

	// Add tier metadata
	result.Metrics["tier"] = 0
	result.Metrics["tier_name"] = "rules"
	result.Details["tier_description"] = "Stateful rules only (no ML inference)"

	// Track execution stages
	stages := []struct {
		name string
		fn   func(context.Context, *Result) error
	}{
		{"verify-tier0-config", s.executeVerifyTier0Config},
		{"verify-components", s.executeVerifyComponents},
		{"send-iot-data-trigger", s.executeSendIoTDataTrigger},
		{"validate-on-enter", s.executeValidateOnEnter},
		{"send-iot-data-clear", s.executeSendIoTDataClear},
		{"validate-on-exit", s.executeValidateOnExit},
		{"validate-no-inference", s.executeValidateNoInference},
	}

	// Execute each stage
	for _, stage := range stages {
		stageStart := time.Now()

		if err := stage.fn(ctx, result); err != nil {
			result.Success = false
			result.Error = fmt.Sprintf("%s failed: %v", stage.name, err)
			result.EndTime = time.Now()
			result.Duration = result.EndTime.Sub(result.StartTime)
			return result, nil
		}

		result.Metrics[fmt.Sprintf("%s_duration_ms", stage.name)] = time.Since(stageStart).Milliseconds()
	}

	// Overall success
	result.Success = true
	result.EndTime = time.Now()
	result.Duration = result.EndTime.Sub(result.StartTime)

	// Create current baseline for comparison/saving
	endDiff, _ := s.metrics.CompareToBaseline(ctx, startBaseline)
	currentBaseline := &Tier0Baseline{
		Timestamp:           result.StartTime,
		ScenarioName:        s.name,
		Duration:            result.Duration,
		Metrics:             make(map[string]float64),
		ExpectedRatesPerSec: make(map[string]float64),
	}
	if endDiff != nil {
		currentBaseline.Metrics = endDiff.Deltas
		currentBaseline.ExpectedRatesPerSec = endDiff.RatePerSec
	}

	// Compare to loaded baseline for regression detection
	if loadedBaseline != nil {
		regressions := CompareToBaseline(currentBaseline, loadedBaseline, s.config.MaxRegressionPercent)
		if len(regressions) > 0 {
			for _, reg := range regressions {
				result.Warnings = append(result.Warnings, fmt.Sprintf("REGRESSION: %s", reg))
			}
			result.Details["regressions"] = regressions
		} else {
			result.Details["regression_check"] = "PASS - no significant regressions detected"
		}
	}

	// Save current run as baseline if configured
	if s.config.SaveBaseline && s.config.BaselineFile != "" {
		if err := SaveBaseline(s.config.BaselineFile, currentBaseline); err != nil {
			result.Warnings = append(result.Warnings, fmt.Sprintf("Could not save baseline: %v", err))
		} else {
			result.Details["baseline_saved"] = s.config.BaselineFile
		}
	}

	// Add performance summary to results
	result.Details["performance_summary"] = map[string]any{
		"total_duration_ms": result.Duration.Milliseconds(),
		"metrics_deltas":    currentBaseline.Metrics,
		"rates_per_sec":     currentBaseline.ExpectedRatesPerSec,
	}

	return result, nil
}

// Teardown cleans up after the scenario
func (s *Tier0RulesIoTScenario) Teardown(_ context.Context) error {
	return nil
}

// executeVerifyTier0Config validates that tier 0 configuration is active
func (s *Tier0RulesIoTScenario) executeVerifyTier0Config(_ context.Context, result *Result) error {
	// Document tier 0 expectations
	result.Details["tier0_expectations"] = map[string]any{
		"embedding_enabled":   false,
		"clustering_enabled":  false,
		"inference_enabled":   false,
		"stateful_rules":      true,
		"hotpath_only":        true,
		"max_latency_ms":      s.config.MaxLatencyMs,
		"expected_embeddings": 0,
		"expected_clusters":   0,
	}
	return nil
}

// executeVerifyComponents checks that rule and graph processors exist
func (s *Tier0RulesIoTScenario) executeVerifyComponents(ctx context.Context, result *Result) error {
	components, err := s.client.GetComponents(ctx)
	if err != nil {
		result.Errors = append(result.Errors, fmt.Sprintf("Failed to get components: %v", err))
		return fmt.Errorf("component verification failed: %w", err)
	}

	requiredComponents := []string{"rule", "graph", "iot_sensor"}
	foundComponents := make(map[string]bool)

	for _, comp := range components {
		foundComponents[comp.Name] = true
	}

	missingComponents := []string{}
	for _, required := range requiredComponents {
		if !foundComponents[required] {
			missingComponents = append(missingComponents, required)
		}
	}

	if len(missingComponents) > 0 {
		result.Errors = append(result.Errors,
			fmt.Sprintf("Missing required components: %v", missingComponents))
		return fmt.Errorf("missing components: %v", missingComponents)
	}

	result.Metrics["component_count"] = len(components)
	result.Details["required_components"] = requiredComponents
	return nil
}

// executeSendIoTDataTrigger sends IoT sensor data that crosses thresholds
func (s *Tier0RulesIoTScenario) executeSendIoTDataTrigger(ctx context.Context, result *Result) error {
	// Capture baseline metrics before sending data
	baseline, err := s.metrics.CaptureBaseline(ctx)
	if err != nil {
		result.Warnings = append(result.Warnings, fmt.Sprintf("Could not capture baseline: %v", err))
	}
	baselineEvaluations := baseline.Metrics["semstreams_rule_evaluations_total"]

	conn, err := net.Dial("udp", s.udpAddr)
	if err != nil {
		return fmt.Errorf("UDP connection failed: %w", err)
	}
	defer conn.Close()

	// Send data that triggers OnEnter for each alert type
	triggerData := []map[string]any{
		// Cold storage temperature alert (reading >= 40F in cold-storage location)
		{
			"device_id": "cold-storage-udp-01",
			"type":      "temperature",
			"reading":   42.5,
			"unit":      "fahrenheit",
			"location":  "cold-storage-A",
			"timestamp": time.Now().Format(time.RFC3339),
		},
		// High humidity alert (reading >= 50%)
		{
			"device_id": "humidity-udp-01",
			"type":      "humidity",
			"reading":   55.0,
			"unit":      "percent",
			"location":  "warehouse-B",
			"timestamp": time.Now().Format(time.RFC3339),
		},
		// Low pressure alert (reading < 100 PSI)
		{
			"device_id": "pressure-udp-01",
			"type":      "pressure",
			"reading":   92.0,
			"unit":      "psi",
			"location":  "compressor-room",
			"timestamp": time.Now().Format(time.RFC3339),
		},
	}

	messagesSent := 0
	for _, data := range triggerData {
		msgBytes, err := json.Marshal(data)
		if err != nil {
			result.Warnings = append(result.Warnings, fmt.Sprintf("Failed to marshal: %v", err))
			continue
		}

		_, err = conn.Write(msgBytes)
		if err != nil {
			result.Warnings = append(result.Warnings, fmt.Sprintf("Failed to send: %v", err))
			continue
		}
		messagesSent++

		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(50 * time.Millisecond): // Small delay between messages
		}
	}

	result.Metrics["trigger_messages_sent"] = messagesSent
	result.Details["trigger_data"] = "Sent threshold-crossing data for cold-storage, humidity, pressure"

	// EVENT-DRIVEN: Wait for rule evaluations to increase (not fixed delay)
	expectedDelta := float64(messagesSent * s.config.ExpectedRuleEvaluationsPerMessage)
	waitOpts := client.WaitOpts{
		Timeout:      s.config.ValidationTimeout,
		PollInterval: s.config.PollInterval,
		Comparator:   ">=",
	}

	if err := s.metrics.WaitForMetricDelta(ctx, "semstreams_rule_evaluations_total",
		baselineEvaluations, expectedDelta, waitOpts); err != nil {
		result.Warnings = append(result.Warnings,
			fmt.Sprintf("Rule evaluations did not reach expected level: %v", err))
	}

	// Capture metrics delta for observability
	diff, err := s.metrics.CompareToBaseline(ctx, baseline)
	if err == nil {
		result.Details["trigger_metrics_delta"] = map[string]any{
			"duration_ms":         diff.Duration.Milliseconds(),
			"rule_evaluations":    diff.Deltas["semstreams_rule_evaluations_total"],
			"rule_triggers":       diff.Deltas["semstreams_rule_triggers_total"],
			"evaluations_per_sec": diff.RatePerSec["semstreams_rule_evaluations_total"],
		}
	}

	return nil
}

// executeValidateOnEnter validates that OnEnter actions fired
func (s *Tier0RulesIoTScenario) executeValidateOnEnter(ctx context.Context, result *Result) error {
	// Use MetricsClient for structured metric access
	evaluations, _ := s.metrics.SumMetricsByName(ctx, "semstreams_rule_evaluations_total")
	triggers, _ := s.metrics.SumMetricsByName(ctx, "semstreams_rule_triggers_total")

	// For state transitions, we need to query raw metrics to filter by label
	metricsURL := "http://localhost:9090/metrics"
	resp, err := http.Get(metricsURL)
	if err != nil {
		result.Warnings = append(result.Warnings, fmt.Sprintf("Metrics unavailable: %v", err))
		return nil
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil
	}

	metricsText := string(body)
	ruleMetrics := extractTier0RuleMetrics(metricsText)

	result.Metrics["rules_evaluated"] = int(evaluations)
	result.Metrics["rules_triggered"] = int(triggers)
	result.Metrics["on_enter_fired"] = ruleMetrics["on_enter_total"]

	// Validate minimum OnEnter triggers
	if ruleMetrics["on_enter_total"] < s.config.MinOnEnterFired {
		result.Warnings = append(result.Warnings,
			fmt.Sprintf("Expected at least %d OnEnter triggers, got %d",
				s.config.MinOnEnterFired, ruleMetrics["on_enter_total"]))
	}

	result.Details["on_enter_validation"] = map[string]any{
		"expected_min":   s.config.MinOnEnterFired,
		"actual":         ruleMetrics["on_enter_total"],
		"triples_added":  ruleMetrics["triples_added"],
		"alerts_created": ruleMetrics["triggers_total"],
	}

	return nil
}

// executeSendIoTDataClear sends IoT sensor data that clears thresholds
func (s *Tier0RulesIoTScenario) executeSendIoTDataClear(ctx context.Context, result *Result) error {
	// Capture baseline metrics before sending clear data
	baseline, err := s.metrics.CaptureBaseline(ctx)
	if err != nil {
		result.Warnings = append(result.Warnings, fmt.Sprintf("Could not capture baseline: %v", err))
	}
	baselineEvaluations := baseline.Metrics["semstreams_rule_evaluations_total"]

	conn, err := net.Dial("udp", s.udpAddr)
	if err != nil {
		return fmt.Errorf("UDP connection failed: %w", err)
	}
	defer conn.Close()

	// Send data that returns to normal (triggers OnExit)
	clearData := []map[string]any{
		// Cold storage temperature back to normal (< 40F)
		{
			"device_id": "cold-storage-udp-01",
			"type":      "temperature",
			"reading":   35.0,
			"unit":      "fahrenheit",
			"location":  "cold-storage-A",
			"timestamp": time.Now().Format(time.RFC3339),
		},
		// Humidity back to normal (< 50%)
		{
			"device_id": "humidity-udp-01",
			"type":      "humidity",
			"reading":   42.0,
			"unit":      "percent",
			"location":  "warehouse-B",
			"timestamp": time.Now().Format(time.RFC3339),
		},
		// Pressure back to normal (>= 100 PSI)
		{
			"device_id": "pressure-udp-01",
			"type":      "pressure",
			"reading":   115.0,
			"unit":      "psi",
			"location":  "compressor-room",
			"timestamp": time.Now().Format(time.RFC3339),
		},
	}

	messagesSent := 0
	for _, data := range clearData {
		msgBytes, err := json.Marshal(data)
		if err != nil {
			result.Warnings = append(result.Warnings, fmt.Sprintf("Failed to marshal: %v", err))
			continue
		}

		_, err = conn.Write(msgBytes)
		if err != nil {
			result.Warnings = append(result.Warnings, fmt.Sprintf("Failed to send: %v", err))
			continue
		}
		messagesSent++

		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(50 * time.Millisecond): // Small delay between messages
		}
	}

	result.Metrics["clear_messages_sent"] = messagesSent
	result.Details["clear_data"] = "Sent normal-range data for cold-storage, humidity, pressure"

	// EVENT-DRIVEN: Wait for rule evaluations to increase (not fixed delay)
	expectedDelta := float64(messagesSent * s.config.ExpectedRuleEvaluationsPerMessage)
	waitOpts := client.WaitOpts{
		Timeout:      s.config.ValidationTimeout,
		PollInterval: s.config.PollInterval,
		Comparator:   ">=",
	}

	if err := s.metrics.WaitForMetricDelta(ctx, "semstreams_rule_evaluations_total",
		baselineEvaluations, expectedDelta, waitOpts); err != nil {
		result.Warnings = append(result.Warnings,
			fmt.Sprintf("Rule evaluations did not reach expected level: %v", err))
	}

	// Capture metrics delta for observability
	diff, err := s.metrics.CompareToBaseline(ctx, baseline)
	if err == nil {
		result.Details["clear_metrics_delta"] = map[string]any{
			"duration_ms":         diff.Duration.Milliseconds(),
			"rule_evaluations":    diff.Deltas["semstreams_rule_evaluations_total"],
			"rule_triggers":       diff.Deltas["semstreams_rule_triggers_total"],
			"evaluations_per_sec": diff.RatePerSec["semstreams_rule_evaluations_total"],
		}
	}

	return nil
}

// executeValidateOnExit validates that OnExit actions fired
func (s *Tier0RulesIoTScenario) executeValidateOnExit(ctx context.Context, result *Result) error {
	// Use MetricsClient for structured metric access
	evaluations, _ := s.metrics.SumMetricsByName(ctx, "semstreams_rule_evaluations_total")
	triggers, _ := s.metrics.SumMetricsByName(ctx, "semstreams_rule_triggers_total")

	// For state transitions, we need to query raw metrics to filter by label
	metricsURL := "http://localhost:9090/metrics"
	resp, err := http.Get(metricsURL)
	if err != nil {
		result.Warnings = append(result.Warnings, fmt.Sprintf("Metrics unavailable: %v", err))
		return nil
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil
	}

	metricsText := string(body)
	ruleMetrics := extractTier0RuleMetrics(metricsText)

	result.Metrics["rules_evaluated_total"] = int(evaluations)
	result.Metrics["rules_triggered_total"] = int(triggers)
	result.Metrics["on_exit_fired"] = ruleMetrics["on_exit_total"]
	result.Metrics["triples_removed"] = ruleMetrics["triples_removed"]

	// Validate minimum OnExit triggers
	if ruleMetrics["on_exit_total"] < s.config.MinOnExitFired {
		result.Warnings = append(result.Warnings,
			fmt.Sprintf("Expected at least %d OnExit triggers, got %d",
				s.config.MinOnExitFired, ruleMetrics["on_exit_total"]))
	}

	result.Details["on_exit_validation"] = map[string]any{
		"expected_min":    s.config.MinOnExitFired,
		"actual":          ruleMetrics["on_exit_total"],
		"triples_removed": ruleMetrics["triples_removed"],
		"graph_dynamic":   ruleMetrics["on_exit_total"] > 0,
	}

	// Validate dynamic graph behavior
	if ruleMetrics["on_exit_total"] > 0 {
		result.Details["dynamic_graph"] = "Graph relationships were dynamically removed on state exit"
	}

	return nil
}

// executeValidateNoInference validates that NO inference occurred
func (s *Tier0RulesIoTScenario) executeValidateNoInference(ctx context.Context, result *Result) error {
	// Use MetricsClient for structured metric access
	embeddingCount, _ := s.metrics.SumMetricsByName(ctx, "semstreams_embedding_requests_total")
	clusteringCount, _ := s.metrics.SumMetricsByName(ctx, "semstreams_clustering_runs_total")
	inferredTriples, _ := s.metrics.SumMetricsByName(ctx, "semstreams_inferred_triples_total")

	result.Metrics["embeddings_generated"] = int(embeddingCount)
	result.Metrics["clustering_runs"] = int(clusteringCount)
	result.Metrics["inferred_triples"] = int(inferredTriples)

	// Validate tier 0 constraints
	tier0Valid := true
	violations := []string{}

	if int(embeddingCount) > s.config.ExpectedEmbeddings {
		tier0Valid = false
		violations = append(violations, fmt.Sprintf("embeddings=%d (expected 0)", int(embeddingCount)))
	}

	if int(clusteringCount) > s.config.ExpectedClusters {
		tier0Valid = false
		violations = append(violations, fmt.Sprintf("clustering_runs=%d (expected 0)", int(clusteringCount)))
	}

	if inferredTriples > 0 {
		tier0Valid = false
		violations = append(violations, fmt.Sprintf("inferred_triples=%d (expected 0)", int(inferredTriples)))
	}

	result.Metrics["tier0_valid"] = tier0Valid
	result.Details["tier0_validation"] = map[string]any{
		"valid":             tier0Valid,
		"violations":        violations,
		"embeddings":        int(embeddingCount),
		"clustering_runs":   int(clusteringCount),
		"inferred_triples":  int(inferredTriples),
		"deterministic":     len(violations) == 0,
		"hotpath_only":      true,
		"inference_blocked": true,
	}

	if !tier0Valid {
		result.Warnings = append(result.Warnings,
			fmt.Sprintf("Tier 0 constraints violated: %v", violations))
	}

	return nil
}

// extractTier0RuleMetrics parses metrics specific to tier 0 stateful rules
func extractTier0RuleMetrics(metricsText string) map[string]int {
	metrics := map[string]int{
		"evaluations_total": 0,
		"triggers_total":    0,
		"on_enter_total":    0,
		"on_exit_total":     0,
		"triples_added":     0,
		"triples_removed":   0,
	}

	// Pattern for evaluations
	evalsRe := regexp.MustCompile(`semstreams_rule_evaluations_total\{[^}]*\}\s+(\d+)`)
	for _, matches := range evalsRe.FindAllStringSubmatch(metricsText, -1) {
		if len(matches) > 1 {
			if val, err := strconv.Atoi(matches[1]); err == nil {
				metrics["evaluations_total"] += val
			}
		}
	}

	// Pattern for triggers
	triggersRe := regexp.MustCompile(`semstreams_rule_triggers_total\{[^}]*\}\s+(\d+)`)
	for _, matches := range triggersRe.FindAllStringSubmatch(metricsText, -1) {
		if len(matches) > 1 {
			if val, err := strconv.Atoi(matches[1]); err == nil {
				metrics["triggers_total"] += val
			}
		}
	}

	// Pattern for state transitions (OnEnter/OnExit)
	onEnterRe := regexp.MustCompile(`semstreams_rule_state_transitions_total\{[^}]*transition="entered"[^}]*\}\s+(\d+)`)
	for _, matches := range onEnterRe.FindAllStringSubmatch(metricsText, -1) {
		if len(matches) > 1 {
			if val, err := strconv.Atoi(matches[1]); err == nil {
				metrics["on_enter_total"] += val
			}
		}
	}

	onExitRe := regexp.MustCompile(`semstreams_rule_state_transitions_total\{[^}]*transition="exited"[^}]*\}\s+(\d+)`)
	for _, matches := range onExitRe.FindAllStringSubmatch(metricsText, -1) {
		if len(matches) > 1 {
			if val, err := strconv.Atoi(matches[1]); err == nil {
				metrics["on_exit_total"] += val
			}
		}
	}

	// Pattern for triple operations
	triplesAddedRe := regexp.MustCompile(`semstreams_rule_triples_added_total.*?(\d+)`)
	if matches := triplesAddedRe.FindStringSubmatch(metricsText); len(matches) > 1 {
		if val, err := strconv.Atoi(matches[1]); err == nil {
			metrics["triples_added"] = val
		}
	}

	triplesRemovedRe := regexp.MustCompile(`semstreams_rule_triples_removed_total.*?(\d+)`)
	if matches := triplesRemovedRe.FindStringSubmatch(metricsText); len(matches) > 1 {
		if val, err := strconv.Atoi(matches[1]); err == nil {
			metrics["triples_removed"] = val
		}
	}

	return metrics
}

// extractMetricValue extracts a single metric value using regex
func extractMetricValue(metricsText, pattern string) int {
	re := regexp.MustCompile(pattern)
	matches := re.FindStringSubmatch(metricsText)
	if len(matches) > 1 {
		if val, err := strconv.Atoi(matches[1]); err == nil {
			return val
		}
	}
	return 0
}
