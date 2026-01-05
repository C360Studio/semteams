package rule

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	gtypes "github.com/c360/semstreams/graph"
	"github.com/c360/semstreams/pkg/errs"
	"github.com/nats-io/nats.go/jetstream"
)

// watchEntityStates creates KV watchers for entity state changes
func (rp *Processor) watchEntityStates(ctx context.Context) error {
	// Skip if no entity watch patterns configured
	if len(rp.config.EntityWatchPatterns) == 0 {
		rp.logger.Info("No entity watch patterns configured, skipping KV watch setup")
		return nil
	}

	// Get ENTITY_STATES bucket
	// Create or get ENTITY_STATES bucket using resilient pattern
	entityBucket, err := rp.natsClient.CreateKeyValueBucket(ctx, jetstream.KeyValueConfig{
		Bucket:      "ENTITY_STATES",
		Description: "Entity state storage",
		History:     10,
		TTL:         7 * 24 * time.Hour, // 7 days
		MaxBytes:    -1,                 // Unlimited
	})
	if err != nil {
		rp.logger.Warn("Failed to create/access ENTITY_STATES bucket, skipping entity watch", "error", err)
		return nil // Don't fail - rules can still process messages
	}

	// Use configured patterns
	patterns := rp.config.EntityWatchPatterns

	for _, pattern := range patterns {
		watcher, err := entityBucket.Watch(ctx, pattern)
		if err != nil {
			return errs.Wrap(err, "RuleProcessor", "watchEntityStates", "create watcher")
		}

		// Store watcher for cleanup
		rp.entityWatchers = append(rp.entityWatchers, watcher)

		// Start goroutine to handle updates
		go rp.handleEntityUpdates(ctx, watcher)

		rp.logger.Info("Started KV watcher", "pattern", pattern)
	}

	return nil
}

// handleEntityUpdates processes updates from a NATS KV watcher
func (rp *Processor) handleEntityUpdates(ctx context.Context, watcher jetstream.KeyWatcher) {
	defer func() {
		if r := recover(); r != nil {
			rp.logger.Error("Panic in handleEntityUpdates", "error", r)
		}
	}()

	for {
		select {
		case <-ctx.Done():
			return
		case <-rp.shutdown:
			return
		case entry, ok := <-watcher.Updates():
			if !ok {
				// Channel closed, watcher stopped
				return
			}
			if entry == nil {
				// Nil entry indicates initial state complete, continue watching
				continue
			}

			entityKey := entry.Key()
			revision := entry.Revision()

			// Check if we generated this revision (feedback loop prevention)
			// If this update came from our own rule action, skip re-evaluation
			if rp.shouldSkipEvaluation(entityKey, revision) {
				rp.clearOwnRevision(entityKey) // One-time skip, clear after use
				rp.logger.Debug("Skipping self-generated update",
					"entity", entityKey,
					"revision", revision)
				continue
			}

			// Determine action based on operation and revision
			action := "UPDATED"
			if entry.Operation() == jetstream.KeyValueDelete {
				action = "DELETED"
			} else if revision == 1 {
				action = "CREATED"
			}

			// Handle deletion by removing from coalescer and evaluating immediately
			if action == "DELETED" {
				if rp.entityCoalescer != nil {
					rp.entityCoalescer.Remove(entityKey)
				}
				// Still evaluate rules for deletion event
				rp.evaluateRulesForEntityState(ctx, entityKey, action, nil)
				continue
			}

			// If debounce is disabled (coalescer is nil), evaluate immediately
			// Otherwise, collect entity ID for batched evaluation
			if rp.entityCoalescer == nil {
				// Bypass: unmarshal state and evaluate immediately without batching
				var state gtypes.EntityState
				if err := json.Unmarshal(entry.Value(), &state); err != nil {
					rp.logger.Warn("Failed to unmarshal entity state for immediate evaluation",
						"entity", entityKey, "error", err)
					continue
				}
				rp.evaluateRulesForEntityState(ctx, entityKey, action, &state)
			} else {
				// Batched: the coalescer will fetch current state at evaluation time
				rp.entityCoalescer.Add(entityKey)
			}
		}
	}
}

// evaluateEntitiesInBatch fetches current state and evaluates rules for a batch of entities.
// Called by CoalescingSet callback after the debounce window expires.
func (rp *Processor) evaluateEntitiesInBatch(ctx context.Context, entityIDs []string) {
	if len(entityIDs) == 0 {
		return
	}

	// Track metrics
	if rp.metrics != nil {
		rp.metrics.debounceDelaysTotal.Add(float64(len(entityIDs)))
	}

	rp.logger.Debug("Evaluating batched entities", "count", len(entityIDs))

	for _, entityID := range entityIDs {
		// Fetch current state from KV
		entityState, action, err := rp.fetchCurrentEntityState(ctx, entityID)
		if err != nil {
			rp.logger.Warn("Failed to fetch entity state for rule evaluation",
				"entityID", entityID, "error", err)
			continue
		}

		// Evaluate rules against current state
		rp.evaluateRulesForEntityState(ctx, entityID, action, entityState)
	}
}

// fetchCurrentEntityState retrieves the current state of an entity from KV.
// Returns the state, action type (CREATED/UPDATED/DELETED), and any error.
func (rp *Processor) fetchCurrentEntityState(ctx context.Context, entityID string) (*gtypes.EntityState, string, error) {
	// Get ENTITY_STATES bucket (should already be available from watchEntityStates)
	entityBucket, err := rp.natsClient.GetKeyValueBucket(ctx, "ENTITY_STATES")
	if err != nil {
		return nil, "", fmt.Errorf("get ENTITY_STATES bucket: %w", err)
	}

	entry, err := entityBucket.Get(ctx, entityID)
	if err != nil {
		if errors.Is(err, jetstream.ErrKeyNotFound) {
			// Entity was deleted between add and evaluation
			return nil, "DELETED", nil
		}
		return nil, "", fmt.Errorf("get entity state: %w", err)
	}

	var state gtypes.EntityState
	if err := json.Unmarshal(entry.Value(), &state); err != nil {
		return nil, "", fmt.Errorf("unmarshal entity state: %w", err)
	}

	action := "UPDATED"
	if entry.Revision() == 1 {
		action = "CREATED"
	}

	return &state, action, nil
}
