# Feature Specification: Triples Architecture Evolution

**Feature Branch**: `003-triples-architecture`
**Created**: 2025-11-27
**Status**: Draft
**Input**: User description: "from docs/ROADMAP-GRAPH-ARCHITECTURE-EVOLUTION.md"

## User Scenarios & Testing *(mandatory)*

### User Story 1 - Forward Relationship Traversal (Priority: P1)

As a developer querying the graph, I want to traverse relationships in the forward direction (from source entity to target entities) without relying on stored Edge structures, so that relationship data has a single source of truth (Triples) and I can query bidirectionally with consistent performance.

**Why this priority**: OUTGOING_INDEX is the foundational capability that enables all subsequent changes. Without forward traversal via index, we cannot deprecate the Edges field or implement stateful rules that need to check existing relationships.

**Independent Test**: Query "what entities does drone.001 relate to?" using the new OUTGOING_INDEX and verify results match the current Edge-based traversal.

**Acceptance Scenarios**:

1. **Given** an entity with relationship triples stored, **When** I query OUTGOING_INDEX for that entity, **Then** I receive all outgoing relationships with predicate and target entity ID.
2. **Given** an entity with multiple relationship types (fleet membership, operator control), **When** I query OUTGOING_INDEX, **Then** I receive all relationship types in a single query.
3. **Given** a new relationship triple is added to an entity, **When** the entity is processed, **Then** OUTGOING_INDEX is updated to include the new relationship.
4. **Given** a relationship triple is removed from an entity, **When** the entity is processed, **Then** OUTGOING_INDEX no longer contains that relationship.

---

### User Story 2 - Automatic Relationship Retraction (Priority: P2)

As a system operator, I want proximity and other conditional relationships to be automatically removed when conditions no longer hold, so that the graph remains accurate without manual cleanup or stale data accumulation.

**Why this priority**: Dynamic relationships are a core use case (drone proximity, operational zones, alert states). Currently, relationships created by rules persist even when conditions change, leading to incorrect graph state.

**Independent Test**: Create a proximity rule, trigger it with two nearby entities, move entities apart, verify the proximity relationship is automatically removed.

**Acceptance Scenarios**:

1. **Given** a stateful rule with on_enter action creating a relationship, **When** the rule condition becomes true for an entity pair, **Then** the relationship triple is created exactly once.
2. **Given** a stateful rule with on_exit action removing a relationship, **When** the rule condition becomes false for an entity pair, **Then** the relationship triple is removed.
3. **Given** entities that remain in proximity, **When** multiple position updates occur, **Then** the on_enter action fires only on the first transition (not repeatedly).
4. **Given** a relationship triple with an expiration time (TTL), **When** the expiration time passes, **Then** the triple is automatically removed from the entity.

---

### User Story 3 - Unified Graph Queries (Priority: P3)

As a PathRAG query engine, I want all relationships (including community membership) to be queryable as triples, so that graph traversal algorithms can discover all entity connections through a single query pattern.

**Why this priority**: Community membership is currently stored in a separate index and invisible to PathRAG traversal. This creates an inconsistent query model and limits graph analysis capabilities.

**Independent Test**: Run a PathRAG traversal that discovers community membership as a traversable relationship, not a separate lookup.

**Acceptance Scenarios**:

1. **Given** an entity assigned to a community by LPA detection, **When** I query for entity relationships, **Then** the community membership appears as a relationship triple.
2. **Given** PathRAG traversing from an entity, **When** it encounters community membership, **Then** it can traverse to the community entity and other members.
3. **Given** community detection is configured with create_triples enabled, **When** communities are detected, **Then** both COMMUNITY_INDEX and relationship triples are created (dual-write for compatibility).

---

### User Story 4 - Simplified Data Model (Priority: P4)

As a developer working with SemStreams, I want entity data to have a single representation (Triples), so that I don't need to understand the relationship between Triples, Edges, and Properties or worry about keeping them synchronized.

**Why this priority**: The current dual representation (Triples + Edges + Properties) creates cognitive overhead and maintenance burden. This is a cleanup story that depends on P1-P3 being complete.

**Independent Test**: Access entity properties and relationships using only triple-based methods; verify all legacy access patterns emit deprecation warnings, then are removed.

**Acceptance Scenarios**:

1. **Given** an entity with property triples, **When** I call GetPropertyValue(predicate), **Then** I receive the property value without accessing Node.Properties map.
2. **Given** code accessing entity.Node.Properties directly, **When** the code runs, **Then** a deprecation warning is logged (Phase 4).
3. **Given** code accessing entity.Edges directly, **When** the code runs, **Then** a deprecation warning is logged (Phase 4).
4. **Given** the final EntityState structure, **When** I inspect it, **Then** it contains only ID, Type, Triples, ObjectRef, Version, and UpdatedAt.

---

### Edge Cases

- **Consistency guarantees**: OUTGOING_INDEX and INCOMING_INDEX updates occur in sequence within the same HandleUpdate call. If partial failure occurs, the entity revision provides rollback capability via NATS KV history.
- **Orphaned INCOMING_INDEX cleanup on entity deletion**: When an entity is deleted, INCOMING_INDEX entries pointing TO that entity from other entities must be cleaned up. The cleanup sequence is: (1) Read deleted entity's outgoing relationships from OUTGOING_INDEX, (2) For each target entity, remove the deleted entity from their INCOMING_INDEX entry, (3) Delete OUTGOING_INDEX entry, (4) Delete INCOMING_INDEX entry for the deleted entity itself. This ensures no orphaned references remain.
- **Orphaned rule state cleanup**: When an entity is deleted, StateTracker.DeleteAllForEntity(entityID) is called to remove associated rule states. This is triggered by HandleDelete in the index manager.
- **on_exit action failure**: Actions use at-least-once semantics. Failed actions are logged and the rule state is NOT updated, allowing retry on next entity change. After 3 consecutive failures, an alert is raised.
- **Community churn management**: Community triple updates use the same dual-write pattern as other triples. Rapid membership changes are handled by the existing NATS KV revision conflict resolution.
- **Deprecation period queries**: Both old (Edges/Properties) and new (GetOutgoing/GetTriple) access patterns work during Phase 4. Deprecation warnings guide migration without breaking functionality.

## Requirements *(mandatory)*

### Functional Requirements

**Phase 1: Foundation**

- **FR-001**: System MUST create and maintain an OUTGOING_INDEX bucket in IndexManager.
- **FR-002**: OUTGOING_INDEX MUST store outgoing relationships keyed by source entity ID.
- **FR-003**: Each OUTGOING_INDEX entry MUST contain predicate and target entity ID.
- **FR-004**: System MUST provide GetOutgoing(ctx, entityID) query method returning all outgoing relationships.
- **FR-005**: OUTGOING_INDEX MUST be updated atomically with INCOMING_INDEX when relationship triples change.
- **FR-005a**: When an entity is deleted, system MUST read its OUTGOING_INDEX to identify target entities.
- **FR-005b**: For each target entity identified in FR-005a, system MUST remove the deleted entity from that target's INCOMING_INDEX entry.
- **FR-005c**: The cleanup sequence MUST be: cleanup INCOMING references first, then delete OUTGOING entry.
- **FR-006**: Triple struct MUST support an optional ExpiresAt field for TTL-based expiration.
- **FR-006a**: Relationship detection MUST use Triple.IsRelationship() which validates Object as a 6-part EntityID.
- **FR-006b**: System MUST NOT use hardcoded predicate name lists for relationship detection.

**Phase 2: Stateful Rules**

- **FR-007**: System MUST create a RULE_STATE KV bucket for tracking rule match states.
- **FR-008**: Rule schema MUST support on_enter actions that fire when condition transitions to true.
- **FR-009**: Rule schema MUST support on_exit actions that fire when condition transitions to false.
- **FR-010**: Rule schema MUST support while_true actions that fire on every update while condition holds.
- **FR-011**: System MUST provide add_triple action type for creating relationship triples via mutation API.
- **FR-012**: System MUST provide remove_triple action type for removing relationship triples.
- **FR-013**: Triples created by rules MUST support optional TTL as fallback expiration.
- **FR-014**: System MUST implement a cleanup worker that removes expired triples.

**Phase 3: Community Alignment**

- **FR-015**: Community Detection MUST support a create_triples configuration option.
- **FR-016**: When create_triples is enabled, community membership MUST be written as relationship triples.
- **FR-017**: Community relationship triples MUST use "graph.community.member_of" predicate.
- **FR-018**: System MUST continue to support COMMUNITY_INDEX for fast community lookups (dual-write).

**Phase 4: Deprecations**

- **FR-019**: System MUST log deprecation warnings when EntityState.Edges is accessed.
- **FR-020**: System MUST log deprecation warnings when Node.Properties is accessed.
- **FR-021**: System MUST provide GetTriple(predicate) helper method on EntityState.
- **FR-022**: System MUST provide GetPropertyValue(predicate) helper method on EntityState.

**Phase 5: Removal**

- **FR-023**: System MUST remove Edges field from EntityState.
- **FR-024**: System MUST remove Properties field from NodeProperties.
- **FR-025**: System MUST remove extractPropertiesAndRelationships() processing.
- **FR-026**: System MUST provide migration tooling for existing data.

### Key Entities

- **Triple**: The atomic unit of semantic data. Contains Subject (entity ID), Predicate (relationship/property type), Object (value or entity reference), plus metadata (Source, Timestamp, Confidence, ExpiresAt).

- **OutgoingEntry**: Index entry representing a forward relationship. Contains Predicate and ToEntityID.

- **RuleMatchState**: Tracks whether a rule's condition is currently matching for a specific entity or entity pair. Contains RuleID, EntityKey, IsMatching, LastTransition, TransitionAt, SourceRevision.

- **EntityState** (Target): Simplified entity representation containing only ID, Type, Triples, ObjectRef, Version, and UpdatedAt.

### Assumptions

- The vocabulary system supports the predicates needed for community membership and rule-created relationships.
- NATS KV provides sufficient consistency guarantees for index updates.
- The existing mutation API can handle the volume of relationship triple operations from stateful rules.
- Existing code that accesses Edges and Properties can be migrated during the deprecation period.
- Performance of triple-based property access is acceptable (can use internal predicate-keyed map for optimization).

## Success Criteria *(mandatory)*

### Measurable Outcomes

- **SC-001**: OUTGOING_INDEX queries return identical results to current Edge-based traversal for all test entities.
- **SC-002**: Stateful rules correctly detect and fire on_enter exactly once per condition transition to true.
- **SC-003**: Stateful rules correctly detect and fire on_exit exactly once per condition transition to false.
- **SC-004**: Expired triples are cleaned up within 60 seconds of expiration time.
- **SC-005**: Community membership is queryable as a standard relationship triple when create_triples is enabled.
- **SC-006**: PathRAG can traverse community membership relationships without special-case code.
- **SC-007**: All existing tests pass after each phase without regression.
- **SC-008**: Query performance for OUTGOING_INDEX is within 10% of Edge-based traversal.
- **SC-009**: After Phase 5, EntityState storage size is reduced compared to current structure.
- **SC-010**: Developer documentation clearly explains the single source of truth model.
- **SC-011**: After entity deletion, no INCOMING_INDEX entries reference the deleted entity.
- **SC-012**: Relationship detection uses Triple.IsRelationship() consistently - no hardcoded predicate lists exist in codebase.
