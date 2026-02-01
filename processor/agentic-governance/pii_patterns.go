package agenticgovernance

import (
	"regexp"
)

// PIIPattern defines detection and redaction for a PII type
type PIIPattern struct {
	Type        PIIType
	Regex       *regexp.Regexp
	Validator   func(string) bool // Optional additional validation
	Replacement string
	Confidence  float64
}

// DefaultPIIPatterns provides common PII detection patterns
var DefaultPIIPatterns = map[PIIType]*PIIPattern{
	PIITypeEmail: {
		Type:        PIITypeEmail,
		Regex:       regexp.MustCompile(`\b[A-Za-z0-9._%+\-]+@[A-Za-z0-9.\-]+\.[A-Za-z]{2,}\b`),
		Replacement: "[EMAIL_REDACTED]",
		Confidence:  0.95,
	},
	PIITypePhone: {
		Type:        PIITypePhone,
		Regex:       regexp.MustCompile(`\b(?:\+?1[-.\s]?)?\(?([0-9]{3})\)?[-.\s]?([0-9]{3})[-.\s]?([0-9]{4})\b`),
		Replacement: "[PHONE_REDACTED]",
		Confidence:  0.90,
	},
	PIITypeSSN: {
		Type:        PIITypeSSN,
		Regex:       regexp.MustCompile(`\b\d{3}[-\s]?\d{2}[-\s]?\d{4}\b`),
		Validator:   validateSSN,
		Replacement: "[SSN_REDACTED]",
		Confidence:  0.98,
	},
	PIITypeCreditCard: {
		Type:        PIITypeCreditCard,
		Regex:       regexp.MustCompile(`\b(?:\d{4}[-\s]?){3}\d{4}\b`),
		Validator:   luhnCheck,
		Replacement: "[CARD_REDACTED]",
		Confidence:  0.92,
	},
	PIITypeAPIKey: {
		Type:        PIITypeAPIKey,
		Regex:       regexp.MustCompile(`\b(?:sk-|pk-|api[-_]?key[-_:]?\s*)[A-Za-z0-9_\-]{20,}\b`),
		Validator:   isHighEntropy,
		Replacement: "[API_KEY_REDACTED]",
		Confidence:  0.85,
	},
	PIITypeIPAddress: {
		Type:        PIITypeIPAddress,
		Regex:       regexp.MustCompile(`\b(?:[0-9]{1,3}\.){3}[0-9]{1,3}\b`),
		Validator:   validateIPv4,
		Replacement: "[IP_REDACTED]",
		Confidence:  0.90,
	},
}

// validateSSN validates a potential SSN
func validateSSN(s string) bool {
	// Check format: must be XXX-XX-XXXX or XXXXXXXXX
	formatRegex := regexp.MustCompile(`^\d{3}-\d{2}-\d{4}$|^\d{9}$`)
	if !formatRegex.MatchString(s) {
		return false
	}

	// Remove separators
	clean := regexp.MustCompile(`[-\s]`).ReplaceAllString(s, "")

	// SSN must be 9 digits
	if len(clean) != 9 {
		return false
	}

	// Check for all digits
	for _, c := range clean {
		if c < '0' || c > '9' {
			return false
		}
	}

	// Area number (first 3 digits) cannot be 000, 666, or 900-999
	area := clean[:3]
	if area == "000" || area == "666" {
		return false
	}
	if area[0] == '9' {
		return false
	}

	// Group number (middle 2 digits) cannot be 00
	if clean[3:5] == "00" {
		return false
	}

	// Serial number (last 4 digits) cannot be 0000
	if clean[5:9] == "0000" {
		return false
	}

	return true
}

// luhnCheck validates a potential credit card number using the Luhn algorithm
func luhnCheck(s string) bool {
	// Remove separators
	clean := regexp.MustCompile(`[-\s]`).ReplaceAllString(s, "")

	// Credit card must be 13-19 digits
	if len(clean) < 13 || len(clean) > 19 {
		return false
	}

	// Check for all digits
	for _, c := range clean {
		if c < '0' || c > '9' {
			return false
		}
	}

	// Luhn algorithm
	sum := 0
	alternate := false

	for i := len(clean) - 1; i >= 0; i-- {
		n := int(clean[i] - '0')

		if alternate {
			n *= 2
			if n > 9 {
				n -= 9
			}
		}

		sum += n
		alternate = !alternate
	}

	return sum%10 == 0
}

// isHighEntropy checks if a string has high entropy (likely an API key)
func isHighEntropy(s string) bool {
	if len(s) < 20 {
		return false
	}

	// Count character classes
	hasUpper := false
	hasLower := false
	hasDigit := false
	hasSpecial := false

	for _, c := range s {
		switch {
		case c >= 'A' && c <= 'Z':
			hasUpper = true
		case c >= 'a' && c <= 'z':
			hasLower = true
		case c >= '0' && c <= '9':
			hasDigit = true
		case c == '_' || c == '-':
			hasSpecial = true
		}
	}

	// High entropy strings typically have multiple character classes
	classCount := 0
	if hasUpper {
		classCount++
	}
	if hasLower {
		classCount++
	}
	if hasDigit {
		classCount++
	}
	if hasSpecial {
		classCount++
	}

	return classCount >= 3
}

// validateIPv4 validates a potential IPv4 address
func validateIPv4(s string) bool {
	// Parse octets
	octets := regexp.MustCompile(`\.`).Split(s, -1)
	if len(octets) != 4 {
		return false
	}

	for _, octet := range octets {
		if len(octet) == 0 || len(octet) > 3 {
			return false
		}

		// Check for leading zeros (except "0" itself)
		if len(octet) > 1 && octet[0] == '0' {
			return false
		}

		// Parse as integer
		n := 0
		for _, c := range octet {
			if c < '0' || c > '9' {
				return false
			}
			n = n*10 + int(c-'0')
		}

		if n > 255 {
			return false
		}
	}

	return true
}

// GetPIIPattern returns the pattern for a PII type
func GetPIIPattern(piiType PIIType) (*PIIPattern, bool) {
	pattern, ok := DefaultPIIPatterns[piiType]
	return pattern, ok
}

// CompileCustomPattern creates a PIIPattern from a definition
func CompileCustomPattern(def PIIPatternDef) (*PIIPattern, error) {
	regex, err := regexp.Compile(def.Pattern)
	if err != nil {
		return nil, err
	}

	return &PIIPattern{
		Type:        def.Type,
		Regex:       regex,
		Replacement: def.Replacement,
		Confidence:  def.Confidence,
	}, nil
}
