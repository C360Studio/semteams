// Package iotsensor provides an example domain processor demonstrating the correct
// Graphable implementation pattern for SemStreams.
//
// This package serves as a reference implementation showing how to:
//   - Create domain-specific payloads that implement the Graphable interface
//   - Generate federated 6-part entity IDs with organizational context
//   - Produce semantic triples using registered vocabulary predicates
//   - Transform incoming JSON into meaningful graph structures
//
// IoT sensors are used as a neutral example domain that is:
//   - Simple enough to understand quickly
//   - Complex enough to demonstrate real patterns
//   - Universally understood across industries
//   - Not tied to any specific customer domain
//
// For production use, copy this example and adapt it to your domain vocabulary.
package iotsensor

import (
	"fmt"
	"time"

	"github.com/c360/semstreams/message"
)

// SensorReading represents an IoT sensor measurement. It implements the Graphable
// interface with federated entity IDs and semantic predicates.
//
// This is an example of a domain-specific payload that encodes semantic understanding
// of the data, as opposed to generic processors that make semantic decisions without
// domain knowledge.
type SensorReading struct {
	// Input fields (from incoming JSON)
	DeviceID   string    // e.g., "sensor-042"
	SensorType string    // e.g., "temperature", "humidity", "pressure"
	Value      float64   // e.g., 23.5
	Unit       string    // e.g., "celsius", "percent", "hpa"
	ObservedAt time.Time // When measurement was taken

	// Entity reference fields (computed by processor)
	ZoneEntityID string // e.g., "acme.logistics.facility.zone.area.warehouse-7"

	// Context fields (set by processor from config)
	OrgID    string // e.g., "acme"
	Platform string // e.g., "logistics"
}

// EntityID returns a deterministic 6-part federated entity ID following the pattern:
// {org}.{platform}.{domain}.{system}.{type}.{instance}
//
// Example: "acme.logistics.environmental.sensor.temperature.sensor-042"
//
// The 6 parts provide:
//   - org: Organization identifier (multi-tenancy)
//   - platform: Platform/product within the organization
//   - domain: Business domain (environmental, logistics, etc.)
//   - system: System or subsystem (sensor, actuator, etc.)
//   - type: Entity type within the system (temperature, humidity, etc.)
//   - instance: Unique instance identifier
func (s *SensorReading) EntityID() string {
	return fmt.Sprintf("%s.%s.environmental.sensor.%s.%s",
		s.OrgID,
		s.Platform,
		s.SensorType,
		s.DeviceID,
	)
}

// Triples returns semantic facts about this sensor reading using domain-appropriate
// predicates from the vocabulary system.
//
// Each triple follows the Subject-Predicate-Object pattern where:
//   - Subject: This entity's ID (self-reference)
//   - Predicate: Semantic property using dotted notation (domain.category.property)
//   - Object: The value (literal) or entity reference (another entity ID)
//
// The triples produced demonstrate:
//   - Unit-specific predicates (sensor.measurement.celsius vs generic "value")
//   - Entity references (geo.location.zone points to Zone entity, not a string)
//   - Classification triples (sensor.classification.type)
//   - Temporal tracking (time.observation.recorded)
func (s *SensorReading) Triples() []message.Triple {
	entityID := s.EntityID()

	triples := []message.Triple{
		// Measurement value with unit-specific predicate
		{
			Subject:    entityID,
			Predicate:  fmt.Sprintf("sensor.measurement.%s", s.Unit),
			Object:     s.Value,
			Source:     "iot_sensor",
			Timestamp:  s.ObservedAt,
			Confidence: 1.0,
		},
		// Sensor type classification
		{
			Subject:    entityID,
			Predicate:  "sensor.classification.type",
			Object:     s.SensorType,
			Source:     "iot_sensor",
			Timestamp:  s.ObservedAt,
			Confidence: 1.0,
		},
		// Location as entity reference (not string!)
		{
			Subject:    entityID,
			Predicate:  "geo.location.zone",
			Object:     s.ZoneEntityID,
			Source:     "iot_sensor",
			Timestamp:  s.ObservedAt,
			Confidence: 1.0,
		},
		// Observation timestamp
		{
			Subject:    entityID,
			Predicate:  "time.observation.recorded",
			Object:     s.ObservedAt,
			Source:     "iot_sensor",
			Timestamp:  s.ObservedAt,
			Confidence: 1.0,
		},
	}

	return triples
}

// ZoneEntityID generates a federated 6-part entity ID for a zone.
// This is the single source of truth for zone entity ID format, ensuring
// consistency between Zone.EntityID() and any references to zones.
//
// Example: ZoneEntityID("acme", "logistics", "area", "warehouse-7")
// Returns: "acme.logistics.facility.zone.area.warehouse-7"
func ZoneEntityID(orgID, platform, zoneType, zoneID string) string {
	return fmt.Sprintf("%s.%s.facility.zone.%s.%s",
		orgID,
		platform,
		zoneType,
		zoneID,
	)
}

// Zone represents a location zone entity. It demonstrates how entity references
// work in triples - SensorReading references Zone by entity ID, not by string.
type Zone struct {
	ZoneID   string // e.g., "warehouse-7"
	ZoneType string // e.g., "warehouse", "office", "outdoor"
	Name     string // e.g., "Main Warehouse"

	// Context fields
	OrgID    string
	Platform string
}

// EntityID returns a deterministic 6-part federated entity ID for the zone.
// Example: "acme.logistics.facility.zone.area.warehouse-7"
func (z *Zone) EntityID() string {
	return ZoneEntityID(z.OrgID, z.Platform, z.ZoneType, z.ZoneID)
}

// Triples returns semantic facts about this zone.
func (z *Zone) Triples() []message.Triple {
	entityID := z.EntityID()
	now := time.Now()

	return []message.Triple{
		{
			Subject:    entityID,
			Predicate:  "facility.zone.name",
			Object:     z.Name,
			Source:     "iot_sensor",
			Timestamp:  now,
			Confidence: 1.0,
		},
		{
			Subject:    entityID,
			Predicate:  "facility.zone.type",
			Object:     z.ZoneType,
			Source:     "iot_sensor",
			Timestamp:  now,
			Confidence: 1.0,
		},
	}
}
