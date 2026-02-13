package agentic

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/c360studio/semstreams/test/e2e/client"
	"github.com/c360studio/semstreams/test/e2e/scenarios"
)

// TrustGraphConfig holds configuration for TrustGraph E2E tests.
type TrustGraphConfig struct {
	// Enabled controls whether TrustGraph tests run.
	Enabled bool `json:"enabled"`

	// MockServerURL is the URL for the TrustGraph mock server.
	MockServerURL string `json:"mock_server_url"`

	// KGCoreID is the knowledge core ID used by the output component.
	KGCoreID string `json:"kg_core_id"`

	// Collection is the collection name used by the output component.
	Collection string `json:"collection"`

	// ImportTimeout is how long to wait for import polling.
	ImportTimeout time.Duration `json:"import_timeout"`

	// ExportTimeout is how long to wait for exports to arrive.
	ExportTimeout time.Duration `json:"export_timeout"`
}

// DefaultTrustGraphConfig returns default TrustGraph test configuration.
func DefaultTrustGraphConfig() *TrustGraphConfig {
	return &TrustGraphConfig{
		Enabled:       true,
		MockServerURL: "http://localhost:38182",
		KGCoreID:      "semstreams-e2e",
		Collection:    "operational",
		ImportTimeout: 10 * time.Second,
		ExportTimeout: 15 * time.Second,
	}
}

// testExportEntityID is the ID of the entity we inject to test export.
const testExportEntityID = "c360.test.trustgraph.export.sensor.export-001"

// TestExportEntity represents an entity to export for testing.
type TestExportEntity struct {
	ID         string          `json:"id"`
	Type       string          `json:"type"`
	Source     string          `json:"source"`
	Properties map[string]any  `json:"properties"`
	Triples    []client.Triple `json:"triples"`
	Version    int             `json:"version"`
	UpdatedAt  string          `json:"updated_at"`
}

// createTestExportEntity creates an entity for export testing.
// It has a non-trustgraph source so it should be exported.
func createTestExportEntity() *TestExportEntity {
	now := time.Now().UTC().Format(time.RFC3339)
	return &TestExportEntity{
		ID:     testExportEntityID,
		Type:   "sensor",
		Source: "e2e-test", // Not "trustgraph", so should be exported
		Properties: map[string]any{
			"name":        "Export Test Sensor",
			"description": "Sensor for TrustGraph export testing",
			"location":    "test-facility",
		},
		Triples: []client.Triple{
			{
				Subject:   testExportEntityID,
				Predicate: "http://www.w3.org/2000/01/rdf-schema#label",
				Object:    "Export Test Sensor",
			},
			{
				Subject:   testExportEntityID,
				Predicate: "http://www.w3.org/1999/02/22-rdf-syntax-ns#type",
				Object:    "http://semstreams.io/vocab/Sensor",
			},
			{
				Subject:   testExportEntityID,
				Predicate: "http://semstreams.io/vocab/location",
				Object:    "test-facility",
			},
		},
		Version:   1,
		UpdatedAt: now,
	}
}

// verifyTrustGraphImport tests that the trustgraph-input component polls and imports triples.
func (s *Scenario) verifyTrustGraphImport(ctx context.Context, result *scenarios.Result) error {
	tgConfig := s.getTrustGraphConfig()
	if !tgConfig.Enabled {
		result.Details["trustgraph_import_skipped"] = "TrustGraph tests disabled"
		return nil
	}

	// Create TrustGraph mock client
	tgClient := client.NewTrustGraphMockClient(tgConfig.MockServerURL)

	// Check if mock server is healthy
	if err := tgClient.Health(ctx); err != nil {
		result.Warnings = append(result.Warnings, fmt.Sprintf("TrustGraph mock server not reachable: %v", err))
		result.Details["trustgraph_import_skipped"] = "mock server not reachable"
		return nil
	}

	result.Details["trustgraph_mock_healthy"] = true

	// Wait for the input component to poll (2s interval in config)
	// We expect at least some triples to be queried
	stats, err := tgClient.WaitForQueried(ctx, 1, tgConfig.ImportTimeout)
	if err != nil {
		return fmt.Errorf("failed waiting for triples query: %w", err)
	}

	result.Details["trustgraph_triples_queried"] = stats.TriplesQueried
	result.Details["trustgraph_import_triples"] = stats.ImportTriples

	if stats.TriplesQueried == 0 {
		result.Warnings = append(result.Warnings,
			"No triples queried within timeout - trustgraph-input may not be configured or polling")
		result.Details["trustgraph_import_success"] = false
		return nil
	}

	result.Details["trustgraph_import_success"] = true

	// Verify entities were created in ENTITY_STATES
	// The mock seeds triples for threat-001 and sensor-zone7
	// The input component should convert these to entities
	entities, err := s.nats.GetAllEntityIDs(ctx)
	if err != nil {
		result.Warnings = append(result.Warnings, fmt.Sprintf("Failed to list entities: %v", err))
	} else {
		// Count entities that look like they came from TrustGraph
		trustgraphEntities := 0
		for _, entityID := range entities {
			// Entities imported from TrustGraph should have the configured org mapping
			// Based on the config, trustgraph.ai URIs map to intel.trustgraph.knowledge.entity.concept
			if strings.Contains(entityID, "intel") || strings.Contains(entityID, "trustgraph") {
				trustgraphEntities++
			}
		}
		result.Details["trustgraph_entities_created"] = trustgraphEntities
	}

	return nil
}

// injectExportEntity publishes a local entity for export testing.
func (s *Scenario) injectExportEntity(ctx context.Context, result *scenarios.Result) error {
	tgConfig := s.getTrustGraphConfig()
	if !tgConfig.Enabled {
		return nil
	}

	// Check mock server health first
	tgClient := client.NewTrustGraphMockClient(tgConfig.MockServerURL)
	if err := tgClient.Health(ctx); err != nil {
		result.Details["trustgraph_export_skipped"] = "mock server not reachable"
		return nil
	}

	// Create test entity for export
	testEntity := createTestExportEntity()
	entityData, err := json.Marshal(testEntity)
	if err != nil {
		return fmt.Errorf("failed to marshal export entity: %w", err)
	}

	// Store entity in ENTITY_STATES bucket
	// The trustgraph-output component should pick this up and export it
	if err := s.nats.PutKV(ctx, client.BucketEntityStates, testExportEntityID, entityData); err != nil {
		return fmt.Errorf("failed to store export entity: %w", err)
	}

	result.Details["trustgraph_export_entity_id"] = testExportEntityID
	result.Details["trustgraph_export_entity_source"] = testEntity.Source

	// Also publish to the entity subject to trigger the output component
	if err := s.nats.Publish(ctx, "entity.sensor."+testExportEntityID, entityData); err != nil {
		result.Warnings = append(result.Warnings, fmt.Sprintf("Failed to publish entity: %v", err))
	}

	return nil
}

// verifyTrustGraphExport tests that the trustgraph-output component exports entities.
func (s *Scenario) verifyTrustGraphExport(ctx context.Context, result *scenarios.Result) error {
	tgConfig := s.getTrustGraphConfig()
	if !tgConfig.Enabled {
		result.Details["trustgraph_export_skipped"] = "TrustGraph tests disabled"
		return nil
	}

	// Create TrustGraph mock client
	tgClient := client.NewTrustGraphMockClient(tgConfig.MockServerURL)

	// Check if mock server is healthy
	if err := tgClient.Health(ctx); err != nil {
		result.Details["trustgraph_export_skipped"] = "mock server not reachable"
		return nil
	}

	// Wait for triples to be stored
	stored, err := tgClient.WaitForStored(ctx, tgConfig.KGCoreID, tgConfig.Collection, 1, tgConfig.ExportTimeout)
	if err != nil {
		return fmt.Errorf("failed waiting for stored triples: %w", err)
	}

	result.Details["trustgraph_triples_stored"] = stored.Count
	result.Details["trustgraph_export_core"] = stored.Core
	result.Details["trustgraph_export_collection"] = stored.Collection

	if stored.Count == 0 {
		result.Warnings = append(result.Warnings,
			"No triples stored within timeout - trustgraph-output may not be configured or processing")
		result.Details["trustgraph_export_success"] = false
		return nil
	}

	result.Details["trustgraph_export_success"] = true

	return nil
}

// verifyLoopPrevention ensures imported entities are not re-exported.
func (s *Scenario) verifyLoopPrevention(ctx context.Context, result *scenarios.Result) error {
	tgConfig := s.getTrustGraphConfig()
	if !tgConfig.Enabled {
		result.Details["trustgraph_loop_prevention_skipped"] = "TrustGraph tests disabled"
		return nil
	}

	// Create TrustGraph mock client
	tgClient := client.NewTrustGraphMockClient(tgConfig.MockServerURL)

	// Check if mock server is healthy
	if err := tgClient.Health(ctx); err != nil {
		result.Details["trustgraph_loop_prevention_skipped"] = "mock server not reachable"
		return nil
	}

	// Get stored triples
	stored, err := tgClient.GetStoredTriples(ctx, tgConfig.KGCoreID, tgConfig.Collection)
	if err != nil {
		return fmt.Errorf("failed to get stored triples: %w", err)
	}

	// Check if any stored triples contain TrustGraph URIs
	// If they do, it means imported entities were re-exported (loop!)
	containsTrustGraphURIs := client.ContainsTrustGraphURI(stored.Triples)

	result.Details["trustgraph_loop_prevention_stored_count"] = stored.Count
	result.Details["trustgraph_loop_prevention_has_trustgraph_uris"] = containsTrustGraphURIs

	if containsTrustGraphURIs {
		result.Warnings = append(result.Warnings,
			"Stored triples contain TrustGraph URIs - loop prevention may not be working correctly")
		result.Details["trustgraph_loop_prevention_success"] = false
	} else {
		result.Details["trustgraph_loop_prevention_success"] = true
	}

	return nil
}

// cleanupTrustGraph cleans up test resources created by TrustGraph tests.
func (s *Scenario) cleanupTrustGraph(ctx context.Context) error {
	// Delete test export entity
	_ = s.nats.DeleteKV(ctx, client.BucketEntityStates, testExportEntityID)
	return nil
}

// getTrustGraphConfig returns the TrustGraph test configuration.
func (s *Scenario) getTrustGraphConfig() *TrustGraphConfig {
	if s.trustgraphConfig == nil {
		return DefaultTrustGraphConfig()
	}
	return s.trustgraphConfig
}
