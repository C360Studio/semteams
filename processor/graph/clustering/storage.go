package clustering

import (
	"context"
	"encoding/json"
	stderrors "errors"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/c360/semstreams/message"
	"github.com/c360/semstreams/pkg/errs"
	"github.com/nats-io/nats.go/jetstream"
)

const (
	// CommunityBucket is the NATS KV bucket for storing communities
	CommunityBucket = "COMMUNITY_INDEX"

	// Key patterns:
	// - graph.community.{level}.{id} - Community data
	// - graph.community.entity.{level}.{entityID} - Entity -> Community mapping
)

var (
	// communityIDPattern matches community ID format: comm-{level}-{label}
	communityIDPattern = regexp.MustCompile(`^comm-(\d+)-(.+)$`)
)

// CommunityStorageConfig configures community storage behavior
type CommunityStorageConfig struct {
	// CreateTriples enables creation of member_of triples during SaveCommunity
	CreateTriples bool

	// TriplePredicate specifies the predicate to use for community membership triples
	// Default: "graph.community.member_of"
	TriplePredicate string
}

// NATSCommunityStorage implements CommunityStorage using NATS KV
type NATSCommunityStorage struct {
	kv             jetstream.KeyValue
	config         CommunityStorageConfig
	createdTriples []message.Triple      // Track created triples for testing
	testStore      map[string]*Community // In-memory store for testing when kv is nil
}

// NewNATSCommunityStorage creates a new NATS-backed community storage
// with default configuration (no triple creation)
func NewNATSCommunityStorage(kv jetstream.KeyValue) *NATSCommunityStorage {
	storage := &NATSCommunityStorage{
		kv: kv,
		config: CommunityStorageConfig{
			CreateTriples: false,
		},
		createdTriples: make([]message.Triple, 0),
	}

	// Initialize in-memory test store if KV is nil
	if kv == nil {
		storage.testStore = make(map[string]*Community)
	}

	return storage
}

// NewNATSCommunityStorageWithConfig creates a new NATS-backed community storage
// with custom configuration for triple creation
func NewNATSCommunityStorageWithConfig(kv jetstream.KeyValue, config CommunityStorageConfig) *NATSCommunityStorage {
	// Apply default predicate if not specified
	if config.CreateTriples && config.TriplePredicate == "" {
		config.TriplePredicate = "graph.community.member_of"
	}

	storage := &NATSCommunityStorage{
		kv:             kv,
		config:         config,
		createdTriples: make([]message.Triple, 0),
	}

	// Initialize in-memory test store if KV is nil
	if kv == nil {
		storage.testStore = make(map[string]*Community)
	}

	return storage
}

// SaveCommunity persists a community to NATS KV
func (s *NATSCommunityStorage) SaveCommunity(ctx context.Context, community *Community) error {
	if community == nil {
		return errs.WrapInvalid(errs.ErrMissingConfig, "NATSCommunityStorage", "SaveCommunity", "community is nil")
	}

	// If KV is available, persist to NATS
	if s.kv != nil {
		// Serialize community
		data, err := json.Marshal(community)
		if err != nil {
			return errs.WrapInvalid(err, "NATSCommunityStorage", "SaveCommunity", "marshal community")
		}

		// Store community data
		communityKey := communityKey(community.Level, community.ID)
		if _, err := s.kv.Put(ctx, communityKey, data); err != nil {
			return errs.WrapTransient(err, "NATSCommunityStorage", "SaveCommunity", "put community")
		}

		// Index entity -> community mappings
		for _, entityID := range community.Members {
			entityKey := entityCommunityKey(community.Level, entityID)
			if _, err := s.kv.Put(ctx, entityKey, []byte(community.ID)); err != nil {
				return errs.WrapTransient(err, "NATSCommunityStorage", "SaveCommunity", "put entity mapping")
			}
		}
	} else if s.testStore != nil {
		// Use in-memory store for testing
		s.testStore[community.ID] = community
	}

	// Create member_of triples if enabled
	if s.config.CreateTriples {
		triples := s.createCommunityTriples(community)
		s.createdTriples = append(s.createdTriples, triples...)
	}

	return nil
}

// createCommunityTriples generates member_of triples for a community
func (s *NATSCommunityStorage) createCommunityTriples(community *Community) []message.Triple {
	triples := make([]message.Triple, 0, len(community.Members))
	timestamp := time.Now()

	for _, memberID := range community.Members {
		triple := message.Triple{
			Subject:    memberID,
			Predicate:  s.config.TriplePredicate,
			Object:     community.ID,
			Source:     "community_detection",
			Timestamp:  timestamp,
			Confidence: 1.0,
		}
		triples = append(triples, triple)
	}

	return triples
}

// GetCreatedTriples returns all triples created during SaveCommunity operations
// This method is primarily for testing and verification purposes
func (s *NATSCommunityStorage) GetCreatedTriples() []message.Triple {
	return s.createdTriples
}

// GetCommunity retrieves a community by ID
func (s *NATSCommunityStorage) GetCommunity(ctx context.Context, id string) (*Community, error) {
	if id == "" {
		return nil, errs.WrapInvalid(errs.ErrMissingConfig, "NATSCommunityStorage", "GetCommunity", "id is empty")
	}

	// If using test store, return from memory
	if s.kv == nil && s.testStore != nil {
		community, ok := s.testStore[id]
		if !ok {
			return nil, nil
		}
		return community, nil
	}

	// If KV is nil without test store, return nil
	if s.kv == nil {
		return nil, nil
	}

	// Extract level from community ID using regex
	level, err := extractLevelFromID(id)
	if err != nil {
		return nil, errs.WrapInvalid(err, "NATSCommunityStorage", "GetCommunity", "parse community ID")
	}

	// Fetch community data
	key := communityKey(level, id)
	entry, err := s.kv.Get(ctx, key)
	if err != nil {
		if err == jetstream.ErrKeyNotFound {
			// Not found is not an error - return nil
			return nil, nil
		}
		return nil, errs.WrapTransient(err, "NATSCommunityStorage", "GetCommunity", "get community")
	}

	// Deserialize
	var community Community
	if err := json.Unmarshal(entry.Value(), &community); err != nil {
		return nil, errs.WrapInvalid(err, "NATSCommunityStorage", "GetCommunity", "unmarshal community")
	}

	return &community, nil
}

// GetCommunitiesByLevel retrieves all communities at a level
func (s *NATSCommunityStorage) GetCommunitiesByLevel(ctx context.Context, level int) ([]*Community, error) {
	// NATS KV doesn't support prefix scans in all implementations
	// We'll use Watch with prefix filter

	prefix := communityPrefix(level)
	communities := make([]*Community, 0)

	// Use Keys to get all keys with prefix
	keys, err := s.kv.Keys(ctx)
	if err != nil {
		// Empty bucket returns ErrKeyNotFound or "no keys found" error
		if stderrors.Is(err, jetstream.ErrKeyNotFound) || strings.Contains(err.Error(), "no keys found") {
			return communities, nil
		}
		return nil, errs.WrapTransient(err, "NATSCommunityStorage", "GetCommunitiesByLevel", "list keys")
	}

	for _, key := range keys {
		if !strings.HasPrefix(key, prefix) {
			continue
		}

		// Skip entity mapping keys
		if strings.Contains(key, ".entity.") {
			continue
		}

		entry, err := s.kv.Get(ctx, key)
		if err != nil {
			continue // Skip errors for individual entries
		}

		var community Community
		if err := json.Unmarshal(entry.Value(), &community); err != nil {
			continue
		}

		communities = append(communities, &community)
	}

	return communities, nil
}

// GetAllCommunities returns all communities across all levels
// Used by the LPA detector to archive enhanced communities before Clear()
func (s *NATSCommunityStorage) GetAllCommunities(ctx context.Context) ([]*Community, error) {
	// If using test store, return from memory
	if s.kv == nil && s.testStore != nil {
		communities := make([]*Community, 0, len(s.testStore))
		for _, c := range s.testStore {
			communities = append(communities, c)
		}
		return communities, nil
	}

	// If KV is nil without test store, return empty
	if s.kv == nil {
		return nil, nil
	}

	keys, err := s.kv.Keys(ctx)
	if err != nil {
		// Empty bucket returns ErrKeyNotFound or "no keys found" error
		if stderrors.Is(err, jetstream.ErrKeyNotFound) || strings.Contains(err.Error(), "no keys found") {
			return nil, nil
		}
		return nil, errs.WrapTransient(err, "NATSCommunityStorage", "GetAllCommunities", "list keys")
	}

	var communities []*Community
	for _, key := range keys {
		// Skip non-community keys
		if !strings.HasPrefix(key, "graph.community.") {
			continue
		}

		// Skip entity mapping keys (format: graph.community.entity.{level}.{entityID})
		if strings.Contains(key, ".entity.") {
			continue
		}

		entry, err := s.kv.Get(ctx, key)
		if err != nil {
			continue // Skip errors for individual entries
		}

		var community Community
		if err := json.Unmarshal(entry.Value(), &community); err != nil {
			continue // Skip unparseable entries
		}

		communities = append(communities, &community)
	}

	return communities, nil
}

// GetEntityCommunity retrieves the community for an entity at a level
func (s *NATSCommunityStorage) GetEntityCommunity(ctx context.Context, entityID string, level int) (*Community, error) {
	if entityID == "" {
		return nil, errs.WrapInvalid(errs.ErrMissingConfig, "NATSCommunityStorage", "GetEntityCommunity", "entityID is empty")
	}

	// Lookup entity -> community mapping
	key := entityCommunityKey(level, entityID)
	entry, err := s.kv.Get(ctx, key)
	if err != nil {
		if err == jetstream.ErrKeyNotFound {
			// Entity not in any community is not an error - return nil
			return nil, nil
		}
		return nil, errs.WrapTransient(err, "NATSCommunityStorage", "GetEntityCommunity", "get entity mapping")
	}

	communityID := string(entry.Value())

	// Fetch community data
	return s.GetCommunity(ctx, communityID)
}

// DeleteCommunity removes a community
func (s *NATSCommunityStorage) DeleteCommunity(ctx context.Context, id string) error {
	if id == "" {
		return errs.WrapInvalid(errs.ErrMissingConfig, "NATSCommunityStorage", "DeleteCommunity", "id is empty")
	}

	// Get community to find members
	community, err := s.GetCommunity(ctx, id)
	if err != nil {
		return err
	}

	// Handle case where community doesn't exist
	if community == nil {
		return nil // Already deleted
	}

	// Delete entity mappings - accumulate errors
	var deleteErrs []error
	for _, entityID := range community.Members {
		entityKey := entityCommunityKey(community.Level, entityID)
		if err := s.kv.Delete(ctx, entityKey); err != nil {
			deleteErrs = append(deleteErrs, fmt.Errorf("failed to delete mapping for %s: %w", entityID, err))
		}
	}

	// Delete community data
	communityKey := communityKey(community.Level, id)
	if err := s.kv.Delete(ctx, communityKey); err != nil {
		deleteErrs = append(deleteErrs, fmt.Errorf("failed to delete community: %w", err))
	}

	// Return combined error if any occurred
	if len(deleteErrs) > 0 {
		return errs.WrapTransient(
			fmt.Errorf("%d deletion errors: %v", len(deleteErrs), deleteErrs),
			"NATSCommunityStorage",
			"DeleteCommunity",
			"partial deletion failure",
		)
	}

	return nil
}

// Clear removes all communities
func (s *NATSCommunityStorage) Clear(ctx context.Context) error {
	// Purge all keys with community prefix
	keys, err := s.kv.Keys(ctx)
	if err != nil {
		// Empty bucket returns ErrKeyNotFound or "no keys found" error
		if stderrors.Is(err, jetstream.ErrKeyNotFound) || strings.Contains(err.Error(), "no keys found") {
			return nil
		}
		return errs.WrapTransient(err, "NATSCommunityStorage", "Clear", "list keys")
	}

	// Delete all community keys - accumulate errors
	var deleteErrs []error
	for _, key := range keys {
		if strings.HasPrefix(key, "graph.community.") {
			if err := s.kv.Delete(ctx, key); err != nil {
				deleteErrs = append(deleteErrs, fmt.Errorf("failed to delete %s: %w", key, err))
			}
		}
	}

	// Return combined error if any occurred
	if len(deleteErrs) > 0 {
		return errs.WrapTransient(
			fmt.Errorf("%d deletion errors: %v", len(deleteErrs), deleteErrs),
			"NATSCommunityStorage",
			"Clear",
			"partial clear failure",
		)
	}

	return nil
}

// Key generation helpers

func communityKey(level int, communityID string) string {
	return fmt.Sprintf("graph.community.%d.%s", level, communityID)
}

func communityPrefix(level int) string {
	return fmt.Sprintf("graph.community.%d.", level)
}

func entityCommunityKey(level int, entityID string) string {
	return fmt.Sprintf("graph.community.entity.%d.%s", level, entityID)
}

// extractLevelFromID parses the level from a community ID using regex
func extractLevelFromID(id string) (int, error) {
	matches := communityIDPattern.FindStringSubmatch(id)
	if len(matches) != 3 {
		return 0, fmt.Errorf("invalid community ID format: %s (expected comm-{level}-{label})", id)
	}

	level, err := strconv.Atoi(matches[1])
	if err != nil {
		return 0, fmt.Errorf("invalid level in ID: %w", err)
	}

	// Sanity check on level range
	if level < 0 || level >= 100 {
		return 0, fmt.Errorf("level out of range: %d (must be 0-99)", level)
	}

	return level, nil
}
