// Package main generates domain-specific GraphQL resolvers from .graphql schema files.
//
// This tool creates type-safe GraphQL APIs for production applications by generating
// Go resolver code that delegates to gateway/graphql.BaseResolver.
//
// Output:
//   - resolver.go - Query resolvers implementing gqlgen interface
//   - models.go - Entity-to-GraphQL type converters
//   - converters.go - Property extraction helpers
//
// Usage:
//
//	domain-graphql-generator -config=config.json -output=generated
//
// Note: This generates code from GraphQL Schemas (.graphql files), not Component Schemas.
// For component config schema export, see cmd/component-schema-exporter.
package main
