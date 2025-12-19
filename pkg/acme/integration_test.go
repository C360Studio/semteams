//go:build integration
// +build integration

package acme

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

// TestACMEIntegration_Fallback tests ACME error handling when server is unreachable.
// This tests our code's error handling path, not step-ca internals.
func TestACMEIntegration_Fallback(t *testing.T) {
	// Create temporary storage directory
	tempDir := t.TempDir()
	storagePath := filepath.Join(tempDir, "acme-storage")

	// Configure ACME client with invalid URL (should fail)
	config := Config{
		DirectoryURL:  "https://invalid-step-ca:9000/acme/acme/directory",
		Email:         "test@semstreams.local",
		Domains:       []string{"semstreams.local"},
		ChallengeType: "http-01",
		RenewBefore:   8 * time.Hour,
		StoragePath:   storagePath,
	}

	// Creating client should fail (no valid ACME server)
	_, err := NewClient(config)
	assert.Error(t, err, "Should fail with invalid ACME server")
	assert.Contains(t, err.Error(), "acme.Client.initializeLegoClient", "Error should indicate ACME client init failure")
}
