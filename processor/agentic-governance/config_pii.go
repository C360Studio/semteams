package agenticgovernance

import (
	"fmt"
	"regexp"

	"github.com/c360studio/semstreams/pkg/errs"
)

// PIIFilterConfig holds PII redaction filter configuration
type PIIFilterConfig struct {
	Types               []PIIType         `json:"types" schema:"type:array,description:PII types to detect,category:basic"`
	Strategy            RedactionStrategy `json:"strategy" schema:"type:string,description:Redaction strategy (mask hash remove label),category:basic,default:label"`
	MaskChar            string            `json:"mask_char,omitempty" schema:"type:string,description:Masking character for mask strategy,category:advanced,default:*"`
	ConfidenceThreshold float64           `json:"confidence_threshold" schema:"type:float,description:Confidence threshold (0.0-1.0),category:advanced,default:0.85"`
	AllowedTypes        []PIIType         `json:"allowed_types,omitempty" schema:"type:array,description:PII types allowed through without redaction,category:advanced"`
	CustomPatterns      []PIIPatternDef   `json:"custom_patterns,omitempty" schema:"type:array,description:Custom PII patterns,category:advanced"`
}

// PIIPatternDef defines a custom PII pattern
type PIIPatternDef struct {
	Type        PIIType `json:"type" schema:"type:string,description:PII type identifier,category:basic"`
	Pattern     string  `json:"pattern" schema:"type:string,description:Regex pattern,category:basic"`
	Replacement string  `json:"replacement" schema:"type:string,description:Replacement text,category:basic"`
	Confidence  float64 `json:"confidence" schema:"type:float,description:Detection confidence,category:advanced,default:0.90"`
}

// PIIType categorizes different kinds of PII
type PIIType string

// PII types define categories of personally identifiable information.
const (
	PIITypeEmail      PIIType = "email"
	PIITypePhone      PIIType = "phone"
	PIITypeSSN        PIIType = "ssn"
	PIITypeCreditCard PIIType = "credit_card"
	PIITypeAPIKey     PIIType = "api_key"
	PIITypeIPAddress  PIIType = "ip_address"
)

// RedactionStrategy determines how PII is handled
type RedactionStrategy string

// Redaction strategies define how PII is replaced in text.
const (
	// RedactionMask replaces characters with a masking character
	RedactionMask RedactionStrategy = "mask"

	// RedactionHash replaces PII with a deterministic hash
	RedactionHash RedactionStrategy = "hash"

	// RedactionRemove completely removes PII from text
	RedactionRemove RedactionStrategy = "remove"

	// RedactionLabel replaces PII with a labeled placeholder
	RedactionLabel RedactionStrategy = "label"
)

// Validate checks PII filter configuration
func (c *PIIFilterConfig) Validate() error {
	if c.ConfidenceThreshold < 0 || c.ConfidenceThreshold > 1 {
		return errs.WrapInvalid(fmt.Errorf("confidence_threshold must be between 0.0 and 1.0"), "PIIFilterConfig", "Validate", "validate confidence_threshold")
	}

	// Validate custom patterns compile
	for i, pattern := range c.CustomPatterns {
		if _, err := regexp.Compile(pattern.Pattern); err != nil {
			return errs.WrapInvalid(err, "PIIFilterConfig", "Validate", fmt.Sprintf("validate custom_patterns[%d] regex", i))
		}
	}

	return nil
}

// DefaultPIIConfig returns default PII filter configuration
func DefaultPIIConfig() *PIIFilterConfig {
	return &PIIFilterConfig{
		Types:               []PIIType{PIITypeEmail, PIITypePhone, PIITypeSSN, PIITypeCreditCard, PIITypeAPIKey},
		Strategy:            RedactionLabel,
		MaskChar:            "*",
		ConfidenceThreshold: 0.85,
	}
}
