# Quickstart: Remove Legacy RDF Predicates

**Feature**: 001-predicate-notation
**Date**: 2025-11-26

## Validation Guide

After implementation, verify the feature works correctly:

### 1. Run Tests

```bash
go test -race ./message/... ./processor/json_to_entity/...
```

**Expected**: All tests pass with no race conditions.

### 2. Verify Triple Generation

```go
payload := &message.EntityPayload{
    ID:         "test.platform.domain.system.type.001",
    Type:       "sensors.temperature",
    Class:      message.EntityClassObject,
    Properties: map[string]any{"celsius": 23.5},
}

triples := payload.Triples()

// Verify only property triples exist
for _, triple := range triples {
    // Should see: sensors.temperature.celsius
    // Should NOT see: rdf:type or rdf:class
    fmt.Printf("Predicate: %s\n", triple.Predicate)
}
```

**Expected**: Only property predicates like `sensors.temperature.celsius`, no `rdf:type` or `rdf:class`.

### 3. Verify No Legacy Predicates Remain

```bash
grep -r "rdf:type\|rdf:class" --include="*.go" .
```

**Expected**: No Go files contain `rdf:type` or `rdf:class`.

### 4. Lint Check

```bash
go fmt ./...
revive ./...
```

**Expected**: No formatting changes, no linting errors.

## Success Criteria Checklist

- [ ] All tests pass with `-race` flag
- [ ] EntityPayload.Triples() returns only property triples
- [ ] No `rdf:type` or `rdf:class` in Go code
- [ ] Documentation examples use real predicates (e.g., `geo.location.latitude`)
- [ ] `go fmt` reports no changes
- [ ] `revive` reports no errors
