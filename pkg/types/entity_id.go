package types

import (
	"fmt"
	"strings"

	"github.com/c360studio/semstreams/pkg/errs"
)

// EntityID represents a complete entity identifier with semantic structure.
// Follows the pattern: org.platform.domain.system.type.instance for federated entity management.
//
// Examples:
//   - EntityID{Org: "c360", Platform: "platform1", Domain: "robotics", System: "gcs1", Type: "drone", Instance: "1"} -> "c360.platform1.robotics.gcs1.drone.1"
//   - EntityID{Org: "c360", Platform: "platform1", Domain: "robotics", System: "mav1", Type: "battery", Instance: "0"} -> "c360.platform1.robotics.mav1.battery.0"
//
// EntityID enables federated entity identification where multiple sources may have
// entities with the same local ID but different canonical identities.
type EntityID struct {
	// Federation hierarchy (3 parts)
	Org      string // Organization namespace (e.g., "c360")
	Platform string // Platform/instance ID (e.g., "platform1")
	System   string // System/source ID - RUNTIME from message (e.g., "mav1", "gcs255")

	// Domain hierarchy (2 parts)
	Domain string // Data domain (e.g., "robotics")
	Type   string // Entity type (e.g., "drone", "battery")

	// Instance identifier (1 part)
	Instance string // Simple instance ID (e.g., "1", "42")
}

// Key returns the full 6-part dotted notation in domain-first format
// This implements the Keyable interface for unified semantic keys.
func (eid EntityID) Key() string {
	return fmt.Sprintf("%s.%s.%s.%s.%s.%s",
		eid.Org, eid.Platform, eid.Domain, eid.System, eid.Type, eid.Instance)
}

// String returns the same as Key() for backwards compatibility
func (eid EntityID) String() string {
	return eid.Key()
}

// EntityType returns the EntityType component of this EntityID
func (eid EntityID) EntityType() EntityType {
	return EntityType{Domain: eid.Domain, Type: eid.Type}
}

// IsValid checks if the EntityID has all required fields populated
func (eid EntityID) IsValid() bool {
	return eid.Org != "" && eid.Platform != "" && eid.System != "" && eid.Domain != "" && eid.Type != "" &&
		eid.Instance != ""
}

// ParseEntityID creates EntityID from dotted string format.
// Expects exactly 6 parts: org.platform.domain.system.type.instance
// Returns an error if the format is invalid.
func ParseEntityID(s string) (EntityID, error) {
	parts := strings.Split(s, ".")
	if len(parts) != 6 {
		return EntityID{}, errs.WrapInvalid(errs.ErrInvalidData, "EntityID", "ParseEntityID",
			fmt.Sprintf("expected 6 parts, got %d", len(parts)))
	}

	// Check that no part is empty
	for i, part := range parts {
		if part == "" {
			return EntityID{}, errs.WrapInvalid(errs.ErrInvalidData, "EntityID", "ParseEntityID",
				fmt.Sprintf("part %d is empty", i+1))
		}
	}

	return EntityID{
		Org:      parts[0],
		Platform: parts[1],
		Domain:   parts[2],
		System:   parts[3],
		Type:     parts[4],
		Instance: parts[5],
	}, nil
}

// TypePrefix returns the 5-part prefix identifying the entity type level.
// Format: org.platform.domain.system.type
// This groups all instances of the same type (siblings).
//
// Example:
//
//	eid := EntityID{Org: "c360", Platform: "logistics", Domain: "environmental",
//	                System: "sensor", Type: "temperature", Instance: "cold-storage-01"}
//	eid.TypePrefix() // Returns "c360.logistics.environmental.sensor.temperature"
func (eid EntityID) TypePrefix() string {
	return fmt.Sprintf("%s.%s.%s.%s.%s",
		eid.Org, eid.Platform, eid.Domain, eid.System, eid.Type)
}

// SystemPrefix returns the 4-part prefix identifying the system level.
// Format: org.platform.domain.system
// This groups all entity types within the same system.
//
// Example:
//
//	eid := EntityID{Org: "c360", Platform: "logistics", Domain: "environmental",
//	                System: "sensor", Type: "temperature", Instance: "cold-storage-01"}
//	eid.SystemPrefix() // Returns "c360.logistics.environmental.sensor"
func (eid EntityID) SystemPrefix() string {
	return fmt.Sprintf("%s.%s.%s.%s",
		eid.Org, eid.Platform, eid.Domain, eid.System)
}

// DomainPrefix returns the 3-part prefix identifying the domain level.
// Format: org.platform.domain
// This groups all systems within the same domain.
//
// Example:
//
//	eid := EntityID{Org: "c360", Platform: "logistics", Domain: "environmental",
//	                System: "sensor", Type: "temperature", Instance: "cold-storage-01"}
//	eid.DomainPrefix() // Returns "c360.logistics.environmental"
func (eid EntityID) DomainPrefix() string {
	return fmt.Sprintf("%s.%s.%s",
		eid.Org, eid.Platform, eid.Domain)
}

// PlatformPrefix returns the 2-part prefix identifying the platform level.
// Format: org.platform
// This groups all domains within the same platform.
//
// Example:
//
//	eid := EntityID{Org: "c360", Platform: "logistics", Domain: "environmental",
//	                System: "sensor", Type: "temperature", Instance: "cold-storage-01"}
//	eid.PlatformPrefix() // Returns "c360.logistics"
func (eid EntityID) PlatformPrefix() string {
	return fmt.Sprintf("%s.%s", eid.Org, eid.Platform)
}

// HasPrefix checks if this EntityID has the given prefix.
// Used for hierarchical grouping and sibling detection.
//
// Example:
//
//	eid := EntityID{Org: "c360", Platform: "logistics", Domain: "environmental",
//	                System: "sensor", Type: "temperature", Instance: "cold-storage-01"}
//	eid.HasPrefix("c360.logistics.environmental.sensor.temperature") // true (same type)
//	eid.HasPrefix("c360.logistics.environmental.sensor")             // true (same system)
//	eid.HasPrefix("c360.logistics.environmental")                    // true (same domain)
//	eid.HasPrefix("c360.logistics.facility")                         // false (different domain)
func (eid EntityID) HasPrefix(prefix string) bool {
	key := eid.Key()
	// Exact match or prefix with dot separator
	return key == prefix || strings.HasPrefix(key, prefix+".")
}

// IsSibling checks if another EntityID is a sibling (same type-level prefix).
// Siblings are entities of the same type within the same system.
//
// Example:
//
//	sensor1 := EntityID{Org: "c360", Platform: "logistics", Domain: "environmental",
//	                    System: "sensor", Type: "temperature", Instance: "cold-storage-01"}
//	sensor2 := EntityID{Org: "c360", Platform: "logistics", Domain: "environmental",
//	                    System: "sensor", Type: "temperature", Instance: "cold-storage-02"}
//	sensor1.IsSibling(sensor2) // true - same type prefix
//
//	humid := EntityID{Org: "c360", Platform: "logistics", Domain: "environmental",
//	                  System: "sensor", Type: "humidity", Instance: "zone-a"}
//	sensor1.IsSibling(humid) // false - different type
func (eid EntityID) IsSibling(other EntityID) bool {
	return eid.TypePrefix() == other.TypePrefix() && eid.Instance != other.Instance
}

// IsSameSystem checks if another EntityID is in the same system.
// Entities in the same system may have different types but share the system-level prefix.
func (eid EntityID) IsSameSystem(other EntityID) bool {
	return eid.SystemPrefix() == other.SystemPrefix()
}

// IsSameDomain checks if another EntityID is in the same domain.
// Entities in the same domain may have different systems but share the domain-level prefix.
func (eid EntityID) IsSameDomain(other EntityID) bool {
	return eid.DomainPrefix() == other.DomainPrefix()
}
