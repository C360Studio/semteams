package component

import (
	"encoding/json"
	"fmt"
	"sync"

	"github.com/c360studio/semstreams/pkg/errs"
	"github.com/c360studio/semstreams/pkg/types"
)

// PayloadFactory creates a payload instance for a specific message type.
// The factory returns an any to avoid import cycles.
// The actual payload should implement the message.Payload interface.
type PayloadFactory func() any

// PayloadBuilder creates a typed payload from field mappings.
// Used by workflow variable interpolation to construct typed payloads
// from step output maps. Returns error if required fields are missing
// or field values cannot be converted to the target type.
// Returns any to avoid import cycles - the actual payload should implement message.Payload.
//
// OPTIONAL: If not provided, BuildPayload falls back to JSON marshal/unmarshal
// using the Factory to create the target type. Custom builders are only needed
// for performance optimization of high-frequency payload types.
type PayloadBuilder func(fields map[string]any) (any, error)

// PayloadRegistration holds factory and metadata for a payload type.
// This follows the same pattern as component Registration but is specific
// to message payload types.
type PayloadRegistration struct {
	Factory     PayloadFactory `json:"-"`           // Factory function (not serializable)
	Builder     PayloadBuilder `json:"-"`           // Builder function (not serializable)
	Domain      string         `json:"domain"`      // Message domain (e.g., "robotics", "sensors")
	Category    string         `json:"category"`    // Message category (e.g., "heartbeat", "gps")
	Version     string         `json:"version"`     // Schema version (e.g., "v1", "v2")
	Description string         `json:"description"` // Human-readable description
	Example     map[string]any `json:"example"`     // Optional example payload data
}

// MessageType returns the formatted message type string for this registration.
// Format: "domain.category.version" (e.g., "robotics.heartbeat.v1")
func (pr *PayloadRegistration) MessageType() string {
	return fmt.Sprintf("%s.%s.%s", pr.Domain, pr.Category, pr.Version)
}

// PayloadRegistry manages payload factories for message deserialization.
// It provides thread-safe registration and lookup of payload factories,
// enabling BaseMessage.UnmarshalJSON to recreate typed payloads from JSON.
//
// The registry follows the same patterns as the component Registry but is
// specifically designed for message payload types.
type PayloadRegistry struct {
	registrations map[string]*PayloadRegistration // Registry by message type string
	mu            sync.RWMutex                    // Protects the map
}

// NewPayloadRegistry creates a new empty payload registry.
func NewPayloadRegistry() *PayloadRegistry {
	return &PayloadRegistry{
		registrations: make(map[string]*PayloadRegistration),
	}
}

// RegisterPayload registers a payload factory with validation.
// The message type is derived from the registration's Domain, Category, and Version fields.
// Returns an error if validation fails or the type is already registered.
func (pr *PayloadRegistry) RegisterPayload(registration *PayloadRegistration) error {
	if registration == nil {
		return errs.WrapInvalid(
			errs.ErrInvalidConfig,
			"PayloadRegistry",
			"RegisterPayload",
			"registration validation",
		)
	}

	if registration.Factory == nil {
		return errs.WrapInvalid(
			errs.ErrInvalidConfig,
			"PayloadRegistry",
			"RegisterPayload",
			"factory function validation",
		)
	}

	// Builder is optional - see BuildPayload for fallback behavior

	if registration.Domain == "" {
		return errs.WrapInvalid(errs.ErrInvalidConfig, "PayloadRegistry", "RegisterPayload", "domain validation")
	}

	if registration.Category == "" {
		return errs.WrapInvalid(errs.ErrInvalidConfig, "PayloadRegistry", "RegisterPayload", "category validation")
	}

	if registration.Version == "" {
		return errs.WrapInvalid(errs.ErrInvalidConfig, "PayloadRegistry", "RegisterPayload", "version validation")
	}

	// Verify factory produces payload with matching Schema()
	if err := validateSchemaConsistency(registration); err != nil {
		return err
	}

	msgType := registration.MessageType()

	pr.mu.Lock()
	defer pr.mu.Unlock()

	if _, exists := pr.registrations[msgType]; exists {
		return errs.WrapInvalid(
			fmt.Errorf("payload type '%s' is already registered", msgType),
			"PayloadRegistry",
			"RegisterPayload",
			"duplicate payload check",
		)
	}

	pr.registrations[msgType] = registration
	return nil
}

// CreatePayload creates a payload instance using the registered factory.
// Returns nil if the message type is not registered.
// This allows BaseMessage.UnmarshalJSON to handle unknown types gracefully
// by falling back to GenericPayload.
func (pr *PayloadRegistry) CreatePayload(domain, category, version string) any {
	typeStr := fmt.Sprintf("%s.%s.%s", domain, category, version)

	pr.mu.RLock()
	registration, exists := pr.registrations[typeStr]
	pr.mu.RUnlock()

	if !exists {
		return nil
	}

	return registration.Factory()
}

// BuildPayload creates a typed payload from field mappings.
// If a custom Builder is registered, it is used for efficient field mapping.
// Otherwise, falls back to JSON marshal/unmarshal using the Factory.
//
// Returns an error if the message type is not registered or if building fails.
// This is used by workflow variable interpolation to construct typed payloads
// from step output maps.
// Returns any to avoid import cycles - the actual payload implements message.Payload.
func (pr *PayloadRegistry) BuildPayload(domain, category, version string, fields map[string]any) (any, error) {
	typeStr := fmt.Sprintf("%s.%s.%s", domain, category, version)

	pr.mu.RLock()
	registration, exists := pr.registrations[typeStr]
	pr.mu.RUnlock()

	if !exists {
		return nil, fmt.Errorf("payload type %q not registered", typeStr)
	}

	// Use custom builder if available (optimization path)
	if registration.Builder != nil {
		return registration.Builder(fields)
	}

	// Fallback: JSON round-trip using Factory
	// This works for any payload type without requiring custom builder code
	data, err := json.Marshal(fields)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal fields for %s: %w", typeStr, err)
	}

	payload := registration.Factory()
	if err := json.Unmarshal(data, payload); err != nil {
		return nil, fmt.Errorf("failed to unmarshal into %s: %w", typeStr, err)
	}

	return payload, nil
}

// GetRegistration returns the payload registration for a specific message type.
// Returns the registration and true if found, nil and false otherwise.
func (pr *PayloadRegistry) GetRegistration(msgType string) (*PayloadRegistration, bool) {
	pr.mu.RLock()
	defer pr.mu.RUnlock()

	registration, exists := pr.registrations[msgType]
	if !exists {
		return nil, false
	}

	// Return a copy to prevent external modification of the factory function
	return &PayloadRegistration{
		Domain:      registration.Domain,
		Category:    registration.Category,
		Version:     registration.Version,
		Description: registration.Description,
		Example:     registration.Example,
		// Factory is intentionally not copied for safety
	}, true
}

// ListPayloads returns all registered payload types.
// Returns a copy of the registrations map to prevent external modification.
func (pr *PayloadRegistry) ListPayloads() map[string]*PayloadRegistration {
	pr.mu.RLock()
	defer pr.mu.RUnlock()

	// Return a copy to prevent external modification
	result := make(map[string]*PayloadRegistration, len(pr.registrations))
	for msgType, registration := range pr.registrations {
		// Create a copy without the factory function for safety
		result[msgType] = &PayloadRegistration{
			Domain:      registration.Domain,
			Category:    registration.Category,
			Version:     registration.Version,
			Description: registration.Description,
			Example:     registration.Example,
			// Factory is intentionally not copied for safety
		}
	}

	return result
}

// ListByDomain returns all payload registrations for a specific domain.
// This is useful for discovering what message types are available within
// a particular domain (e.g., "robotics", "sensors").
func (pr *PayloadRegistry) ListByDomain(domain string) []*PayloadRegistration {
	pr.mu.RLock()
	defer pr.mu.RUnlock()

	var result []*PayloadRegistration
	for _, registration := range pr.registrations {
		if registration.Domain == domain {
			// Create a copy without the factory function for safety
			result = append(result, &PayloadRegistration{
				Domain:      registration.Domain,
				Category:    registration.Category,
				Version:     registration.Version,
				Description: registration.Description,
				Example:     registration.Example,
				// Factory is intentionally not copied for safety
			})
		}
	}

	return result
}

// schemaProvider is an interface for payloads that provide schema information.
// This matches the message.Payload interface's Schema() method signature.
type schemaProvider interface {
	Schema() types.Type
}

// validateSchemaConsistency checks that a factory-produced payload's Schema()
// method returns values matching the registration. This catches mismatches
// between Schema() implementation and PayloadRegistration at init() time,
// preventing runtime deserialization failures.
func validateSchemaConsistency(reg *PayloadRegistration) error {
	testPayload := reg.Factory()
	if testPayload == nil {
		return errs.WrapInvalid(
			errs.ErrInvalidConfig,
			"PayloadRegistry",
			"RegisterPayload",
			"factory returned nil payload",
		)
	}

	// Check if payload implements Schema() method
	sp, ok := testPayload.(schemaProvider)
	if !ok {
		// Payload doesn't implement Schema() - skip validation
		// This allows non-message.Payload types to be registered
		return nil
	}

	// Verify Schema() returns matching values
	schema := sp.Schema()
	if schema.Domain != reg.Domain || schema.Category != reg.Category || schema.Version != reg.Version {
		return errs.WrapInvalid(
			fmt.Errorf(
				"Schema() returns {Domain:%q Category:%q Version:%q} but registration expects {Domain:%q Category:%q Version:%q}",
				schema.Domain, schema.Category, schema.Version,
				reg.Domain, reg.Category, reg.Version,
			),
			"PayloadRegistry",
			"RegisterPayload",
			"schema consistency check",
		)
	}

	return nil
}
