package types

import "fmt"

// Type provides structured type information for messages.
// It enables type-safe routing and processing by clearly identifying
// the domain, category, and version of each message.
//
// Type constants should be defined in domain packages to maintain
// clear ownership and avoid coupling. This package only provides the
// type definition itself.
//
// Example definition in a domain package:
//
//	var GPSMessage = types.Type{
//	    Domain:   "sensors",
//	    Category: "gps",
//	    Version:  "v1",
//	}
type Type struct {
	// Domain identifies the business or system domain.
	// Examples: "sensors", "robotics", "finance"
	Domain string `json:"domain"`

	// Category identifies the specific message type within the domain.
	// Examples: "gps", "temperature", "heartbeat", "trade"
	Category string `json:"category"`

	// Version identifies the schema version.
	// Format: "v1", "v2", etc. Enables schema evolution.
	Version string `json:"version"`
}

// Key returns the dotted notation representation: "domain.category.version"
// This implements the Keyable interface for unified semantic keys.
func (mt Type) Key() string {
	return fmt.Sprintf("%s.%s.%s", mt.Domain, mt.Category, mt.Version)
}

// String returns the same as Key() for backwards compatibility
func (mt Type) String() string {
	return mt.Key()
}

// IsValid checks if the Type has all required fields populated
// with non-empty values.
func (mt Type) IsValid() bool {
	return mt.Domain != "" && mt.Category != "" && mt.Version != ""
}

// Equal compares two Type instances for equality.
// Returns true if all fields (Domain, Category, Version) are identical.
func (mt Type) Equal(other Type) bool {
	return mt.Domain == other.Domain &&
		mt.Category == other.Category &&
		mt.Version == other.Version
}
