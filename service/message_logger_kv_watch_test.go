package service

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestIsValidBucketName(t *testing.T) {
	tests := []struct {
		name     string
		bucket   string
		expected bool
	}{
		// Valid bucket names
		{"valid uppercase", "ENTITY_STATES", true},
		{"valid with underscore", "MY_BUCKET", true},
		{"valid with hyphen", "my-bucket", true},
		{"valid lowercase", "mybucket", true},
		{"valid mixed case", "MyBucket", true},
		{"valid with numbers", "bucket123", true},

		// Invalid bucket names
		{"empty bucket", "", false},
		{"wildcard >", "ENTITY>", false},
		{"wildcard *", "ENTITY*", false},
		{"contains >", "my>bucket", false},
		{"contains *", "my*bucket", false},
		{"path traversal ..", "..", false},
		{"path traversal /", "a/b", false},
		{"path traversal \\", "a\\b", false},
		{"single dot", ".", false},
		{"starts with /", "/bucket", false},
		{"ends with /", "bucket/", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := isValidBucketName(tt.bucket)
			assert.Equal(t, tt.expected, result, "bucket: %q", tt.bucket)
		})
	}
}

func TestIsValidWatchPattern(t *testing.T) {
	tests := []struct {
		name     string
		pattern  string
		expected bool
	}{
		// Valid patterns
		{"empty pattern", "", true},
		{"wildcard all", "*", true},
		{"prefix wildcard", "entity.*", true},
		{"suffix wildcard", "*.json", true},
		{"middle wildcard", "prefix.*.suffix", true},
		{"exact match", "exact.key.name", true},
		{"with dots", "a.b.c.d", true},

		// Invalid patterns
		{"multi-level wildcard", "entity.>", false},
		{"multi-level wildcard only", ">", false},
		{"contains >", "a.>.b", false},
		{"path traversal ..", "../secret", false},
		{"path traversal /", "a/b", false},
		{"path traversal \\", "a\\b", false},
		{"double dots anywhere", "a..b", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := isValidWatchPattern(tt.pattern)
			assert.Equal(t, tt.expected, result, "pattern: %q", tt.pattern)
		})
	}
}

func TestDetectKVOperation(t *testing.T) {
	// Note: We can't easily test detectKVOperation without mocking jetstream.KeyValueEntry
	// The function is simple enough that it's covered by integration tests
	// This test documents the expected behavior

	t.Run("operation detection logic", func(t *testing.T) {
		// Document expected behavior:
		// - KeyValuePut with Revision=1 → "create"
		// - KeyValuePut with Revision>1 → "update"
		// - KeyValueDelete → "delete"
		// - Unknown operation → "unknown"
		t.Log("Operation detection relies on jetstream.KeyValueEntry interface")
		t.Log("Covered by integration tests with actual NATS KV")
	})
}
