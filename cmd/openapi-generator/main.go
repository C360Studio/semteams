// Package main provides a command-line tool for generating OpenAPI specifications.
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"reflect"
	"sort"

	"github.com/c360studio/semstreams/component"
	"github.com/c360studio/semstreams/componentregistry"
	wfschema "github.com/c360studio/semstreams/processor/workflow/schema"
	"github.com/c360studio/semstreams/service"
)

func main() {
	// Parse command-line flags
	registryPkg := flag.String("registry", "./componentregistry", "Package containing RegisterAll()")
	outDir := flag.String("out", "./schemas", "Output directory for schemas")
	openapiOut := flag.String("openapi", "./specs/openapi.v3.yaml", "Output path for OpenAPI spec")
	flag.Parse()

	log.Printf("OpenAPI Generator")
	log.Printf("  Registry: %s", *registryPkg)
	log.Printf("  Output dir: %s", *outDir)
	log.Printf("  OpenAPI spec: %s", *openapiOut)

	// Initialize component registry
	registry := component.NewRegistry()

	// Register all components
	if err := componentregistry.Register(registry); err != nil {
		log.Fatalf("Failed to register components: %v", err)
	}

	// Get all registered factories
	factories := registry.ListFactories()
	log.Printf("Found %d component types", len(factories))

	// Load meta-schema for validation
	metaSchemaPath, err := loadMetaSchemaPath()
	if err != nil {
		log.Printf("⚠️  Meta-schema not found, skipping validation: %v", err)
		metaSchemaPath = ""
	} else {
		log.Printf("Using meta-schema: %s", metaSchemaPath)
	}

	// Create output directory
	if err := os.MkdirAll(*outDir, 0755); err != nil {
		log.Fatalf("Failed to create output directory: %v", err)
	}

	// Extract and write component configuration schemas
	var componentSchemas []ComponentSchema
	for name, registration := range factories {
		schema := extractSchema(name, registration)

		// Validate schema against meta-schema
		if metaSchemaPath != "" {
			if err := validateSchema(schema, metaSchemaPath); err != nil {
				log.Fatalf("Schema validation failed for %s: %v", name, err)
			}
		}

		componentSchemas = append(componentSchemas, schema)

		// Write to versioned JSON file
		outFile := filepath.Join(*outDir, fmt.Sprintf("%s.v1.json", name))
		if err := writeJSONSchema(outFile, schema); err != nil {
			log.Fatalf("Failed to write schema for %s: %v", name, err)
		}

		log.Printf("  ✓ Generated component schema: %s", outFile)
	}

	// Generate workflow definition schema from Go structs
	if err := generateWorkflowDefinitionSchema(*outDir); err != nil {
		log.Fatalf("Failed to generate workflow definition schema: %v", err)
	}
	log.Printf("  ✓ Generated workflow definition schema")

	// Get all registered service OpenAPI specs
	serviceSpecs := service.GetAllOpenAPISpecs()
	log.Printf("Found %d service OpenAPI specs", len(serviceSpecs))

	// Generate OpenAPI spec
	if *openapiOut != "" {
		openapiDir := filepath.Dir(*openapiOut)
		if err := os.MkdirAll(openapiDir, 0755); err != nil {
			log.Fatalf("Failed to create OpenAPI directory: %v", err)
		}

		// Sort component schemas by ID for deterministic output
		sort.Slice(componentSchemas, func(i, j int) bool {
			return componentSchemas[i].ID < componentSchemas[j].ID
		})

		openapi := generateOpenAPISpec(componentSchemas, serviceSpecs, *outDir)
		if err := writeYAMLFile(*openapiOut, openapi); err != nil {
			log.Fatalf("Failed to write OpenAPI spec: %v", err)
		}

		log.Printf("  ✓ Generated OpenAPI spec: %s", *openapiOut)
	}

	log.Printf("✅ OpenAPI generation complete!")
}

// ComponentSchema represents the exported component schema
type ComponentSchema struct {
	Schema      string                    `json:"$schema"`
	ID          string                    `json:"$id"`
	Type        string                    `json:"type"`
	Title       string                    `json:"title"`
	Description string                    `json:"description"`
	Properties  map[string]PropertySchema `json:"properties"`
	Required    []string                  `json:"required"`
	Metadata    ComponentMetadata         `json:"x-component-metadata"`
}

// ComponentMetadata holds component metadata for OpenAPI integration
type ComponentMetadata struct {
	Name     string `json:"name"`
	Type     string `json:"type"`     // "input", "processor", "output", "storage"
	Protocol string `json:"protocol"` // "udp", "tcp", "websocket", etc.
	Domain   string `json:"domain"`   // "robotics", "semantic", "network", "storage"
	Version  string `json:"version"`
}

// PropertySchema represents a JSON Schema property definition
type PropertySchema struct {
	Type        string                    `json:"type"`
	Description string                    `json:"description,omitempty"`
	Default     any                       `json:"default,omitempty"`
	Enum        []string                  `json:"enum,omitempty"`
	Minimum     *int                      `json:"minimum,omitempty"`
	Maximum     *int                      `json:"maximum,omitempty"`
	Items       *PropertySchema           `json:"items,omitempty"`      // For array types
	Category    string                    `json:"category,omitempty"`   // UI organization: "basic" or "advanced"
	Properties  map[string]PropertySchema `json:"properties,omitempty"` // Nested properties for object types
	Required    []string                  `json:"required,omitempty"`   // Required nested fields for object types
}

// extractSchema converts a component registration to a JSON Schema
func extractSchema(name string, registration *component.Registration) ComponentSchema {
	// Convert component.PropertySchema to JSON Schema PropertySchema
	properties := convertProperties(registration.Schema.Properties)

	// Ensure Required is an empty array instead of nil
	required := registration.Schema.Required
	if required == nil {
		required = []string{}
	}

	return ComponentSchema{
		Schema:      "http://json-schema.org/draft-07/schema#",
		ID:          fmt.Sprintf("%s.v1.json", name),
		Type:        "object",
		Title:       fmt.Sprintf("%s Configuration", name),
		Description: registration.Description,
		Properties:  properties,
		Required:    required,
		Metadata: ComponentMetadata{
			Name:     name,
			Type:     registration.Type,
			Protocol: registration.Protocol,
			Domain:   registration.Domain,
			Version:  registration.Version,
		},
	}
}

// convertProperties recursively converts component PropertySchema to JSON Schema PropertySchema
func convertProperties(props map[string]component.PropertySchema) map[string]PropertySchema {
	result := make(map[string]PropertySchema)
	for propName, propSchema := range props {
		jsonSchemaProp := PropertySchema{
			Type:        mapTypeToJSONSchema(propSchema.Type),
			Description: propSchema.Description,
			Default:     propSchema.Default,
			Enum:        propSchema.Enum,
			Minimum:     propSchema.Minimum,
			Maximum:     propSchema.Maximum,
			Category:    propSchema.Category,
		}

		// Handle array types
		if propSchema.Type == "array" {
			if propSchema.Items != nil {
				jsonSchemaProp.Items = convertPropertySchemaPtr(propSchema.Items)
			} else {
				jsonSchemaProp.Items = &PropertySchema{Type: "string"}
			}
		}

		// Handle nested object types - recursively convert properties
		if propSchema.Type == "object" && len(propSchema.Properties) > 0 {
			jsonSchemaProp.Properties = convertProperties(propSchema.Properties)
			if len(propSchema.Required) > 0 {
				jsonSchemaProp.Required = propSchema.Required
			}
		}

		result[propName] = jsonSchemaProp
	}
	return result
}

// convertPropertySchemaPtr converts a component.PropertySchema pointer to local PropertySchema
func convertPropertySchemaPtr(src *component.PropertySchema) *PropertySchema {
	if src == nil {
		return nil
	}
	result := &PropertySchema{
		Type:        mapTypeToJSONSchema(src.Type),
		Description: src.Description,
		Default:     src.Default,
		Enum:        src.Enum,
		Minimum:     src.Minimum,
		Maximum:     src.Maximum,
	}
	if len(src.Properties) > 0 {
		result.Properties = convertProperties(src.Properties)
	}
	if len(src.Required) > 0 {
		result.Required = src.Required
	}
	if src.Items != nil {
		result.Items = convertPropertySchemaPtr(src.Items)
	}
	return result
}

// mapTypeToJSONSchema maps component property types to JSON Schema types
func mapTypeToJSONSchema(propType string) string {
	switch propType {
	case "int", "float":
		return "number"
	case "bool":
		return "boolean"
	case "array":
		return "array"
	case "object":
		return "object"
	default:
		return "string"
	}
}

// writeJSONSchema writes a component schema to a JSON file
func writeJSONSchema(filename string, schema ComponentSchema) error {
	data, err := json.MarshalIndent(schema, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal schema: %w", err)
	}

	if err := os.WriteFile(filename, data, 0644); err != nil {
		return fmt.Errorf("failed to write file: %w", err)
	}

	return nil
}

// collectResponseTypes gathers all unique response types from service specs
func collectResponseTypes(specs map[string]*service.OpenAPISpec) []reflect.Type {
	seen := make(map[reflect.Type]bool)
	var types []reflect.Type

	for _, spec := range specs {
		for _, t := range spec.ResponseTypes {
			if !seen[t] {
				seen[t] = true
				types = append(types, t)
			}
		}
	}

	return types
}

// generateWorkflowDefinitionSchema generates JSON Schema for workflow definitions.
// This includes the Definition, StepDef, ActionDef, InputRef, and OutputDef types.
// Uses $defs and $ref to handle recursive types (StepDef contains Steps []StepDef).
func generateWorkflowDefinitionSchema(outDir string) error {
	// Build schema with definitions for all types to handle recursion
	defs := make(map[string]any)

	// Known types that should use $ref when encountered as fields
	knownTypeNames := map[string]string{
		"Definition":   "#/$defs/Definition",
		"TriggerDef":   "#/$defs/TriggerDef",
		"StepDef":      "#/$defs/StepDef",
		"ActionDef":    "#/$defs/ActionDef",
		"ConditionDef": "#/$defs/ConditionDef",
		"InputRef":     "#/$defs/InputRef",
		"OutputDef":    "#/$defs/OutputDef",
	}

	// Generate schemas for all workflow types
	types := []struct {
		name string
		typ  reflect.Type
	}{
		{"Definition", reflect.TypeOf(wfschema.Definition{})},
		{"TriggerDef", reflect.TypeOf(wfschema.TriggerDef{})},
		{"StepDef", reflect.TypeOf(wfschema.StepDef{})},
		{"ActionDef", reflect.TypeOf(wfschema.ActionDef{})},
		{"ConditionDef", reflect.TypeOf(wfschema.ConditionDef{})},
		{"InputRef", reflect.TypeOf(wfschema.InputRef{})},
		{"OutputDef", reflect.TypeOf(wfschema.OutputDef{})},
	}

	for _, t := range types {
		// Generate full struct schema directly (not via schemaFromTypeWithRefs)
		// to avoid returning $ref for top-level definitions
		defs[t.name] = schemaFromStructWithRefs(t.typ, knownTypeNames)
	}

	// Build full schema with $defs
	fullSchema := map[string]any{
		"$schema":     "http://json-schema.org/draft-07/schema#",
		"$id":         "workflow-definition.v1.json",
		"title":       "Workflow Definition",
		"description": "Schema for SemStreams workflow definitions (ADR-020)",
		"$ref":        "#/$defs/Definition",
		"$defs":       defs,
	}

	// Write to file
	outFile := filepath.Join(outDir, "workflow-definition.v1.json")
	data, err := json.MarshalIndent(fullSchema, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal workflow schema: %w", err)
	}

	if err := os.WriteFile(outFile, data, 0644); err != nil {
		return fmt.Errorf("failed to write workflow schema: %w", err)
	}

	return nil
}

// schemaFromTypeWithRefs generates JSON Schema but uses $ref for known workflow types
// to avoid infinite recursion with self-referencing types like StepDef.
func schemaFromTypeWithRefs(t reflect.Type, knownTypeNames map[string]string) map[string]any {
	// Handle pointers
	if t.Kind() == reflect.Pointer {
		schema := schemaFromTypeWithRefs(t.Elem(), knownTypeNames)
		schema["nullable"] = true
		return schema
	}

	// For struct types, check if we should use a $ref
	if t.Kind() == reflect.Struct {
		typeName := t.Name()
		// If this is a known type, return $ref instead of expanding
		if ref, ok := knownTypeNames[typeName]; ok {
			return map[string]any{"$ref": ref}
		}

		// Generate struct schema inline (not a known type)
		return schemaFromStructWithRefs(t, knownTypeNames)
	}

	// For slices, check element type
	if t.Kind() == reflect.Slice {
		elemSchema := schemaFromTypeWithRefs(t.Elem(), knownTypeNames)
		return map[string]any{
			"type":  "array",
			"items": elemSchema,
		}
	}

	// For maps, check value type
	if t.Kind() == reflect.Map {
		valueSchema := schemaFromTypeWithRefs(t.Elem(), knownTypeNames)
		return map[string]any{
			"type":                 "object",
			"additionalProperties": valueSchema,
		}
	}

	// Fall back to basic type handling
	return schemaFromType(t)
}

// schemaFromStructWithRefs generates a struct schema using $refs for known types.
func schemaFromStructWithRefs(t reflect.Type, knownTypeNames map[string]string) map[string]any {
	properties := make(map[string]any)
	required := []string{}

	for i := range t.NumField() {
		field := t.Field(i)

		if !field.IsExported() {
			continue
		}

		jsonTag := field.Tag.Get("json")
		if jsonTag == "-" {
			continue
		}

		name, opts := parseJSONTag(jsonTag)
		if name == "" {
			name = field.Name
		}

		fieldSchema := schemaFromTypeWithRefs(field.Type, knownTypeNames)

		if desc := field.Tag.Get("description"); desc != "" {
			fieldSchema["description"] = desc
		}

		properties[name] = fieldSchema

		if !contains(opts, "omitempty") && field.Type.Kind() != reflect.Pointer {
			required = append(required, name)
		}
	}

	schema := map[string]any{
		"type":       "object",
		"properties": properties,
	}

	if len(required) > 0 {
		schema["required"] = required
	}

	return schema
}

func contains(s, substr string) bool {
	return len(s) > 0 && len(substr) > 0 && (s == substr || len(s) > len(substr) && (s[:len(substr)] == substr || s[len(s)-len(substr):] == substr || containsMiddle(s, substr)))
}

func containsMiddle(s, substr string) bool {
	for i := 1; i < len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
