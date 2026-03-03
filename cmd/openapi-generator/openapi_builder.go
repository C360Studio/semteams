package main

import (
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"

	"github.com/c360studio/semstreams/service"
	"gopkg.in/yaml.v3"
)

// generateOpenAPISpec generates an OpenAPI 3.0 specification from:
// 1. Component configuration schemas
// 2. Service endpoint paths from the registry
// 3. Response type schemas via reflection
func generateOpenAPISpec(components []ComponentSchema, serviceSpecs map[string]*service.OpenAPISpec, schemaDir string) OpenAPIDocument {
	// Build paths from service registry
	paths := buildPathsFromRegistry(serviceSpecs)

	// Build schemas from component configs and response types
	schemas := buildAllSchemas(components, serviceSpecs, schemaDir)

	// Collect tags from all services
	tags := buildTagsFromRegistry(serviceSpecs)

	return OpenAPIDocument{
		OpenAPI: "3.0.3",
		Info: InfoObject{
			Title:       "SemStreams API",
			Description: "HTTP API for component discovery, configuration, flow management, and runtime operations",
			Version:     "1.0.0",
		},
		Servers: []ServerObject{
			{URL: "http://localhost:8080", Description: "Development server"},
			{URL: "http://localhost", Description: "Production server (via reverse proxy)"},
		},
		Paths:      paths,
		Components: ComponentsObject{Schemas: schemas},
		Tags:       tags,
	}
}

// buildPathsFromRegistry creates OpenAPI paths from the service registry
func buildPathsFromRegistry(specs map[string]*service.OpenAPISpec) map[string]PathItem {
	paths := make(map[string]PathItem)

	// Get sorted spec names for deterministic output
	var names []string
	for name := range specs {
		names = append(names, name)
	}
	sort.Strings(names)

	for _, name := range names {
		spec := specs[name]
		for path, pathSpec := range spec.Paths {
			pathItem := convertPathSpec(pathSpec)
			paths[path] = pathItem
		}
	}

	return paths
}

// convertPathSpec converts service.PathSpec to local PathItem
func convertPathSpec(ps service.PathSpec) PathItem {
	item := PathItem{}

	if ps.GET != nil {
		item.Get = convertOperation(ps.GET)
	}
	if ps.POST != nil {
		item.Post = convertOperation(ps.POST)
	}
	if ps.PUT != nil {
		item.Put = convertOperation(ps.PUT)
	}
	if ps.PATCH != nil {
		item.Patch = convertOperation(ps.PATCH)
	}
	if ps.DELETE != nil {
		item.Delete = convertOperation(ps.DELETE)
	}

	return item
}

// convertOperation converts service.OperationSpec to local Operation
func convertOperation(op *service.OperationSpec) *Operation {
	operation := &Operation{
		Summary:     op.Summary,
		Description: op.Description,
		Tags:        op.Tags,
		Responses:   make(map[string]Response),
	}

	// Convert parameters
	for _, p := range op.Parameters {
		operation.Parameters = append(operation.Parameters, Parameter{
			Name:        p.Name,
			In:          p.In,
			Required:    p.Required,
			Description: p.Description,
			Schema:      SchemaRef{Type: p.Schema.Type},
		})
	}

	// Convert request body
	if op.RequestBody != nil {
		contentType := op.RequestBody.ContentType
		if contentType == "" {
			contentType = "application/json"
		}
		operation.RequestBody = &RequestBodyObject{
			Description: op.RequestBody.Description,
			Required:    op.RequestBody.Required,
			Content: map[string]MediaType{
				contentType: {Schema: SchemaRef{Ref: op.RequestBody.SchemaRef}},
			},
		}
	}

	// Convert responses
	for code, resp := range op.Responses {
		response := Response{
			Description: resp.Description,
		}

		// If there's a schema reference, add content type
		if resp.SchemaRef != "" {
			contentType := resp.ContentType
			if contentType == "" {
				contentType = "application/json"
			}

			var schema SchemaRef
			if resp.IsArray {
				// Generate inline array with $ref items
				schema = SchemaRef{
					Type:  "array",
					Items: &SchemaRef{Ref: resp.SchemaRef},
				}
			} else {
				schema = SchemaRef{Ref: resp.SchemaRef}
			}

			response.Content = map[string]MediaType{
				contentType: {Schema: schema},
			}
		} else if resp.ContentType != "" && resp.ContentType != "text/event-stream" {
			// Non-SSE endpoint with content type but no schema
			response.Content = map[string]MediaType{
				resp.ContentType: {
					Schema: SchemaRef{Type: "object"},
				},
			}
		}

		operation.Responses[code] = response
	}

	return operation
}

// buildTagsFromRegistry collects all unique tags from service specs
func buildTagsFromRegistry(specs map[string]*service.OpenAPISpec) []TagObject {
	tagMap := make(map[string]TagObject)

	for _, spec := range specs {
		for _, tag := range spec.Tags {
			if _, exists := tagMap[tag.Name]; !exists {
				tagMap[tag.Name] = TagObject{
					Name:        tag.Name,
					Description: tag.Description,
				}
			}
		}
	}

	// Sort tags by name for deterministic output
	var tags []TagObject
	var names []string
	for name := range tagMap {
		names = append(names, name)
	}
	sort.Strings(names)

	for _, name := range names {
		tags = append(tags, tagMap[name])
	}

	return tags
}

// buildAllSchemas builds schemas from component configs and response types
func buildAllSchemas(components []ComponentSchema, serviceSpecs map[string]*service.OpenAPISpec, schemaDir string) map[string]any {
	schemas := make(map[string]any)

	// Add component configuration schemas
	addComponentSchemas(schemas, components, schemaDir)

	// Add response type schemas from reflection
	addResponseSchemas(schemas, serviceSpecs)

	return schemas
}

// addComponentSchemas adds component configuration schemas to the map
func addComponentSchemas(schemas map[string]any, components []ComponentSchema, schemaDir string) {
	// Build oneOf array with references to all component schemas
	var schemaRefs []SchemaRef
	for _, comp := range components {
		schemaRefs = append(schemaRefs, SchemaRef{
			Ref: fmt.Sprintf("../%s/%s", filepath.Base(schemaDir), comp.ID),
		})
	}

	schemas["ComponentType"] = map[string]any{
		"type": "object",
		"properties": map[string]any{
			"id":          map[string]string{"type": "string", "description": "Component ID"},
			"name":        map[string]string{"type": "string", "description": "Human-readable name"},
			"type":        map[string]string{"type": "string", "description": "Component type (input/processor/output/storage)"},
			"protocol":    map[string]string{"type": "string", "description": "Technical protocol (udp, tcp, etc.)"},
			"domain":      map[string]string{"type": "string", "description": "Business domain (robotics, semantic, etc.)"},
			"description": map[string]string{"type": "string", "description": "Component description"},
			"version":     map[string]string{"type": "string", "description": "Component version"},
			"category":    map[string]string{"type": "string", "description": "Component category"},
			"schema": map[string]any{
				"description": "Component configuration schema",
				"oneOf":       schemaRefs,
			},
		},
		"required": []string{"id", "name", "type"},
	}
}

// addResponseSchemas generates and adds response and request body type schemas via reflection
func addResponseSchemas(schemas map[string]any, serviceSpecs map[string]*service.OpenAPISpec) {
	seen := make(map[reflect.Type]bool)

	// Get sorted spec names for deterministic output
	var names []string
	for name := range serviceSpecs {
		names = append(names, name)
	}
	sort.Strings(names)

	for _, name := range names {
		spec := serviceSpecs[name]
		for _, t := range spec.ResponseTypes {
			if seen[t] {
				continue
			}
			seen[t] = true

			typeName := typeNameFromReflect(t)
			schemas[typeName] = schemaFromType(t)
		}
		for _, t := range spec.RequestBodyTypes {
			if seen[t] {
				continue
			}
			seen[t] = true

			typeName := typeNameFromReflect(t)
			schemas[typeName] = schemaFromType(t)
		}
	}
}

// writeYAMLFile writes a struct to a YAML file
func writeYAMLFile(filename string, data any) error {
	// Marshal to YAML with proper indentation
	yamlData, err := yaml.Marshal(data)
	if err != nil {
		return fmt.Errorf("failed to marshal YAML: %w", err)
	}

	// Add header comment
	header := []byte(strings.TrimSpace(`
# OpenAPI 3.0 Specification for SemStreams API
# Generated by openapi-generator tool
# DO NOT EDIT MANUALLY - This file is auto-generated from component and service registrations
`) + "\n\n")

	content := append(header, yamlData...)

	if err := os.WriteFile(filename, content, 0644); err != nil {
		return fmt.Errorf("failed to write file: %w", err)
	}

	return nil
}
