// Package client provides test utilities for E2E NATS validation
package client

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestNewNATSValidationClient_InvalidURL(t *testing.T) {
	// Test that invalid NATS URL returns error
	ctx := context.Background()
	client, err := NewNATSValidationClient(ctx, "invalid://not-a-nats-url")

	// Should fail to connect with invalid URL
	assert.Error(t, err)
	assert.Nil(t, client)
}

func TestNATSValidationClient_CountEntities_BucketNotExists(t *testing.T) {
	// This test verifies behavior when bucket doesn't exist
	// When run without NATS, it should return 0, nil (graceful degradation)
	ctx := context.Background()
	client, err := NewNATSValidationClient(ctx, "nats://localhost:4222")
	if err != nil {
		t.Skip("NATS not available, skipping integration test")
	}
	defer client.Close(ctx)

	// Count on non-existent bucket should return 0, nil
	count, err := client.CountEntities(ctx)
	// Either bucket exists and has count, or doesn't exist and returns 0
	assert.NoError(t, err)
	assert.GreaterOrEqual(t, count, 0)
}

func TestNATSValidationClient_GetEntity_NotFound(t *testing.T) {
	ctx := context.Background()
	client, err := NewNATSValidationClient(ctx, "nats://localhost:4222")
	if err != nil {
		t.Skip("NATS not available, skipping integration test")
	}
	defer client.Close(ctx)

	// Get non-existent entity should return nil, error
	entity, err := client.GetEntity(ctx, "non-existent-entity-id-12345")
	assert.Error(t, err)
	assert.Nil(t, entity)
}

func TestNATSValidationClient_ValidateIndexPopulated_Empty(t *testing.T) {
	ctx := context.Background()
	client, err := NewNATSValidationClient(ctx, "nats://localhost:4222")
	if err != nil {
		t.Skip("NATS not available, skipping integration test")
	}
	defer client.Close(ctx)

	// Non-existent index should return false, nil (not an error)
	populated, err := client.ValidateIndexPopulated(ctx, "NON_EXISTENT_INDEX")
	assert.NoError(t, err)
	assert.False(t, populated)
}

func TestNATSValidationClient_BucketExists(t *testing.T) {
	ctx := context.Background()
	client, err := NewNATSValidationClient(ctx, "nats://localhost:4222")
	if err != nil {
		t.Skip("NATS not available, skipping integration test")
	}
	defer client.Close(ctx)

	// Non-existent bucket should return false
	exists, err := client.BucketExists(ctx, "DEFINITELY_NOT_A_BUCKET_12345")
	assert.NoError(t, err)
	assert.False(t, exists)
}

func TestNATSValidationClient_Close(t *testing.T) {
	ctx := context.Background()
	client, err := NewNATSValidationClient(ctx, "nats://localhost:4222")
	if err != nil {
		t.Skip("NATS not available, skipping integration test")
	}

	// Close should not error
	err = client.Close(ctx)
	assert.NoError(t, err)

	// Double close should also not error
	err = client.Close(ctx)
	assert.NoError(t, err)
}

func TestNATSValidationClient_ConnectionTimeout(t *testing.T) {
	// Test connection timeout with unreachable server
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	// Try to connect to a definitely unreachable address
	client, err := NewNATSValidationClient(ctx, "nats://192.0.2.1:4222") // TEST-NET-1, not routable

	assert.Error(t, err)
	assert.Nil(t, client)
}
