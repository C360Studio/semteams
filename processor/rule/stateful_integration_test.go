// Package rule - Integration tests for stateful rule evaluation
//go:build integration
// +build integration

package rule

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/c360studio/semstreams/natsclient"
)

// TestStatefulEvaluator_Integration tests the full integration with NATS KV
func TestStatefulEvaluator_Integration(t *testing.T) {
	// This test requires a running NATS server with JetStream enabled
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Create NATS test client with JetStream and KV enabled
	testClient := natsclient.NewTestClient(t, natsclient.WithJetStream(), natsclient.WithKV())
	defer testClient.Terminate()

	// Create processor with default config
	config := DefaultConfig()
	processor, err := NewProcessor(testClient.Client, &config)
	if err != nil {
		t.Fatalf("Failed to create processor: %v", err)
	}

	// Initialize processor
	if err := processor.Initialize(); err != nil {
		t.Fatalf("Failed to initialize processor: %v", err)
	}

	// Start processor - this should initialize StateTracker
	if err := processor.Start(ctx); err != nil {
		t.Fatalf("Failed to start processor: %v", err)
	}
	defer processor.Stop(5 * time.Second)

	// Verify StateTracker was initialized
	if processor.stateTracker == nil {
		t.Error("StateTracker was not initialized")
	}

	// Verify StatefulEvaluator was initialized
	if processor.statefulEvaluator == nil {
		t.Error("StatefulEvaluator was not initialized")
	}

	// Test basic state tracking
	if processor.stateTracker != nil {
		testState := MatchState{
			RuleID:         "test-rule",
			EntityKey:      "test-entity",
			IsMatching:     true,
			LastTransition: string(TransitionEntered),
			TransitionAt:   time.Now(),
			LastChecked:    time.Now(),
		}

		// Set state
		if err := processor.stateTracker.Set(ctx, testState); err != nil {
			t.Fatalf("Failed to set state: %v", err)
		}

		// Get state
		retrieved, err := processor.stateTracker.Get(ctx, "test-rule", "test-entity")
		if err != nil {
			t.Fatalf("Failed to get state: %v", err)
		}

		if !retrieved.IsMatching {
			t.Errorf("Retrieved state IsMatching = false, want true")
		}

		if retrieved.LastTransition != string(TransitionEntered) {
			t.Errorf("Retrieved state LastTransition = %v, want %v", retrieved.LastTransition, TransitionEntered)
		}

		// Clean up
		if err := processor.stateTracker.Delete(ctx, "test-rule", "test-entity"); err != nil {
			t.Fatalf("Failed to delete state: %v", err)
		}
	}
}

// TestStatefulEvaluator_StateSurvivesRestart proves that rule match state persisted
// in the RULE_STATE KV bucket survives a processor stop/start cycle. Because the
// StateTracker reads per-request from KV (not from an in-memory cache), restart
// recovery is inherently built in. This test proves that explicitly.
func TestStatefulEvaluator_StateSurvivesRestart(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	testClient := natsclient.NewTestClient(t, natsclient.WithJetStream(), natsclient.WithKV())
	defer testClient.Terminate()

	config := DefaultConfig()

	// --- First processor instance ---
	proc1, err := NewProcessor(testClient.Client, &config)
	require.NoError(t, err)
	require.NoError(t, proc1.Initialize())
	require.NoError(t, proc1.Start(ctx))

	require.NotNil(t, proc1.stateTracker, "StateTracker should be initialized")

	// Persist state with specific values we'll check after restart
	testState := MatchState{
		RuleID:         "restart-rule",
		EntityKey:      "restart-entity",
		IsMatching:     true,
		LastTransition: string(TransitionEntered),
		Iteration:      3,
		MaxIterations:  10,
		SourceRevision: 42,
		TransitionAt:   time.Now(),
		LastChecked:    time.Now(),
		FieldValues:    map[string]string{"status": "active", "priority": "high"},
	}

	require.NoError(t, proc1.stateTracker.Set(ctx, testState))

	// Verify state is readable before stop
	retrieved, err := proc1.stateTracker.Get(ctx, "restart-rule", "restart-entity")
	require.NoError(t, err)
	assert.True(t, retrieved.IsMatching)
	assert.Equal(t, 3, retrieved.Iteration)

	// Stop first processor
	require.NoError(t, proc1.Stop(5*time.Second))

	// --- Second processor instance (simulates restart) ---
	proc2, err := NewProcessor(testClient.Client, &config)
	require.NoError(t, err)
	require.NoError(t, proc2.Initialize())
	require.NoError(t, proc2.Start(ctx))
	defer proc2.Stop(5 * time.Second)

	require.NotNil(t, proc2.stateTracker, "StateTracker should be initialized on restart")

	// The new StateTracker should read the same state from KV
	restored, err := proc2.stateTracker.Get(ctx, "restart-rule", "restart-entity")
	require.NoError(t, err, "State should survive processor restart")

	assert.True(t, restored.IsMatching, "IsMatching should be preserved")
	assert.Equal(t, string(TransitionEntered), restored.LastTransition, "LastTransition should be preserved")
	assert.Equal(t, 3, restored.Iteration, "Iteration should be preserved")
	assert.Equal(t, 10, restored.MaxIterations, "MaxIterations should be preserved")
	assert.Equal(t, uint64(42), restored.SourceRevision, "SourceRevision should be preserved")
	assert.Equal(t, "active", restored.FieldValues["status"], "FieldValues should be preserved")
	assert.Equal(t, "high", restored.FieldValues["priority"], "FieldValues should be preserved")
}

// TestStatefulEvaluator_NoDuplicateFiringAfterRestart proves that when a rule's
// match state is "entered" before restart, re-evaluating the same entity (still
// matching) after restart does NOT produce a duplicate TransitionEntered event.
func TestStatefulEvaluator_NoDuplicateFiringAfterRestart(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	testClient := natsclient.NewTestClient(t, natsclient.WithJetStream(), natsclient.WithKV())
	defer testClient.Terminate()

	config := DefaultConfig()

	// --- First processor: set state as already entered ---
	proc1, err := NewProcessor(testClient.Client, &config)
	require.NoError(t, err)
	require.NoError(t, proc1.Initialize())
	require.NoError(t, proc1.Start(ctx))

	require.NotNil(t, proc1.stateTracker)

	// Set state: rule was already matching (entered), iteration 1
	testState := MatchState{
		RuleID:         "dedup-rule",
		EntityKey:      "dedup-entity",
		IsMatching:     true,
		LastTransition: string(TransitionEntered),
		Iteration:      1,
		TransitionAt:   time.Now(),
		LastChecked:    time.Now(),
	}
	require.NoError(t, proc1.stateTracker.Set(ctx, testState))

	// Stop
	require.NoError(t, proc1.Stop(5*time.Second))

	// --- Second processor: simulate re-evaluation ---
	proc2, err := NewProcessor(testClient.Client, &config)
	require.NoError(t, err)
	require.NoError(t, proc2.Initialize())
	require.NoError(t, proc2.Start(ctx))
	defer proc2.Stop(5 * time.Second)

	// Read persisted state
	restored, err := proc2.stateTracker.Get(ctx, "dedup-rule", "dedup-entity")
	require.NoError(t, err)

	// Simulate re-evaluation: entity still matches the rule condition
	wasMatching := restored.IsMatching // true
	nowMatching := true                // still matches

	transition := DetectTransition(wasMatching, nowMatching)

	// Key assertion: no duplicate "entered" transition
	assert.Equal(t, TransitionNone, transition,
		"Re-evaluating an already-matching entity should NOT produce TransitionEntered")

	// Iteration should NOT increment for TransitionNone
	assert.Equal(t, 1, restored.Iteration,
		"Iteration should stay at 1 — no re-entry occurred")
}

// TestStatefulEvaluator_ConcurrentEntityUpdates proves that concurrent writes to
// the same RuleID+EntityKey in the StateTracker do not corrupt KV state.
// NATS KV Put is unconditional (last-write-wins), so all writes should succeed.
func TestStatefulEvaluator_ConcurrentEntityUpdates(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	testClient := natsclient.NewTestClient(t, natsclient.WithJetStream(), natsclient.WithKV())
	defer testClient.Terminate()

	config := DefaultConfig()
	proc, err := NewProcessor(testClient.Client, &config)
	require.NoError(t, err)
	require.NoError(t, proc.Initialize())
	require.NoError(t, proc.Start(ctx))
	defer proc.Stop(5 * time.Second)

	require.NotNil(t, proc.stateTracker)

	const goroutines = 20
	var wg sync.WaitGroup
	wg.Add(goroutines)

	// Each goroutine writes a different SourceRevision to the same key
	for i := range goroutines {
		go func(rev int) {
			defer wg.Done()
			state := MatchState{
				RuleID:         "concurrent-rule",
				EntityKey:      "concurrent-entity",
				IsMatching:     rev%2 == 0, // alternating
				LastTransition: string(TransitionEntered),
				SourceRevision: uint64(rev),
				Iteration:      rev,
				TransitionAt:   time.Now(),
				LastChecked:    time.Now(),
			}
			// All writes should succeed — KV Put is unconditional
			err := proc.stateTracker.Set(ctx, state)
			assert.NoError(t, err, "Concurrent Set() should not error")
		}(i)
	}

	wg.Wait()

	// Final state should be one of the 20 writes — no corruption
	final, err := proc.stateTracker.Get(ctx, "concurrent-rule", "concurrent-entity")
	require.NoError(t, err, "Should read final state without error")

	// SourceRevision should be from one of our goroutines (0-19)
	assert.True(t, final.SourceRevision < uint64(goroutines),
		"SourceRevision %d should be from one of the %d goroutines", final.SourceRevision, goroutines)

	// JSON should not be corrupted — Iteration should match SourceRevision
	assert.Equal(t, int(final.SourceRevision), final.Iteration,
		"Iteration and SourceRevision should be consistent (from same write)")
}
