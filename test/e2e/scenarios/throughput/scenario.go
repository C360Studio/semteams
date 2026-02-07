// Package throughput provides a high-throughput E2E scenario for performance profiling.
package throughput

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"sync"
	"time"

	"github.com/c360studio/semstreams/test/e2e/client"
	"github.com/c360studio/semstreams/test/e2e/config"
	"github.com/c360studio/semstreams/test/e2e/scenarios"
)

// Scenario sends high volumes of messages for performance profiling.
type Scenario struct {
	name        string
	description string
	udpAddr     string
	metrics     *client.MetricsClient
	profile     *client.ProfileClient
	config      *Config
}

// Config holds configuration for the throughput scenario.
type Config struct {
	// Message configuration
	MessageCount int `json:"message_count"` // Total messages to send (default: 10000)
	MessageRate  int `json:"message_rate"`  // Messages per second, 0 = unlimited
	MessageSize  int `json:"message_size"`  // Approximate bytes per message (default: 256)

	// Profile configuration
	ProfileDuration int    `json:"profile_duration"` // CPU profile seconds (default: 30)
	ProfileDir      string `json:"profile_dir"`      // Directory for profile output

	// Validation
	ValidationTimeout time.Duration `json:"validation_timeout"`  // Max wait for processing
	MinProcessedRatio float64       `json:"min_processed_ratio"` // Min ratio of messages processed (default: 0.9)
}

// DefaultConfig returns sensible defaults for throughput testing.
func DefaultConfig() *Config {
	return &Config{
		MessageCount:      10000,
		MessageRate:       0, // Unlimited
		MessageSize:       256,
		ProfileDuration:   30,
		ProfileDir:        "test/e2e/results/profiles",
		ValidationTimeout: 60 * time.Second,
		MinProcessedRatio: 0.9,
	}
}

// NewScenario creates a new throughput scenario.
func NewScenario(metricsURL, udpAddr string, cfg *Config) *Scenario {
	if cfg == nil {
		cfg = DefaultConfig()
	}
	if udpAddr == "" {
		udpAddr = config.DefaultEndpoints.UDP
	}
	if metricsURL == "" {
		metricsURL = config.DefaultEndpoints.Metrics
	}

	return &Scenario{
		name:        "throughput",
		description: "High-throughput stress test with integrated profiling",
		udpAddr:     udpAddr,
		metrics:     client.NewMetricsClient(metricsURL),
		profile:     client.NewProfileClient(config.DefaultEndpoints.PProf, cfg.ProfileDir),
		config:      cfg,
	}
}

// Name returns the scenario name.
func (s *Scenario) Name() string {
	return s.name
}

// Description returns the scenario description.
func (s *Scenario) Description() string {
	return s.description
}

// Setup prepares the scenario.
func (s *Scenario) Setup(ctx context.Context) error {
	// Verify UDP endpoint is reachable
	conn, err := net.Dial("udp", s.udpAddr)
	if err != nil {
		return fmt.Errorf("cannot reach UDP endpoint %s: %w", s.udpAddr, err)
	}
	_ = conn.Close()

	// Verify metrics endpoint
	if err := s.metrics.Health(ctx); err != nil {
		return fmt.Errorf("metrics endpoint not healthy: %w", err)
	}

	return nil
}

// Execute runs the throughput test.
func (s *Scenario) Execute(ctx context.Context) (*scenarios.Result, error) {
	result := &scenarios.Result{
		ScenarioName: s.name,
		StartTime:    time.Now(),
		Success:      false,
		Metrics:      make(map[string]any),
		Details:      make(map[string]any),
		Errors:       []string{},
		Warnings:     []string{},
	}

	// Setup profiling if available
	profilingEnabled := s.profile.IsAvailable(ctx)
	result.Details["profiling_enabled"] = profilingEnabled
	if profilingEnabled {
		cleanup := s.startProfiling(ctx, result)
		defer cleanup()
	}

	// Capture baseline metrics
	baseline, err := s.metrics.CaptureBaseline(ctx)
	if err != nil {
		result.Warnings = append(result.Warnings, fmt.Sprintf("Failed to capture metrics baseline: %v", err))
	}

	// Send messages
	sendStart := time.Now()
	sendResult, err := s.sendMessages(ctx, result)
	if err != nil {
		result.Error = fmt.Sprintf("send failed: %v", err)
		result.EndTime = time.Now()
		result.Duration = result.EndTime.Sub(result.StartTime)
		return result, nil
	}

	result.Metrics["messages_sent"] = sendResult.MessagesSent
	result.Metrics["send_duration_ms"] = sendResult.Duration.Milliseconds()
	result.Metrics["send_rate_msg_sec"] = sendResult.Rate
	result.Details["send_summary"] = fmt.Sprintf(
		"Sent %d messages in %v (%.0f msg/sec)",
		sendResult.MessagesSent, sendResult.Duration, sendResult.Rate)

	// Wait for processing with timeout
	processingStart := time.Now()
	if err := s.waitForProcessing(ctx, baseline, sendResult.MessagesSent); err != nil {
		result.Warnings = append(result.Warnings, fmt.Sprintf("Processing validation: %v", err))
	}
	result.Metrics["processing_wait_ms"] = time.Since(processingStart).Milliseconds()

	// Capture final metrics delta
	s.captureMetricsDelta(ctx, baseline, result)

	// Capture final profiles
	if profilingEnabled {
		s.captureFinalProfiles(ctx, result)
	}

	// Calculate end-to-end throughput
	totalDuration := time.Since(sendStart)
	result.Metrics["total_throughput_msg_sec"] = float64(sendResult.MessagesSent) / totalDuration.Seconds()

	result.Success = true
	result.EndTime = time.Now()
	result.Duration = result.EndTime.Sub(result.StartTime)

	return result, nil
}

// startProfiling captures baseline heap and starts CPU profiling.
// Returns a cleanup function that waits for CPU profile completion.
func (s *Scenario) startProfiling(ctx context.Context, result *scenarios.Result) func() {
	// Capture baseline heap before load
	if path, err := s.profile.CaptureHeap(ctx, "throughput-baseline"); err != nil {
		result.Warnings = append(result.Warnings, fmt.Sprintf("Failed to capture baseline heap: %v", err))
	} else {
		result.Details["profile_baseline_heap"] = path
	}

	// Start CPU profile async (runs during the load phase)
	var cpuProfilePath string
	var cpuProfileErr error
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		cpuProfilePath, cpuProfileErr = s.profile.CaptureCPU(ctx, "throughput", s.config.ProfileDuration)
	}()

	return func() {
		wg.Wait()
		if cpuProfileErr != nil {
			result.Warnings = append(result.Warnings, fmt.Sprintf("CPU profile failed: %v", cpuProfileErr))
		} else if cpuProfilePath != "" {
			result.Details["profile_cpu"] = cpuProfilePath
		}
	}
}

// captureMetricsDelta captures and records the metrics delta from baseline.
func (s *Scenario) captureMetricsDelta(ctx context.Context, baseline *client.MetricsBaseline, result *scenarios.Result) {
	if baseline == nil {
		return
	}
	diff, err := s.metrics.CompareToBaseline(ctx, baseline)
	if err != nil {
		return
	}
	result.Metrics["total_duration_sec"] = diff.Duration.Seconds()
	if rate, ok := diff.RatePerSec["semstreams_udp_datagrams_received_total"]; ok {
		result.Metrics["udp_receive_rate"] = rate
	}
	if rate, ok := diff.RatePerSec["semstreams_file_lines_written_total"]; ok {
		result.Metrics["file_write_rate"] = rate
	}
}

// captureFinalProfiles captures heap and goroutine profiles after the test.
func (s *Scenario) captureFinalProfiles(ctx context.Context, result *scenarios.Result) {
	if path, err := s.profile.CaptureHeap(ctx, "throughput-final"); err != nil {
		result.Warnings = append(result.Warnings, fmt.Sprintf("Failed to capture final heap: %v", err))
	} else {
		result.Details["profile_final_heap"] = path
	}

	if path, err := s.profile.CaptureGoroutine(ctx, "throughput-final"); err != nil {
		result.Warnings = append(result.Warnings, fmt.Sprintf("Failed to capture goroutine profile: %v", err))
	} else {
		result.Details["profile_goroutine"] = path
	}
}

// Teardown cleans up after the scenario.
func (s *Scenario) Teardown(_ context.Context) error {
	return nil
}

// SendResult holds the results of the send phase.
type SendResult struct {
	MessagesSent int
	Duration     time.Duration
	Rate         float64 // messages per second
}

// sendMessages sends messages at the configured rate.
func (s *Scenario) sendMessages(ctx context.Context, result *scenarios.Result) (*SendResult, error) {
	conn, err := net.Dial("udp", s.udpAddr)
	if err != nil {
		return nil, fmt.Errorf("UDP connection failed: %w", err)
	}
	defer conn.Close()

	// Pre-generate messages to avoid allocation during send loop
	messages := s.generateMessages(s.config.MessageCount, s.config.MessageSize)
	result.Details["messages_pregenerated"] = len(messages)

	// Rate limiting setup
	var ticker *time.Ticker
	if s.config.MessageRate > 0 {
		interval := time.Second / time.Duration(s.config.MessageRate)
		ticker = time.NewTicker(interval)
		defer ticker.Stop()
		result.Details["rate_limit_msg_sec"] = s.config.MessageRate
	} else {
		result.Details["rate_limit_msg_sec"] = "unlimited"
	}

	start := time.Now()
	sent := 0
	errors := 0

	for i, msg := range messages {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		default:
		}

		if ticker != nil {
			<-ticker.C
		}

		if _, err := conn.Write(msg); err != nil {
			errors++
			if errors < 10 {
				result.Warnings = append(result.Warnings, fmt.Sprintf("Send error at msg %d: %v", i, err))
			}
			continue
		}
		sent++

		// Progress logging every 1000 messages
		if sent%1000 == 0 && sent > 0 {
			elapsed := time.Since(start)
			rate := float64(sent) / elapsed.Seconds()
			fmt.Printf("  Sent %d/%d messages (%.0f msg/sec)\n", sent, s.config.MessageCount, rate)
		}
	}

	elapsed := time.Since(start)
	result.Metrics["send_errors"] = errors

	return &SendResult{
		MessagesSent: sent,
		Duration:     elapsed,
		Rate:         float64(sent) / elapsed.Seconds(),
	}, nil
}

// generateMessages pre-generates all test messages.
func (s *Scenario) generateMessages(count, targetSize int) [][]byte {
	messages := make([][]byte, count)

	// Calculate padding to reach target size
	baseMsg := map[string]any{
		"type":      "throughput-test",
		"sequence":  0,
		"timestamp": time.Now().UnixNano(),
		"data": map[string]any{
			"value":   0,
			"padding": "",
		},
	}

	baseBytes, _ := json.Marshal(baseMsg)
	paddingNeeded := targetSize - len(baseBytes)
	if paddingNeeded < 0 {
		paddingNeeded = 0
	}

	padding := make([]byte, paddingNeeded)
	for i := range padding {
		padding[i] = 'x'
	}
	paddingStr := string(padding)

	for i := 0; i < count; i++ {
		msg := map[string]any{
			"type":      "throughput-test",
			"sequence":  i,
			"timestamp": time.Now().UnixNano(),
			"value":     i * 10, // Varying values for filter testing
			"data": map[string]any{
				"index":   i,
				"padding": paddingStr,
			},
		}
		msgBytes, _ := json.Marshal(msg)
		messages[i] = msgBytes
	}

	return messages
}

// waitForProcessing waits for messages to be processed.
func (s *Scenario) waitForProcessing(ctx context.Context, baseline *client.MetricsBaseline, expectedCount int) error {
	if baseline == nil {
		// No baseline, just wait a fixed time
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(5 * time.Second):
			return nil
		}
	}

	// Wait for file lines written to reach expected count
	deadline := time.Now().Add(s.config.ValidationTimeout)
	minExpected := float64(expectedCount) * s.config.MinProcessedRatio

	for time.Now().Before(deadline) {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(500 * time.Millisecond):
		}

		diff, err := s.metrics.CompareToBaseline(ctx, baseline)
		if err != nil {
			continue
		}

		// Check file output lines
		if delta, ok := diff.Deltas["semstreams_file_lines_written_total"]; ok {
			if delta >= minExpected {
				return nil
			}
		}
	}

	return fmt.Errorf("timeout waiting for processing (expected %.0f messages)", minExpected)
}
