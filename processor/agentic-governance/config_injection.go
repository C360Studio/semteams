package agenticgovernance

import (
	"fmt"
	"regexp"
)

// InjectionFilterConfig holds injection detection filter configuration
type InjectionFilterConfig struct {
	ConfidenceThreshold float64               `json:"confidence_threshold" schema:"type:float,description:Confidence threshold for blocking (0.0-1.0),category:basic,default:0.80"`
	Patterns            []InjectionPatternDef `json:"patterns,omitempty" schema:"type:array,description:Injection patterns to detect,category:advanced"`
	EnabledPatterns     []string              `json:"enabled_patterns,omitempty" schema:"type:array,description:Built-in pattern names to enable,category:basic"`
}

// InjectionPatternDef defines an injection detection pattern
type InjectionPatternDef struct {
	Name        string   `json:"name" schema:"type:string,description:Pattern identifier,category:basic"`
	Pattern     string   `json:"pattern" schema:"type:string,description:Regex pattern,category:basic"`
	Description string   `json:"description" schema:"type:string,description:Pattern description,category:basic"`
	Severity    Severity `json:"severity" schema:"type:string,description:Violation severity,category:basic,default:high"`
	Confidence  float64  `json:"confidence" schema:"type:float,description:Detection confidence,category:advanced,default:0.90"`
}

// Severity levels for violations
type Severity string

// Severity levels define the threat level of violations.
const (
	SeverityCritical Severity = "critical"
	SeverityHigh     Severity = "high"
	SeverityMedium   Severity = "medium"
	SeverityLow      Severity = "low"
)

// Validate checks injection filter configuration
func (c *InjectionFilterConfig) Validate() error {
	if c.ConfidenceThreshold < 0 || c.ConfidenceThreshold > 1 {
		return fmt.Errorf("confidence_threshold must be between 0.0 and 1.0")
	}

	// Validate patterns compile
	for i, pattern := range c.Patterns {
		if _, err := regexp.Compile(pattern.Pattern); err != nil {
			return fmt.Errorf("patterns[%d]: invalid regex: %w", i, err)
		}
	}

	return nil
}

// DefaultInjectionConfig returns default injection filter configuration
func DefaultInjectionConfig() *InjectionFilterConfig {
	return &InjectionFilterConfig{
		ConfidenceThreshold: 0.80,
		EnabledPatterns:     []string{"instruction_override", "jailbreak_persona", "system_injection", "delimiter_injection", "role_confusion"},
	}
}
