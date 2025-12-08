// Package main provides the E2E test CLI for SemStreams core components
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"

	// SemStreams E2E infrastructure
	"github.com/c360/semstreams/test/e2e/client"
	"github.com/c360/semstreams/test/e2e/config"
	"github.com/c360/semstreams/test/e2e/results"
	scenarios "github.com/c360/semstreams/test/e2e/scenarios"
)

var (
	// Version information (set by build)
	version = "dev"
	commit  = "unknown"
	date    = "unknown"
)

func main() {
	// Parse command-line flags
	flags := parseCommandLineFlags()

	// Handle version and list commands
	if handleVersionCommand(flags.showVersion) {
		return
	}
	if handleListCommand(flags.listScenarios) {
		return
	}

	// Setup logger
	logger := setupLogger(flags.verbose)

	// Handle compare command
	if flags.compare {
		exitCode := handleCompareCommand(logger, flags.outputDir)
		os.Exit(exitCode)
	}

	// Handle compare-tiers command
	if flags.compareTiers {
		exitCode := handleCompareTiersCommand(logger, flags.outputDir)
		os.Exit(exitCode)
	}

	// Handle analyze-comparison command
	if flags.analyzeComparison {
		exitCode := handleAnalyzeComparisonCommand(logger, flags.outputDir)
		os.Exit(exitCode)
	}

	// Create clients and setup context
	edgeClient, cloudClient, ctx := setupClientsAndContext(logger, flags.baseURL, flags.cloudURL)

	// Run scenarios and exit
	exitCode := runScenarios(ctx, logger, edgeClient, cloudClient, flags)
	os.Exit(exitCode)
}

// cliFlags holds parsed command-line flags
type cliFlags struct {
	scenarioName  string
	verbose       bool
	baseURL       string
	cloudURL      string
	udpEndpoint   string
	wsEndpoint    string
	showVersion   bool
	listScenarios bool
	// New flags for kitchen sink variant testing
	variant           string // "core" or "ml"
	outputDir         string // Directory for results output
	compare           bool   // Generate comparison report from existing results
	compareTiers      bool   // Generate tier comparison report (0 vs 1 vs 2)
	analyzeComparison bool   // Generate Core vs ML search comparison report
	metricsURL        string // Prometheus metrics endpoint URL
}

// parseCommandLineFlags parses and returns command-line flags
func parseCommandLineFlags() *cliFlags {
	flags := &cliFlags{}

	flag.StringVar(&flags.scenarioName, "scenario", "",
		"Run specific scenario (core-health, core-dataflow, core-federation, or 'all')")
	flag.BoolVar(&flags.verbose, "verbose", false, "Enable verbose logging")
	flag.StringVar(&flags.baseURL, "base-url", config.DefaultEndpoints.HTTP, "SemStreams HTTP endpoint (edge)")
	flag.StringVar(&flags.cloudURL, "cloud-url", "http://localhost:8081",
		"SemStreams cloud HTTP endpoint (federation only)")
	flag.StringVar(&flags.udpEndpoint, "udp-endpoint", config.DefaultEndpoints.UDP, "UDP test endpoint")
	flag.StringVar(&flags.wsEndpoint, "ws-endpoint", "ws://localhost:8082/stream",
		"WebSocket endpoint (federation only)")
	flag.BoolVar(&flags.showVersion, "version", false, "Show version information")
	flag.BoolVar(&flags.listScenarios, "list", false, "List available scenarios")
	// New flags for kitchen sink variant testing
	flag.StringVar(&flags.variant, "variant", "core",
		"Test variant (core=CI-safe no ML, ml=full ML stack)")
	flag.StringVar(&flags.outputDir, "output-dir", "",
		"Directory for saving results JSON (empty=no output)")
	flag.BoolVar(&flags.compare, "compare", false,
		"Generate comparison report from existing results in output-dir")
	flag.BoolVar(&flags.compareTiers, "compare-tiers", false,
		"Generate tier comparison report (Tier 0 vs 1 vs 2) from existing results")
	flag.BoolVar(&flags.analyzeComparison, "analyze-comparison", false,
		"Generate Core vs ML search comparison report with Jaccard and correlation metrics")
	flag.StringVar(&flags.metricsURL, "metrics-url", "http://localhost:9090",
		"Prometheus metrics endpoint URL")

	// Support environment variables for Docker Compose
	if envURL := os.Getenv("SEMSTREAMS_BASE_URL"); envURL != "" {
		flags.baseURL = envURL
	}
	if envUDP := os.Getenv("UDP_ENDPOINT"); envUDP != "" {
		flags.udpEndpoint = envUDP
	}
	if envVariant := os.Getenv("E2E_VARIANT"); envVariant != "" {
		flags.variant = envVariant
	}
	if envOutput := os.Getenv("E2E_OUTPUT_DIR"); envOutput != "" {
		flags.outputDir = envOutput
	}

	flag.Parse()
	return flags
}

// handleVersionCommand shows version information and returns true if version flag is set
func handleVersionCommand(showVersion bool) bool {
	if !showVersion {
		return false
	}

	fmt.Printf("SemStreams E2E Test Runner\n")
	fmt.Printf("Version: %s\n", version)
	fmt.Printf("Commit:  %s\n", commit)
	fmt.Printf("Date:    %s\n", date)
	return true
}

// handleListCommand shows available scenarios and returns true if list flag is set
func handleListCommand(listScenarios bool) bool {
	if !listScenarios {
		return false
	}

	fmt.Println("Available scenarios:")
	fmt.Println("\nProtocol Layer:")
	fmt.Printf("  core-health         - Validates core component health (UDP, JSONFilter, JSONMap, File, HTTP POST, WebSocket)\n")
	fmt.Printf("  core-dataflow       - Tests complete data pipeline: UDP → JSONFilter → JSONMap → File\n")
	fmt.Printf("  core-federation     - Tests federation: Edge (UDP → WebSocket Out) → Cloud (WebSocket In → File)\n")
	fmt.Println("\nSemantic Layer:")
	fmt.Printf("  semantic-basic       - Basic semantic processing: UDP → JSONGeneric → Graph Processor\n")
	fmt.Printf("  semantic-indexes     - Core semantic indexes (fast, no external dependencies)\n")
	fmt.Printf("  semantic-kitchen-sink - Comprehensive semantic: Indexes + Embedding + Metrics + HTTP Gateway\n")
	fmt.Println("\nRule Processor:")
	fmt.Printf("  rules-graph          - Rule → Graph integration with metrics validation\n")
	fmt.Println("\nTiered Inference (see docs/e2e/tiers.md):")
	fmt.Printf("  tier0-rules-iot      - Tier 0: Rules-only (stateful rules, no ML inference)\n")
	fmt.Printf("  tier1-native         - Tier 1: Native inference (BM25 + LPA + statistical)\n")
	fmt.Printf("  tier2-llm            - Tier 2: LLM inference (neural embeddings + summaries)\n")
	fmt.Println("\nTest Suites:")
	fmt.Printf("  all                 - Runs all core scenarios (excludes federation and kitchen sink)\n")
	fmt.Printf("  semantic            - Runs all semantic scenarios\n")
	fmt.Printf("  rules               - Runs all rule processor scenarios\n")
	return true
}

// setupLogger creates and configures the logger
func setupLogger(verbose bool) *slog.Logger {
	logLevel := slog.LevelInfo
	if verbose {
		logLevel = slog.LevelDebug
	}

	opts := &slog.HandlerOptions{
		Level: logLevel,
	}

	logger := slog.New(slog.NewTextHandler(os.Stdout, opts))
	slog.SetDefault(logger)
	return logger
}

// setupClientsAndContext creates clients and sets up signal handling
func setupClientsAndContext(logger *slog.Logger, baseURL, cloudURL string) (
	*client.ObservabilityClient,
	*client.ObservabilityClient,
	context.Context,
) {
	edgeClient := client.NewObservabilityClient(baseURL)
	cloudClient := client.NewObservabilityClient(cloudURL)

	ctx, cancel := context.WithCancel(context.Background())

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)
	go func() {
		<-sigChan
		logger.Info("Received interrupt signal, shutting down...")
		cancel()
	}()

	return edgeClient, cloudClient, ctx
}

// runScenarios runs the appropriate scenarios based on flags
func runScenarios(
	ctx context.Context,
	logger *slog.Logger,
	edgeClient, cloudClient *client.ObservabilityClient,
	flags *cliFlags,
) int {
	logger.Info("Connecting to SemStreams",
		"base_url", flags.baseURL,
		"udp_endpoint", flags.udpEndpoint,
	)

	if flags.scenarioName == "" || flags.scenarioName == "all" {
		logger.Info("Running all core scenarios...")
		return runAllScenarios(ctx, logger, edgeClient, flags.udpEndpoint)
	} else if flags.scenarioName == "semantic" {
		logger.Info("Running all semantic scenarios...")
		return runSemanticScenarios(ctx, logger, edgeClient, flags.udpEndpoint)
	} else if flags.scenarioName == "rules" {
		logger.Info("Running all rule processor scenarios...")
		return runRulesScenarios(ctx, logger, edgeClient, flags.udpEndpoint)
	}

	// Run specific scenario
	scenario := createScenario(edgeClient, cloudClient, flags)
	if scenario == nil {
		logger.Error("Unknown scenario", "name", flags.scenarioName)
		fmt.Println("\nAvailable scenarios:")
		fmt.Println("  core-health            - Validates core component health")
		fmt.Println("  core-dataflow          - Tests complete data pipeline")
		fmt.Println("  core-federation        - Tests edge-to-cloud federation")
		fmt.Println("  semantic-basic         - Basic semantic processing")
		fmt.Println("  semantic-indexes       - Core semantic indexes (fast)")
		fmt.Println("  semantic-kitchen-sink  - Comprehensive semantic stack")
		fmt.Println("  rules-graph            - Rule → Graph integration")
		return 1
	}

	logger.Info("Running scenario", "name", flags.scenarioName)
	return runScenario(ctx, logger, scenario)
}

// createScenario creates a specific scenario by name
func createScenario(
	edgeClient *client.ObservabilityClient,
	cloudClient *client.ObservabilityClient,
	flags *cliFlags,
) scenarios.Scenario {
	switch flags.scenarioName {
	case "core-health", "health":
		return scenarios.NewCoreHealthScenario(edgeClient, nil)
	case "core-dataflow", "dataflow":
		return scenarios.NewCoreDataflowScenario(edgeClient, flags.udpEndpoint, nil)
	case "core-federation", "federation":
		return scenarios.NewCoreFederationScenario(edgeClient, cloudClient, flags.udpEndpoint, flags.wsEndpoint, nil)
	case "semantic-basic", "basic":
		return scenarios.NewSemanticBasicScenario(edgeClient, flags.udpEndpoint, nil)
	case "semantic-indexes", "indexes":
		return scenarios.NewSemanticIndexesScenario(edgeClient, flags.udpEndpoint, nil)
	case "semantic-kitchen-sink", "kitchen-sink", "kitchen":
		cfg := scenarios.DefaultSemanticKitchenSinkConfig()
		cfg.MetricsURL = flags.metricsURL
		cfg.GatewayURL = flags.baseURL + "/api-gateway"
		cfg.OutputDir = flags.outputDir
		return scenarios.NewSemanticKitchenSinkScenario(edgeClient, flags.udpEndpoint, cfg)
	case "rules-graph", "rules-graph-integration", "rules":
		return scenarios.NewRulesGraphScenario(edgeClient, flags.udpEndpoint, nil)
	case "tier0-rules-iot", "tier0":
		return scenarios.NewTier0RulesIoTScenario(edgeClient, flags.udpEndpoint, nil)
	case "tier1-native", "tier1":
		// Tier 1 uses kitchen-sink with core variant (BM25 fallback)
		cfg := scenarios.DefaultSemanticKitchenSinkConfig()
		cfg.MetricsURL = flags.metricsURL
		cfg.GatewayURL = flags.baseURL + "/api-gateway"
		cfg.OutputDir = flags.outputDir
		return scenarios.NewSemanticKitchenSinkScenario(edgeClient, flags.udpEndpoint, cfg)
	case "tier2-llm", "tier2":
		// Tier 2 uses kitchen-sink with ml variant (neural embeddings + LLM summaries)
		cfg := scenarios.DefaultSemanticKitchenSinkConfig()
		cfg.MetricsURL = flags.metricsURL
		cfg.GatewayURL = flags.baseURL + "/api-gateway"
		cfg.OutputDir = flags.outputDir
		return scenarios.NewSemanticKitchenSinkScenario(edgeClient, flags.udpEndpoint, cfg)
	default:
		return nil
	}
}

// runScenario executes a single scenario
func runScenario(ctx context.Context, logger *slog.Logger, scenario scenarios.Scenario) int {
	logger.Info("Setting up scenario", "name", scenario.Name())

	if err := scenario.Setup(ctx); err != nil {
		logger.Error("Scenario setup failed", "error", err)
		return 1
	}

	logger.Info("Executing scenario", "name", scenario.Name())
	result, err := scenario.Execute(ctx)

	// Always cleanup
	logger.Info("Tearing down scenario", "name", scenario.Name())
	if teardownErr := scenario.Teardown(ctx); teardownErr != nil {
		logger.Warn("Teardown failed", "error", teardownErr)
	}

	if err != nil {
		logger.Error("Scenario failed", "error", err)
		return 1
	}

	if !result.Success {
		logger.Error("Scenario completed with failure",
			"error", result.Error,
			"duration", result.Duration)
		return 1
	}

	logger.Info("Scenario completed successfully",
		"duration", result.Duration,
		"metrics", result.Metrics)

	return 0
}

// runAllScenarios executes all core scenarios
func runAllScenarios(
	ctx context.Context,
	logger *slog.Logger,
	obsClient *client.ObservabilityClient,
	udpEndpoint string,
) int {
	tests := []scenarios.Scenario{
		scenarios.NewCoreHealthScenario(obsClient, nil),
		scenarios.NewCoreDataflowScenario(obsClient, udpEndpoint, nil),
	}

	passed := 0
	failed := 0

	for _, scenario := range tests {
		logger.Info("Running scenario", "name", scenario.Name())
		exitCode := runScenario(ctx, logger, scenario)

		if exitCode == 0 {
			passed++
			logger.Info("Scenario PASSED", "name", scenario.Name())
		} else {
			failed++
			logger.Error("Scenario FAILED", "name", scenario.Name())
		}
	}

	logger.Info("Test suite complete",
		"passed", passed,
		"failed", failed,
		"total", len(tests))

	if failed > 0 {
		return 1
	}
	return 0
}

// runSemanticScenarios executes all semantic scenarios
func runSemanticScenarios(
	ctx context.Context,
	logger *slog.Logger,
	obsClient *client.ObservabilityClient,
	udpEndpoint string,
) int {
	tests := []scenarios.Scenario{
		scenarios.NewSemanticBasicScenario(obsClient, udpEndpoint, nil),
		scenarios.NewSemanticIndexesScenario(obsClient, udpEndpoint, nil),
		scenarios.NewSemanticKitchenSinkScenario(obsClient, udpEndpoint, nil),
	}

	passed := 0
	failed := 0

	for _, scenario := range tests {
		logger.Info("Running semantic scenario", "name", scenario.Name())
		exitCode := runScenario(ctx, logger, scenario)

		if exitCode == 0 {
			passed++
			logger.Info("Semantic scenario PASSED", "name", scenario.Name())
		} else {
			failed++
			logger.Error("Semantic scenario FAILED", "name", scenario.Name())
		}
	}

	logger.Info("Semantic test suite complete",
		"passed", passed,
		"failed", failed,
		"total", len(tests))

	if failed > 0 {
		return 1
	}
	return 0
}

// runRulesScenarios executes all rule processor scenarios
func runRulesScenarios(
	ctx context.Context,
	logger *slog.Logger,
	obsClient *client.ObservabilityClient,
	udpEndpoint string,
) int {
	tests := []scenarios.Scenario{
		scenarios.NewRulesGraphScenario(obsClient, udpEndpoint, nil),
	}

	passed := 0
	failed := 0

	for _, scenario := range tests {
		logger.Info("Running rule processor scenario", "name", scenario.Name())
		exitCode := runScenario(ctx, logger, scenario)

		if exitCode == 0 {
			passed++
			logger.Info("Rule processor scenario PASSED", "name", scenario.Name())
		} else {
			failed++
			logger.Error("Rule processor scenario FAILED", "name", scenario.Name())
		}
	}

	logger.Info("Rule processor test suite complete",
		"passed", passed,
		"failed", failed,
		"total", len(tests))

	if failed > 0 {
		return 1
	}
	return 0
}

// handleCompareCommand generates comparison report from existing results
func handleCompareCommand(logger *slog.Logger, outputDir string) int {
	if outputDir == "" {
		logger.Error("Output directory required for comparison (use --output-dir)")
		return 1
	}

	logger.Info("Generating comparison report", "output_dir", outputDir)

	writer := results.NewWriter(outputDir)

	// List all available runs
	files, err := writer.ListRuns()
	if err != nil {
		logger.Error("Failed to list runs", "error", err)
		return 1
	}

	if len(files) < 2 {
		logger.Warn("Need at least 2 test runs to compare", "found", len(files))
		return 1
	}

	// Find core and ml variant runs (look for latest of each)
	var coreRun, mlRun *results.TestRun
	for i := len(files) - 1; i >= 0; i-- {
		run, err := writer.LoadRun(files[i])
		if err != nil {
			logger.Warn("Failed to load run", "file", files[i], "error", err)
			continue
		}

		if run.Config.Variant == "core" && coreRun == nil {
			coreRun = run
		} else if run.Config.Variant == "ml" && mlRun == nil {
			mlRun = run
		}

		if coreRun != nil && mlRun != nil {
			break
		}
	}

	if coreRun == nil || mlRun == nil {
		logger.Warn("Need both core and ml variant runs to compare",
			"has_core", coreRun != nil,
			"has_ml", mlRun != nil)
		return 1
	}

	// Compare: baseline=core, current=ml
	comparison := results.Compare(coreRun, mlRun)

	// Write comparison report
	filepath, err := writer.WriteComparison(comparison)
	if err != nil {
		logger.Error("Failed to write comparison report", "error", err)
		return 1
	}

	// Print summary
	printComparisonSummary(logger, coreRun, mlRun, comparison, filepath)

	return 0
}

// printComparisonSummary outputs a human-readable comparison
func printComparisonSummary(
	logger *slog.Logger,
	coreRun, mlRun *results.TestRun,
	comparison *results.Comparison,
	filepath string,
) {
	fmt.Println("\n=== Kitchen Sink Variant Comparison ===")
	fmt.Printf("Core variant: %s\n", coreRun.Timestamp.Format(time.RFC3339))
	fmt.Printf("ML variant:   %s\n", mlRun.Timestamp.Format(time.RFC3339))

	fmt.Println("\n--- Duration ---")
	fmt.Printf("Core: %s\n", coreRun.DurationStr)
	fmt.Printf("ML:   %s\n", mlRun.DurationStr)

	fmt.Println("\n--- Success ---")
	fmt.Printf("Core: %d/%d passed (%.0f%%)\n",
		coreRun.Summary.PassedScenarios,
		coreRun.Summary.TotalScenarios,
		coreRun.Summary.SuccessRate*100)
	fmt.Printf("ML:   %d/%d passed (%.0f%%)\n",
		mlRun.Summary.PassedScenarios,
		mlRun.Summary.TotalScenarios,
		mlRun.Summary.SuccessRate*100)

	fmt.Println("\n--- Overall Comparison ---")
	fmt.Printf("Status Changes:    %d\n", comparison.Overall.StatusChanges)
	fmt.Printf("Improvements:      %d\n", comparison.Overall.Improvements)
	fmt.Printf("Regressions:       %d\n", comparison.Overall.Regressions)
	fmt.Printf("Metrics Improved:  %d\n", comparison.Overall.MetricsImproved)
	fmt.Printf("Metrics Regressed: %d\n", comparison.Overall.MetricsRegressed)

	if len(comparison.Diffs) > 0 {
		fmt.Println("\n--- Scenario Diffs ---")
		for _, diff := range comparison.Diffs {
			status := "unchanged"
			if diff.StatusChanged {
				if diff.CurrentSuccess {
					status = "IMPROVED"
				} else {
					status = "REGRESSED"
				}
			}
			fmt.Printf("  %s: %s (duration delta: %dms)\n",
				diff.ScenarioName, status, diff.DurationChangeMs)
		}
	}

	logger.Info("Comparison report written", "file", filepath)
}

// handleAnalyzeComparisonCommand generates Core vs ML search comparison report
func handleAnalyzeComparisonCommand(logger *slog.Logger, outputDir string) int {
	if outputDir == "" {
		// Use default output directory
		outputDir = "test/e2e/results"
	}

	logger.Info("Analyzing Core vs ML search comparison", "output_dir", outputDir)

	report, err := analyzeComparison(outputDir)
	if err != nil {
		logger.Error("Failed to analyze comparison", "error", err)
		fmt.Printf("\nError: %v\n", err)
		fmt.Println("\nTo generate comparison files, run:")
		fmt.Println("  1. Run with Core: ./e2e --scenario semantic-kitchen-sink --output-dir test/e2e/results")
		fmt.Println("  2. Run with ML:   ./e2e --scenario semantic-kitchen-sink --output-dir test/e2e/results (with semembed)")
		fmt.Println("  3. Analyze:       ./e2e --analyze-comparison --output-dir test/e2e/results")
		return 1
	}

	// Print the report
	printAnalysisReport(report)

	// Optionally save report to JSON
	reportFile := fmt.Sprintf("%s/comparison-report-%s.json", outputDir, time.Now().Format("20060102-150405"))
	data, err := json.MarshalIndent(report, "", "  ")
	if err == nil {
		if err := os.WriteFile(reportFile, data, 0644); err == nil {
			logger.Info("Report saved", "file", reportFile)
		}
	}

	return 0
}

// handleCompareTiersCommand generates a tier comparison report (Tier 0 vs 1 vs 2)
func handleCompareTiersCommand(logger *slog.Logger, outputDir string) int {
	if outputDir == "" {
		outputDir = "test/e2e/results"
	}

	logger.Info("Generating tier comparison report", "output_dir", outputDir)

	// Build tier comparison report
	report := TierComparisonReport{
		GeneratedAt: time.Now(),
		OutputDir:   outputDir,
		Tiers:       make(map[string]TierMetrics),
	}

	// Define tier expectations
	tierExpectations := map[string]TierExpectation{
		"tier0": {
			Name:               "Rules-Only",
			ExpectedEmbeddings: 0,
			ExpectedClusters:   0,
			ExpectedInference:  false,
		},
		"tier1": {
			Name:               "Native (BM25 + LPA)",
			ExpectedEmbeddings: -1, // Any non-zero
			ExpectedClusters:   -1, // Any non-zero
			ExpectedInference:  true,
		},
		"tier2": {
			Name:               "LLM (Neural + Summaries)",
			ExpectedEmbeddings: -1, // Any non-zero
			ExpectedClusters:   -1, // Any non-zero
			ExpectedInference:  true,
		},
	}

	// Print the report
	fmt.Println("\n=== Tier Comparison Report ===")
	fmt.Printf("Generated: %s\n", report.GeneratedAt.Format(time.RFC3339))
	fmt.Printf("Output Dir: %s\n\n", outputDir)

	fmt.Println("Tier Expectations:")
	fmt.Println("------------------")
	for tier, exp := range tierExpectations {
		embStr := "0"
		if exp.ExpectedEmbeddings < 0 {
			embStr = ">0"
		}
		clustStr := "0"
		if exp.ExpectedClusters < 0 {
			clustStr = ">0"
		}
		fmt.Printf("  %s (%s):\n", tier, exp.Name)
		fmt.Printf("    Embeddings: %s\n", embStr)
		fmt.Printf("    Clusters: %s\n", clustStr)
		fmt.Printf("    Inference: %v\n", exp.ExpectedInference)
	}

	fmt.Println("\nTo run all tiers and generate comparison data:")
	fmt.Println("  task e2e:tiers")
	fmt.Println("\nThis will run tier0 → tier1 → tier2 sequentially and output results.")

	// Save report to JSON
	tierReportFile := fmt.Sprintf("%s/tier-comparison-%s.json", outputDir, time.Now().Format("20060102-150405"))
	data, err := json.MarshalIndent(report, "", "  ")
	if err == nil {
		if err := os.WriteFile(tierReportFile, data, 0644); err == nil {
			logger.Info("Report saved", "file", tierReportFile)
		}
	}

	return 0
}

// TierComparisonReport holds the comparison data across tiers
type TierComparisonReport struct {
	GeneratedAt time.Time              `json:"generated_at"`
	OutputDir   string                 `json:"output_dir"`
	Tiers       map[string]TierMetrics `json:"tiers"`
}

// TierMetrics holds metrics for a single tier
type TierMetrics struct {
	Tier             int     `json:"tier"`
	Name             string  `json:"name"`
	DurationMs       int64   `json:"duration_ms"`
	EntitiesStored   int     `json:"entities_stored"`
	EmbeddingsGen    int     `json:"embeddings_generated"`
	CommunitiesFound int     `json:"communities_found"`
	RulesEvaluated   int     `json:"rules_evaluated"`
	RulesTriggered   int     `json:"rules_triggered"`
	SearchQuality    float64 `json:"search_quality"`
}

// TierExpectation defines expected behavior for a tier
type TierExpectation struct {
	Name               string
	ExpectedEmbeddings int  // -1 means any non-zero
	ExpectedClusters   int  // -1 means any non-zero
	ExpectedInference  bool
}
