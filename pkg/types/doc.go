// Package types provides core type definitions for the semantic event mesh.
//
// This package contains foundational types that are shared across multiple
// packages without creating import cycles. It includes:
//
//   - Keyable interface: Foundation for dotted notation semantic keys
//   - Type: Message type identifier (domain.category.version)
//   - EntityType: Entity type identifier (domain.type)
//   - EntityID: 6-part federated entity identifier
//
// These types enable NATS wildcard queries and consistent storage patterns
// through their Key() methods which produce dotted notation strings.
//
// # Example Usage
//
//	// Message type for routing
//	msgType := types.Type{Domain: "sensors", Category: "gps", Version: "v1"}
//	subject := "events." + msgType.Key() // "events.sensors.gps.v1"
//
//	// Entity identification
//	entityID := types.EntityID{
//	    Org: "c360", Platform: "prod", Domain: "robotics",
//	    System: "gcs1", Type: "drone", Instance: "42",
//	}
//	key := entityID.Key() // "c360.prod.robotics.gcs1.drone.42"
package types
