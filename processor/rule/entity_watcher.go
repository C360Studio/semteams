package rule

import (
	"context"
	"encoding/json"
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

			// Determine action based on operation and revision
			action := "UPDATED"
			if entry.Operation() == jetstream.KeyValueDelete {
				action = "DELETED"
			} else if entry.Revision() == 1 {
				action = "CREATED"
			}

			// Unmarshal EntityState from KV
			var entityState *gtypes.EntityState
			if entry.Operation() != jetstream.KeyValueDelete {
				var state gtypes.EntityState
				if err := json.Unmarshal(entry.Value(), &state); err != nil {
					rp.recordError(fmt.Sprintf("failed to unmarshal entity state: %v", err))
					continue
				}
				entityState = &state
			}

			// Process through rules with direct EntityState evaluation
			// This bypasses the message transformation layer
			rp.evaluateRulesForEntityState(ctx, entry.Key(), action, entityState)
		}
	}
}
