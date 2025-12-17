# component-schema-exporter

Exports JSON Schema documents describing SemStreams component configurations.

> **Note:** This exports **Component Schemas** (config metadata from struct tags).
> Unrelated to GraphQL Schemas used by `domain-graphql-generator`.

## What It Does

1. Reads component registrations from `componentregistry`
2. Extracts `schema:"..."` struct tags from config types
3. Outputs JSON Schema files to `schemas/*.json`
4. Generates OpenAPI 3.0 spec at `specs/openapi.v3.yaml`

## Usage

```bash
task schema:generate
```

Or directly:

```bash
go run ./cmd/component-schema-exporter \
  -out ./schemas \
  -openapi ./specs/openapi.v3.yaml
```

### Flags

| Flag | Default | Description |
|------|---------|-------------|
| `-registry` | `./componentregistry` | Package containing RegisterAll() |
| `-out` | `./schemas` | Output directory for schemas |
| `-openapi` | `./specs/openapi.v3.yaml` | Output path for OpenAPI spec |

## Output

- `schemas/*.v1.json` - One JSON Schema per component type
- `specs/openapi.v3.yaml` - OpenAPI spec for component discovery API

## CI Integration

The `schema-validation` CI job ensures schemas stay in sync:

1. Regenerates schemas from current code
2. Runs unit tests
3. Fails if output differs from committed files

This prevents schema drift—if you change a component's config struct, you must regenerate and commit the updated schemas.

## Related

- [Architecture](../../docs/basics/02-architecture.md) - Schema concepts explained
- [domain-graphql-generator](../domain-graphql-generator/README.md) - GraphQL codegen (different concept)
