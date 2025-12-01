package component

import (
	"encoding/json"
	"testing"
)

// TestListFactories_PreservesSchemaAndName verifies that ListFactories copies
// the Name and Schema fields. This is critical for schema generation tooling.
// Regression test for bug where ListFactories omitted these fields.
func TestListFactories_PreservesSchemaAndName(t *testing.T) {
	registry := NewRegistry()

	// Create a schema with properties
	schema := ConfigSchema{
		Properties: map[string]PropertySchema{
			"port": {
				Type:        "int",
				Description: "Listen port",
			},
			"host": {
				Type:        "string",
				Description: "Hostname",
			},
		},
		Required: []string{"port"},
	}

	// Mock factory function
	mockFactory := func(_ json.RawMessage, _ Dependencies) (Discoverable, error) {
		return nil, nil
	}

	reg := &Registration{
		Name:        "test-component",
		Factory:     mockFactory,
		Type:        "input",
		Protocol:    "tcp",
		Domain:      "network",
		Description: "Test component",
		Version:     "1.0.0",
		Schema:      schema,
	}

	err := registry.RegisterFactory("test-component", reg)
	if err != nil {
		t.Fatalf("Failed to register: %v", err)
	}

	// Retrieve via ListFactories
	factories := registry.ListFactories()
	retrieved := factories["test-component"]
	if retrieved == nil {
		t.Fatal("Factory not found")
	}

	// Verify Name is preserved
	if retrieved.Name != "test-component" {
		t.Errorf("Name not preserved: got %q, want %q", retrieved.Name, "test-component")
	}

	// Verify Schema.Properties is preserved (critical for schema generation)
	if retrieved.Schema.Properties == nil {
		t.Fatal("Schema.Properties is nil - ListFactories must copy Schema field")
	}

	if len(retrieved.Schema.Properties) != 2 {
		t.Errorf("Schema.Properties length: got %d, want 2", len(retrieved.Schema.Properties))
	}

	// Verify specific properties exist
	if _, ok := retrieved.Schema.Properties["port"]; !ok {
		t.Error("Schema.Properties missing 'port' field")
	}
	if _, ok := retrieved.Schema.Properties["host"]; !ok {
		t.Error("Schema.Properties missing 'host' field")
	}

	// Verify property details
	portProp := retrieved.Schema.Properties["port"]
	if portProp.Type != "int" {
		t.Errorf("port.Type: got %q, want %q", portProp.Type, "int")
	}
	if portProp.Description != "Listen port" {
		t.Errorf("port.Description: got %q, want %q", portProp.Description, "Listen port")
	}

	// Verify Required is preserved
	if len(retrieved.Schema.Required) != 1 || retrieved.Schema.Required[0] != "port" {
		t.Errorf("Schema.Required: got %v, want [port]", retrieved.Schema.Required)
	}

	// Verify Factory is NOT copied (security measure)
	if retrieved.Factory != nil {
		t.Error("Factory should not be copied in ListFactories for safety")
	}
}
