# Contracts: Semantic System Refactor

**Feature**: 004-semantic-refactor
**Date**: 2025-01-29

## Overview

This is an internal refactor with no new API contracts. The changes affect the internal EntityState structure but do not change:

- GraphQL schema (resolvers updated to use new structure)
- NATS message formats (unchanged)
- External API endpoints (none affected)

## Internal Changes

The following internal Go types are modified:

### EntityState (graph/types.go)

- `Node.ID` → `ID` (promoted to top-level)
- `ObjectRef string` → `StorageRef *message.StorageReference`
- `MessageType string` → `MessageType message.Type`
- `Node.Type`, `Node.Position`, `Node.Status` removed

### Deleted Types

- `NodeProperties` struct
- `Position` struct
- `EntityStatus` enum
- `EntityClass` enum (message package)
- `EntityRole` enum (message package)

## GraphQL Impact

The GraphQL schema remains unchanged. Resolvers are updated internally to:

1. Access `state.ID` instead of `state.Node.ID`
2. Use `message.ParseEntityID()` for type extraction
3. Handle `StorageRef` instead of `ObjectRef`

No client-facing changes required.

## NATS KV Impact

Entity storage key format unchanged:

```text
ENTITY_STATES bucket
Key: {entity_id}  (6-part dotted format)
Value: JSON-serialized EntityState
```

The JSON structure changes slightly but this is greenfield with no data migration needed.
