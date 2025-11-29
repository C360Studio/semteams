# Feature Specification: Graph Package Consolidation

**Feature Branch**: `005-graph-package-consolidation`
**Created**: 2025-11-29
**Status**: Draft
**Input**: User description: "Graph Package Consolidation - review docs/ADR-PACKAGE-RESPONSIBILITIES-CONSOLIDATION.md and ensure that any changes caused by spec 004-semantic-refactor are accounted for during spec gen"

## Overview

This feature implements the package consolidation defined in ADR-PACKAGE-RESPONSIBILITIES-CONSOLIDATION.md, eliminating technical debt from duplicate packages, misplaced code, and Java-style anti-patterns. The 004-semantic-refactor has already completed some prerequisites (removing NodeProperties, deleting entity_payload.go), so this spec focuses on the remaining consolidation work.

### Current State (Post 004-semantic-refactor)

| Item | ADR Requirement | Current Status |
|------|-----------------|----------------|
| NodeProperties removed from EntityState | Required | Done (004-semantic-refactor) |
| message/entity_payload.go deleted | Required | Done (004-semantic-refactor) |
| message/entity_types.go deleted | Required | Done (004-semantic-refactor) |
| types/graph/ eliminated | Required | Pending (9 files still exist) |
| Graphable moved to graph/ | Required | Pending (still in message/) |
| graph/federation.go deleted | Required | Pending (file still exists) |
| pkg/graphclustering moved | Required | Pending (still in pkg/) |
| pkg/graphinterfaces deleted | Required | Pending (still exists) |
| pkg/embedding moved | Required | Pending (still in pkg/, 2 consumers in indexmanager) |

## User Scenarios & Testing

### User Story 1 - Eliminate Duplicate Type Packages (Priority: P1)

As a developer working on the semstreams codebase, I want a single authoritative package for graph types so that I don't have to decide between `graph/` and `types/graph/` when importing EntityState and related types.

**Why this priority**: The duplicate `types/graph/` package creates confusion and risks type drift. Every new developer must learn which package to use. Eliminating this removes a constant source of bugs and cognitive overhead.

**Independent Test**: Can be fully tested by verifying all imports use `graph/` package and `types/graph/` directory no longer exists.

**Acceptance Scenarios**:

1. **Given** a codebase with imports from `types/graph/`, **When** the migration is complete, **Then** all 9 files use `graph/` imports instead
2. **Given** the `types/graph/` directory exists, **When** consolidation is complete, **Then** the directory is deleted and `go build ./...` succeeds
3. **Given** a developer wants to use EntityState, **When** they look for the type, **Then** only one package (`graph/`) contains it

---

### User Story 2 - Move Graphable Interface to graph/ Package (Priority: P1)

As a developer implementing a domain payload, I want to find the Graphable interface in the `graph/` package where I expect graph-related contracts to live, rather than in `message/` which should only contain transport primitives.

**Why this priority**: Graphable is a graph integration contract - components wanting graph integration naturally look to the `graph/` package. Having it in `message/` violates the principle of least surprise.

**Independent Test**: Can be fully tested by verifying `graph/graphable.go` exists and all Graphable implementers import from `graph.Graphable`.

**Acceptance Scenarios**:

1. **Given** `message/graphable.go` exists, **When** migration is complete, **Then** file is moved to `graph/graphable.go`
2. **Given** 9 files reference `message.Graphable`, **When** migration is complete, **Then** all reference `graph.Graphable`
3. **Given** a developer implements a new Graphable payload, **When** they look for the interface, **Then** they find it in the `graph/` package

---

### User Story 3 - Delete Federation Redundancy from graph/ (Priority: P2)

As a developer working with entity identity, I want federation information to live in a single location (message/) so that I don't have two incompatible ID formats and unclear conversion paths.

**Why this priority**: Federation is fundamentally about identity, which is encoded in the EntityID 6-part format. Having separate `graph.FederatedEntity` and `message.FederationMeta` creates confusion about which to use when.

**Independent Test**: Can be fully tested by verifying `graph/federation.go` is deleted and any code needing federation uses `message.EntityID` fields directly.

**Acceptance Scenarios**:

1. **Given** `graph/federation.go` exists with BuildGlobalID and FederatedEntity, **When** consolidation is complete, **Then** the file is deleted
2. **Given** code uses `graph.FederatedEntity`, **When** migration is complete, **Then** code uses `message.EntityID` fields instead
3. **Given** federation vocabulary predicates are hardcoded strings, **When** migration is complete, **Then** vocabulary constants are used

---

### User Story 4 - Relocate graphclustering to processor/graph/ (Priority: P2)

As a developer maintaining the graph system, I want graphclustering inside `processor/graph/` where its only consumer lives, eliminating the import cycle that necessitated the `pkg/graphinterfaces` hack.

**Why this priority**: The `pkg/` convention means "reusable library code for external consumers," but graphclustering has exactly ONE external consumer - a test file. Moving it eliminates the import cycle naturally.

**Independent Test**: Can be fully tested by verifying `processor/graph/clustering/` exists and `pkg/graphclustering/` is deleted.

**Acceptance Scenarios**:

1. **Given** `pkg/graphclustering/` exists, **When** migration is complete, **Then** code is moved to `processor/graph/clustering/`
2. **Given** import paths reference `pkg/graphclustering`, **When** migration is complete, **Then** all imports use `processor/graph/clustering`
3. **Given** the move is complete, **When** building, **Then** no import cycles exist and `go build ./...` succeeds

---

### User Story 5 - Delete graphinterfaces Hack Package (Priority: P2)

As a developer, I want idiomatic Go code without Java-style getter interfaces that exist solely to break import cycles.

**Why this priority**: The `pkg/graphinterfaces` package with its 10-method Community interface is a Java anti-pattern. Once graphclustering moves inside processor/graph/, this cycle-breaking hack is unnecessary.

**Independent Test**: Can be fully tested by verifying `pkg/graphinterfaces/` is deleted and Community uses direct field access.

**Acceptance Scenarios**:

1. **Given** `pkg/graphinterfaces/community.go` exists with 10 getter methods, **When** consolidation is complete, **Then** the directory is deleted
2. **Given** code calls `comm.GetID()`, **When** migration is complete, **Then** code uses `comm.ID` directly
3. **Given** Community struct has getter methods, **When** migration is complete, **Then** all getters are removed

---

### User Story 6 - Relocate embedding to processor/graph/ (Priority: P2)

As a developer maintaining the graph system, I want the embedding package inside `processor/graph/` where its only consumers (indexmanager) live, following the same pattern as graphclustering.

**Why this priority**: The `pkg/` convention means "reusable library code for external consumers," but embedding has exactly 2 consumers, both in `processor/graph/indexmanager/`. Moving it co-locates the code with its consumers.

**Independent Test**: Can be fully tested by verifying `processor/graph/embedding/` exists and `pkg/embedding/` is deleted.

**Acceptance Scenarios**:

1. **Given** `pkg/embedding/` exists, **When** migration is complete, **Then** code is moved to `processor/graph/embedding/`
2. **Given** import paths reference `pkg/embedding`, **When** migration is complete, **Then** all imports use `processor/graph/embedding`
3. **Given** the move is complete, **When** building, **Then** `go build ./...` succeeds

---

### User Story 7 - Document Package Ownership (Priority: P3)

As a new developer joining the project, I want clear documentation about which package owns which concerns so that I can make correct architectural decisions.

**Why this priority**: Even after consolidation, developers need to understand the boundaries. Documentation prevents future drift.

**Independent Test**: Can be fully tested by reviewing README files in message/, graph/, and processor/graph/ packages.

**Acceptance Scenarios**:

1. **Given** package READMEs exist, **When** documentation is complete, **Then** each clearly states its ownership scope
2. **Given** a developer asks "where does X belong?", **When** they read the docs, **Then** the answer is unambiguous

---

### Edge Cases

- What happens if an import cycle is accidentally reintroduced?
  - Build will fail immediately with clear error message
- How do we handle in-flight PRs that use old import paths?
  - This is a greenfield project; no backward compatibility needed
- What if some Graphable implementers are in external packages?
  - Analysis shows all implementers are internal to this repo

## Requirements

### Functional Requirements

- **FR-001**: System MUST migrate all 9 files importing `types/graph` to use `graph` package
- **FR-002**: System MUST delete the `types/graph/` directory after migration
- **FR-003**: System MUST move `message/graphable.go` to `graph/graphable.go`
- **FR-004**: System MUST update all Graphable references (9 files) to use `graph.Graphable`
- **FR-005**: System MUST delete `graph/federation.go` entirely
- **FR-006**: System MUST move `pkg/graphclustering/` to `processor/graph/clustering/`
- **FR-007**: System MUST update all clustering import paths
- **FR-008**: System MUST delete `pkg/graphinterfaces/` directory
- **FR-009**: System MUST remove all getter methods from Community struct (10 methods)
- **FR-010**: System MUST update all callers to use direct field access (e.g., `comm.ID` not `comm.GetID()`)
- **FR-011**: System MUST move `pkg/embedding/` to `processor/graph/embedding/`
- **FR-012**: System MUST update all embedding import paths (2 files in indexmanager)
- **FR-013**: System MUST update package READMEs with clear ownership documentation
- **FR-014**: System MUST ensure `go build ./...` succeeds with no import cycles after each phase

### Key Entities

- **EntityState**: Complete persisted entity representation (lives in `graph/`)
- **Graphable**: Interface for graph-compatible payloads (moving from `message/` to `graph/`)
- **Community**: Clustering community structure (moving from `pkg/graphclustering/` to `processor/graph/clustering/`)
- **Embedder**: Vector embedding interface and implementations (moving from `pkg/embedding/` to `processor/graph/embedding/`)
- **EntityID**: 6-part federated identifier (remains in `message/` - single source of truth for identity)

## Success Criteria

### Measurable Outcomes

- **SC-001**: Zero files import from `types/graph/` package (was 9)
- **SC-002**: `types/graph/` directory does not exist
- **SC-003**: `message/graphable.go` does not exist; `graph/graphable.go` exists
- **SC-004**: Zero files reference `message.Graphable` (all use `graph.Graphable`)
- **SC-005**: `graph/federation.go` does not exist
- **SC-006**: `pkg/graphclustering/` directory does not exist; `processor/graph/clustering/` exists
- **SC-007**: `pkg/graphinterfaces/` directory does not exist
- **SC-008**: Community struct has zero getter methods (was 10)
- **SC-009**: `pkg/embedding/` directory does not exist; `processor/graph/embedding/` exists
- **SC-010**: `go build ./...` completes with exit code 0
- **SC-011**: `go test -race ./...` passes for all affected packages
- **SC-012**: All package READMEs document their ownership scope

## Assumptions

- This is a greenfield project; no backward compatibility or deprecation periods required
- All Graphable implementers are internal to this repository (verified by grep analysis)
- The 004-semantic-refactor changes (NodeProperties removal, entity_payload deletion) are already merged
- Phase ordering from ADR is correct: Phase 6 (move graphclustering) must precede Phase 7 (delete graphinterfaces)

## Dependencies

- **004-semantic-refactor**: Must be complete (provides simplified EntityState structure)
- **ADR-PACKAGE-RESPONSIBILITIES-CONSOLIDATION.md**: Defines the target architecture

## Out of Scope

- Performance optimizations to graphclustering algorithms
- Adding new features to clustering functionality
- Changes to NATS KV storage patterns
- GraphQL schema changes (beyond import path updates)
