package iotsensor

import (
	"errors"
	"fmt"
	"time"
)

// Config holds the configuration for the IoT sensor processor.
// It provides the organizational context that is applied to all processed readings.
type Config struct {
	// OrgID is the organization identifier (e.g., "acme")
	// This becomes the first part of federated entity IDs.
	OrgID string

	// Platform is the platform/product identifier (e.g., "logistics")
	// This becomes the second part of federated entity IDs.
	Platform string
}

// Validate checks that the configuration is valid.
func (c Config) Validate() error {
	if c.OrgID == "" {
		return errors.New("OrgID is required")
	}
	if c.Platform == "" {
		return errors.New("Platform is required")
	}
	return nil
}

// Processor transforms incoming JSON sensor data into Graphable payloads.
// It applies the organizational context from configuration and produces
// SensorReading instances with proper federated entity IDs and semantic triples.
//
// This demonstrates the correct pattern for domain processors:
//   - Configuration provides organizational context
//   - Process method transforms data with domain understanding
//   - Output is a Graphable payload, not generic JSON
type Processor struct {
	config Config
}

// NewProcessor creates a new IoT sensor processor with the given configuration.
func NewProcessor(config Config) *Processor {
	return &Processor{
		config: config,
	}
}

// Process transforms incoming JSON data into a SensorReading.
//
// Expected JSON format:
//
//	{
//	  "device_id": "sensor-042",
//	  "type": "temperature",
//	  "reading": 23.5,
//	  "unit": "celsius",
//	  "location": "warehouse-7",
//	  "timestamp": "2025-11-26T10:30:00Z"
//	}
//
// The processor:
//  1. Extracts fields from the incoming JSON
//  2. Applies organizational context from config
//  3. Returns a SensorReading that implements Graphable
//
// This method demonstrates domain-specific transformation logic:
//   - Field extraction with proper type handling
//   - Context enrichment from configuration
//   - Validation of required fields
func (p *Processor) Process(input map[string]any) (*SensorReading, error) {
	// Extract required fields
	deviceID, err := getString(input, "device_id")
	if err != nil {
		return nil, fmt.Errorf("missing device_id: %w", err)
	}

	sensorType, err := getString(input, "type")
	if err != nil {
		return nil, fmt.Errorf("missing type: %w", err)
	}

	value, err := getFloat64(input, "reading")
	if err != nil {
		return nil, fmt.Errorf("missing reading: %w", err)
	}

	unit, err := getString(input, "unit")
	if err != nil {
		return nil, fmt.Errorf("missing unit: %w", err)
	}

	locationID, err := getString(input, "location")
	if err != nil {
		return nil, fmt.Errorf("missing location: %w", err)
	}

	// Extract zone type (optional, default to "area")
	zoneType := "area"
	if zt, ok := input["zone_type"].(string); ok && zt != "" {
		zoneType = zt
	}

	// Parse timestamp (optional, default to now)
	var observedAt time.Time
	if ts, ok := input["timestamp"].(string); ok {
		parsed, err := time.Parse(time.RFC3339, ts)
		if err != nil {
			observedAt = time.Now()
		} else {
			observedAt = parsed
		}
	} else {
		observedAt = time.Now()
	}

	// Extract serial number (optional, for ALIAS_INDEX)
	var serialNumber string
	if serial, ok := input["serial"].(string); ok {
		serialNumber = serial
	}

	// Extract coordinates (optional, for SPATIAL_INDEX)
	var latitude, longitude, altitude *float64
	if lat, err := getFloat64(input, "latitude"); err == nil {
		latitude = &lat
	}
	if lon, err := getFloat64(input, "longitude"); err == nil {
		longitude = &lon
	}
	if alt, err := getFloat64(input, "altitude"); err == nil {
		altitude = &alt
	}

	// Build the Graphable payload with organizational context
	// Processor computes the zone entity ID - this is domain knowledge
	reading := &SensorReading{
		DeviceID:     deviceID,
		SensorType:   sensorType,
		Value:        value,
		Unit:         unit,
		ObservedAt:   observedAt,
		SerialNumber: serialNumber,
		Latitude:     latitude,
		Longitude:    longitude,
		Altitude:     altitude,
		ZoneEntityID: ZoneEntityID(p.config.OrgID, p.config.Platform, zoneType, locationID),
		OrgID:        p.config.OrgID,
		Platform:     p.config.Platform,
	}

	return reading, nil
}

// Helper functions for type-safe field extraction

func getString(m map[string]any, key string) (string, error) {
	v, ok := m[key]
	if !ok {
		return "", fmt.Errorf("field %q not found", key)
	}
	s, ok := v.(string)
	if !ok {
		return "", fmt.Errorf("field %q is not a string: %T", key, v)
	}
	return s, nil
}

func getFloat64(m map[string]any, key string) (float64, error) {
	v, ok := m[key]
	if !ok {
		return 0, fmt.Errorf("field %q not found", key)
	}
	switch val := v.(type) {
	case float64:
		return val, nil
	case float32:
		return float64(val), nil
	case int:
		return float64(val), nil
	case int64:
		return float64(val), nil
	default:
		return 0, fmt.Errorf("field %q is not a number: %T", key, v)
	}
}

// ParseZoneEntityID extracts zone type and zone ID from a full zone entity ID.
// Zone entity ID format: org.platform.facility.zone.{zoneType}.{zoneID}
// Example: "c360.logistics.facility.zone.area.cold-storage-1" -> ("area", "cold-storage-1")
// Returns empty strings if the entity ID is not a valid zone format.
func ParseZoneEntityID(entityID string) (zoneType, zoneID string) {
	parts := splitEntityID(entityID)
	if len(parts) != 6 {
		return "", ""
	}
	// Validate it's a zone entity (parts 2-3 should be "facility.zone")
	if parts[2] != "facility" || parts[3] != "zone" {
		return "", ""
	}
	return parts[4], parts[5]
}

// splitEntityID splits an entity ID into its parts.
func splitEntityID(entityID string) []string {
	if entityID == "" {
		return nil
	}
	var parts []string
	start := 0
	for i := 0; i < len(entityID); i++ {
		if entityID[i] == '.' {
			parts = append(parts, entityID[start:i])
			start = i + 1
		}
	}
	parts = append(parts, entityID[start:])
	return parts
}
