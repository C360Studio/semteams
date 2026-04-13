package teamsgovernance

import (
	"context"
	"crypto/sha256"
	"fmt"
	"strings"

	"github.com/c360studio/semstreams/pkg/errs"
)

// PIIFilter detects and redacts personally identifiable information
type PIIFilter struct {
	// Patterns maps PII types to their detection patterns
	Patterns map[PIIType]*PIIPattern

	// Strategy determines how detected PII is handled
	Strategy RedactionStrategy

	// MaskChar is the character used for masking
	MaskChar string

	// AllowedPII lists PII types that are permitted through
	AllowedPII map[PIIType]bool

	// ConfidenceThreshold for detection (0.0-1.0)
	ConfidenceThreshold float64
}

// PIIDetection records a detected PII instance
type PIIDetection struct {
	Type       PIIType
	Value      string
	Start      int
	End        int
	Confidence float64
}

// NewPIIFilter creates a new PII filter from configuration
func NewPIIFilter(config *PIIFilterConfig) (*PIIFilter, error) {
	filter := &PIIFilter{
		Patterns:            make(map[PIIType]*PIIPattern),
		Strategy:            config.Strategy,
		MaskChar:            config.MaskChar,
		AllowedPII:          make(map[PIIType]bool),
		ConfidenceThreshold: config.ConfidenceThreshold,
	}

	if filter.Strategy == "" {
		filter.Strategy = RedactionLabel
	}

	if filter.MaskChar == "" {
		filter.MaskChar = "*"
	}

	if filter.ConfidenceThreshold == 0 {
		filter.ConfidenceThreshold = 0.85
	}

	// Add default patterns for configured types
	for _, piiType := range config.Types {
		if pattern, ok := GetPIIPattern(piiType); ok {
			filter.Patterns[piiType] = pattern
		}
	}

	// Add custom patterns
	for _, def := range config.CustomPatterns {
		pattern, err := CompileCustomPattern(def)
		if err != nil {
			return nil, errs.WrapInvalid(err, "PIIFilter", "NewPIIFilter", fmt.Sprintf("compile custom pattern %s", def.Type))
		}
		filter.Patterns[def.Type] = pattern
	}

	// Mark allowed types
	for _, piiType := range config.AllowedTypes {
		filter.AllowedPII[piiType] = true
	}

	return filter, nil
}

// Name returns the filter name
func (f *PIIFilter) Name() string {
	return "pii_redaction"
}

// Process detects and redacts PII in the message
func (f *PIIFilter) Process(_ context.Context, msg *Message) (*FilterResult, error) {
	text := msg.Content.Text
	detected := make([]PIIDetection, 0)

	// Find all PII matches
	for piiType, pattern := range f.Patterns {
		// Skip if this type is allowed
		if f.isAllowed(piiType) {
			continue
		}

		matches := pattern.Regex.FindAllStringSubmatchIndex(text, -1)
		for _, match := range matches {
			if len(match) < 2 {
				continue
			}

			start, end := match[0], match[1]
			value := text[start:end]

			// Apply optional validator
			if pattern.Validator != nil && !pattern.Validator(value) {
				continue
			}

			// Check confidence threshold
			if pattern.Confidence < f.ConfidenceThreshold {
				continue
			}

			detected = append(detected, PIIDetection{
				Type:       piiType,
				Value:      value,
				Start:      start,
				End:        end,
				Confidence: pattern.Confidence,
			})
		}
	}

	// If no PII detected, pass through unchanged
	if len(detected) == 0 {
		return NewFilterResult(true).WithConfidence(1.0), nil
	}

	// Sort detections by start position (reverse order for safe replacement)
	sortDetectionsByPosition(detected)

	// Apply redactions to the text
	modified := text
	for _, detection := range detected {
		replacement := f.redact(detection.Type, detection.Value)
		modified = modified[:detection.Start] + replacement + modified[detection.End:]
	}

	// Create modified message
	modifiedMsg := msg.Clone()
	modifiedMsg.Content.Text = modified

	// Add PII metadata
	piiTypes := f.uniqueTypes(detected)
	modifiedMsg.SetMetadata("pii_redacted", map[string]any{
		"types":      piiTypes,
		"count":      len(detected),
		"strategy":   f.Strategy,
		"detections": summarizeDetections(detected),
	})

	return NewFilterResult(true).
		WithModified(modifiedMsg).
		WithConfidence(f.averageConfidence(detected)).
		WithMetadata("pii_types_detected", piiTypes).
		WithMetadata("pii_count", len(detected)), nil
}

// isAllowed checks if a PII type is allowed through
func (f *PIIFilter) isAllowed(piiType PIIType) bool {
	return f.AllowedPII[piiType]
}

// redact applies the redaction strategy
func (f *PIIFilter) redact(piiType PIIType, value string) string {
	pattern, ok := f.Patterns[piiType]
	if !ok {
		return "[REDACTED]"
	}

	switch f.Strategy {
	case RedactionMask:
		return strings.Repeat(f.MaskChar, len(value))

	case RedactionHash:
		hash := sha256.Sum256([]byte(value))
		return fmt.Sprintf("[%s_HASH:%x]", strings.ToUpper(string(piiType)), hash[:4])

	case RedactionRemove:
		return ""

	case RedactionLabel:
		return pattern.Replacement

	default:
		return pattern.Replacement
	}
}

// averageConfidence calculates average confidence of detections
func (f *PIIFilter) averageConfidence(detected []PIIDetection) float64 {
	if len(detected) == 0 {
		return 1.0
	}

	sum := 0.0
	for _, d := range detected {
		sum += d.Confidence
	}
	return sum / float64(len(detected))
}

// uniqueTypes returns unique PII types from detections
func (f *PIIFilter) uniqueTypes(detected []PIIDetection) []string {
	seen := make(map[PIIType]bool)
	types := make([]string, 0)

	for _, d := range detected {
		if !seen[d.Type] {
			seen[d.Type] = true
			types = append(types, string(d.Type))
		}
	}

	return types
}

// sortDetectionsByPosition sorts detections by start position in reverse order
// This allows safe in-place replacement from end to start
func sortDetectionsByPosition(detected []PIIDetection) {
	// Simple bubble sort (usually small number of detections)
	for i := 0; i < len(detected)-1; i++ {
		for j := i + 1; j < len(detected); j++ {
			if detected[i].Start < detected[j].Start {
				detected[i], detected[j] = detected[j], detected[i]
			}
		}
	}
}

// summarizeDetections creates a summary of detections for metadata
func summarizeDetections(detected []PIIDetection) []map[string]any {
	summary := make([]map[string]any, len(detected))
	for i, d := range detected {
		summary[i] = map[string]any{
			"type":       d.Type,
			"confidence": d.Confidence,
			"length":     d.End - d.Start,
		}
	}
	return summary
}
