# Data Model: Semantic System Refactor

**Feature**: 004-semantic-refactor
**Date**: 2025-01-29
**Status**: Design Complete

## EntityState (After Refactor)

The simplified EntityState structure with triples as single source of truth:

```go
// EntityState represents complete local graph state for an entity.
// Triples are the single source of truth for all semantic properties.
type EntityState struct {
    // ID is the 6-part entity identifier: org.platform.domain.system.type.instance
    // Used as NATS KV key for storage and retrieval.
    ID string `json:"id"`

    // Triples contains all semantic facts about this entity.
    // Properties, relationships, and domain-specific data are all stored as triples.
    Triples []message.Triple `json:"triples"`

    // StorageRef optionally points to where the full original message is stored.
    // Supports "store once, reference anywhere" pattern for large payloads.
    // Nil if message was not stored or storage reference not available.
    StorageRef *message.StorageReference `json:"storage_ref,omitempty"`

    // MessageType records the original message type that created/updated this entity.
    // Provides provenance and enables filtering by message source.
    MessageType message.Type `json:"message_type"`

    // Version is incremented on each update for optimistic concurrency control.
    Version uint64 `json:"version"`

    // UpdatedAt records when this entity state was last modified.
    UpdatedAt time.Time `json:"updated_at"`
}
```

## Field Comparison (Before → After)

| Before | After | Change |
|--------|-------|--------|
| `Node.ID` | `ID` | Promoted to top-level |
| `Node.Type` | (deleted) | Use `message.ParseEntityID(state.ID).Type` |
| `Node.Position` | (deleted) | Dead code - spatial index reads from triples |
| `Node.Status` | (deleted) | Domain responsibility - emit as triples |
| `ObjectRef string` | `StorageRef *message.StorageReference` | Full metadata preserved |
| `MessageType string` | `MessageType message.Type` | Proper typed struct |
| `Triples` | `Triples` | Unchanged |
| `Version` | `Version` | Unchanged |
| `UpdatedAt` | `UpdatedAt` | Unchanged |

## Supporting Types

### message.StorageReference (Unchanged)

```go
type StorageReference struct {
    StorageInstance string `json:"storage_instance"` // Which storage holds the data
    Key             string `json:"key"`              // Storage-specific retrieval key
    ContentType     string `json:"content_type"`     // MIME type of stored content
    Size            int64  `json:"size,omitempty"`   // Optional size hint in bytes
}
```

### message.Type (Unchanged)

```go
type Type struct {
    Domain   string // Business domain (e.g., "sensors", "robotics")
    Category string // Message category (e.g., "temperature", "heartbeat")
    Version  string // Schema version (e.g., "v1", "v2")
}
```

### message.Triple (Unchanged)

```go
type Triple struct {
    Subject    string    `json:"subject"`    // Entity ID
    Predicate  string    `json:"predicate"`  // Property name (dotted notation)
    Object     any       `json:"object"`     // Value or entity reference
    Source     string    `json:"source"`     // Origin of this fact
    Timestamp  time.Time `json:"timestamp"`  // When fact was recorded
    Confidence float64   `json:"confidence"` // Confidence score (0.0-1.0)
}
```

### message.EntityID (Unchanged - used for parsing)

```go
type EntityID struct {
    Org      string // Organization namespace
    Platform string // Platform/instance ID
    System   string // System/source ID
    Domain   string // Data domain
    Type     string // Entity type
    Instance string // Instance identifier
}

// ParseEntityID creates EntityID from dotted string format.
func ParseEntityID(s string) (EntityID, error)
```

## Deleted Types

### NodeProperties (DELETE)

```go
// DELETE - Fields promoted or removed
type NodeProperties struct {
    ID       string       // → EntityState.ID
    Type     string       // → Use message.ParseEntityID().Type
    Position *Position    // DELETE - dead code
    Status   EntityStatus // DELETE - domain responsibility
}
```

### Position (DELETE)

```go
// DELETE - Spatial data comes from triples
type Position struct {
    Latitude  float64
    Longitude float64
    Altitude  float64
}
```

### EntityStatus (DELETE)

```go
// DELETE - Domain-specific, not framework concern
type EntityStatus string

const (
    StatusActive    EntityStatus = "active"
    StatusWarning   EntityStatus = "warning"
    StatusCritical  EntityStatus = "critical"
    StatusEmergency EntityStatus = "emergency"
    StatusInactive  EntityStatus = "inactive"
    StatusUnknown   EntityStatus = "unknown"
)
```

### EntityClass (DELETE from message package)

```go
// DELETE - Unused, framework can't determine
type EntityClass string

const (
    ClassObject  EntityClass = "Object"
    ClassEvent   EntityClass = "Event"
    ClassAgent   EntityClass = "Agent"
    ClassPlace   EntityClass = "Place"
    ClassProcess EntityClass = "Process"
    ClassThing   EntityClass = "Thing"
)
```

### EntityRole (DELETE from message package)

```go
// DELETE - Unused, per-message context not per-entity
type EntityRole string

const (
    RolePrimary   EntityRole = "primary"
    RoleObserved  EntityRole = "observed"
    RoleComponent EntityRole = "component"
    RoleSource    EntityRole = "source"
    RoleTarget    EntityRole = "target"
    RoleContext   EntityRole = "context"
    RoleRelated   EntityRole = "related"
)
```

## Entity Relationships

```text
┌─────────────────────────────────────────────────────────────────┐
│                      EntityState                                 │
│                                                                 │
│  ID ────────────────────────────────────────────────────────┐   │
│  │                                                          │   │
│  │  6-part format: org.platform.domain.system.type.instance │   │
│  │  Example: acme.logistics.environmental.sensor.temp.s042  │   │
│  │                                                          │   │
│  └──────────────── Can be parsed with ──────────────────────┘   │
│                    message.ParseEntityID()                      │
│                                                                 │
│  Triples[] ─────────────────────────────────────────────────┐   │
│  │                                                          │   │
│  │  All semantic facts about this entity:                   │   │
│  │  - Properties: {subject: ID, predicate: "temp", obj: 23} │   │
│  │  - Relationships: {subj: ID, pred: "located_in", obj: Z} │   │
│  │  - Domain status: {subj: ID, pred: "sensor.status", ...} │   │
│  │                                                          │   │
│  └──────────────────────────────────────────────────────────┘   │
│                                                                 │
│  StorageRef? ───────────────────────────────────────────────┐   │
│  │                                                          │   │
│  │  Optional pointer to original message:                   │   │
│  │  - StorageInstance: "message-store"                      │   │
│  │  - Key: "2025/01/29/14/msg_abc123"                       │   │
│  │  - ContentType: "application/json"                       │   │
│  │  - Size: 1024                                            │   │
│  │                                                          │   │
│  └──────────────────────────────────────────────────────────┘   │
│                                                                 │
│  MessageType ───────────────────────────────────────────────┐   │
│  │                                                          │   │
│  │  Typed message classification:                           │   │
│  │  - Domain: "sensors"                                     │   │
│  │  - Category: "temperature"                               │   │
│  │  - Version: "v1"                                         │   │
│  │                                                          │   │
│  └──────────────────────────────────────────────────────────┘   │
│                                                                 │
│  Version: uint64 (optimistic concurrency)                       │
│  UpdatedAt: time.Time (last modification)                       │
│                                                                 │
└─────────────────────────────────────────────────────────────────┘
```

## Validation Rules

### EntityState

| Field | Validation | Notes |
|-------|------------|-------|
| ID | Required, 6-part dotted format | Must be valid EntityID |
| Triples | Required, non-nil slice | May be empty slice |
| StorageRef | Optional (nil allowed) | When present, all fields required |
| MessageType | Required | Domain and Category must be non-empty |
| Version | Auto-incremented | Starts at 1 |
| UpdatedAt | Auto-set | Set on create/update |

### ID Format

```text
org.platform.domain.system.type.instance
 │      │       │      │     │      │
 │      │       │      │     │      └── Instance identifier (required)
 │      │       │      │     └───────── Entity type (required)
 │      │       │      └────────────── System ID (required)
 │      │       └───────────────────── Business domain (required)
 │      └───────────────────────────── Platform (required)
 └──────────────────────────────────── Organization (required)
```

## Migration Notes

### Code Changes Required

1. **state.Node.ID → state.ID**
   - Direct field access, no method call needed

2. **state.Node.Type → message.ParseEntityID(state.ID).Type**
   - Function call required, handle error for malformed IDs
   - Example:
     ```go
     eid, err := message.ParseEntityID(state.ID)
     if err != nil {
         return fmt.Errorf("invalid entity ID: %w", err)
     }
     entityType := eid.Type
     ```

3. **state.ObjectRef → state.StorageRef**
   - Check for nil before accessing
   - Full StorageReference available when present

4. **Status handling → Domain triples**
   - Remove hardcoded StatusActive
   - Domains emit status via triples if needed
