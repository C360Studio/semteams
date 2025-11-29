# Quickstart: Semantic System Refactor Migration

**Feature**: 004-semantic-refactor
**Date**: 2025-01-29

## Overview

This guide helps developers migrate code from the old EntityState structure (with NodeProperties) to the simplified structure (with top-level ID).

## Before and After

### Old Structure

```go
type EntityState struct {
    Node        NodeProperties   `json:"node"`
    Triples     []message.Triple `json:"triples"`
    ObjectRef   string           `json:"object_ref"`
    MessageType string           `json:"message_type"`
    Version     uint64           `json:"version"`
    UpdatedAt   time.Time        `json:"updated_at"`
}

type NodeProperties struct {
    ID       string       `json:"id"`
    Type     string       `json:"type"`
    Position *Position    `json:"position,omitempty"`
    Status   EntityStatus `json:"status"`
}
```

### New Structure

```go
type EntityState struct {
    ID          string                   `json:"id"`
    Triples     []message.Triple         `json:"triples"`
    StorageRef  *message.StorageReference `json:"storage_ref,omitempty"`
    MessageType message.Type             `json:"message_type"`
    Version     uint64                   `json:"version"`
    UpdatedAt   time.Time                `json:"updated_at"`
}
```

## Migration Patterns

### 1. Entity ID Access

**Before:**

```go
entityID := state.Node.ID
```

**After:**

```go
entityID := state.ID
```

### 2. Entity Type Access

**Before:**

```go
entityType := state.Node.Type
```

**After:**

```go
eid, err := message.ParseEntityID(state.ID)
if err != nil {
    return fmt.Errorf("invalid entity ID %s: %w", state.ID, err)
}
entityType := eid.Type
```

Or if you're confident the ID is valid (e.g., just retrieved from storage):

```go
eid, _ := message.ParseEntityID(state.ID)
entityType := eid.Type
```

### 3. Storage Reference Access

**Before:**

```go
if state.ObjectRef != "" {
    key := state.ObjectRef
    // Only had the key, no other metadata
}
```

**After:**

```go
if state.StorageRef != nil {
    key := state.StorageRef.Key
    instance := state.StorageRef.StorageInstance
    contentType := state.StorageRef.ContentType
    size := state.StorageRef.Size
}
```

### 4. Message Type Access

**Before:**

```go
msgType := state.MessageType // string like "sensors.temperature.v1"
```

**After:**

```go
msgType := state.MessageType // message.Type struct
domain := state.MessageType.Domain       // "sensors"
category := state.MessageType.Category   // "temperature"
version := state.MessageType.Version     // "v1"
fullKey := state.MessageType.Key()       // "sensors.temperature.v1"
```

### 5. Creating EntityState

**Before:**

```go
state := &gtypes.EntityState{
    Node: gtypes.NodeProperties{
        ID:     entityID,
        Type:   extractTypeFromID(entityID),
        Status: gtypes.StatusActive,
    },
    Triples:     triples,
    ObjectRef:   ref.Key,
    MessageType: msgType.String(),
    Version:     1,
    UpdatedAt:   time.Now(),
}
```

**After:**

```go
state := &gtypes.EntityState{
    ID:          entityID,
    Triples:     triples,
    StorageRef:  ref,  // Full *message.StorageReference or nil
    MessageType: msgType,  // message.Type struct
    Version:     1,
    UpdatedAt:   time.Now(),
}
```

### 6. Status Handling

**Before (WRONG - was hardcoded):**

```go
state.Node.Status = gtypes.StatusActive  // Always hardcoded!
```

**After (Domain responsibility):**

```go
// Status is now a domain concern - emit as triple if needed
statusTriple := message.Triple{
    Subject:   entityID,
    Predicate: "robotics.drone.status",  // Domain-specific predicate
    Object:    "armed",                   // Domain-specific value
    Source:    "domain_processor",
    Timestamp: time.Now(),
}
state.Triples = append(state.Triples, statusTriple)
```

## Deleted Types

The following types no longer exist. Remove all references:

| Type | Was In | Migration |
|------|--------|-----------|
| `NodeProperties` | graph/types.go | Use EntityState directly |
| `Position` | graph/types.go | Use geo.location.* triples |
| `EntityStatus` | graph/types.go | Use domain-specific triples |
| `EntityClass` | message/entity_types.go | Use domain-specific triples |
| `EntityRole` | message/entity_types.go | Use domain-specific triples |

## Deleted Files

The following files no longer exist:

- `message/entity_types.go`
- `message/entity_types_test.go`
- `message/entity_payload.go`

## Search and Replace Guide

Use these patterns to find code needing updates:

```bash
# Find all Node.ID references
grep -r "\.Node\.ID" --include="*.go"

# Find all Node.Type references
grep -r "\.Node\.Type" --include="*.go"

# Find all Node.Status references
grep -r "\.Node\.Status" --include="*.go"

# Find all Node.Position references
grep -r "\.Node\.Position" --include="*.go"

# Find all ObjectRef references
grep -r "ObjectRef" --include="*.go"

# Find all EntityClass references
grep -r "EntityClass\|ClassThing\|ClassObject" --include="*.go"

# Find all EntityRole references
grep -r "EntityRole\|RolePrimary" --include="*.go"

# Find all EntityStatus references
grep -r "EntityStatus\|StatusActive" --include="*.go"
```

## Testing After Migration

```bash
# Run all tests with race detection
go test -race ./...

# Check for compile errors
go build ./...

# Run linting
go fmt ./...
revive ./...
```

## Common Errors and Fixes

### "state.Node undefined"

The NodeProperties struct no longer exists. Change:

```go
state.Node.ID  →  state.ID
```

### "EntityStatus undefined"

EntityStatus enum no longer exists. Remove status handling or use domain triples.

### "EntityClass undefined"

EntityClass enum no longer exists. Remove or use domain-specific classification triples.

### "ObjectRef undefined"

ObjectRef field renamed and typed. Change:

```go
state.ObjectRef  →  state.StorageRef.Key  // (check nil first!)
```

### "cannot use string as message.Type"

MessageType is now a struct. Change:

```go
MessageType: msgType.String()  →  MessageType: msgType
```
