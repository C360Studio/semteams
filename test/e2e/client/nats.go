// Package client provides test utilities for SemStreams E2E tests
package client

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"

	"github.com/nats-io/nats.go/jetstream"

	"github.com/c360/semstreams/natsclient"
)

// EntityState represents an entity stored in NATS KV
type EntityState struct {
	ID         string         `json:"id"`
	Type       string         `json:"type"`
	Properties map[string]any `json:"properties"`
	UpdatedAt  string         `json:"updated_at,omitempty"`
}

// NATSValidationClient wraps natsclient.Client for E2E test validation
type NATSValidationClient struct {
	client *natsclient.Client
	closed bool
	mu     sync.Mutex
}

// BucketEntityStates is the KV bucket name for entity states
const BucketEntityStates = "ENTITY_STATES"

// NewNATSValidationClient creates a new NATS validation client
func NewNATSValidationClient(ctx context.Context, natsURL string) (*NATSValidationClient, error) {
	client, err := natsclient.NewClient(natsURL)
	if err != nil {
		return nil, fmt.Errorf("failed to create NATS client: %w", err)
	}

	if err := client.Connect(ctx); err != nil {
		return nil, fmt.Errorf("failed to connect to NATS: %w", err)
	}

	return &NATSValidationClient{
		client: client,
	}, nil
}

// Close closes the NATS connection
func (c *NATSValidationClient) Close(ctx context.Context) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.closed {
		return nil
	}
	c.closed = true

	if c.client != nil {
		return c.client.Close(ctx)
	}
	return nil
}

// CountEntities counts the number of entities in the ENTITY_STATES bucket
// Returns 0, nil if bucket doesn't exist (graceful degradation)
func (c *NATSValidationClient) CountEntities(ctx context.Context) (int, error) {
	bucket, err := c.client.GetKeyValueBucket(ctx, BucketEntityStates)
	if err != nil {
		// Bucket doesn't exist - return 0, not error
		if isBucketNotFoundError(err) {
			return 0, nil
		}
		return 0, fmt.Errorf("failed to get bucket: %w", err)
	}

	keys, err := bucket.Keys(ctx)
	if err != nil {
		// Handle empty bucket
		if isNoKeysError(err) {
			return 0, nil
		}
		return 0, fmt.Errorf("failed to list keys: %w", err)
	}

	return len(keys), nil
}

// GetEntity retrieves an entity by ID from the ENTITY_STATES bucket
func (c *NATSValidationClient) GetEntity(ctx context.Context, entityID string) (*EntityState, error) {
	bucket, err := c.client.GetKeyValueBucket(ctx, BucketEntityStates)
	if err != nil {
		return nil, fmt.Errorf("failed to get bucket: %w", err)
	}

	entry, err := bucket.Get(ctx, entityID)
	if err != nil {
		return nil, fmt.Errorf("entity not found: %w", err)
	}

	var entity EntityState
	if err := json.Unmarshal(entry.Value(), &entity); err != nil {
		return nil, fmt.Errorf("failed to unmarshal entity: %w", err)
	}

	return &entity, nil
}

// ValidateIndexPopulated checks if an index bucket has entries
// Returns false, nil if bucket doesn't exist (graceful degradation)
func (c *NATSValidationClient) ValidateIndexPopulated(ctx context.Context, indexName string) (bool, error) {
	bucket, err := c.client.GetKeyValueBucket(ctx, indexName)
	if err != nil {
		// Bucket doesn't exist - return false, not error
		if isBucketNotFoundError(err) {
			return false, nil
		}
		return false, fmt.Errorf("failed to get index bucket: %w", err)
	}

	keys, err := bucket.Keys(ctx)
	if err != nil {
		// Handle empty bucket
		if isNoKeysError(err) {
			return false, nil
		}
		return false, fmt.Errorf("failed to list index keys: %w", err)
	}

	return len(keys) > 0, nil
}

// BucketExists checks if a KV bucket exists
func (c *NATSValidationClient) BucketExists(ctx context.Context, bucketName string) (bool, error) {
	_, err := c.client.GetKeyValueBucket(ctx, bucketName)
	if err != nil {
		if isBucketNotFoundError(err) {
			return false, nil
		}
		return false, fmt.Errorf("failed to check bucket: %w", err)
	}
	return true, nil
}

// ListBuckets lists all KV buckets
func (c *NATSValidationClient) ListBuckets(ctx context.Context) ([]string, error) {
	return c.client.ListKeyValueBuckets(ctx)
}

// isBucketNotFoundError checks if an error indicates a bucket doesn't exist
func isBucketNotFoundError(err error) bool {
	if err == nil {
		return false
	}
	// JetStream returns specific errors for bucket not found
	return err == jetstream.ErrBucketNotFound ||
		err == jetstream.ErrKeyNotFound
}

// isNoKeysError checks if an error indicates no keys exist
func isNoKeysError(err error) bool {
	if err == nil {
		return false
	}
	return err == jetstream.ErrNoKeysFound
}
