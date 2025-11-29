# ADR: Package Responsibilities Consolidation

**Status:** Approved (with revisions)
**Author:** Architecture Review
**Reviewer:** Architect Agent
**Date:** 2025-11-29
**Related:** ADR-GRAPH-PACKAGE-CONSOLIDATION.md, 003-triples-architecture

> **Note:** This is a greenfield project. No backward compatibility or deprecation
> periods required. Delete legacy code immediately upon migration.

---

## Context

Following the initial implementation of 003-triples-architecture, a comprehensive review revealed significant technical debt and confusion about package responsibilities across `message/`, `graph/`, and `types/graph/`. This ADR extends ADR-GRAPH-PACKAGE-CONSOLIDATION.md to address the broader architectural issues.

### Problem Summary

The codebase has accumulated six critical issues:

1. **Type Duplication**: `/types/graph/` is a stale duplicate of `/graph/` with 9 importers
2. **Entity Representation Fragmentation**: Three separate representations of "entity" without clear ownership
3. **Federation Format Mismatch**: Two incompatible global ID formats
4. **Java-Style Anti-Patterns**: `pkg/graphinterfaces` exists solely to break import cycles
5. **Misplaced Package**: `pkg/graphclustering` has exactly ONE external consumer (a test file)
6. **Fragmented Graph Packages**: 9 graph-related directories with unclear boundaries

---

## The Graph Package Landscape

### Current State: 9 Directories, Maximum Confusion

```
LOCATION                          PURPOSE                           CONSUMERS
─────────────────────────────────────────────────────────────────────────────────
/graph/                           Core types (EntityState, etc)     48 files
/graph/query/                     External query client             3 files
/types/graph/                     STALE DUPLICATE                   9 files (DEPRECATED)
/processor/graph/                 GraphProcessor runtime            Internal
/processor/graph/indexmanager/    Index operations                  Internal
/processor/graph/querymanager/    Query + caching service           Internal + graphclustering
/processor/graph/datamanager/     Data operations                   Internal
/processor/graph/messagemanager/  Message handling                  Internal
/pkg/graphclustering/             Community detection (LPA)         1 TEST FILE ONLY
/pkg/graphinterfaces/             Interface to break cycles         8 files
/gateway/graphql/                 GraphQL API gateway               External
/component/flowgraph/             Flow graph execution              Unrelated
```

### Who Actually Uses graphclustering?

```bash
$ grep -r "pkg/graphclustering" --include="*.go" | grep -v "pkg/graphclustering/"

# Result: ONLY ONE FILE
processor/graph/graphrag_integration_test.go
```

**The entire `pkg/graphclustering` package (506 lines of README, 1000+ lines of code) has exactly ONE external consumer - and it's a test file.**

### Why graphclustering Is in pkg/ (And Why It Shouldn't Be)

The `pkg/` convention in Go means "reusable library code for external consumers." But:

1. **No external consumers exist** - only GraphProcessor tests use it
2. **Tightly coupled to processor/graph/querymanager** - requires `Querier` interface
3. **Uses internal types** - imports `graph.EntityState`
4. **Created pkg/graphinterfaces** to break cycles - a symptom of wrong location

### The Import Cycle That Created graphinterfaces

```
DESIRED:
  pkg/graphclustering → processor/graph/querymanager (for Querier)
  processor/graph/querymanager → pkg/graphclustering (for Community)

RESULT: Import cycle!

HACK SOLUTION:
  Created pkg/graphinterfaces with Community interface
  Now both packages import the interface package

PROPER SOLUTION:
  Move graphclustering INTO processor/graph/ where it belongs
```

---

## Issue 1: Message/Graph Boundary Confusion

### Current State: Three Entity Representations

```
Layer 1: INPUT (message package)
├─ EntityID: 6-part federated ID
│   Format: org.platform.domain.system.type.instance
│   Example: "c360.platform1.robotics.mav1.drone.0"
│
├─ EntityPayload: Generic entity message type
│   Fields: ID (string), Type (string), Properties (map)
│   Implements: Graphable interface
│
└─ Graphable interface: Self-declaring entities
    Methods: EntityID() string, Triples() []Triple

Layer 2: STORAGE (graph package)
├─ EntityState: Complete local graph state
│   Fields: Node (NodeProperties), Triples ([]message.Triple)
│
├─ NodeProperties: Query-essential properties
│   Fields: ID (string), Type (string), Position, Status
│   NOTE: ID here is LOCAL (e.g., "drone_001"), not federated
│
└─ Triple: RDF-like semantic statements
    Subject uses EntityID.Key() format

Layer 3: FEDERATION (split across both packages)
├─ message.FederationMeta: Message-level federation
│   Fields: UID (uuid.UUID), Platform (PlatformConfig)
│
└─ graph.FederatedEntity: Storage-level federation
    Fields: LocalID, GlobalID, PlatformID, Region, MessageUID
```

### The Confusion

| Concept | message/ | graph/ | Problem |
|---------|----------|--------|---------|
| Entity ID | `EntityID` (6-part) | `NodeProperties.ID` (local) | Different formats, unclear mapping |
| Entity payload | `EntityPayload` | `EntityState` | Both represent "an entity" |
| Properties | `EntityPayload.Properties` | `EntityState.Triples` | Different storage models |
| Federation | `FederationMeta` | `FederatedEntity` | Two systems, unclear conversion |

### Key Questions Without Clear Answers

1. **What is the difference between `EntityPayload.ID` and `NodeProperties.ID`?**
   - EntityPayload.ID: Full 6-part federated ID
   - NodeProperties.ID: Simple local ID like "drone_001"
   - No documentation explains this distinction

2. **Where should EntityID validation live?**
   - Currently only in `message/triple.go` (IsValidEntityID)
   - Graph package cannot validate entity IDs without importing message

3. **How do Triple.Subject and NodeProperties.ID relate?**
   - Triple.Subject uses EntityID.Key() format (6-part)
   - NodeProperties.ID uses local format
   - Code must handle both, but mapping is implicit

---

## Issue 2: Federation Format Mismatch

### Two Incompatible ID Formats

```go
// message/types.go - EntityID.Key()
// Format: org.platform.domain.system.type.instance
// Example: "c360.platform1.robotics.mav1.drone.0"

// graph/federation.go - BuildGlobalID()
// Format: platform_id:region:local_id
// Example: "us-west-prod:gulf_mexico:drone_1"
```

### Why This Matters

- GraphProcessor stores entities using one format
- RuleProcessor queries using another format
- Federation enrichment creates triples with mixed formats
- No clear specification of which format to use when

### Current Federation Flow

```
Message arrives
  ↓
GetPlatform(msg) → message.FederationMeta
  ↓
BuildFederatedEntity(localID, msg) → graph.FederatedEntity
  ↓
EnrichEntityState(state, fed) → Triples with federation properties
  ↓
Later: GetFederationInfo(state) reconstructs from triples
```

**Problems:**
- Information loss in write→read cycle
- Triple predicates use unstructured names ("local_id", "global_id")
- No vocabulary constants for federation predicates

---

## Issue 3: EntityPayload Overlap with Graph Package

### EntityPayload Purpose

From `message/entity_payload.go`:

```go
// EntityPayload represents a generic entity message that implements Graphable.
// This payload type enables conversion from GenericJSON to graph-processable entities.
```

### Overlap Analysis

| Feature | EntityPayload | EntityState |
|---------|---------------|-------------|
| Entity ID | `ID string` | `Node.ID string` |
| Entity Type | `Type string` | `Node.Type string` |
| Properties | `Properties map[string]any` | Derived from `Triples` |
| Position | Not included | `Node.Position *Position` |
| Status | Not included | `Node.Status EntityStatus` |
| Triples | Generated via `Triples()` method | Stored directly |
| Confidence | `Confidence float64` | Per-triple confidence |
| Source | `Source string` | Per-triple source |

### The Problem

EntityPayload is essentially a "message-side view" of what becomes EntityState. But:
- They use different property storage (map vs triples)
- They have different field sets
- Conversion logic is scattered across GraphProcessor
- No formal contract defines the transformation

---

## Decision

### Principle 1: Clear Package Ownership

```text
message/ owns: TRANSPORT PRIMITIVES ONLY
├─ EntityID: Canonical 6-part federated format (the global ID)
├─ Triple: RDF-like facts (semantic unit)
├─ FederationMeta: Federation metadata (part of identity)
└─ Payload: Base payload interface (Schema, Validate)

graph/ owns: GRAPH CONTRACTS & STORAGE TYPES
├─ Graphable: Interface for graph-compatible payloads (MOVED from message/)
├─ EntityState: Complete persisted entity representation
├─ NodeProperties: Query-optimized metadata subset
├─ EntityStatus: Operational state enumeration
├─ Query types: EntityCriteria, QueryDirection, etc.
└─ Helpers: Property access, triple filtering

DELETED:
└─ EntityPayload: Generic Graphable impl - UNUSED, DELETE
```

**Key insight**: Components wanting graph integration naturally look to `graph/` package.
The `Graphable` interface belongs where its consumers expect to find it.

### Principle 2: EntityID is the Single Source of Truth for Identity

- All entity identification uses `message.EntityID` 6-part format
- `NodeProperties.ID` stores the full EntityID.Key() string
- **Delete** `BuildGlobalID()` colon-delimited format (no deprecation - greenfield)
- Triple.Subject always uses EntityID.Key() format

### Principle 3: Graphable → EntityState is One-Way Transformation

```text
graph.Graphable (interface)  →  graph.EntityState (storage)
       ↓                              ↓
  Triples() []message.Triple    EntityState.Triples
       ↓                              ↓
  Runtime facts                  Persisted facts
```

- Domain payloads implement `graph.Graphable`
- `Graphable.Triples()` generates triples at runtime
- `EntityState` stores triples persistently
- No reverse transformation (storage → payload)

### Principle 4: Federation is Part of Identity (Lives in message/)

Federation is fundamentally about **identity** - which platform/org/region an entity belongs to.
This is encoded in the EntityID 6-part format itself:

```text
EntityID = org.platform.domain.system.type.instance
           ^^^^^^^^^^^^^^^^^^^
           Federation info is EMBEDDED in the ID
```

**Therefore:**

- `message.EntityID` already contains federation (org, platform, system)
- `message.FederationMeta` extracts platform context from messages
- **Delete** `graph.FederatedEntity` and `graph/federation.go` entirely
- Federation predicates in triples use vocabulary constants, not separate storage

**Rationale:** Having federation in both `message/` and `graph/` created confusion.
The EntityID IS the federated identifier. No separate "FederatedEntity" needed.

---

## Migration Plan

### Phase 1: Eliminate types/graph (Priority: HIGH)

**Per ADR-GRAPH-PACKAGE-CONSOLIDATION.md:**

Files to migrate (9 total):
```
processor/rule/entity_watcher.go
processor/rule/expression/evaluator.go
processor/rule/expression/evaluator_test.go
processor/rule/expression/types.go
processor/rule/kv_test_helpers.go
processor/rule/test_rule_factory.go
processor/rule/rule_integration_test.go
processor/graph/cleanup.go
processor/graph/cleanup_test.go
```

Change pattern:
```diff
-import gtypes "github.com/c360/semstreams/types/graph"
+import gtypes "github.com/c360/semstreams/graph"
```

**Effort:** 4-6 hours

### Phase 2: Document Entity Representation Boundaries (Priority: HIGH)

1. Add package-level documentation clarifying ownership
2. Document NodeProperties.ID vs EntityID.Key() relationship
3. Create examples showing message → storage transformation
4. Update all READMEs with clear guidance

**Effort:** 4-6 hours

### Phase 3: Move Graphable to graph/, Delete EntityPayload (Priority: HIGH)

Graphable is a graph integration contract - it belongs in `graph/` package.
EntityPayload has zero usage - delete it entirely.

1. **Move** `message/graphable.go` → `graph/graphable.go`
2. **Update import**: `graph.Graphable` (was `message.Graphable`)
3. **Update all implementers** (~15 files): change `message.Graphable` → `graph.Graphable`
4. **Delete** `message/entity_payload.go` entirely (zero usage outside its own file)
5. **Update** `message/doc.go` to remove EntityPayload references

**Effort:** 4-6 hours

### Phase 4: Delete Federation from graph/ (Priority: HIGH)

Federation belongs in `message/` as part of identity. Delete the redundant graph layer:

1. **Delete** `graph/federation.go` entirely (BuildGlobalID, FederatedEntity, etc.)
2. **Delete** `EnrichEntityState()` - federation info comes from EntityID
3. Add federation vocabulary constants to `vocabulary/` package
4. Update any code using `graph.FederatedEntity` to use `message.EntityID` fields

**Effort:** 4-6 hours

### Phase 5: Add Validation Helpers to Graph Package (Priority: MEDIUM)

```go
// graph/entity.go (new file)

// ValidateEntityID checks if string is valid 6-part EntityID format
func ValidateEntityID(s string) error

// ParseEntityID parses and validates a 6-part EntityID string
func ParseEntityID(s string) (message.EntityID, error)

// ExtractLocalID extracts the Instance part from an EntityID string
func ExtractLocalID(entityIDKey string) string
```

**Effort:** 2-3 hours

---

## Import Dependency Diagram

```text
┌─────────────────────────────────────────────────────────────────┐
│ message/ - TRANSPORT PRIMITIVES ONLY                             │
├─────────────────────────────────────────────────────────────────┤
│ types.go        │ EntityID (6-part federated), Type, EntityType │
│ triple.go       │ Triple, TripleGenerator, IsValidEntityID      │
│ federation.go   │ FederationMeta, GetPlatform, GetUID           │
│ payload.go      │ Payload interface (Schema, Validate)          │
│                 │                                               │
│ DELETED:        │ graphable.go (moved to graph/)                │
│                 │ entity_payload.go (unused, deleted)           │
└─────────────────────────────────────────────────────────────────┘
                              │
                              ↓ imports (for Triple type)
┌─────────────────────────────────────────────────────────────────┐
│ graph/ - GRAPH CONTRACTS & STORAGE TYPES                         │
├─────────────────────────────────────────────────────────────────┤
│ graphable.go    │ Graphable interface (MOVED HERE from message/)│
│ types.go        │ EntityState, NodeProperties, EntityStatus     │
│ helpers.go      │ GetPropertyValue, MergeTriples                │
│ query_types.go  │ QueryDirection, EntityCriteria                │
│ events.go       │ GraphEvent, EntityChanged                     │
│ mutation_*.go   │ Request/Response types                        │
│                 │                                               │
│ DELETED:        │ federation.go (federation in message/)        │
└─────────────────────────────────────────────────────────────────┘
                              │
              ┌───────────────┼───────────────┐
              ↓               ↓               ↓
       ┌────────────┐  ┌────────────┐  ┌────────────┐
       │graph/query │  │processor/  │  │processor/  │
       │            │  │graph/      │  │rule/       │
       │ External   │  │ Internal   │  │            │
       │ client API │  │ service    │  │ Rule eval  │
       └────────────┘  └────────────┘  └────────────┘
                              │
                    ┌─────────┴─────────┐
                    ↓                   ↓
             ┌────────────┐      ┌────────────┐
             │clustering/ │      │querymanager│
             │ (MOVED)    │ ←──→ │            │
             └────────────┘      └────────────┘
              No cycle! Both inside processor/graph/

DELETED:
├── types/graph/              (stale duplicate)
├── graph/federation.go       (federation in message/)
├── message/graphable.go      (moved to graph/)
├── message/entity_payload.go (unused - deleted entirely)
├── pkg/graphclustering/      (moved to processor/graph/clustering)
└── pkg/graphinterfaces/      (cycle-breaking hack no longer needed)
```

---

## Consequences

### Positive

- **Single source of truth** for entity identity (message.EntityID)
- **Clear ownership** boundaries between packages
- **Federation consolidated** in message/ as part of identity
- **Eliminated type drift** risk from duplicate packages
- **Removed Java anti-patterns** (getter interfaces)
- **3 packages deleted** (types/graph, pkg/graphinterfaces, pkg/graphclustering moved)

### Negative

- Migration effort required (~31-43 hours)
- Some existing code patterns will need updating

### Neutral

- Test patterns remain largely the same
- Performance characteristics unchanged

---

## Success Criteria

- [ ] All 9 files migrated from types/graph to graph
- [ ] types/graph directory **deleted**
- [ ] graph/federation.go **deleted** (federation lives in message/)
- [ ] pkg/graphinterfaces **deleted**
- [ ] pkg/graphclustering **moved** to processor/graph/clustering
- [ ] Community struct has **no getter methods** (direct field access)
- [ ] Package READMEs updated with ownership documentation
- [ ] NodeProperties.ID stores full EntityID.Key() format
- [ ] All tests passing
- [ ] `go build ./...` succeeds with no import cycles

---

## Issue 4: Java-Style Anti-Patterns in pkg/graphinterfaces

### The Problem

The `pkg/graphinterfaces` package exists solely to break import cycles by defining an interface that mirrors a concrete struct. This is a classic Java anti-pattern that Go idioms explicitly reject.

```go
// pkg/graphinterfaces/community.go
// 39 lines of getter methods on an interface

type Community interface {
    GetID() string
    GetLevel() int
    GetMembers() []string
    GetParentID() *string
    GetKeywords() []string
    GetRepEntities() []string
    GetStatisticalSummary() string
    GetLLMSummary() string
    GetSummaryStatus() string
    GetMetadata() map[string]interface{}
}
```

```go
// pkg/graphclustering/types.go
// Concrete struct with 10 getter methods to satisfy interface

type Community struct {
    ID                 string
    Level              int
    Members            []string
    // ... more fields
}

func (c *Community) GetID() string { return c.ID }
func (c *Community) GetLevel() int { return c.Level }
// ... 8 more getter methods
```

### Why This Is Wrong

1. **Go interfaces should be small and behavior-focused**
   - Idiomatic: `io.Reader`, `io.Writer`, `fmt.Stringer`
   - Anti-pattern: Interface that mirrors struct fields

2. **Getters are not idiomatic Go**
   - Java: `getID()`, `getLevel()`, `getMembers()`
   - Go: Export the field directly: `ID`, `Level`, `Members`

3. **Interface package to break cycles indicates design flaw**
   - The real problem is circular dependencies between packages
   - The solution should be restructuring, not adding indirection

4. **10 methods on an interface is a code smell**
   - Go proverb: "The bigger the interface, the weaker the abstraction"

### Current Import Cycle Being "Solved"

```
pkg/graphclustering → processor/graph/querymanager (for GraphProvider)
processor/graph/querymanager → pkg/graphinterfaces (for Community interface)
```

The cycle exists because:
- `graphclustering` needs to query the graph
- `querymanager` wants to return community data

### Proper Go Solution

**Option A: Accept the concrete type dependency**
- `querymanager` imports `graphclustering.Community` directly
- No interface needed for data transfer objects

**Option B: Define interface at point of use**
- `querymanager` defines its own small interface for what it needs
- `graphclustering.Community` implicitly satisfies it

**Option C: Extract shared types to a types package**
- Move `Community` struct to a lower-level package both can import
- No interface wrappers needed

### Affected Files (8 files import graphinterfaces)

```
processor/graph/graphrag_integration_test.go
processor/graph/querymanager/graphrag_search_test.go
processor/graph/querymanager/graphrag_search.go
processor/graph/querymanager/interface.go
gateway/graphql/graphql_test.go
gateway/graphql/base_resolver.go
pkg/graphinterfaces/community.go
pkg/graphclustering/types.go
```

---

## Issue 5: Multiple Interface Definitions in graphclustering

The `pkg/graphclustering` package defines too many interfaces internally:

```go
// types.go
type CommunityDetector interface { ... }  // 5 methods
type GraphProvider interface { ... }       // 3 methods
type CommunityStorage interface { ... }    // 5 methods

// lpa.go
type EntityProvider interface { ... }      // 1 method

// summarizer.go
type CommunitySummarizer interface { ... } // 1 method
```

### Issues

1. **Too many abstractions for internal use**
   - These interfaces are implementation details, not public contracts
   - Only one implementation of each exists

2. **GraphProvider duplicates querymanager.Direction**
   - Uses string "outgoing"/"incoming"/"both"
   - Should use `querymanager.Direction` type directly

3. **CommunityStorage is essentially a repository pattern**
   - Standard CRUD operations
   - Could use generic repository interface

---

## Issue 6: Interface Segregation Violations

### GraphProvider Interface

```go
type GraphProvider interface {
    GetAllEntityIDs(ctx context.Context) ([]string, error)
    GetNeighbors(ctx context.Context, entityID string, direction string) ([]string, error)
    GetEdgeWeight(ctx context.Context, fromID, toID string) (float64, error)
}
```

**Problem:** `QueryManagerGraphProvider.GetAllEntityIDs()` returns an error saying "not implemented - use PredicateGraphProvider instead"

This violates the Liskov Substitution Principle. If a method can't be implemented, the interface is wrong.

### Recommendation

Split into smaller interfaces:
```go
type NeighborProvider interface {
    GetNeighbors(ctx context.Context, entityID string, direction Direction) ([]string, error)
}

type EntityEnumerator interface {
    GetAllEntityIDs(ctx context.Context) ([]string, error)
}

type EdgeWeightProvider interface {
    GetEdgeWeight(ctx context.Context, fromID, toID string) (float64, error)
}
```

---

## Updated Migration Plan

### Phase 6: Move graphclustering to processor/graph/clustering (Priority: HIGH)

This MUST happen before Phase 7. Moving the package eliminates the import cycle.

1. **Move package**: `mv pkg/graphclustering processor/graph/clustering`
2. **Update all import paths**:
   - FROM: `github.com/c360/semstreams/pkg/graphclustering`
   - TO: `github.com/c360/semstreams/processor/graph/clustering`
3. **Verify no import cycles**: `go build ./...`
4. **Update package name** in all files: `package clustering`

**Effort:** 6-8 hours

### Phase 7: Delete graphinterfaces, Remove Getters (Priority: HIGH)

Now that graphclustering is inside processor/graph/, the cycle-breaking interface is unnecessary.

1. **Delete pkg/graphinterfaces** entirely
2. **Remove all getter methods** from Community struct (10 methods)
3. **Update all callers** to use direct field access:
   - `comm.GetID()` → `comm.ID`
   - `comm.GetMembers()` → `comm.Members`
4. **Update querymanager** to import `clustering.Community` directly

**Effort:** 4-6 hours

### Phase 8: Consolidate graphclustering Interfaces (Priority: LOW)

Clean up internal interfaces in the clustering package.

1. **Fix Liskov violation**: Split GraphProvider into smaller interfaces
2. **Use querymanager.Direction** instead of string
3. **Remove single-implementation interfaces** where not needed for testing

**Effort:** 4-6 hours

---

## Updated Success Criteria

- [ ] All 9 files migrated from types/graph to graph
- [ ] types/graph directory **deleted**
- [ ] message/graphable.go **moved** to graph/graphable.go
- [ ] message/entity_payload.go **deleted** (zero usage)
- [ ] graph/federation.go **deleted** (federation lives in message/)
- [ ] pkg/graphinterfaces **deleted**
- [ ] pkg/graphclustering **moved** to processor/graph/clustering
- [ ] Community struct has **no getter methods** (direct field access)
- [ ] All Graphable implementers updated to `graph.Graphable`
- [ ] Package READMEs updated with ownership documentation
- [ ] NodeProperties.ID stores full EntityID.Key() format
- [ ] All tests passing
- [ ] `go build ./...` succeeds with no import cycles

---

## Total Effort Estimate

| Phase | Description | Effort | Dependencies |
|-------|-------------|--------|--------------|
| 1 | Eliminate types/graph | 4-6 hours | None |
| 2 | Document entity boundaries | 6-8 hours | Phase 1 |
| 3 | Move Graphable to graph/, delete EntityPayload | 4-6 hours | Phase 1 |
| 4 | Delete federation from graph/ | 4-6 hours | Phase 2 |
| 5 | Add validation helpers | 2-3 hours | Phase 4 |
| 6 | Move graphclustering to processor/graph | 6-8 hours | Phase 1 |
| 7 | Delete graphinterfaces, remove getters | 4-6 hours | **Phase 6** |
| 8 | Consolidate graphclustering interfaces | 4-6 hours | Phase 7 |
| **Total** | | **35-49 hours** | |

> **Phase Order Note:** Phase 6 (move graphclustering) MUST complete before Phase 7
> (delete graphinterfaces) because the interface exists to break the import cycle.
> Once graphclustering is inside processor/graph/, the cycle no longer exists.

---

## Proposed Graph Package Reorganization

### Target State: Clear Boundaries

```text
AFTER CONSOLIDATION:

/message/                            TRANSPORT PRIMITIVES ONLY
├── types.go                         EntityID (6-part federated format)
├── federation.go                    FederationMeta (platform extraction)
├── triple.go                        Triple, TripleGenerator
├── payload.go                       Payload interface (Schema, Validate)
└── behaviors.go                     Behavioral interface docs
                                     (NO graphable.go - moved to graph/)
                                     (NO entity_payload.go - deleted!)

/graph/                              GRAPH CONTRACTS & STORAGE TYPES
├── graphable.go                     Graphable interface (MOVED HERE)
├── types.go                         EntityState, NodeProperties, EntityStatus
├── helpers.go                       Property access utilities
├── query_types.go                   QueryDirection, EntityCriteria
├── events.go                        GraphEvent types
└── mutation_*.go                    Request/Response types
                                     (NO federation.go - deleted!)

/graph/query/                        External read-only client
├── interface.go                     Client interface
└── client.go                        Implementation

/processor/graph/                    GraphProcessor and ALL internal services
├── processor.go                     Main GraphProcessor
├── mutations.go                     Mutation handling
├── indexmanager/                    Index operations (unchanged)
├── querymanager/                    Query + caching (unchanged)
├── datamanager/                     Data operations (unchanged)
├── messagemanager/                  Message handling (unchanged)
└── clustering/                      MOVED FROM pkg/graphclustering
    ├── types.go                     Community struct (NO GETTERS)
    ├── lpa.go                       Label Propagation Algorithm
    ├── storage.go                   NATS KV storage
    ├── summarizer.go                Statistical/LLM summarization
    └── pagerank.go                  PageRank for representative entities

/gateway/graphql/                    GraphQL API (unchanged)

DELETED:
├── /types/graph/                    Stale duplicate
├── /graph/federation.go             Federation is in message/ (identity)
├── /message/graphable.go            Moved to graph/
├── /message/entity_payload.go       Unused - deleted entirely
├── /pkg/graphclustering/            Moved to processor/graph/clustering
└── /pkg/graphinterfaces/            No longer needed (no cycle)
```

### Why This Works

1. **No import cycles**: `clustering` is inside `processor/graph/`, can import querymanager directly
2. **Clear ownership**: All graph processing in `processor/graph/`
3. **pkg/ is for truly reusable code**: buffer, cache, retry, worker (remain)
4. **graph/ is for types only**: No runtime logic, just data structures
5. **graph/query/ is for external consumers**: Read-only access without processor internals

### Migration Steps for graphclustering

```bash
# 1. Move package
mv pkg/graphclustering processor/graph/clustering

# 2. Update import paths in all files
# FROM: github.com/c360/semstreams/pkg/graphclustering
# TO:   github.com/c360/semstreams/processor/graph/clustering

# 3. Remove getter methods from Community struct
# Direct field access is Go idiomatic

# 4. Delete pkg/graphinterfaces (no longer needed)
rm -rf pkg/graphinterfaces

# 5. Update querymanager to use clustering.Community directly
# No interface needed - just import the struct
```

---

## References

- ADR-GRAPH-PACKAGE-CONSOLIDATION.md - Initial consolidation proposal
- 003-triples-architecture - Feature branch that exposed these issues
- message/doc.go - Current message package documentation
- graph/types.go - Current graph type definitions
- [Go Proverbs](https://go-proverbs.github.io/)
  - "The bigger the interface, the weaker the abstraction"
  - "Don't just check errors, handle them gracefully"

---

## Architect Review Notes

**Review Date:** 2025-11-29
**Verdict:** APPROVED WITH REVISIONS (incorporated below)

### Key Findings from Review

1. **Phase ordering corrected**: Phase 6 (move graphclustering) must precede Phase 7 (delete graphinterfaces)
2. **Federation boundary clarified**: Federation is part of identity, belongs in message/ package
3. **Effort estimates adjusted**: Documentation and large moves need more time than initially estimated
4. **Greenfield simplification**: No deprecation periods needed - delete legacy code immediately

### Additional Revision (v2.1)

5. **message/ is transport only**: Graphable moved to graph/ where consumers expect it
6. **EntityPayload deleted**: Zero usage outside its own file - dead code
7. **Clear package semantics**: message/ = transport primitives, graph/ = graph contracts + storage

### Validated Claims

| Claim | Status |
|-------|--------|
| graphclustering has 1 external consumer | ✅ Verified |
| types/graph has 9 importers | ✅ Verified |
| graphinterfaces has ~8 importers | ✅ ~7 found |
| GetAllEntityIDs throws "not implemented" | ✅ Verified at provider.go:31-36 |
| 10 getter methods on Community | ✅ Verified |
| EntityPayload has 0 external usage | ✅ Verified (only used in own file) |

### Risks Identified

- **Medium**: Moving 5000+ LOC (graphclustering) - mitigate with comprehensive testing
- **Medium**: Moving Graphable (~15 files to update) - mechanical but many files
- **Low**: All other phases are mechanical refactoring

---

**Version:** 2.1.0 | **Updated:** 2025-11-29 | **Status:** Approved
