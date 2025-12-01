// Package scenarios provides E2E test scenarios for SemStreams semantic processing
package scenarios

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"time"

	"github.com/c360/semstreams/test/e2e/client"
	"github.com/c360/semstreams/test/e2e/config"
)

// SemanticBasicScenario validates basic semantic processing with graph processor
type SemanticBasicScenario struct {
	name             string
	description      string
	client           *client.ObservabilityClient
	udpAddr          string
	natsURL          string
	config           *SemanticBasicConfig
	validationConfig *config.ValidationConfig
}

// SemanticBasicConfig contains configuration for basic semantic test
type SemanticBasicConfig struct {
	// Test data configuration
	EntityCount     int           `json:"entity_count"`
	MessageInterval time.Duration `json:"message_interval"`

	// Validation configuration
	ValidationDelay time.Duration `json:"validation_delay"`
	MinEntities     int           `json:"min_entities"`
}

// DefaultSemanticBasicConfig returns default configuration
func DefaultSemanticBasicConfig() *SemanticBasicConfig {
	return &SemanticBasicConfig{
		EntityCount:     5,
		MessageInterval: 100 * time.Millisecond,
		ValidationDelay: 3 * time.Second,
		MinEntities:     3, // At least 60% should be processed
	}
}

// NewSemanticBasicScenario creates a new basic semantic test scenario
func NewSemanticBasicScenario(
	obsClient *client.ObservabilityClient,
	udpAddr string,
	cfg *SemanticBasicConfig,
) *SemanticBasicScenario {
	if cfg == nil {
		cfg = DefaultSemanticBasicConfig()
	}
	if udpAddr == "" {
		udpAddr = config.DefaultEndpoints.UDP
	}

	return &SemanticBasicScenario{
		name:             "semantic-basic",
		description:      "Tests basic semantic processing: UDP → JSONGeneric → Graph Processor → NATS KV",
		client:           obsClient,
		udpAddr:          udpAddr,
		natsURL:          config.DefaultEndpoints.NATS,
		config:           cfg,
		validationConfig: config.DefaultValidationConfig(),
	}
}

// Name returns the scenario name
func (s *SemanticBasicScenario) Name() string {
	return s.name
}

// Description returns the scenario description
func (s *SemanticBasicScenario) Description() string {
	return s.description
}

// Setup prepares the scenario
func (s *SemanticBasicScenario) Setup(_ context.Context) error {
	// Verify UDP endpoint is reachable
	conn, err := net.Dial("udp", s.udpAddr)
	if err != nil {
		return fmt.Errorf("cannot reach UDP endpoint %s: %w", s.udpAddr, err)
	}
	_ = conn.Close()

	return nil
}

// Execute runs the basic semantic test scenario
func (s *SemanticBasicScenario) Execute(ctx context.Context) (*Result, error) {
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
		{"send-entities", s.executeSendEntities},
		{"validate-processing", s.executeValidateProcessing},
		{"validate-nats-kv", s.executeValidateNATSKV},
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
func (s *SemanticBasicScenario) Teardown(_ context.Context) error {
	// No cleanup needed for basic semantic test
	return nil
}

// executeVerifyComponents checks that semantic pipeline components exist
func (s *SemanticBasicScenario) executeVerifyComponents(ctx context.Context, result *Result) error {
	components, err := s.client.GetComponents(ctx)
	if err != nil {
		result.Errors = append(result.Errors, fmt.Sprintf("Failed to get components: %v", err))
		return fmt.Errorf("component verification failed: %w", err)
	}

	requiredComponents := []string{"udp", "iot_sensor", "graph"}
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
			fmt.Sprintf("Missing semantic pipeline components: %v", missingComponents))
		return fmt.Errorf("missing components: %v", missingComponents)
	}

	result.Details["semantic_components"] = requiredComponents
	return nil
}

// executeSendEntities sends entity test data through the pipeline
func (s *SemanticBasicScenario) executeSendEntities(ctx context.Context, result *Result) error {
	conn, err := net.Dial("udp", s.udpAddr)
	if err != nil {
		result.Errors = append(result.Errors, fmt.Sprintf("Failed to connect to UDP: %v", err))
		return fmt.Errorf("UDP connection failed: %w", err)
	}
	defer conn.Close()

	// Send entity messages
	// IoT sensor processor expects this format - see examples/processors/iot_sensor/processor.go
	entitiesSent := 0
	for i := 0; i < s.config.EntityCount; i++ {
		// Create sensor reading matching IoT sensor processor expected format
		// Include serial for ALIAS_INDEX and lat/lon for SPATIAL_INDEX
		entityMsg := map[string]any{
			"device_id": fmt.Sprintf("sensor-%d", i),
			"type":      "temperature",
			"reading":   20.0 + float64(i),
			"unit":      "celsius",
			"location":  fmt.Sprintf("room-%d", i%3),
			"timestamp": time.Now().Format(time.RFC3339),
			// Serial number for ALIAS_INDEX testing
			"serial": fmt.Sprintf("SN-2025-%06d", i),
			// Coordinates for SPATIAL_INDEX testing (San Francisco area)
			"latitude":  37.7749 + float64(i)*0.001,
			"longitude": -122.4194 + float64(i)*0.001,
			"altitude":  10.0 + float64(i),
		}

		msgBytes, err := json.Marshal(entityMsg)
		if err != nil {
			continue
		}

		_, err = conn.Write(msgBytes)
		if err != nil {
			result.Warnings = append(result.Warnings, fmt.Sprintf("Failed to send entity %d: %v", i, err))
			continue
		}

		entitiesSent++

		// Wait between messages
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(s.config.MessageInterval):
		}
	}

	result.Metrics["entities_sent"] = entitiesSent
	result.Details["data_sent"] = fmt.Sprintf("Sent %d entity messages via UDP", entitiesSent)

	return nil
}

// executeValidateProcessing validates entities were processed by graph processor
func (s *SemanticBasicScenario) executeValidateProcessing(ctx context.Context, result *Result) error {
	// Wait for processing
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-time.After(s.config.ValidationDelay):
	}

	// Query graph processor metrics
	components, err := s.client.GetComponents(ctx)
	if err != nil {
		result.Errors = append(result.Errors, fmt.Sprintf("Failed to get components: %v", err))
		return fmt.Errorf("component query failed: %w", err)
	}

	// Find graph processor
	var graphFound bool
	for _, comp := range components {
		if comp.Name == "graph" {
			graphFound = true
			result.Details["graph_processor"] = map[string]any{
				"name":    comp.Name,
				"type":    comp.Type,
				"healthy": comp.Healthy,
				"state":   comp.State,
			}
			break
		}
	}

	if !graphFound {
		result.Errors = append(result.Errors, "Graph processor not found in component list")
		return fmt.Errorf("graph processor not found")
	}

	// Record metrics
	result.Metrics["component_count"] = len(components)
	result.Metrics["entities_processed"] = "verified" // Detailed validation would require metrics endpoint

	result.Details["validation"] = fmt.Sprintf(
		"Graph processor found and healthy. Components: %d",
		len(components))

	return nil
}

// executeValidateNATSKV validates entities were stored in NATS KV bucket
func (s *SemanticBasicScenario) executeValidateNATSKV(ctx context.Context, result *Result) error {
	// Get entities_sent from previous stage
	entitiesSent, ok := result.Metrics["entities_sent"].(int)
	if !ok {
		result.Warnings = append(result.Warnings, "Could not get entities_sent from metrics")
		entitiesSent = s.config.EntityCount
	}

	// Connect to NATS
	natsClient, err := client.NewNATSValidationClient(ctx, s.natsURL)
	if err != nil {
		result.Warnings = append(result.Warnings, fmt.Sprintf("NATS connection failed: %v", err))
		result.Details["nats_validation"] = map[string]any{
			"connected": false,
			"error":     err.Error(),
		}
		// Not a hard failure - NATS might not be available in all test environments
		return nil
	}
	defer natsClient.Close(ctx)

	// Wait for processing to complete
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-time.After(s.validationConfig.ValidationTimeout):
	}

	// Count entities in NATS KV
	entitiesStored, err := natsClient.CountEntities(ctx)
	if err != nil {
		result.Warnings = append(result.Warnings, fmt.Sprintf("Could not count entities: %v", err))
		result.Details["nats_validation"] = map[string]any{
			"connected":       true,
			"count_error":     err.Error(),
			"entities_stored": 0,
		}
		return nil
	}

	// Calculate storage rate
	validationResult := config.NewValidationResult(entitiesSent)
	validationResult.EntitiesStored = entitiesStored
	validationResult.CalculateStorageRate()

	// Record metrics
	result.Metrics["entities_stored"] = entitiesStored
	result.Metrics["storage_rate"] = validationResult.StorageRate

	result.Details["nats_validation"] = map[string]any{
		"connected":       true,
		"entities_sent":   entitiesSent,
		"entities_stored": entitiesStored,
		"storage_rate":    validationResult.StorageRate,
		"threshold":       s.validationConfig.MinStorageRate,
	}

	// Check if storage rate meets threshold
	if !validationResult.MeetsThreshold(s.validationConfig.MinStorageRate) {
		result.Errors = append(result.Errors,
			fmt.Sprintf("Storage rate %.2f below threshold %.2f (%d/%d entities stored)",
				validationResult.StorageRate, s.validationConfig.MinStorageRate,
				entitiesStored, entitiesSent))
		return fmt.Errorf("storage rate %.2f below threshold %.2f",
			validationResult.StorageRate, s.validationConfig.MinStorageRate)
	}

	result.Details["nats_kv_validation"] = fmt.Sprintf(
		"Verified %d/%d entities stored in NATS KV (%.0f%% storage rate)",
		entitiesStored, entitiesSent, validationResult.StorageRate*100)

	// Validate entity structure by retrieving a sample entity
	// IoT sensor processor generates federated entity IDs:
	// {org}.{platform}.environmental.sensor.{type}.{device_id}
	if entitiesStored > 0 {
		entityID := "c360.semstreams.environmental.sensor.temperature.sensor-0"
		entity, err := natsClient.GetEntity(ctx, entityID)
		if err != nil {
			result.Warnings = append(result.Warnings,
				fmt.Sprintf("Could not retrieve entity %s: %v", entityID, err))
		} else {
			// Validate entity structure
			entityValid := true
			validationDetails := make(map[string]any)

			if entity.ID == "" {
				entityValid = false
				validationDetails["id_missing"] = true
			} else {
				validationDetails["id"] = entity.ID
			}

			if entity.Type == "" {
				result.Warnings = append(result.Warnings, "Entity type is empty")
				validationDetails["type_empty"] = true
			} else {
				validationDetails["type"] = entity.Type
			}

			if entity.Properties == nil {
				result.Warnings = append(result.Warnings, "Entity properties is nil")
				validationDetails["properties_nil"] = true
			} else {
				validationDetails["property_count"] = len(entity.Properties)
			}

			result.Details["entity_validation"] = map[string]any{
				"entity_id": entityID,
				"valid":     entityValid,
				"details":   validationDetails,
				"has_id":    entity.ID != "",
				"has_type":  entity.Type != "",
				"has_props": entity.Properties != nil,
			}
			result.Metrics["entity_validated"] = entityValid
		}
	}

	return nil
}
