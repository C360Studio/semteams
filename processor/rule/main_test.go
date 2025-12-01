package rule

import (
	"os"
	"testing"
)

// TestMain runs all tests. Integration tests that need NATS should check
// INTEGRATION_TESTS env var themselves and skip if not set.
// This allows unit tests to run without the env var.
func TestMain(m *testing.M) {
	os.Exit(m.Run())
}
