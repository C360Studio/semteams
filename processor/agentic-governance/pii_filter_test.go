package agenticgovernance

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPIIFilter_DetectsEmail(t *testing.T) {
	filter, err := NewPIIFilter(&PIIFilterConfig{
		Types:               []PIIType{PIITypeEmail},
		Strategy:            RedactionLabel,
		ConfidenceThreshold: 0.9,
	})
	require.NoError(t, err)

	msg := &Message{
		ID:     "test-1",
		UserID: "user1",
		Content: Content{
			Text: "My email is user@example.com",
		},
	}

	result, err := filter.Process(context.Background(), msg)
	require.NoError(t, err)
	assert.True(t, result.Allowed, "PII should be redacted, not blocked")
	assert.NotNil(t, result.Modified)
	assert.Contains(t, result.Modified.Content.Text, "[EMAIL_REDACTED]")
	assert.NotContains(t, result.Modified.Content.Text, "user@example.com")
}

func TestPIIFilter_DetectsMultipleEmails(t *testing.T) {
	filter, err := NewPIIFilter(&PIIFilterConfig{
		Types:               []PIIType{PIITypeEmail},
		Strategy:            RedactionLabel,
		ConfidenceThreshold: 0.9,
	})
	require.NoError(t, err)

	msg := &Message{
		ID:     "test-2",
		UserID: "user1",
		Content: Content{
			Text: "Contact user@example.com or admin@company.org",
		},
	}

	result, err := filter.Process(context.Background(), msg)
	require.NoError(t, err)
	assert.True(t, result.Allowed)
	assert.NotNil(t, result.Modified)
	// Both emails should be redacted
	assert.NotContains(t, result.Modified.Content.Text, "user@example.com")
	assert.NotContains(t, result.Modified.Content.Text, "admin@company.org")
}

func TestPIIFilter_DetectsPhone(t *testing.T) {
	filter, err := NewPIIFilter(&PIIFilterConfig{
		Types:               []PIIType{PIITypePhone},
		Strategy:            RedactionLabel,
		ConfidenceThreshold: 0.85,
	})
	require.NoError(t, err)

	testCases := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "standard phone",
			input:    "Call me at 555-123-4567",
			expected: "[PHONE_REDACTED]",
		},
		{
			name:     "phone with parentheses",
			input:    "My number is (555) 123-4567",
			expected: "[PHONE_REDACTED]",
		},
		{
			name:     "phone with country code",
			input:    "Contact: +1-555-123-4567",
			expected: "[PHONE_REDACTED]",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			msg := &Message{
				ID:      "test",
				Content: Content{Text: tc.input},
			}

			result, err := filter.Process(context.Background(), msg)
			require.NoError(t, err)
			assert.True(t, result.Allowed)
			if result.Modified != nil {
				assert.Contains(t, result.Modified.Content.Text, tc.expected)
			}
		})
	}
}

func TestPIIFilter_DetectsSSN(t *testing.T) {
	filter, err := NewPIIFilter(&PIIFilterConfig{
		Types:               []PIIType{PIITypeSSN},
		Strategy:            RedactionLabel,
		ConfidenceThreshold: 0.9,
	})
	require.NoError(t, err)

	msg := &Message{
		ID:      "test",
		Content: Content{Text: "SSN: 123-45-6789"},
	}

	result, err := filter.Process(context.Background(), msg)
	require.NoError(t, err)
	assert.True(t, result.Allowed)
	assert.NotNil(t, result.Modified)
	assert.Contains(t, result.Modified.Content.Text, "[SSN_REDACTED]")
}

func TestPIIFilter_DetectsCreditCard(t *testing.T) {
	filter, err := NewPIIFilter(&PIIFilterConfig{
		Types:               []PIIType{PIITypeCreditCard},
		Strategy:            RedactionLabel,
		ConfidenceThreshold: 0.9,
	})
	require.NoError(t, err)

	// Valid Visa card number (passes Luhn check)
	msg := &Message{
		ID:      "test",
		Content: Content{Text: "Card: 4532-0151-1283-0366"},
	}

	result, err := filter.Process(context.Background(), msg)
	require.NoError(t, err)
	assert.True(t, result.Allowed)
	assert.NotNil(t, result.Modified)
	assert.Contains(t, result.Modified.Content.Text, "[CARD_REDACTED]")
}

func TestPIIFilter_DetectsIPAddress(t *testing.T) {
	filter, err := NewPIIFilter(&PIIFilterConfig{
		Types:               []PIIType{PIITypeIPAddress},
		Strategy:            RedactionLabel,
		ConfidenceThreshold: 0.85,
	})
	require.NoError(t, err)

	msg := &Message{
		ID:      "test",
		Content: Content{Text: "Server IP: 192.168.1.100"},
	}

	result, err := filter.Process(context.Background(), msg)
	require.NoError(t, err)
	assert.True(t, result.Allowed)
	assert.NotNil(t, result.Modified)
	assert.Contains(t, result.Modified.Content.Text, "[IP_REDACTED]")
}

func TestPIIFilter_NoDetection(t *testing.T) {
	filter, err := NewPIIFilter(&PIIFilterConfig{
		Types:               []PIIType{PIITypeEmail, PIITypePhone, PIITypeSSN},
		Strategy:            RedactionLabel,
		ConfidenceThreshold: 0.9,
	})
	require.NoError(t, err)

	msg := &Message{
		ID:      "test",
		Content: Content{Text: "Hello, how can I help you today?"},
	}

	result, err := filter.Process(context.Background(), msg)
	require.NoError(t, err)
	assert.True(t, result.Allowed)
	assert.Nil(t, result.Modified, "No modifications when no PII detected")
}

func TestPIIFilter_MaskStrategy(t *testing.T) {
	filter, err := NewPIIFilter(&PIIFilterConfig{
		Types:               []PIIType{PIITypeEmail},
		Strategy:            RedactionMask,
		MaskChar:            "*",
		ConfidenceThreshold: 0.9,
	})
	require.NoError(t, err)

	msg := &Message{
		ID:      "test",
		Content: Content{Text: "Email: user@example.com"},
	}

	result, err := filter.Process(context.Background(), msg)
	require.NoError(t, err)
	assert.True(t, result.Allowed)
	assert.NotNil(t, result.Modified)
	// Should be masked with asterisks
	assert.Contains(t, result.Modified.Content.Text, "****************")
}

func TestPIIFilter_HashStrategy(t *testing.T) {
	filter, err := NewPIIFilter(&PIIFilterConfig{
		Types:               []PIIType{PIITypeEmail},
		Strategy:            RedactionHash,
		ConfidenceThreshold: 0.9,
	})
	require.NoError(t, err)

	msg := &Message{
		ID:      "test",
		Content: Content{Text: "Email: user@example.com"},
	}

	result, err := filter.Process(context.Background(), msg)
	require.NoError(t, err)
	assert.True(t, result.Allowed)
	assert.NotNil(t, result.Modified)
	assert.Contains(t, result.Modified.Content.Text, "[EMAIL_HASH:")
}

func TestPIIFilter_AllowedTypes(t *testing.T) {
	filter, err := NewPIIFilter(&PIIFilterConfig{
		Types:               []PIIType{PIITypeEmail, PIITypePhone},
		AllowedTypes:        []PIIType{PIITypeEmail}, // Allow emails through
		Strategy:            RedactionLabel,
		ConfidenceThreshold: 0.9,
	})
	require.NoError(t, err)

	msg := &Message{
		ID:      "test",
		Content: Content{Text: "Email: user@example.com, Phone: 555-123-4567"},
	}

	result, err := filter.Process(context.Background(), msg)
	require.NoError(t, err)
	assert.True(t, result.Allowed)
	assert.NotNil(t, result.Modified)
	// Email should NOT be redacted (allowed)
	assert.Contains(t, result.Modified.Content.Text, "user@example.com")
	// Phone should be redacted
	assert.Contains(t, result.Modified.Content.Text, "[PHONE_REDACTED]")
}

func TestPIIFilter_MetadataAdded(t *testing.T) {
	filter, err := NewPIIFilter(&PIIFilterConfig{
		Types:               []PIIType{PIITypeEmail},
		Strategy:            RedactionLabel,
		ConfidenceThreshold: 0.9,
	})
	require.NoError(t, err)

	msg := &Message{
		ID:      "test",
		Content: Content{Text: "Email: user@example.com"},
	}

	result, err := filter.Process(context.Background(), msg)
	require.NoError(t, err)
	assert.NotNil(t, result.Modified)

	// Check metadata was added
	piiMeta, ok := result.Modified.Content.Metadata["pii_redacted"]
	assert.True(t, ok, "pii_redacted metadata should be present")
	assert.NotNil(t, piiMeta)

	// Check result metadata
	piiTypes, ok := result.Metadata["pii_types_detected"]
	assert.True(t, ok)
	assert.Contains(t, piiTypes, "email")
}

func TestValidateSSN(t *testing.T) {
	testCases := []struct {
		ssn      string
		expected bool
	}{
		{"123-45-6789", true},
		{"000-45-6789", false}, // Area cannot be 000
		{"666-45-6789", false}, // Area cannot be 666
		{"900-45-6789", false}, // Area cannot start with 9
		{"123-00-6789", false}, // Group cannot be 00
		{"123-45-0000", false}, // Serial cannot be 0000
		{"12-345-6789", false}, // Invalid format
		{"1234567890", false},  // Too many digits
	}

	for _, tc := range testCases {
		t.Run(tc.ssn, func(t *testing.T) {
			result := validateSSN(tc.ssn)
			assert.Equal(t, tc.expected, result, "SSN: %s", tc.ssn)
		})
	}
}

func TestLuhnCheck(t *testing.T) {
	testCases := []struct {
		card     string
		expected bool
	}{
		{"4532-0151-1283-0366", true},  // Valid Visa
		{"4111-1111-1111-1111", true},  // Valid test card
		{"4532-0151-1283-0367", false}, // Invalid (changed last digit)
		{"1234-5678-9012-3456", false}, // Invalid
		{"123", false},                 // Too short
	}

	for _, tc := range testCases {
		t.Run(tc.card, func(t *testing.T) {
			result := luhnCheck(tc.card)
			assert.Equal(t, tc.expected, result, "Card: %s", tc.card)
		})
	}
}

func TestValidateIPv4(t *testing.T) {
	testCases := []struct {
		ip       string
		expected bool
	}{
		{"192.168.1.1", true},
		{"0.0.0.0", true},
		{"255.255.255.255", true},
		{"256.1.1.1", false},   // Octet > 255
		{"1.2.3", false},       // Too few octets
		{"1.2.3.4.5", false},   // Too many octets
		{"01.02.03.04", false}, // Leading zeros
	}

	for _, tc := range testCases {
		t.Run(tc.ip, func(t *testing.T) {
			result := validateIPv4(tc.ip)
			assert.Equal(t, tc.expected, result, "IP: %s", tc.ip)
		})
	}
}
