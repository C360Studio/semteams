package reactive

import (
	"context"
	"encoding/json"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/nats-io/nats.go/jetstream"
)

// storeTestState is a typed state struct for testing that embeds ExecutionState.
// Named differently to avoid conflict with TestState in evaluator_test.go.
type storeTestState struct {
	ExecutionState
	CustomField string `json:"custom_field"`
	Counter     int    `json:"counter"`
}

func (s *storeTestState) GetExecutionState() *ExecutionState {
	return &s.ExecutionState
}

// storeKVEntry implements jetstream.KeyValueEntry for store tests.
// Named differently to avoid conflict with mockKVEntry in dispatcher_test.go.
type storeKVEntry struct {
	key       string
	value     []byte
	revision  uint64
	created   time.Time
	operation jetstream.KeyValueOp
}

func (e *storeKVEntry) Key() string                     { return e.key }
func (e *storeKVEntry) Value() []byte                   { return e.value }
func (e *storeKVEntry) Revision() uint64                { return e.revision }
func (e *storeKVEntry) Created() time.Time              { return e.created }
func (e *storeKVEntry) Operation() jetstream.KeyValueOp { return e.operation }
func (e *storeKVEntry) Bucket() string                  { return "test-bucket" }
func (e *storeKVEntry) Delta() uint64                   { return 0 }

// storeKVBucket implements a minimal in-memory KV bucket for store tests.
type storeKVBucket struct {
	mu      sync.RWMutex
	entries map[string]*storeKVEntry
	nextRev uint64
	watcher *storeKVWatcher
}

func newStoreKVBucket() *storeKVBucket {
	return &storeKVBucket{
		entries: make(map[string]*storeKVEntry),
		nextRev: 1,
	}
}

func (m *storeKVBucket) Get(_ context.Context, key string) (jetstream.KeyValueEntry, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	entry, ok := m.entries[key]
	if !ok {
		return nil, jetstream.ErrKeyNotFound
	}
	return entry, nil
}

func (m *storeKVBucket) Create(_ context.Context, key string, value []byte, _ ...jetstream.KVCreateOpt) (uint64, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if _, exists := m.entries[key]; exists {
		return 0, jetstream.ErrKeyExists
	}

	rev := m.nextRev
	m.nextRev++

	m.entries[key] = &storeKVEntry{
		key:       key,
		value:     value,
		revision:  rev,
		created:   time.Now(),
		operation: jetstream.KeyValuePut,
	}

	m.notifyWatcher(m.entries[key])
	return rev, nil
}

func (m *storeKVBucket) Update(_ context.Context, key string, value []byte, revision uint64) (uint64, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	entry, ok := m.entries[key]
	if !ok {
		return 0, jetstream.ErrKeyNotFound
	}

	if entry.revision != revision {
		return 0, jetstream.ErrKeyExists // JetStream uses this for revision mismatch
	}

	rev := m.nextRev
	m.nextRev++

	m.entries[key] = &storeKVEntry{
		key:       key,
		value:     value,
		revision:  rev,
		created:   entry.created,
		operation: jetstream.KeyValuePut,
	}

	m.notifyWatcher(m.entries[key])
	return rev, nil
}

func (m *storeKVBucket) Delete(_ context.Context, key string, _ ...jetstream.KVDeleteOpt) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	entry, ok := m.entries[key]
	if !ok {
		return jetstream.ErrKeyNotFound
	}

	rev := m.nextRev
	m.nextRev++

	deleteEntry := &storeKVEntry{
		key:       key,
		value:     nil,
		revision:  rev,
		created:   entry.created,
		operation: jetstream.KeyValueDelete,
	}

	delete(m.entries, key)
	m.notifyWatcher(deleteEntry)
	return nil
}

func (m *storeKVBucket) Keys(_ context.Context, _ ...jetstream.WatchOpt) ([]string, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if len(m.entries) == 0 {
		return nil, jetstream.ErrNoKeysFound
	}

	keys := make([]string, 0, len(m.entries))
	for k := range m.entries {
		keys = append(keys, k)
	}
	return keys, nil
}

func (m *storeKVBucket) Watch(_ context.Context, _ string, _ ...jetstream.WatchOpt) (jetstream.KeyWatcher, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.watcher = &storeKVWatcher{
		updates: make(chan jetstream.KeyValueEntry, 100),
	}

	// Send nil to indicate initial state complete
	m.watcher.updates <- nil

	return m.watcher, nil
}

func (m *storeKVBucket) notifyWatcher(entry *storeKVEntry) {
	if m.watcher != nil {
		select {
		case m.watcher.updates <- entry:
		default:
		}
	}
}

// Implement other KeyValue methods as no-ops or panics for test completeness
func (m *storeKVBucket) Bucket() string { return "test-bucket" }
func (m *storeKVBucket) Put(_ context.Context, _ string, _ []byte) (uint64, error) {
	panic("not implemented")
}
func (m *storeKVBucket) PutString(_ context.Context, _ string, _ string) (uint64, error) {
	panic("not implemented")
}
func (m *storeKVBucket) GetRevision(_ context.Context, _ string, _ uint64) (jetstream.KeyValueEntry, error) {
	panic("not implemented")
}
func (m *storeKVBucket) Purge(_ context.Context, _ string, _ ...jetstream.KVDeleteOpt) error {
	panic("not implemented")
}
func (m *storeKVBucket) History(_ context.Context, _ string, _ ...jetstream.WatchOpt) ([]jetstream.KeyValueEntry, error) {
	panic("not implemented")
}
func (m *storeKVBucket) WatchAll(_ context.Context, _ ...jetstream.WatchOpt) (jetstream.KeyWatcher, error) {
	panic("not implemented")
}
func (m *storeKVBucket) WatchFiltered(_ context.Context, _ []string, _ ...jetstream.WatchOpt) (jetstream.KeyWatcher, error) {
	panic("not implemented")
}
func (m *storeKVBucket) ListKeys(_ context.Context, _ ...jetstream.WatchOpt) (jetstream.KeyLister, error) {
	panic("not implemented")
}
func (m *storeKVBucket) ListKeysFiltered(_ context.Context, _ ...string) (jetstream.KeyLister, error) {
	panic("not implemented")
}
func (m *storeKVBucket) PurgeDeletes(_ context.Context, _ ...jetstream.KVPurgeOpt) error {
	panic("not implemented")
}
func (m *storeKVBucket) Status(_ context.Context) (jetstream.KeyValueStatus, error) {
	panic("not implemented")
}

type storeKVWatcher struct {
	updates chan jetstream.KeyValueEntry
	stopped bool
}

func (w *storeKVWatcher) Updates() <-chan jetstream.KeyValueEntry {
	return w.updates
}

func (w *storeKVWatcher) Stop() error {
	w.stopped = true
	close(w.updates)
	return nil
}

// Test helpers
func storeTestStateFactory() any {
	return &storeTestState{}
}

func TestNewExecutionStore(t *testing.T) {
	bucket := newStoreKVBucket()

	t.Run("creates store with defaults", func(t *testing.T) {
		store := NewExecutionStore(nil, bucket)

		if store == nil {
			t.Fatal("expected store to be created")
		}
		if store.logger == nil {
			t.Error("expected logger to be set to default")
		}
		if store.bucket != bucket {
			t.Error("expected bucket to be set")
		}
	})

	t.Run("creates store with options", func(t *testing.T) {
		store := NewExecutionStore(nil, bucket,
			WithKeyPrefix("test."),
			WithStoreStateFactory(storeTestStateFactory),
		)

		if store.keyPrefix != "test." {
			t.Errorf("expected keyPrefix 'test.', got %q", store.keyPrefix)
		}
		if store.stateFactory == nil {
			t.Error("expected stateFactory to be set")
		}
	})
}

func TestCreateExecution(t *testing.T) {
	t.Run("creates execution successfully", func(t *testing.T) {
		bucket := newStoreKVBucket()
		store := NewExecutionStore(nil, bucket,
			WithKeyPrefix("workflow."),
			WithStoreStateFactory(storeTestStateFactory),
		)

		entry, err := store.CreateExecution(context.Background(), "exec-1", "plan-review", 5*time.Minute)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if entry.Key != "workflow.exec-1" {
			t.Errorf("expected key 'workflow.exec-1', got %q", entry.Key)
		}
		if entry.Revision == 0 {
			t.Error("expected non-zero revision")
		}

		state, ok := entry.State.(*storeTestState)
		if !ok {
			t.Fatalf("expected *storeTestState, got %T", entry.State)
		}
		if state.ID != "exec-1" {
			t.Errorf("expected ID 'exec-1', got %q", state.ID)
		}
		if state.WorkflowID != "plan-review" {
			t.Errorf("expected WorkflowID 'plan-review', got %q", state.WorkflowID)
		}
		if state.Status != StatusRunning {
			t.Errorf("expected status Running, got %q", state.Status)
		}
		if state.Deadline == nil {
			t.Error("expected deadline to be set")
		}
	})

	t.Run("fails without state factory", func(t *testing.T) {
		bucket := newStoreKVBucket()
		store := NewExecutionStore(nil, bucket)

		_, err := store.CreateExecution(context.Background(), "exec-1", "wf", time.Minute)
		if err == nil {
			t.Fatal("expected error without state factory")
		}

		var storeErr *StoreError
		if !errors.As(err, &storeErr) {
			t.Fatalf("expected StoreError, got %T", err)
		}
		if storeErr.Op != "create" {
			t.Errorf("expected op 'create', got %q", storeErr.Op)
		}
	})

	t.Run("fails when execution already exists", func(t *testing.T) {
		bucket := newStoreKVBucket()
		store := NewExecutionStore(nil, bucket,
			WithStoreStateFactory(storeTestStateFactory),
		)

		// Create first execution
		_, err := store.CreateExecution(context.Background(), "exec-1", "wf", time.Minute)
		if err != nil {
			t.Fatalf("first create failed: %v", err)
		}

		// Try to create again
		_, err = store.CreateExecution(context.Background(), "exec-1", "wf", time.Minute)
		if err == nil {
			t.Fatal("expected error for duplicate execution")
		}

		var storeErr *StoreError
		if !errors.As(err, &storeErr) {
			t.Fatalf("expected StoreError, got %T", err)
		}
		if storeErr.Message != "execution already exists" {
			t.Errorf("unexpected message: %q", storeErr.Message)
		}
	})
}

func TestLoadExecution(t *testing.T) {
	t.Run("loads existing execution", func(t *testing.T) {
		bucket := newStoreKVBucket()
		store := NewExecutionStore(nil, bucket,
			WithKeyPrefix("wf."),
			WithStoreStateFactory(storeTestStateFactory),
		)

		// Create execution
		created, err := store.CreateExecution(context.Background(), "exec-1", "wf", time.Minute)
		if err != nil {
			t.Fatalf("create failed: %v", err)
		}

		// Load it back
		loaded, err := store.LoadExecution(context.Background(), "exec-1")
		if err != nil {
			t.Fatalf("load failed: %v", err)
		}

		if loaded.Key != created.Key {
			t.Errorf("key mismatch: %q vs %q", loaded.Key, created.Key)
		}
		if loaded.Revision != created.Revision {
			t.Errorf("revision mismatch: %d vs %d", loaded.Revision, created.Revision)
		}

		state, ok := loaded.State.(*storeTestState)
		if !ok {
			t.Fatalf("expected *storeTestState, got %T", loaded.State)
		}
		if state.ID != "exec-1" {
			t.Errorf("expected ID 'exec-1', got %q", state.ID)
		}
	})

	t.Run("returns nil for non-existent execution", func(t *testing.T) {
		bucket := newStoreKVBucket()
		store := NewExecutionStore(nil, bucket,
			WithStoreStateFactory(storeTestStateFactory),
		)

		entry, err := store.LoadExecution(context.Background(), "non-existent")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if entry != nil {
			t.Error("expected nil entry for non-existent execution")
		}
	})

	t.Run("fails without state factory", func(t *testing.T) {
		bucket := newStoreKVBucket()
		// Create with factory
		store1 := NewExecutionStore(nil, bucket,
			WithStoreStateFactory(storeTestStateFactory),
		)
		_, _ = store1.CreateExecution(context.Background(), "exec-1", "wf", time.Minute)

		// Try to load without factory
		store2 := NewExecutionStore(nil, bucket)
		_, err := store2.LoadExecution(context.Background(), "exec-1")
		if err == nil {
			t.Fatal("expected error without state factory")
		}
	})
}

func TestSaveExecution(t *testing.T) {
	t.Run("saves with correct revision", func(t *testing.T) {
		bucket := newStoreKVBucket()
		store := NewExecutionStore(nil, bucket,
			WithStoreStateFactory(storeTestStateFactory),
		)

		// Create execution
		entry, err := store.CreateExecution(context.Background(), "exec-1", "wf", time.Minute)
		if err != nil {
			t.Fatalf("create failed: %v", err)
		}

		// Modify state
		state := entry.State.(*storeTestState)
		state.CustomField = "updated"
		state.Counter = 42

		// Save
		newRev, err := store.SaveExecution(context.Background(), entry.Key, state, entry.Revision)
		if err != nil {
			t.Fatalf("save failed: %v", err)
		}
		if newRev <= entry.Revision {
			t.Errorf("expected new revision > %d, got %d", entry.Revision, newRev)
		}

		// Verify by loading
		loaded, err := store.LoadExecution(context.Background(), "exec-1")
		if err != nil {
			t.Fatalf("load failed: %v", err)
		}
		loadedState := loaded.State.(*storeTestState)
		if loadedState.CustomField != "updated" {
			t.Errorf("expected CustomField 'updated', got %q", loadedState.CustomField)
		}
		if loadedState.Counter != 42 {
			t.Errorf("expected Counter 42, got %d", loadedState.Counter)
		}
	})

	t.Run("fails with wrong revision", func(t *testing.T) {
		bucket := newStoreKVBucket()
		store := NewExecutionStore(nil, bucket,
			WithStoreStateFactory(storeTestStateFactory),
		)

		entry, err := store.CreateExecution(context.Background(), "exec-1", "wf", time.Minute)
		if err != nil {
			t.Fatalf("create failed: %v", err)
		}

		// Try to save with wrong revision
		_, err = store.SaveExecution(context.Background(), entry.Key, entry.State, entry.Revision+100)
		if err == nil {
			t.Fatal("expected error with wrong revision")
		}

		var storeErr *StoreError
		if !errors.As(err, &storeErr) {
			t.Fatalf("expected StoreError, got %T", err)
		}
		if !storeErr.IsConflict() {
			t.Error("expected conflict error")
		}
	})

	t.Run("fails when key does not exist", func(t *testing.T) {
		bucket := newStoreKVBucket()
		store := NewExecutionStore(nil, bucket,
			WithStoreStateFactory(storeTestStateFactory),
		)

		// Try to save non-existent key
		_, err := store.SaveExecution(context.Background(), "nonexistent", &storeTestState{}, 1)
		if err == nil {
			t.Fatal("expected error for non-existent key")
		}

		var storeErr *StoreError
		if !errors.As(err, &storeErr) {
			t.Fatalf("expected StoreError, got %T", err)
		}
	})
}

func TestDeleteExecution(t *testing.T) {
	t.Run("deletes existing execution", func(t *testing.T) {
		bucket := newStoreKVBucket()
		store := NewExecutionStore(nil, bucket,
			WithStoreStateFactory(storeTestStateFactory),
		)

		// Create execution
		_, err := store.CreateExecution(context.Background(), "exec-1", "wf", time.Minute)
		if err != nil {
			t.Fatalf("create failed: %v", err)
		}

		// Register a task index
		store.RegisterTaskIndex("task-1", "exec-1")

		// Delete
		err = store.DeleteExecution(context.Background(), "exec-1")
		if err != nil {
			t.Fatalf("delete failed: %v", err)
		}

		// Verify deleted
		entry, err := store.LoadExecution(context.Background(), "exec-1")
		if err != nil {
			t.Fatalf("load failed: %v", err)
		}
		if entry != nil {
			t.Error("expected nil entry after delete")
		}
	})

	t.Run("succeeds for non-existent execution", func(t *testing.T) {
		bucket := newStoreKVBucket()
		store := NewExecutionStore(nil, bucket)

		err := store.DeleteExecution(context.Background(), "non-existent")
		if err != nil {
			t.Errorf("expected no error for non-existent, got: %v", err)
		}
	})
}

func TestListExecutions(t *testing.T) {
	t.Run("lists all executions", func(t *testing.T) {
		bucket := newStoreKVBucket()
		store := NewExecutionStore(nil, bucket,
			WithKeyPrefix("wf."),
			WithStoreStateFactory(storeTestStateFactory),
		)

		// Create several executions
		for i := 0; i < 3; i++ {
			id := "exec-" + string(rune('a'+i))
			_, err := store.CreateExecution(context.Background(), id, "test-wf", time.Minute)
			if err != nil {
				t.Fatalf("create %s failed: %v", id, err)
			}
		}

		// List all
		entries, err := store.ListExecutions(context.Background(), nil)
		if err != nil {
			t.Fatalf("list failed: %v", err)
		}
		if len(entries) != 3 {
			t.Errorf("expected 3 entries, got %d", len(entries))
		}
	})

	t.Run("returns empty for empty bucket", func(t *testing.T) {
		bucket := newStoreKVBucket()
		store := NewExecutionStore(nil, bucket,
			WithStoreStateFactory(storeTestStateFactory),
		)

		entries, err := store.ListExecutions(context.Background(), nil)
		if err != nil {
			t.Fatalf("list failed: %v", err)
		}
		if len(entries) != 0 {
			t.Errorf("expected 0 entries, got %d", len(entries))
		}
	})

	t.Run("filters by status", func(t *testing.T) {
		bucket := newStoreKVBucket()
		store := NewExecutionStore(nil, bucket,
			WithStoreStateFactory(storeTestStateFactory),
		)

		// Create running execution
		entry1, _ := store.CreateExecution(context.Background(), "exec-1", "wf", time.Minute)
		// Create completed execution
		entry2, _ := store.CreateExecution(context.Background(), "exec-2", "wf", time.Minute)
		CompleteExecution(entry2.State, "done")
		_, _ = store.SaveExecution(context.Background(), entry2.Key, entry2.State, entry2.Revision)

		// Filter by running
		status := StatusRunning
		entries, err := store.ListExecutions(context.Background(), &ExecutionFilter{Status: &status})
		if err != nil {
			t.Fatalf("list failed: %v", err)
		}
		if len(entries) != 1 {
			t.Errorf("expected 1 running entry, got %d", len(entries))
		}
		if entries[0].Key != entry1.Key {
			t.Errorf("expected key %q, got %q", entry1.Key, entries[0].Key)
		}
	})

	t.Run("filters by workflow ID", func(t *testing.T) {
		bucket := newStoreKVBucket()
		store := NewExecutionStore(nil, bucket,
			WithStoreStateFactory(storeTestStateFactory),
		)

		_, _ = store.CreateExecution(context.Background(), "exec-1", "workflow-a", time.Minute)
		_, _ = store.CreateExecution(context.Background(), "exec-2", "workflow-b", time.Minute)
		_, _ = store.CreateExecution(context.Background(), "exec-3", "workflow-a", time.Minute)

		entries, err := store.ListExecutions(context.Background(), &ExecutionFilter{WorkflowID: "workflow-a"})
		if err != nil {
			t.Fatalf("list failed: %v", err)
		}
		if len(entries) != 2 {
			t.Errorf("expected 2 entries for workflow-a, got %d", len(entries))
		}
	})

	t.Run("filters by active only", func(t *testing.T) {
		bucket := newStoreKVBucket()
		store := NewExecutionStore(nil, bucket,
			WithStoreStateFactory(storeTestStateFactory),
		)

		// Create one running, one completed
		_, _ = store.CreateExecution(context.Background(), "exec-1", "wf", time.Minute)
		entry2, _ := store.CreateExecution(context.Background(), "exec-2", "wf", time.Minute)
		CompleteExecution(entry2.State, "done")
		_, _ = store.SaveExecution(context.Background(), entry2.Key, entry2.State, entry2.Revision)

		entries, err := store.ListExecutions(context.Background(), &ExecutionFilter{ActiveOnly: true})
		if err != nil {
			t.Fatalf("list failed: %v", err)
		}
		if len(entries) != 1 {
			t.Errorf("expected 1 active entry, got %d", len(entries))
		}
	})
}

func TestTaskIndex(t *testing.T) {
	t.Run("registers and looks up task", func(t *testing.T) {
		bucket := newStoreKVBucket()
		store := NewExecutionStore(nil, bucket)

		store.RegisterTaskIndex("task-123", "exec-456")

		key, ok := store.LookupExecutionByTaskID("task-123")
		if !ok {
			t.Fatal("expected task to be found")
		}
		if key != "exec-456" {
			t.Errorf("expected key 'exec-456', got %q", key)
		}
	})

	t.Run("returns false for unknown task", func(t *testing.T) {
		bucket := newStoreKVBucket()
		store := NewExecutionStore(nil, bucket)

		_, ok := store.LookupExecutionByTaskID("unknown")
		if ok {
			t.Error("expected task not to be found")
		}
	})

	t.Run("unregisters task", func(t *testing.T) {
		bucket := newStoreKVBucket()
		store := NewExecutionStore(nil, bucket)

		store.RegisterTaskIndex("task-123", "exec-456")
		store.UnregisterTaskIndex("task-123")

		_, ok := store.LookupExecutionByTaskID("task-123")
		if ok {
			t.Error("expected task to be unregistered")
		}
	})
}

func TestCheckTimeout(t *testing.T) {
	t.Run("times out expired execution", func(t *testing.T) {
		bucket := newStoreKVBucket()
		store := NewExecutionStore(nil, bucket,
			WithStoreStateFactory(storeTestStateFactory),
		)

		// Create execution with very short timeout
		entry, err := store.CreateExecution(context.Background(), "exec-1", "wf", time.Nanosecond)
		if err != nil {
			t.Fatalf("create failed: %v", err)
		}

		// Wait for timeout
		time.Sleep(10 * time.Millisecond)

		// Check timeout
		timedOut, err := store.CheckTimeout(context.Background(), entry)
		if err != nil {
			t.Fatalf("check timeout failed: %v", err)
		}
		if !timedOut {
			t.Error("expected execution to be timed out")
		}

		// Verify state updated
		loaded, _ := store.LoadExecution(context.Background(), "exec-1")
		state := loaded.State.(*storeTestState)
		if state.Status != StatusTimedOut {
			t.Errorf("expected status TimedOut, got %q", state.Status)
		}
	})

	t.Run("does not timeout non-expired execution", func(t *testing.T) {
		bucket := newStoreKVBucket()
		store := NewExecutionStore(nil, bucket,
			WithStoreStateFactory(storeTestStateFactory),
		)

		// Create execution with long timeout
		entry, err := store.CreateExecution(context.Background(), "exec-1", "wf", time.Hour)
		if err != nil {
			t.Fatalf("create failed: %v", err)
		}

		timedOut, err := store.CheckTimeout(context.Background(), entry)
		if err != nil {
			t.Fatalf("check timeout failed: %v", err)
		}
		if timedOut {
			t.Error("expected execution not to be timed out")
		}
	})

	t.Run("skips already terminal execution", func(t *testing.T) {
		bucket := newStoreKVBucket()
		store := NewExecutionStore(nil, bucket,
			WithStoreStateFactory(storeTestStateFactory),
		)

		entry, err := store.CreateExecution(context.Background(), "exec-1", "wf", time.Nanosecond)
		if err != nil {
			t.Fatalf("create failed: %v", err)
		}

		// Mark as completed
		CompleteExecution(entry.State, "done")
		newRev, _ := store.SaveExecution(context.Background(), entry.Key, entry.State, entry.Revision)
		entry.Revision = newRev

		time.Sleep(10 * time.Millisecond)

		// Should not timeout because already terminal
		timedOut, err := store.CheckTimeout(context.Background(), entry)
		if err != nil {
			t.Fatalf("check timeout failed: %v", err)
		}
		if timedOut {
			t.Error("expected no timeout for terminal execution")
		}
	})
}

func TestCheckIterationLimit(t *testing.T) {
	t.Run("escalates when limit exceeded", func(t *testing.T) {
		bucket := newStoreKVBucket()
		store := NewExecutionStore(nil, bucket,
			WithStoreStateFactory(storeTestStateFactory),
		)

		entry, err := store.CreateExecution(context.Background(), "exec-1", "wf", time.Hour)
		if err != nil {
			t.Fatalf("create failed: %v", err)
		}

		// Set iteration to max
		state := entry.State.(*storeTestState)
		state.Iteration = 5
		newRev, _ := store.SaveExecution(context.Background(), entry.Key, state, entry.Revision)
		entry.Revision = newRev

		// Check with limit of 5
		escalated, err := store.CheckIterationLimit(context.Background(), entry, 5)
		if err != nil {
			t.Fatalf("check iteration limit failed: %v", err)
		}
		if !escalated {
			t.Error("expected execution to be escalated")
		}

		// Verify state
		loaded, _ := store.LoadExecution(context.Background(), "exec-1")
		loadedState := loaded.State.(*storeTestState)
		if loadedState.Status != StatusEscalated {
			t.Errorf("expected status Escalated, got %q", loadedState.Status)
		}
	})

	t.Run("does not escalate under limit", func(t *testing.T) {
		bucket := newStoreKVBucket()
		store := NewExecutionStore(nil, bucket,
			WithStoreStateFactory(storeTestStateFactory),
		)

		entry, err := store.CreateExecution(context.Background(), "exec-1", "wf", time.Hour)
		if err != nil {
			t.Fatalf("create failed: %v", err)
		}

		escalated, err := store.CheckIterationLimit(context.Background(), entry, 10)
		if err != nil {
			t.Fatalf("check iteration limit failed: %v", err)
		}
		if escalated {
			t.Error("expected execution not to be escalated")
		}
	})

	t.Run("ignores zero limit", func(t *testing.T) {
		bucket := newStoreKVBucket()
		store := NewExecutionStore(nil, bucket,
			WithStoreStateFactory(storeTestStateFactory),
		)

		entry, err := store.CreateExecution(context.Background(), "exec-1", "wf", time.Hour)
		if err != nil {
			t.Fatalf("create failed: %v", err)
		}

		state := entry.State.(*storeTestState)
		state.Iteration = 100
		newRev, _ := store.SaveExecution(context.Background(), entry.Key, state, entry.Revision)
		entry.Revision = newRev

		// Zero limit means no limit
		escalated, err := store.CheckIterationLimit(context.Background(), entry, 0)
		if err != nil {
			t.Fatalf("check iteration limit failed: %v", err)
		}
		if escalated {
			t.Error("expected no escalation with zero limit")
		}
	})
}

func TestCleanupCompletedExecutions(t *testing.T) {
	t.Run("cleans up old completed executions", func(t *testing.T) {
		bucket := newStoreKVBucket()
		store := NewExecutionStore(nil, bucket,
			WithStoreStateFactory(storeTestStateFactory),
		)

		// Create and complete an execution
		entry, err := store.CreateExecution(context.Background(), "exec-1", "wf", time.Hour)
		if err != nil {
			t.Fatalf("create failed: %v", err)
		}
		CompleteExecution(entry.State, "done")

		// Backdate the completion time
		state := entry.State.(*storeTestState)
		past := time.Now().Add(-2 * time.Hour)
		state.CompletedAt = &past
		_, _ = store.SaveExecution(context.Background(), entry.Key, state, entry.Revision)

		// Create a running execution (should not be cleaned)
		_, _ = store.CreateExecution(context.Background(), "exec-2", "wf", time.Hour)

		// Cleanup with 1 hour retention
		cleaned, err := store.CleanupCompletedExecutions(context.Background(), time.Hour)
		if err != nil {
			t.Fatalf("cleanup failed: %v", err)
		}
		if cleaned != 1 {
			t.Errorf("expected 1 cleaned, got %d", cleaned)
		}

		// Verify exec-1 is gone, exec-2 remains
		entries, _ := store.ListExecutions(context.Background(), nil)
		if len(entries) != 1 {
			t.Errorf("expected 1 entry remaining, got %d", len(entries))
		}
	})

	t.Run("does not clean recent completions", func(t *testing.T) {
		bucket := newStoreKVBucket()
		store := NewExecutionStore(nil, bucket,
			WithStoreStateFactory(storeTestStateFactory),
		)

		entry, err := store.CreateExecution(context.Background(), "exec-1", "wf", time.Hour)
		if err != nil {
			t.Fatalf("create failed: %v", err)
		}
		CompleteExecution(entry.State, "done")
		_, _ = store.SaveExecution(context.Background(), entry.Key, entry.State, entry.Revision)

		// Cleanup with 1 hour retention (recent completion should remain)
		cleaned, err := store.CleanupCompletedExecutions(context.Background(), time.Hour)
		if err != nil {
			t.Fatalf("cleanup failed: %v", err)
		}
		if cleaned != 0 {
			t.Errorf("expected 0 cleaned, got %d", cleaned)
		}
	})
}

func TestWatchExecutions(t *testing.T) {
	t.Run("receives state changes", func(t *testing.T) {
		bucket := newStoreKVBucket()
		store := NewExecutionStore(nil, bucket,
			WithStoreStateFactory(storeTestStateFactory),
		)

		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		var received []*ExecutionEntry
		var mu sync.Mutex

		err := store.WatchExecutions(ctx, func(_ context.Context, entry *ExecutionEntry, _ KVOperation) {
			mu.Lock()
			received = append(received, entry)
			mu.Unlock()
		})
		if err != nil {
			t.Fatalf("watch failed: %v", err)
		}

		// Give watcher time to start
		time.Sleep(10 * time.Millisecond)

		// Create an execution
		_, err = store.CreateExecution(context.Background(), "exec-1", "wf", time.Hour)
		if err != nil {
			t.Fatalf("create failed: %v", err)
		}

		// Wait for notification
		time.Sleep(50 * time.Millisecond)

		mu.Lock()
		count := len(received)
		mu.Unlock()

		if count == 0 {
			t.Error("expected to receive at least one entry")
		}
	})
}

func TestStoreError(t *testing.T) {
	t.Run("formats error with key", func(t *testing.T) {
		err := &StoreError{
			Op:      "create",
			Key:     "exec-1",
			Message: "already exists",
		}

		expected := "store create exec-1: already exists"
		if err.Error() != expected {
			t.Errorf("expected %q, got %q", expected, err.Error())
		}
	})

	t.Run("formats error without key", func(t *testing.T) {
		err := &StoreError{
			Op:      "list",
			Message: "bucket error",
		}

		expected := "store list: bucket error"
		if err.Error() != expected {
			t.Errorf("expected %q, got %q", expected, err.Error())
		}
	})

	t.Run("unwraps cause", func(t *testing.T) {
		cause := errors.New("original error")
		err := &StoreError{
			Op:      "save",
			Key:     "test",
			Message: "failed",
			Cause:   cause,
		}

		unwrapped := errors.Unwrap(err)
		if unwrapped != cause {
			t.Error("expected cause to be unwrapped")
		}
	})

	t.Run("IsNotFound", func(t *testing.T) {
		err := &StoreError{
			Op:    "load",
			Key:   "test",
			Cause: jetstream.ErrKeyNotFound,
		}

		if !err.IsNotFound() {
			t.Error("expected IsNotFound to return true")
		}
	})

	t.Run("IsConflict", func(t *testing.T) {
		err := &StoreError{
			Op:    "save",
			Key:   "test",
			Cause: jetstream.ErrKeyExists,
		}

		if !err.IsConflict() {
			t.Error("expected IsConflict to return true")
		}
	})
}

func TestStoreBuildKey(t *testing.T) {
	t.Run("with prefix", func(t *testing.T) {
		bucket := newStoreKVBucket()
		store := NewExecutionStore(nil, bucket, WithKeyPrefix("workflow."))

		key := store.buildKey("exec-1")
		if key != "workflow.exec-1" {
			t.Errorf("expected 'workflow.exec-1', got %q", key)
		}
	})

	t.Run("without prefix", func(t *testing.T) {
		bucket := newStoreKVBucket()
		store := NewExecutionStore(nil, bucket)

		key := store.buildKey("exec-1")
		if key != "exec-1" {
			t.Errorf("expected 'exec-1', got %q", key)
		}
	})
}

func TestStoreConcurrentAccess(t *testing.T) {
	t.Run("concurrent task index operations", func(_ *testing.T) {
		bucket := newStoreKVBucket()
		store := NewExecutionStore(nil, bucket)

		var wg sync.WaitGroup
		for i := 0; i < 100; i++ {
			wg.Add(1)
			go func(id int) {
				defer wg.Done()
				taskID := "task-" + string(rune('a'+id%26))
				execKey := "exec-" + string(rune('a'+id%26))

				store.RegisterTaskIndex(taskID, execKey)
				_, _ = store.LookupExecutionByTaskID(taskID)
				store.UnregisterTaskIndex(taskID)
			}(i)
		}
		wg.Wait()
	})

	t.Run("concurrent create operations", func(t *testing.T) {
		bucket := newStoreKVBucket()
		store := NewExecutionStore(nil, bucket,
			WithStoreStateFactory(storeTestStateFactory),
		)

		var wg sync.WaitGroup
		var created int
		var mu sync.Mutex

		for i := 0; i < 10; i++ {
			wg.Add(1)
			go func(id int) {
				defer wg.Done()
				execID := "exec-" + string(rune('a'+id))
				_, err := store.CreateExecution(context.Background(), execID, "wf", time.Hour)
				if err == nil {
					mu.Lock()
					created++
					mu.Unlock()
				}
			}(i)
		}
		wg.Wait()

		if created != 10 {
			t.Errorf("expected 10 created, got %d", created)
		}
	})
}

func TestStoreMatchesFilter(t *testing.T) {
	bucket := newStoreKVBucket()
	store := NewExecutionStore(nil, bucket,
		WithStoreStateFactory(storeTestStateFactory),
	)

	state := &storeTestState{}
	InitializeExecution(state, "exec-1", "wf-1", time.Hour)
	state.Phase = "reviewing"

	entry := &ExecutionEntry{
		State: state,
		Key:   "exec-1",
	}

	t.Run("matches phase", func(t *testing.T) {
		filter := &ExecutionFilter{Phase: "reviewing"}
		if !store.matchesFilter(entry, filter) {
			t.Error("expected filter to match")
		}

		filter = &ExecutionFilter{Phase: "pending"}
		if store.matchesFilter(entry, filter) {
			t.Error("expected filter not to match")
		}
	})

	t.Run("expired only filter", func(t *testing.T) {
		// Not expired
		filter := &ExecutionFilter{ExpiredOnly: true}
		if store.matchesFilter(entry, filter) {
			t.Error("expected non-expired entry not to match ExpiredOnly")
		}

		// Make it expired
		past := time.Now().Add(-time.Hour)
		state.Deadline = &past
		if !store.matchesFilter(entry, filter) {
			t.Error("expected expired entry to match ExpiredOnly")
		}
	})

	t.Run("handles nil state", func(t *testing.T) {
		nilEntry := &ExecutionEntry{
			State: nil,
			Key:   "test",
		}
		filter := &ExecutionFilter{ActiveOnly: true}
		if store.matchesFilter(nilEntry, filter) {
			t.Error("expected nil state not to match")
		}
	})
}

// Verify JSON serialization round-trip
func TestStoreStateSerializationRoundTrip(t *testing.T) {
	state := &storeTestState{
		CustomField: "test-value",
		Counter:     42,
	}
	InitializeExecution(state, "exec-1", "wf-1", time.Hour)
	state.Phase = "reviewing"

	// Serialize
	data, err := json.Marshal(state)
	if err != nil {
		t.Fatalf("marshal failed: %v", err)
	}

	// Deserialize
	loaded := &storeTestState{}
	if err := json.Unmarshal(data, loaded); err != nil {
		t.Fatalf("unmarshal failed: %v", err)
	}

	// Verify fields
	if loaded.ID != state.ID {
		t.Errorf("ID mismatch: %q vs %q", loaded.ID, state.ID)
	}
	if loaded.WorkflowID != state.WorkflowID {
		t.Errorf("WorkflowID mismatch: %q vs %q", loaded.WorkflowID, state.WorkflowID)
	}
	if loaded.Phase != state.Phase {
		t.Errorf("Phase mismatch: %q vs %q", loaded.Phase, state.Phase)
	}
	if loaded.CustomField != state.CustomField {
		t.Errorf("CustomField mismatch: %q vs %q", loaded.CustomField, state.CustomField)
	}
	if loaded.Counter != state.Counter {
		t.Errorf("Counter mismatch: %d vs %d", loaded.Counter, state.Counter)
	}
}
