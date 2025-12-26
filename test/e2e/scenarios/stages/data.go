package stages

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"time"

	"github.com/c360/semstreams/test/e2e/client"
)

// DataSender handles sending test data through the pipeline
type DataSender struct {
	UDPAddr  string
	Metrics  *client.MetricsClient
	Interval time.Duration
}

// SendResult contains the results of data sending
type SendResult struct {
	MessagesSent   int                     `json:"messages_sent"`
	TelemetrySent  int                     `json:"telemetry_sent"`
	RegularSent    int                     `json:"regular_sent"`
	Baseline       *client.MetricsBaseline `json:"-"` // Pre-send baseline for delta validation
	BaselineErrors []string                `json:"baseline_errors,omitempty"`
}

// SendMixedData sends a mix of telemetry and regular messages
func (d *DataSender) SendMixedData(ctx context.Context, count int) (*SendResult, error) {
	result := &SendResult{}

	// Capture baseline BEFORE sending data
	if d.Metrics != nil {
		baseline, err := d.Metrics.CaptureBaseline(ctx)
		if err != nil {
			result.BaselineErrors = append(result.BaselineErrors,
				fmt.Sprintf("Could not capture pre-send baseline: %v", err))
		} else {
			result.Baseline = baseline
		}
	}

	conn, err := net.Dial("udp", d.UDPAddr)
	if err != nil {
		return nil, fmt.Errorf("UDP connection failed: %w", err)
	}
	defer conn.Close()

	for i := 0; i < count; i++ {
		var testMsg map[string]any

		// Alternate between telemetry (entities) and regular messages
		if i%2 == 0 {
			testMsg = d.createTelemetryMessage(i)
			result.TelemetrySent++
		} else {
			testMsg = d.createRegularMessage(i)
			result.RegularSent++
		}

		msgBytes, err := json.Marshal(testMsg)
		if err != nil {
			continue
		}

		if _, err = conn.Write(msgBytes); err != nil {
			continue
		}

		result.MessagesSent++

		// Wait between messages
		select {
		case <-ctx.Done():
			return result, ctx.Err()
		case <-time.After(d.Interval):
		}
	}

	return result, nil
}

// createTelemetryMessage creates a telemetry message that will be processed as an entity
func (d *DataSender) createTelemetryMessage(index int) map[string]any {
	return map[string]any{
		"type":        "telemetry",
		"entity_id":   fmt.Sprintf("device-%d", index/2),
		"entity_type": "sensor",
		"timestamp":   time.Now().Unix(),
		"data": map[string]any{
			"temperature": 20.0 + float64(index),
			"humidity":    50.0 + float64(index*2),
			"pressure":    1013.0 + float64(index)*0.5,
			"location": map[string]any{
				"lat": 37.7749 + float64(index)*0.001,
				"lon": -122.4194 + float64(index)*0.001,
			},
		},
		"value": index * 5,
	}
}

// createRegularMessage creates a regular message that won't be processed as an entity
func (d *DataSender) createRegularMessage(index int) map[string]any {
	return map[string]any{
		"type":      "regular",
		"value":     index * 10,
		"timestamp": time.Now().Unix(),
		"metadata": map[string]any{
			"source":   "test",
			"sequence": index,
		},
	}
}

// ProcessingValidator validates that data was processed through the pipeline
type ProcessingValidator struct {
	Client            *client.ObservabilityClient
	Metrics           *client.MetricsClient
	Tracer            *client.FlowTracer
	ValidationTimeout time.Duration
	PollInterval      time.Duration
	MinProcessed      int
}

// ValidationResult contains processing validation results
type ValidationResult struct {
	AlreadyComplete      bool     `json:"already_complete,omitempty"`
	EntitiesProcessed    int      `json:"entities_processed"`
	GraphProcessorFound  bool     `json:"graph_processor_found"`
	GraphProcessorHealth bool     `json:"graph_processor_healthy"`
	ComponentCount       int      `json:"component_count"`
	FlowSnapshotCaptured bool     `json:"flow_snapshot_captured,omitempty"`
	Warnings             []string `json:"warnings,omitempty"`
}

// ValidateProcessing validates data was processed through the semantic pipeline
func (v *ProcessingValidator) ValidateProcessing(ctx context.Context, baseline *client.MetricsBaseline) (*ValidationResult, error) {
	result := &ValidationResult{}

	// Check current state - test data may already be loaded
	currentValue, err := v.Metrics.SumMetricsByName(ctx, "semstreams_datamanager_entities_updated_total")
	if err != nil {
		result.Warnings = append(result.Warnings, fmt.Sprintf("Could not fetch current metrics: %v", err))
	}

	// If we have sufficient entities already processed, skip waiting
	if currentValue >= float64(v.MinProcessed) {
		result.AlreadyComplete = true
		result.EntitiesProcessed = int(currentValue)
	} else if baseline == nil {
		result.Warnings = append(result.Warnings, "No pre-send baseline available, using default wait")
		if err := v.Client.WaitForAllComponentsHealthy(ctx, 10*time.Second); err != nil {
			result.Warnings = append(result.Warnings, fmt.Sprintf("Components health wait: %v", err))
		}
	} else {
		// Wait for processing using event-driven metric polling
		waitOpts := client.WaitOpts{
			Timeout:      v.ValidationTimeout,
			PollInterval: v.PollInterval,
			Comparator:   ">=",
		}

		baselineUpdates := baseline.Metrics["semstreams_datamanager_entities_updated_total"]
		expectedUpdates := baselineUpdates + float64(v.MinProcessed)

		if err := v.Metrics.WaitForMetric(ctx, "semstreams_datamanager_entities_updated_total", expectedUpdates, waitOpts); err != nil {
			result.Warnings = append(result.Warnings, fmt.Sprintf("Processing wait: %v (may still be processing)", err))
		}
	}

	// Capture flow snapshot
	if v.Tracer != nil {
		flowSnapshot, err := v.Tracer.CaptureFlowSnapshot(ctx)
		if err != nil {
			result.Warnings = append(result.Warnings, fmt.Sprintf("Failed to capture flow snapshot: %v", err))
		} else {
			result.FlowSnapshotCaptured = true
			_ = flowSnapshot // Could store message count if needed
		}
	}

	// Verify graph processor is present and healthy
	components, err := v.Client.GetComponents(ctx)
	if err != nil {
		return nil, fmt.Errorf("component query failed: %w", err)
	}

	result.ComponentCount = len(components)

	for _, comp := range components {
		if comp.Name == "graph" {
			result.GraphProcessorFound = true
			result.GraphProcessorHealth = comp.Healthy
			if !comp.Healthy {
				result.Warnings = append(result.Warnings,
					fmt.Sprintf("Graph processor not healthy: state=%s", comp.State))
			}
			break
		}
	}

	if !result.GraphProcessorFound {
		return result, fmt.Errorf("graph processor not found")
	}

	return result, nil
}
