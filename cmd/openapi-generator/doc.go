// Package main generates a complete OpenAPI 3.0 specification for SemStreams.
//
// This tool generates the OpenAPI spec from three sources:
//  1. Component configuration schemas from component.Registration
//  2. API response schemas generated via reflection from ResponseTypes
//  3. Service endpoint paths from the OpenAPI service registry
//
// Output:
//   - schemas/*.v1.json - JSON Schema files per component type
//   - specs/openapi.v3.yaml - Complete OpenAPI 3.0 specification
//
// Usage:
//
//	task schema:generate
//
// Or directly:
//
//	go run ./cmd/openapi-generator -out ./schemas -openapi ./specs/openapi.v3.yaml
package main
