# fixture-project

A minimal Go project used as the semsource E2E fixture.

Semsource ingests this directory and emits graph entities for each Go symbol,
the go.mod module, and this README. The resulting entity set is small and
deterministic, making it suitable for validating the full ingestion pipeline
during integration tests.

## Contents

- `src/` — Go source files (AST entities)
- `go.mod` — module definition (config entity)
- `Dockerfile` — container definition (config entity)
