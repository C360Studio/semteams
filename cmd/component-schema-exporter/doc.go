// Package main exports JSON Schema documents from SemStreams component registrations.
//
// This tool extracts configuration metadata from component struct tags and generates
// JSON Schema files for UI form generation, configuration validation, and API documentation.
//
// Output:
//   - schemas/*.v1.json - JSON Schema files per component type
//   - specs/openapi.v3.yaml - OpenAPI 3.0 specification
//
// Usage:
//
//	task schema:generate
//
// Or directly:
//
//	go run ./cmd/component-schema-exporter -out ./schemas -openapi ./specs/openapi.v3.yaml
//
// Note: This exports Component Schemas (configuration metadata), not GraphQL Schemas.
// For GraphQL schema code generation, see cmd/domain-graphql-generator.
package main
