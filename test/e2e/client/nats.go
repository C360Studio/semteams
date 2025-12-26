// Package client provides test utilities for SemStreams E2E tests
package client

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"github.com/nats-io/nats.go/jetstream"

	"github.com/c360/semstreams/natsclient"
)

// EntityState represents an entity stored in NATS KV
type EntityState struct {
	ID         string         `json:"id"`
	Type       string         `json:"type"`
	Properties map[string]any `json:"properties"`
	Triples    []Triple       `json:"triples,omitempty"`
	Version    int            `json:"version"`
	UpdatedAt  string         `json:"updated_at,omitempty"`
}

// Triple represents a semantic triple (subject, predicate, object)
type Triple struct {
	Subject   string `json:"subject"`
	Predicate string `json:"predicate"`
	Object    any    `json:"object"`
}

// Community represents a detected community/cluster for E2E testing
type Community struct {
	ID                 string                 `json:"id"`
	Level              int                    `json:"level"`
	Members            []string               `json:"members"`
	ParentID           *string                `json:"parent_id,omitempty"`
	StatisticalSummary string                 `json:"statistical_summary,omitempty"`
	LLMSummary         string                 `json:"llm_summary,omitempty"`
	Keywords           []string               `json:"keywords,omitempty"`
	RepEntities        []string               `json:"rep_entities,omitempty"`
	SummaryStatus      string                 `json:"summary_status,omitempty"`
	Metadata           map[string]interface{} `json:"metadata,omitempty"`
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

// CountBucketKeys counts the number of keys in a specific KV bucket
// Returns 0, nil if bucket doesn't exist (graceful degradation)
func (c *NATSValidationClient) CountBucketKeys(ctx context.Context, bucketName string) (int, error) {
	bucket, err := c.client.GetKeyValueBucket(ctx, bucketName)
	if err != nil {
		if isBucketNotFoundError(err) {
			return 0, nil
		}
		return 0, fmt.Errorf("failed to get bucket %s: %w", bucketName, err)
	}

	keys, err := bucket.Keys(ctx)
	if err != nil {
		if isNoKeysError(err) {
			return 0, nil
		}
		return 0, fmt.Errorf("failed to list keys in %s: %w", bucketName, err)
	}

	return len(keys), nil
}

// GetBucketKeysSample returns a sample of keys from a bucket (first n keys)
// Useful for verifying key patterns without loading all data
func (c *NATSValidationClient) GetBucketKeysSample(ctx context.Context, bucketName string, limit int) ([]string, error) {
	bucket, err := c.client.GetKeyValueBucket(ctx, bucketName)
	if err != nil {
		if isBucketNotFoundError(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to get bucket %s: %w", bucketName, err)
	}

	keys, err := bucket.Keys(ctx)
	if err != nil {
		if isNoKeysError(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to list keys in %s: %w", bucketName, err)
	}

	if len(keys) <= limit {
		return keys, nil
	}
	return keys[:limit], nil
}

// GetEntitySample returns a sample of entities from ENTITY_STATES bucket
// Used for entity structure validation in E2E tests
func (c *NATSValidationClient) GetEntitySample(ctx context.Context, limit int) ([]*EntityState, error) {
	bucket, err := c.client.GetKeyValueBucket(ctx, BucketEntityStates)
	if err != nil {
		if isBucketNotFoundError(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to get entity states bucket: %w", err)
	}

	keys, err := bucket.Keys(ctx)
	if err != nil {
		if isNoKeysError(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to list entity keys: %w", err)
	}

	// Limit the sample size
	sampleSize := limit
	if len(keys) < limit {
		sampleSize = len(keys)
	}

	entities := make([]*EntityState, 0, sampleSize)
	for i := 0; i < sampleSize; i++ {
		entry, err := bucket.Get(ctx, keys[i])
		if err != nil {
			// Skip entities that can't be retrieved
			continue
		}

		var entity EntityState
		if err := json.Unmarshal(entry.Value(), &entity); err != nil {
			// Skip entities that can't be unmarshaled
			continue
		}

		entities = append(entities, &entity)
	}

	return entities, nil
}

// IndexBuckets defines the standard index bucket names
var IndexBuckets = struct {
	EntityStates  string
	Predicate     string
	Incoming      string
	Outgoing      string
	Alias         string
	Spatial       string
	Temporal      string
	Embedding     string
	EmbeddingDedp string
	Community     string
}{
	EntityStates:  "ENTITY_STATES",
	Predicate:     "PREDICATE_INDEX",
	Incoming:      "INCOMING_INDEX",
	Outgoing:      "OUTGOING_INDEX",
	Alias:         "ALIAS_INDEX",
	Spatial:       "SPATIAL_INDEX",
	Temporal:      "TEMPORAL_INDEX",
	Embedding:     "EMBEDDING_INDEX",
	EmbeddingDedp: "EMBEDDING_DEDUP",
	Community:     "COMMUNITY_INDEX",
}

// GetAllCommunities retrieves all communities from the COMMUNITY_INDEX bucket
// Used for comparing statistical vs LLM-enhanced summaries in E2E tests
func (c *NATSValidationClient) GetAllCommunities(ctx context.Context) ([]*Community, error) {
	bucket, err := c.client.GetKeyValueBucket(ctx, IndexBuckets.Community)
	if err != nil {
		if isBucketNotFoundError(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to get community bucket: %w", err)
	}

	keys, err := bucket.Keys(ctx)
	if err != nil {
		if isNoKeysError(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to list community keys: %w", err)
	}

	var communities []*Community
	for _, key := range keys {
		// Skip entity-to-community index entries (they have different structure)
		// Community keys have format: "graph.community.L{level}.{id}"
		// Entity index keys have format: "graph.community.entity.{entityID}"
		if len(key) > 22 && key[:22] == "graph.community.entity" {
			continue
		}

		entry, err := bucket.Get(ctx, key)
		if err != nil {
			// Skip entries that can't be retrieved
			continue
		}

		var comm Community
		if err := json.Unmarshal(entry.Value(), &comm); err != nil {
			// Skip entries that can't be unmarshaled as communities
			continue
		}

		// Only include valid communities (have ID and members)
		if comm.ID != "" && len(comm.Members) > 0 {
			communities = append(communities, &comm)
		}
	}

	return communities, nil
}

// WaitForCommunityEnhancement polls communities until all reach terminal status
// Terminal statuses: "llm-enhanced" or "llm-failed"
// Returns counts of enhanced, failed, and pending communities
func (c *NATSValidationClient) WaitForCommunityEnhancement(
	ctx context.Context,
	timeout time.Duration,
	pollInterval time.Duration,
) (enhanced, failed, pending int, err error) {
	deadline := time.Now().Add(timeout)

	for time.Now().Before(deadline) {
		communities, err := c.GetAllCommunities(ctx)
		if err != nil {
			return 0, 0, 0, fmt.Errorf("failed to get communities: %w", err)
		}

		// If no communities exist yet, wait and retry
		if len(communities) == 0 {
			select {
			case <-ctx.Done():
				return 0, 0, 0, ctx.Err()
			case <-time.After(pollInterval):
				continue
			}
		}

		enhanced, failed, pending = 0, 0, 0
		for _, comm := range communities {
			switch comm.SummaryStatus {
			case "llm-enhanced":
				enhanced++
			case "llm-failed":
				failed++
			default: // "statistical" or empty
				pending++
			}
		}

		// All communities reached terminal status
		if pending == 0 {
			return enhanced, failed, pending, nil
		}

		select {
		case <-ctx.Done():
			return enhanced, failed, pending, ctx.Err()
		case <-time.After(pollInterval):
		}
	}

	// Timeout reached, return current state without error
	return enhanced, failed, pending, nil
}

// StructuralIndexBucket is the KV bucket for structural indices
const StructuralIndexBucket = "STRUCTURAL_INDEX"

// KCoreMetadata contains k-core index metadata
type KCoreMetadata struct {
	MaxCore     int         `json:"max_core"`
	EntityCount int         `json:"entity_count"`
	ComputedAt  string      `json:"computed_at"`
	CoreBuckets map[int]int `json:"core_buckets"` // core number -> count of entities
}

// PivotMetadata contains pivot distance index metadata
type PivotMetadata struct {
	Pivots      []string `json:"pivots"`
	EntityCount int      `json:"entity_count"`
	ComputedAt  string   `json:"computed_at"`
}

// StructuralIndexInfo contains information about structural indices
type StructuralIndexInfo struct {
	BucketExists bool           `json:"bucket_exists"`
	KeyCount     int            `json:"key_count"`
	KCore        *KCoreMetadata `json:"kcore,omitempty"`
	Pivot        *PivotMetadata `json:"pivot,omitempty"`
	SampleKeys   []string       `json:"sample_keys,omitempty"`
}

// GetStructuralIndexInfo retrieves information about structural indices
func (c *NATSValidationClient) GetStructuralIndexInfo(ctx context.Context) (*StructuralIndexInfo, error) {
	info := &StructuralIndexInfo{}

	bucket, err := c.client.GetKeyValueBucket(ctx, StructuralIndexBucket)
	if err != nil {
		if isBucketNotFoundError(err) {
			info.BucketExists = false
			return info, nil
		}
		return nil, fmt.Errorf("failed to get structural index bucket: %w", err)
	}
	info.BucketExists = true

	// Get key count
	keys, err := bucket.Keys(ctx)
	if err != nil && !isNoKeysError(err) {
		return nil, fmt.Errorf("failed to list structural index keys: %w", err)
	}
	info.KeyCount = len(keys)

	// Get sample keys
	if len(keys) > 5 {
		info.SampleKeys = keys[:5]
	} else {
		info.SampleKeys = keys
	}

	// Try to get k-core metadata
	kcoreMeta, err := bucket.Get(ctx, "structural.kcore._meta")
	if err == nil {
		var meta struct {
			MaxCore     int      `json:"max_core"`
			EntityCount int      `json:"entity_count"`
			ComputedAt  string   `json:"computed_at"`
			CoreBuckets []string `json:"core_buckets"`
		}
		if json.Unmarshal(kcoreMeta.Value(), &meta) == nil {
			info.KCore = &KCoreMetadata{
				MaxCore:     meta.MaxCore,
				EntityCount: meta.EntityCount,
				ComputedAt:  meta.ComputedAt,
				CoreBuckets: make(map[int]int),
			}
			// Count entities per core by looking up bucket keys
			for i := 0; i <= meta.MaxCore; i++ {
				coreKey := fmt.Sprintf("structural.kcore.core.%d", i)
				if entry, err := bucket.Get(ctx, coreKey); err == nil {
					var entities []string
					if json.Unmarshal(entry.Value(), &entities) == nil {
						info.KCore.CoreBuckets[i] = len(entities)
					}
				}
			}
		}
	}

	// Try to get pivot metadata
	pivotMeta, err := bucket.Get(ctx, "structural.pivot._meta")
	if err == nil {
		var meta struct {
			Pivots      []string `json:"pivots"`
			EntityCount int      `json:"entity_count"`
			ComputedAt  string   `json:"computed_at"`
		}
		if json.Unmarshal(pivotMeta.Value(), &meta) == nil {
			info.Pivot = &PivotMetadata{
				Pivots:      meta.Pivots,
				EntityCount: meta.EntityCount,
				ComputedAt:  meta.ComputedAt,
			}
		}
	}

	return info, nil
}
