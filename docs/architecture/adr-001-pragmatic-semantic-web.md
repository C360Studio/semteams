# ADR-001: Pragmatic Semantic Web Approach

## Status

Accepted

## Context

SemStreams processes semantic data using triples (subject-predicate-object) and maintains relationship indexes. As the system evolved, we needed to decide how closely to align with RDF/SKOS/OWL standards versus maintaining a simpler, domain-specific approach.

Key considerations:
- RDF provides rich semantics but adds complexity (URIs, blank nodes, reification)
- NATS JetStream KV uses dotted notation for wildcard pattern matching
- Our 6-part entity IDs enable deterministic naming and efficient querying
- Interoperability with RDF tooling is desirable but not critical

## Decision

We adopt a **"Pragmatic Semantic Web"** approach: maintain clean internal dotted notation while providing optional IRI mappings for standards compliance at system boundaries.

### Core Principles

1. **Dotted notation internally** - Predicates use `domain.category.name` format (e.g., `hierarchy.type.member`)
2. **6-part entity IDs** - All resources are named; no blank nodes
3. **Standard IRI mappings** - Document alignment with SKOS/OWL for export/interop
4. **Implicit reification** - Use container entities instead of verbose RDF reification

## Terminology Mapping

| SemStreams Term | RDF Equivalent | Status |
|-----------------|---------------|--------|
| `hierarchy.*.member` | `skos:broader` | Aligned |
| Container entity | `skos:Collection` | Partial (custom triple marker) |
| Entity ID (6-part) | Named resource | Aligned (no blank nodes) |
| `AliasTypeIdentity` | `owl:sameAs` | Aligned |
| `AliasTypeLabel` | `skos:prefLabel` | Aligned |
| `entity.type.class` | `rdf:type` | Reimplemented (custom predicate) |
| Predicate registry | RDFS/OWL ontology | Partial (dotted notation) |

## Consequences

### Positive

- **Simpler implementation** - No URI parsing, namespace management, or blank node handling
- **NATS-native** - Dotted notation enables KV wildcard watches (`hierarchy.>`)
- **Queryable IDs** - 6-part entity IDs support pattern matching
- **Clear semantics** - Domain-specific terminology is self-documenting

### Negative

- **Limited RDF interop** - Direct import/export requires translation layer
- **No automatic inverse** - `skos:narrower` not materialized from `skos:broader`
- **Custom tooling** - Cannot use standard SPARQL engines directly

### Neutral

- **IRI mappings are documentation** - `StandardIRI` field exists but isn't used at runtime
- **Container marking** - Uses `entity.type.class: hierarchy.container` instead of `rdf:type skos:Collection`

## Known Gaps

### 1. IncomingIndex Asymmetry

**Problem**: OutgoingIndex stores `[{Predicate, ToEntityID}]` but IncomingIndex stores only `[entityID]`, losing predicate information.

**Impact**: Cannot filter incoming relationships by predicate type.

**Planned Fix**: Add `IncomingEntry` struct mirroring `OutgoingEntry`:

```go
type IncomingEntry struct {
    Predicate    string `json:"predicate"`
    FromEntityID string `json:"from_entity_id"`
}
```

### 2. No Inverse Predicate Registry

**Problem**: `PredicateMetadata` lacks `InverseOf` and `IsSymmetric` fields.

**Impact**: 
- Cannot declare `author` ↔ `authored_by` inverse pairs
- Symmetric relationships like `sibling` must be created bidirectionally by applications

**Planned Enhancement**:

```go
type PredicateMetadata struct {
    // ...existing fields...
    InverseOf   string  // e.g., "hierarchy.type.contains"
    IsSymmetric bool    // e.g., true for sibling relationships
}
```

### 3. Context Field vs Named Graphs

**Current State**: `Triple.Context` stores correlation metadata (e.g., `"inference.hierarchy"`) but is not indexed or queryable.

**RDF Comparison**: Named graphs are first-class, indexed, and queryable via `FROM NAMED`.

**Planned Enhancement**: Add `CONTEXT_INDEX` bucket for provenance queries.

**Data Model**:
- Key: Context value (e.g., `inference.hierarchy`)
- Value: `[{entity_id, predicate}, ...]` (matches OutgoingIndex pattern)

```go
type ContextEntry struct {
    EntityID  string `json:"entity_id"`
    Predicate string `json:"predicate"`
}
```

**Use Cases**:
- Query "all triples from hierarchy inference" (audit)
- Delete/rollback batch operations
- Compare inference sources (quality analysis)
- Debug GraphRAG community outputs

**Files to Modify**:
- `processor/graph/indexmanager/indexes.go` - Add `ContextIndex` type
- `processor/graph/indexmanager/manager.go` - Register index
- `processor/graph/querymanager/manager.go` - Add query methods
- `natsclient/buckets.go` - Add bucket creation

**Effort**: ~3-4 days

## Implementation Notes

### Key Files

| File | Purpose |
|------|---------|
| `vocabulary/doc.go` | Documents pragmatic approach |
| `vocabulary/predicates.go` | Predicate definitions with IRI mappings |
| `vocabulary/registry.go` | Alias type registration |
| `vocabulary/standards.go` | Standard IRI constants |
| `processor/graph/indexmanager/indexes.go` | Index implementations |
| `processor/graph/inference/hierarchy.go` | Container creation |

### Migration Path

If we decide to implement the IncomingIndex fix:

1. Add `IncomingEntry` type to `indexes.go`
2. Update `IncomingIndex` methods to use new structure
3. Provide migration utility to reindex existing data
4. Update `extractRelationshipsFromTriples` to preserve predicate

## References

- [SKOS Simple Knowledge Organization System](https://www.w3.org/2004/02/skos/)
- [OWL Web Ontology Language](https://www.w3.org/OWL/)
- [RDF 1.1 Concepts](https://www.w3.org/TR/rdf11-concepts/)
- Internal: `vocabulary/doc.go` for detailed philosophy
