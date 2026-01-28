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
	// Store the watcher context for dynamic management
	rp.mu.Lock()
	rp.watcherCtx, rp.watcherCancelFunc = context.WithCancel(ctx)
	watcherCtx := rp.watcherCtx
	rp.mu.Unlock()

	// Skip if no entity watch patterns configured
	if len(rp.config.EntityWatchPatterns) == 0 {
		rp.logger.Info("No entity watch patterns configured, skipping KV watch setup")
		return nil
	}

	// Start watchers for all configured patterns
	for _, pattern := range rp.config.EntityWatchPatterns {
		if err := rp.startWatcherForPattern(watcherCtx, pattern); err != nil {
			rp.logger.Warn("Failed to start watcher for pattern", "pattern", pattern, "error", err)
			// Continue with other patterns - don't fail completely
		}
	}

	return nil
}

// getOrCreateEntityBucket gets or creates the ENTITY_STATES KV bucket
func (rp *Processor) getOrCreateEntityBucket(ctx context.Context) (jetstream.KeyValue, error) {
	return rp.natsClient.CreateKeyValueBucket(ctx, jetstream.KeyValueConfig{
		Bucket:      "ENTITY_STATES",
		Description: "Entity state storage",
		History:     10,
		TTL:         7 * 24 * time.Hour, // 7 days
		MaxBytes:    -1,                 // Unlimited
	})
}

// startWatcherForPattern starts a KV watcher for a specific pattern
// Returns an error if the watcher cannot be started
func (rp *Processor) startWatcherForPattern(ctx context.Context, pattern string) error {
	rp.mu.Lock()
	defer rp.mu.Unlock()

	// Check if watcher already exists for this pattern
	if _, exists := rp.entityWatcherMap[pattern]; exists {
		rp.logger.Debug("Watcher already exists for pattern", "pattern", pattern)
		return nil
	}

	// Get ENTITY_STATES bucket
	entityBucket, err := rp.getOrCreateEntityBucket(ctx)
	if err != nil {
		return fmt.Errorf("failed to get ENTITY_STATES bucket: %w", err)
	}

	watcher, err := entityBucket.Watch(ctx, pattern)
	if err != nil {
		return errs.Wrap(err, "RuleProcessor", "startWatcherForPattern", "create watcher")
	}

	// Store watcher in both slice (for legacy cleanup) and map (for dynamic management)
	rp.entityWatchers = append(rp.entityWatchers, watcher)
	rp.entityWatcherMap[pattern] = watcher

	// Start goroutine to handle updates
	go rp.handleEntityUpdates(ctx, watcher)

	rp.logger.Info("Started KV watcher", "pattern", pattern)
	return nil
}

// stopWatcherForPattern stops a KV watcher for a specific pattern
func (rp *Processor) stopWatcherForPattern(pattern string) error {
	rp.mu.Lock()
	defer rp.mu.Unlock()

	watcher, exists := rp.entityWatcherMap[pattern]
	if !exists {
		rp.logger.Debug("No watcher exists for pattern", "pattern", pattern)
		return nil
	}

	// Stop the watcher
	if err := watcher.Stop(); err != nil {
		rp.logger.Warn("Error stopping watcher for pattern", "pattern", pattern, "error", err)
		// Continue with cleanup even if stop fails
	}

	// Remove from map
	delete(rp.entityWatcherMap, pattern)

	// Remove from slice (find and remove)
	for i, w := range rp.entityWatchers {
		if w == watcher {
			rp.entityWatchers = append(rp.entityWatchers[:i], rp.entityWatchers[i+1:]...)
			break
		}
	}

	rp.logger.Info("Stopped KV watcher", "pattern", pattern)
	return nil
}

// UpdateWatchPatterns dynamically updates the entity watch patterns
// It stops watchers for removed patterns and starts watchers for new patterns
func (rp *Processor) UpdateWatchPatterns(newPatterns []string) error {
	rp.mu.RLock()
	watcherCtx := rp.watcherCtx
	currentPatterns := make(map[string]bool)
	for pattern := range rp.entityWatcherMap {
		currentPatterns[pattern] = true
	}
	rp.mu.RUnlock()

	// If no watcher context, processor not started yet - just update config
	if watcherCtx == nil {
		rp.mu.Lock()
		rp.config.EntityWatchPatterns = newPatterns
		rp.mu.Unlock()
		rp.logger.Info("Updated entity watch patterns (processor not running)", "patterns", newPatterns)
		return nil
	}

	// Build set of new patterns
	newPatternSet := make(map[string]bool)
	for _, p := range newPatterns {
		newPatternSet[p] = true
	}

	// Stop watchers for removed patterns
	for pattern := range currentPatterns {
		if !newPatternSet[pattern] {
			if err := rp.stopWatcherForPattern(pattern); err != nil {
				rp.logger.Warn("Failed to stop watcher for removed pattern", "pattern", pattern, "error", err)
			}
		}
	}

	// Start watchers for new patterns
	for pattern := range newPatternSet {
		if !currentPatterns[pattern] {
			if err := rp.startWatcherForPattern(watcherCtx, pattern); err != nil {
				rp.logger.Warn("Failed to start watcher for new pattern", "pattern", pattern, "error", err)
				// Continue with other patterns
			}
		}
	}

	// Update config
	rp.mu.Lock()
	rp.config.EntityWatchPatterns = newPatterns
	rp.mu.Unlock()

	rp.logger.Info("Updated entity watch patterns dynamically",
		"added", len(newPatternSet)-len(currentPatterns),
		"removed", len(currentPatterns)-len(newPatternSet),
		"total", len(newPatterns))

	return nil
}

// handleEntityUpdates processes updates from a NATS KV watcher
func (rp *Processor) handleEntityUpdates(ctx context.Context, watcher jetstream.KeyWatcher) {
	defer func() {
		if r := recover(); r != nil {
			rp.logger.Error("Panic in handleEntityUpdates", "error", r)
		}
	}()
	// NOTE: watcher.Stop() is called explicitly before each return, not via defer.
	// This avoids a race condition in nats.go where Stop() can race with the
	// internal message handler goroutine when using defer or calling from another goroutine.

	for {
		select {
		case <-ctx.Done():
			watcher.Stop()
			return
		case <-rp.shutdown:
			watcher.Stop()
			return
		case entry, ok := <-watcher.Updates():
			if !ok {
				// Channel closed, watcher stopped externally
				watcher.Stop()
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
