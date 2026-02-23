package workflow

import (
	"fmt"
	"strings"

	"github.com/c360studio/semstreams/component"
)

// ParseTypeString parses a type string in format "domain.category.version" into its parts.
// Returns an error if the format is invalid or any part is empty.
// This matches the validation logic in schema.validateTypeString.
func ParseTypeString(typeStr string) (domain, category, version string, err error) {
	if typeStr == "" {
		return "", "", "", fmt.Errorf("type string cannot be empty")
	}

	parts := strings.Split(typeStr, ".")
	if len(parts) != 3 {
		return "", "", "", fmt.Errorf("type string must be in format 'domain.category.version', got %q", typeStr)
	}

	domain = strings.TrimSpace(parts[0])
	category = strings.TrimSpace(parts[1])
	version = strings.TrimSpace(parts[2])

	if domain == "" {
		return "", "", "", fmt.Errorf("type string domain cannot be empty: %q", typeStr)
	}
	if category == "" {
		return "", "", "", fmt.Errorf("type string category cannot be empty: %q", typeStr)
	}
	if version == "" {
		return "", "", "", fmt.Errorf("type string version cannot be empty: %q", typeStr)
	}

	return domain, category, version, nil
}

// AssemblePayload constructs a typed payload from field mappings and pass-through fields.
// It resolves all mapped paths and pass-through fields from the execution context,
// then delegates to the PayloadRegistry's BuildPayload method.
//
// Parameters:
//   - payloadRegistry: The registry containing payload builders
//   - targetType: The target payload type in "domain.category.version" format
//   - mapping: Maps target field names to source paths (e.g., "user_id" -> "trigger.payload.user")
//   - passThrough: Field names to forward from trigger payload (e.g., ["task_id", "priority"])
//   - resolvePath: Function that resolves a path to its value (typically from interpolation context)
//
// Returns the assembled payload or an error if type parsing fails, path resolution fails,
// or the payload type is not registered.
func AssemblePayload(
	payloadRegistry *component.PayloadRegistry,
	targetType string,
	mapping map[string]string,
	passThrough []string,
	resolvePath func(path string) (any, error),
) (any, error) {
	// Parse the target type string
	domain, category, version, err := ParseTypeString(targetType)
	if err != nil {
		return nil, fmt.Errorf("invalid target type: %w", err)
	}

	// Assemble fields from mapping and pass-through
	fields := make(map[string]any)

	// Resolve mapped fields
	for targetField, sourcePath := range mapping {
		value, err := resolvePath(sourcePath)
		if err != nil {
			return nil, fmt.Errorf("failed to resolve mapping %q -> %q: %w", targetField, sourcePath, err)
		}
		fields[targetField] = value
	}

	// Resolve pass-through fields from trigger payload
	for _, fieldName := range passThrough {
		path := fmt.Sprintf("trigger.payload.%s", fieldName)
		value, err := resolvePath(path)
		if err != nil {
			return nil, fmt.Errorf("failed to resolve pass-through field %q: %w", fieldName, err)
		}
		fields[fieldName] = value
	}

	// Delegate to the registry builder
	payload, err := payloadRegistry.BuildPayload(domain, category, version, fields)
	if err != nil {
		return nil, fmt.Errorf("failed to build payload type %q: %w", targetType, err)
	}

	return payload, nil
}
