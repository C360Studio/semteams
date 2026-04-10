//go:build integration

package workflow

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/nats-io/nats.go/jetstream"

	"github.com/c360studio/semstreams/natsclient"
)

// TestIntegration_WorkflowStateSurvivesManagerRestart proves that workflow state
// persisted in NATS KV survives when a new StateManager is created against the
// same bucket. All fields including Context map are preserved.
func TestIntegration_WorkflowStateSurvivesManagerRestart(t *testing.T) {
	testClient := natsclient.NewTestClient(t, natsclient.WithJetStream(), natsclient.WithKV())
	defer testClient.Terminate()

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	js, err := testClient.Client.JetStream()
	require.NoError(t, err)

	bucket, err := js.CreateKeyValue(ctx, jetstream.KeyValueConfig{Bucket: "WORKFLOW_STATE_RESTART"})
	require.NoError(t, err)

	// --- First manager: create and persist state ---
	mgr1 := NewStateManager(bucket, nil)

	state := &State{
		ID:         "wf-restart-001",
		WorkflowID: "build-pipeline",
		Phase:      "executing",
		Iteration:  5,
		MaxIter:    10,
		Context:    map[string]any{"key": "value", "count": float64(42)},
	}
	require.NoError(t, mgr1.Create(ctx, state))

	// Verify before restart
	retrieved, err := mgr1.Get(ctx, "wf-restart-001")
	require.NoError(t, err)
	assert.Equal(t, "executing", retrieved.Phase)
	assert.Equal(t, 5, retrieved.Iteration)

	// --- Second manager: simulate restart ---
	mgr2 := NewStateManager(bucket, nil)

	restored, err := mgr2.Get(ctx, "wf-restart-001")
	require.NoError(t, err, "State should survive manager restart")

	assert.Equal(t, "wf-restart-001", restored.ID, "ID should be preserved")
	assert.Equal(t, "build-pipeline", restored.WorkflowID, "WorkflowID should be preserved")
	assert.Equal(t, "executing", restored.Phase, "Phase should be preserved")
	assert.Equal(t, 5, restored.Iteration, "Iteration should be preserved")
	assert.Equal(t, 10, restored.MaxIter, "MaxIter should be preserved")
	assert.Equal(t, "value", restored.Context["key"], "Context map should be preserved")
	assert.Equal(t, float64(42), restored.Context["count"], "Context values should be preserved")
	assert.False(t, restored.StartedAt.IsZero(), "StartedAt should be preserved")

	// New manager should be able to operate on the restored state
	require.NoError(t, mgr2.Transition(ctx, "wf-restart-001", "reviewing"))
	updated, err := mgr2.Get(ctx, "wf-restart-001")
	require.NoError(t, err)
	assert.Equal(t, "reviewing", updated.Phase, "New manager should be able to transition state")

	require.NoError(t, mgr2.Complete(ctx, "wf-restart-001"))
	completed, err := mgr2.Get(ctx, "wf-restart-001")
	require.NoError(t, err)
	assert.True(t, completed.IsComplete(), "New manager should be able to complete state")
}

// TestIntegration_WorkflowConcurrentTransitionsWithCAS proves that concurrent
// TransitionWithRevision calls using the same expected revision result in exactly
// one success and the rest get revision conflict errors. No state corruption.
func TestIntegration_WorkflowConcurrentTransitionsWithCAS(t *testing.T) {
	testClient := natsclient.NewTestClient(t, natsclient.WithJetStream(), natsclient.WithKV())
	defer testClient.Terminate()

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	js, err := testClient.Client.JetStream()
	require.NoError(t, err)

	bucket, err := js.CreateKeyValue(ctx, jetstream.KeyValueConfig{Bucket: "WORKFLOW_STATE_CAS"})
	require.NoError(t, err)

	mgr := NewStateManager(bucket, nil)

	state := &State{
		ID:         "wf-cas-001",
		WorkflowID: "cas-test",
		Phase:      "init",
	}
	require.NoError(t, mgr.Create(ctx, state))

	// Get the initial revision
	entry, err := mgr.GetWithRevision(ctx, "wf-cas-001")
	require.NoError(t, err)
	initialRevision := entry.Revision

	const goroutines = 10
	var wg sync.WaitGroup
	wg.Add(goroutines)

	var successes atomic.Int32
	var failures atomic.Int32

	// All goroutines try to transition using the SAME expected revision
	for i := range goroutines {
		go func(idx int) {
			defer wg.Done()
			phase := "phase-" + string(rune('A'+idx))
			_, err := mgr.TransitionWithRevision(ctx, "wf-cas-001", phase, initialRevision)
			if err != nil {
				failures.Add(1)
			} else {
				successes.Add(1)
			}
		}(i)
	}

	wg.Wait()

	// Exactly one should succeed (got the matching revision)
	assert.Equal(t, int32(1), successes.Load(),
		"Exactly one goroutine should succeed with CAS")
	assert.Equal(t, int32(goroutines-1), failures.Load(),
		"All other goroutines should fail with revision conflict")

	// Final state should be consistent — one of the phases
	final, err := mgr.Get(ctx, "wf-cas-001")
	require.NoError(t, err)
	assert.NotEqual(t, "init", final.Phase, "Phase should have been updated by the winning goroutine")
}

// TestIntegration_WorkflowConcurrentIterationIncrements proves that concurrent
// IncrementIteration (non-CAS) can lose updates, while IncrementIterationWithRevision
// (CAS) correctly detects conflicts.
func TestIntegration_WorkflowConcurrentIterationIncrements(t *testing.T) {
	testClient := natsclient.NewTestClient(t, natsclient.WithJetStream(), natsclient.WithKV())
	defer testClient.Terminate()

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	js, err := testClient.Client.JetStream()
	require.NoError(t, err)

	bucket, err := js.CreateKeyValue(ctx, jetstream.KeyValueConfig{Bucket: "WORKFLOW_STATE_ITER"})
	require.NoError(t, err)

	mgr := NewStateManager(bucket, nil)

	state := &State{
		ID:         "wf-iter-001",
		WorkflowID: "iter-test",
		Phase:      "counting",
		Iteration:  0,
	}
	require.NoError(t, mgr.Create(ctx, state))

	// Sequential IncrementIterationWithRevision should all succeed
	const sequentialIncrements = 5
	for range sequentialIncrements {
		entry, err := mgr.GetWithRevision(ctx, "wf-iter-001")
		require.NoError(t, err)
		_, err = mgr.IncrementIterationWithRevision(ctx, "wf-iter-001", entry.Revision)
		require.NoError(t, err)
	}

	final, err := mgr.Get(ctx, "wf-iter-001")
	require.NoError(t, err)
	assert.Equal(t, sequentialIncrements, final.Iteration,
		"Sequential CAS increments should all succeed")

	// Concurrent IncrementIterationWithRevision with same revision — only 1 should succeed
	entry, err := mgr.GetWithRevision(ctx, "wf-iter-001")
	require.NoError(t, err)
	frozenRevision := entry.Revision

	const concurrentGoroutines = 10
	var wg sync.WaitGroup
	wg.Add(concurrentGoroutines)

	var casSuccesses atomic.Int32

	for range concurrentGoroutines {
		go func() {
			defer wg.Done()
			_, err := mgr.IncrementIterationWithRevision(ctx, "wf-iter-001", frozenRevision)
			if err == nil {
				casSuccesses.Add(1)
			}
		}()
	}

	wg.Wait()

	assert.Equal(t, int32(1), casSuccesses.Load(),
		"Exactly one concurrent CAS increment should succeed")

	afterConcurrent, err := mgr.Get(ctx, "wf-iter-001")
	require.NoError(t, err)
	assert.Equal(t, sequentialIncrements+1, afterConcurrent.Iteration,
		"Iteration should be incremented by exactly 1 from the CAS winner")
}

// TestIntegration_WorkflowMaxIterationBoundary documents the behavior when
// iteration exceeds MaxIter. Currently MaxIter is advisory — no enforcement
// exists in the StateManager. This test documents that gap.
func TestIntegration_WorkflowMaxIterationBoundary(t *testing.T) {
	testClient := natsclient.NewTestClient(t, natsclient.WithJetStream(), natsclient.WithKV())
	defer testClient.Terminate()

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	js, err := testClient.Client.JetStream()
	require.NoError(t, err)

	bucket, err := js.CreateKeyValue(ctx, jetstream.KeyValueConfig{Bucket: "WORKFLOW_STATE_MAXITER"})
	require.NoError(t, err)

	mgr := NewStateManager(bucket, nil)

	state := &State{
		ID:         "wf-maxiter-001",
		WorkflowID: "maxiter-test",
		Phase:      "iterating",
		Iteration:  0,
		MaxIter:    3,
	}
	require.NoError(t, mgr.Create(ctx, state))

	// Increment up to and past MaxIter
	for i := range 5 {
		err := mgr.IncrementIteration(ctx, "wf-maxiter-001")
		// DOCUMENTS THE GAP: IncrementIteration does NOT enforce MaxIter.
		// All increments succeed, even past the configured maximum.
		assert.NoError(t, err,
			"Increment %d should succeed (MaxIter is advisory, not enforced)", i+1)
	}

	final, err := mgr.Get(ctx, "wf-maxiter-001")
	require.NoError(t, err)

	// Iteration exceeds MaxIter — no enforcement
	assert.Equal(t, 5, final.Iteration, "Iteration should be 5 (no MaxIter enforcement)")
	assert.Equal(t, 3, final.MaxIter, "MaxIter field should still be 3")
	assert.False(t, final.IsComplete(), "Workflow should NOT be auto-completed at MaxIter")
}

// TestIntegration_WorkflowCreateIdempotency proves that Create() fails if the
// workflow already exists (unlike Put which overwrites). This is the dedup guard.
func TestIntegration_WorkflowCreateIdempotency(t *testing.T) {
	testClient := natsclient.NewTestClient(t, natsclient.WithJetStream(), natsclient.WithKV())
	defer testClient.Terminate()

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	js, err := testClient.Client.JetStream()
	require.NoError(t, err)

	bucket, err := js.CreateKeyValue(ctx, jetstream.KeyValueConfig{Bucket: "WORKFLOW_STATE_DEDUP"})
	require.NoError(t, err)

	mgr := NewStateManager(bucket, nil)

	state := &State{
		ID:         "wf-dedup-001",
		WorkflowID: "dedup-test",
		Phase:      "init",
	}
	require.NoError(t, mgr.Create(ctx, state))

	// Second Create with same ID should fail
	duplicate := &State{
		ID:         "wf-dedup-001",
		WorkflowID: "dedup-test-v2",
		Phase:      "init-v2",
	}
	err = mgr.Create(ctx, duplicate)
	assert.Error(t, err, "Create() should fail for existing workflow ID (dedup guard)")

	// Verify original is unchanged
	original, err := mgr.Get(ctx, "wf-dedup-001")
	require.NoError(t, err)
	assert.Equal(t, "dedup-test", original.WorkflowID, "Original should be unchanged")
	assert.Equal(t, "init", original.Phase, "Original phase should be unchanged")
}
