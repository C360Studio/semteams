# Types Package

Core type definitions for the semantic event mesh, extracted to avoid import cycles.

## Overview

This package contains foundational types that are shared across multiple packages. It was extracted from `message/` to allow `component/` to reference these types without creating an import cycle.

## Types

### Keyable

Interface for types that produce dotted notation keys for NATS subjects:

```go
type Keyable interface {
    Key() string
}
```

### Type

Message type identifier with three parts: domain, category, version.

```go
msgType := types.Type{Domain: "agentic", Category: "task", Version: "v1"}
subject := "events." + msgType.Key() // "events.agentic.task.v1"
```

### EntityType

Entity type identifier with two parts: domain, type.

```go
entityType := types.EntityType{Domain: "robotics", Type: "drone"}
key := entityType.Key() // "robotics.drone"
```

### EntityID

Six-part federated entity identifier following the hierarchy: org.platform.domain.system.type.instance.

```go
entityID := types.EntityID{
    Org: "c360", Platform: "prod", Domain: "robotics",
    System: "gcs1", Type: "drone", Instance: "42",
}
key := entityID.Key() // "c360.prod.robotics.gcs1.drone.42"

// Parse from string
parsed, err := types.ParseEntityID("c360.prod.robotics.gcs1.drone.42")
```

## Import Pattern

The `message/` package re-exports these types as aliases for backwards compatibility:

```go
// These are equivalent:
import "github.com/c360studio/semstreams/pkg/types"
var t types.Type

import "github.com/c360studio/semstreams/message"
var t message.Type  // Alias to types.Type
```

## Why This Package Exists

The payload registry in `component/` needs to validate that payload `Schema()` methods return the correct type. This requires access to the `Type` struct. However:

- `message/` imports `component/` for `CreatePayload()` in `UnmarshalJSON`
- `component/` cannot import `message/` (would create cycle)

Solution: Extract `Type` and related types to `pkg/types/`, which both packages can import.

```
pkg/types/   <── component/ imports for Schema validation
     ^
     |
message/     <── re-exports as aliases for backwards compatibility
     |
     v
component/   <── message/ imports for CreatePayload()
```
