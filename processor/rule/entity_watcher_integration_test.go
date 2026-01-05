//go:build integration

// Integration tests for entity watcher with rule debouncing
// Builder implements these tests - they are NOT locked.
package rule_test

import (
	"context"
	"encoding/json"
	"sync"
	"testing"
	"time"

	"github.com/nats-io/nats.go/jetstream"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	gtypes "github.com/c360/semstreams/graph"
	"github.com/c360/semstreams/message"
	"github.com/c360/semstreams/natsclient"
	"github.com/c360/semstreams/processor/rule"
	"github.com/c360/semstreams/processor/rule/expression"
)

// TestEntityWatcher_RuleTriggerDebouncing verifies that rapid entity updates
// are debounced and rules are evaluated once against the final stable state.
func TestEntityWatcher_RuleTriggerDebouncing(t *testing.T) {
	// Setup NATS client
	testClient, err := natsclient.NewSharedTestClient(
		natsclient.WithJetStream(),
		natsclient.WithKV(),
		natsclient.WithTestTimeout(5*time.Second),
		natsclient.WithStartTimeout(30*time.Second),
	)
	require.NoError(t, err)
	t.Cleanup(func() {
		testClient.Terminate()
	})

	natsClient := testClient.Client
	ctx := context.Background()

	// Create ENTITY_STATES KV bucket
	js, err := natsClient.JetStream()
	require.NoError(t, err)

	kv, err := js.KeyValue(ctx, "ENTITY_STATES")
	if err != nil {
		kv, err = js.CreateKeyValue(ctx, jetstream.KeyValueConfig{
			Bucket:      "ENTITY_STATES",
			Description: "Entity state storage for debounce test",
		})
		require.NoError(t, err)
	}

	// Create a rule that triggers when temperature exceeds 75
	// Uses expression conditions instead of custom rule type
	ruleDef := rule.Definition{
		ID:   "temp_threshold_debounce",
		Type: "expression",
		Name: "Temperature Threshold (Debounced)",
		Conditions: []expression.ConditionExpression{
			{
				Field:    "temperature",
				Operator: "gt",
				Value:    75.0,
				Required: true,
			},
		},
		Logic:   "and",
		Enabled: true,
		Entity: rule.EntityConfig{
			Pattern: "test.debounce.>",
		},
	}

	// Create processor with debouncing (100ms default)
	config := rule.DefaultConfig()
	config.EntityWatchPatterns = []string{"test.debounce.>"}
	config.DebounceDelayMs = 100 * time.Millisecond
	config.InlineRules = []rule.Definition{ruleDef}

	processor, err := rule.NewProcessorWithMetrics(natsClient, &config, nil)
	require.NoError(t, err)

	// Start processor
	err = processor.Start(ctx)
	require.NoError(t, err)
	t.Cleanup(func() {
		processor.Stop(5 * time.Second)
	})

	// Subscribe to rule events to count triggers
	var triggerCount int64
	var triggerMu sync.Mutex

	err = natsClient.Subscribe(ctx, "events.rule.triggered", func(_ context.Context, data []byte) {
		triggerMu.Lock()
		triggerCount++
		triggerMu.Unlock()
	})
	require.NoError(t, err)

	// Wait for KV watcher to initialize
	time.Sleep(500 * time.Millisecond)

	// Test Case 1: Multiple rapid updates coalesce to one evaluation
	t.Run("rapid_updates_coalesce", func(t *testing.T) {
		// Reset trigger count for this test
		triggerMu.Lock()
		triggerCount = 0
		triggerMu.Unlock()

		entityID := "test.debounce.sensor1"

		// Send 5 rapid updates (faster than debounce delay)
		for i := 0; i < 5; i++ {
			state := createEntityStateForDebounce(entityID, 60.0+float64(i)*5.0) // 60, 65, 70, 75, 80
			stateJSON, err := json.Marshal(state)
			require.NoError(t, err)

			_, err = kv.Put(ctx, entityID, stateJSON)
			require.NoError(t, err)

			time.Sleep(20 * time.Millisecond) // Less than debounce delay
		}

		// Wait for debounce to fire (100ms + buffer)
		time.Sleep(200 * time.Millisecond)

		// Should trigger only once with final state (80 > 75)
		triggerMu.Lock()
		triggers := triggerCount
		triggerMu.Unlock()

		assert.Equal(t, int64(1), triggers, "Expected exactly 1 trigger (final value 80 > 75)")
	})

	// Test Case 2: Entity deletion during settling period cancels evaluation
	t.Run("deletion_cancels_pending_evaluation", func(t *testing.T) {
		// Reset trigger count for this test
		triggerMu.Lock()
		triggerCount = 0
		triggerMu.Unlock()

		entityID := "test.debounce.sensor2"

		// Send update
		state := createEntityStateForDebounce(entityID, 80.0)
		stateJSON, err := json.Marshal(state)
		require.NoError(t, err)

		_, err = kv.Put(ctx, entityID, stateJSON)
		require.NoError(t, err)

		// Delete before debounce fires
		time.Sleep(50 * time.Millisecond) // Half the debounce delay
		err = kv.Delete(ctx, entityID)
		require.NoError(t, err)

		// Wait past debounce period
		time.Sleep(150 * time.Millisecond)

		// No trigger should have occurred (evaluation canceled)
		triggerMu.Lock()
		triggers := triggerCount
		triggerMu.Unlock()

		assert.Equal(t, int64(0), triggers, "Expected no trigger after deletion cancels debounce")
	})

	// Test Case 3: Updates after settling trigger new evaluation
	t.Run("new_updates_after_settling", func(t *testing.T) {
		// Reset trigger count for this test
		triggerMu.Lock()
		triggerCount = 0
		triggerMu.Unlock()

		entityID := "test.debounce.sensor3"

		// First update with temp > 75
		state := createEntityStateForDebounce(entityID, 80.0)
		stateJSON, err := json.Marshal(state)
		require.NoError(t, err)

		_, err = kv.Put(ctx, entityID, stateJSON)
		require.NoError(t, err)

		// Wait for debounce
		time.Sleep(200 * time.Millisecond)

		triggerMu.Lock()
		triggers1 := triggerCount
		triggerMu.Unlock()
		assert.Equal(t, int64(1), triggers1, "First update should trigger")

		// Second update after settling
		state = createEntityStateForDebounce(entityID, 85.0)
		stateJSON, err = json.Marshal(state)
		require.NoError(t, err)

		_, err = kv.Put(ctx, entityID, stateJSON)
		require.NoError(t, err)

		// Wait for second debounce
		time.Sleep(200 * time.Millisecond)

		triggerMu.Lock()
		triggers2 := triggerCount
		triggerMu.Unlock()
		assert.Equal(t, int64(2), triggers2, "Second update should trigger new evaluation")
	})
}

// createEntityStateForDebounce creates a test EntityState with temperature triple
// Uses correct types: message.Triple with Object field (not ObjectValue)
func createEntityStateForDebounce(entityID string, temperature float64) *gtypes.EntityState {
	return &gtypes.EntityState{
		ID: entityID,
		Triples: []message.Triple{
			{
				Subject:   entityID,
				Predicate: "temperature",
				Object:    temperature,
			},
		},
		Version:   1,
		UpdatedAt: time.Now(),
	}
}
