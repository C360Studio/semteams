// Package rule - Tests for KV Configuration Integration
package rule

import (
	"context"
	"encoding/json"
	"log/slog"
	"sync"
	"testing"
	"time"

	"github.com/c360studio/semstreams/natsclient"
	"github.com/c360studio/semstreams/processor/rule/expression"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// mockConfigManager wraps a minimal config.Manager-like interface for testing.
// Since config.Manager requires a live NATS connection, we test the ConfigManager
// methods that don't depend on config.Manager directly, and use the kvStore path.

func newTestProcessor(t *testing.T) *Processor {
	t.Helper()
	return &Processor{
		natsClient:      &natsclient.Client{},
		config:          &Config{},
		logger:          slog.Default(),
		rules:           make(map[string]Rule),
		ruleDefinitions: make(map[string]Definition),
		ruleConfigs:     make(map[string]map[string]any),
	}
}

func newTestDefinition(id, name string) Definition {
	return Definition{
		ID:      id,
		Type:    "expression",
		Name:    name,
		Enabled: true,
		Conditions: []expression.ConditionExpression{
			{
				Field:    "test.value",
				Operator: "gte",
				Value:    10.0,
			},
		},
		Logic: "and",
	}
}

func TestConfigManager_SaveRule_ViaKVStore(t *testing.T) {
	processor := newTestProcessor(t)
	mockBucket := newMockKVBucket()
	kvStore := newTestKVStore(mockBucket)

	rcm := &ConfigManager{
		processor: processor,
		kvStore:   kvStore,
		logger:    slog.Default(),
	}

	ctx := context.Background()
	ruleDef := newTestDefinition("test-save-001", "Test Save Rule")

	// Save via kvStore path
	err := rcm.SaveRule(ctx, "test-save-001", ruleDef)
	require.NoError(t, err)

	// Verify it was written to KV
	entry, err := mockBucket.Get(ctx, "rules.test-save-001")
	require.NoError(t, err)

	var stored Definition
	require.NoError(t, json.Unmarshal(entry.Value(), &stored))
	assert.Equal(t, "test-save-001", stored.ID)
	assert.Equal(t, "Test Save Rule", stored.Name)
	assert.Equal(t, "expression", stored.Type)
}

func TestConfigManager_DeleteRule_ViaKVStore(t *testing.T) {
	processor := newTestProcessor(t)
	mockBucket := newMockKVBucket()
	kvStore := newTestKVStore(mockBucket)

	rcm := &ConfigManager{
		processor: processor,
		kvStore:   kvStore,
		logger:    slog.Default(),
	}

	ctx := context.Background()
	ruleDef := newTestDefinition("test-delete-001", "Test Delete Rule")

	// Save first
	err := rcm.SaveRule(ctx, "test-delete-001", ruleDef)
	require.NoError(t, err)

	// Delete
	err = rcm.DeleteRule(ctx, "test-delete-001")
	require.NoError(t, err)

	// Verify it was removed
	_, err = mockBucket.Get(ctx, "rules.test-delete-001")
	assert.Error(t, err)
}

func TestConfigManager_GetRule_ViaKVStore(t *testing.T) {
	processor := newTestProcessor(t)
	mockBucket := newMockKVBucket()
	kvStore := newTestKVStore(mockBucket)

	rcm := &ConfigManager{
		processor: processor,
		kvStore:   kvStore,
		logger:    slog.Default(),
	}

	ctx := context.Background()
	ruleDef := newTestDefinition("test-get-001", "Test Get Rule")

	// Save
	err := rcm.SaveRule(ctx, "test-get-001", ruleDef)
	require.NoError(t, err)

	// Get
	result, err := rcm.GetRule(ctx, "test-get-001")
	require.NoError(t, err)
	assert.Equal(t, "test-get-001", result.ID)
	assert.Equal(t, "Test Get Rule", result.Name)
}

func TestConfigManager_GetRule_NotFound(t *testing.T) {
	processor := newTestProcessor(t)
	mockBucket := newMockKVBucket()
	kvStore := newTestKVStore(mockBucket)

	rcm := &ConfigManager{
		processor: processor,
		kvStore:   kvStore,
		logger:    slog.Default(),
	}

	ctx := context.Background()
	_, err := rcm.GetRule(ctx, "nonexistent")
	assert.Error(t, err)
}

func TestConfigManager_ListRules(t *testing.T) {
	processor := newTestProcessor(t)

	// Populate processor's runtime config so ListRules can read it
	processor.ruleConfigs = map[string]map[string]any{
		"rule-a": {"type": "expression", "name": "Rule A", "enabled": true},
		"rule-b": {"type": "stateful", "name": "Rule B", "enabled": false},
	}

	rcm := &ConfigManager{
		processor: processor,
		logger:    slog.Default(),
	}

	ctx := context.Background()
	rules, err := rcm.ListRules(ctx)
	require.NoError(t, err)
	assert.Len(t, rules, 2)
	assert.Equal(t, "Rule A", rules["rule-a"].Name)
	assert.Equal(t, "Rule B", rules["rule-b"].Name)
}

func TestConfigManager_WatchRules_RegisterCallback(t *testing.T) {
	processor := newTestProcessor(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	rcm := &ConfigManager{
		processor: processor,
		ctx:       ctx,
		cancel:    cancel,
		logger:    slog.Default(),
	}

	var mu sync.Mutex
	var receivedEvents []string

	// Register callback
	err := rcm.WatchRules(ctx, func(ruleID string, _ Rule, operation string) {
		mu.Lock()
		receivedEvents = append(receivedEvents, ruleID+":"+operation)
		mu.Unlock()
	})
	require.NoError(t, err)

	// Simulate notification
	rcm.notifyWatchCallbacks("test-rule", nil, "put")
	rcm.notifyWatchCallbacks("test-rule", nil, "delete")

	mu.Lock()
	assert.Equal(t, []string{"test-rule:put", "test-rule:delete"}, receivedEvents)
	mu.Unlock()
}

func TestConfigManager_WatchRules_NilCallback(t *testing.T) {
	processor := newTestProcessor(t)

	rcm := &ConfigManager{
		processor: processor,
		logger:    slog.Default(),
	}

	err := rcm.WatchRules(context.Background(), nil)
	assert.Error(t, err)
}

func TestConfigManager_WatchRules_MultipleCallbacks(t *testing.T) {
	processor := newTestProcessor(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	rcm := &ConfigManager{
		processor: processor,
		ctx:       ctx,
		cancel:    cancel,
		logger:    slog.Default(),
	}

	var count1, count2 int
	var mu sync.Mutex

	_ = rcm.WatchRules(ctx, func(_ string, _ Rule, _ string) {
		mu.Lock()
		count1++
		mu.Unlock()
	})
	_ = rcm.WatchRules(ctx, func(_ string, _ Rule, _ string) {
		mu.Lock()
		count2++
		mu.Unlock()
	})

	rcm.notifyWatchCallbacks("rule-1", nil, "put")

	mu.Lock()
	assert.Equal(t, 1, count1)
	assert.Equal(t, 1, count2)
	mu.Unlock()
}

func TestConfigManager_HandleRuleEntry_Put(t *testing.T) {
	processor := newTestProcessor(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	rcm := &ConfigManager{
		processor: processor,
		ctx:       ctx,
		cancel:    cancel,
		logger:    slog.Default(),
	}

	// Register a watch callback to verify notification
	var mu sync.Mutex
	var notified bool
	var notifiedOp string
	_ = rcm.WatchRules(ctx, func(ruleID string, _ Rule, op string) {
		mu.Lock()
		notified = true
		notifiedOp = op
		mu.Unlock()
	})

	// Create a valid rule definition
	ruleDef := newTestDefinition("my-rule", "My Rule")
	data, err := json.Marshal(ruleDef)
	require.NoError(t, err)

	// Simulate a KV entry
	entry := &mockKVEntry{
		key:      "rules.my-rule",
		value:    data,
		revision: 1,
		created:  time.Now(),
	}

	// Handle the entry
	rcm.handleRuleEntry(entry)

	// Verify the rule was applied to the processor
	processor.mu.RLock()
	_, exists := processor.ruleConfigs["my-rule"]
	processor.mu.RUnlock()
	assert.True(t, exists, "rule should be in processor's config")

	// Verify callback was notified
	mu.Lock()
	assert.True(t, notified)
	assert.Equal(t, "put", notifiedOp)
	mu.Unlock()
}

func TestConfigManager_HandleRuleEntry_InvalidJSON(t *testing.T) {
	processor := newTestProcessor(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	rcm := &ConfigManager{
		processor: processor,
		ctx:       ctx,
		cancel:    cancel,
		logger:    slog.Default(),
	}

	entry := &mockKVEntry{
		key:      "rules.bad-rule",
		value:    []byte("not valid json"),
		revision: 1,
		created:  time.Now(),
	}

	// Should not panic, just log error
	rcm.handleRuleEntry(entry)

	// Verify the rule was NOT applied
	processor.mu.RLock()
	_, exists := processor.ruleConfigs["bad-rule"]
	processor.mu.RUnlock()
	assert.False(t, exists)
}

// newTestKVStore creates a KVStore using the mock bucket for testing.
// This bypasses natsclient.Client.NewKVStore which requires a real NATS client.
func newTestKVStore(bucket *mockKVBucket) *natsclient.KVStore {
	// We use the package-level constructor pattern:
	// KVStore wraps a jetstream.KeyValue bucket directly.
	// Since mockKVBucket implements the jetstream.KeyValue interface methods we use,
	// we construct via the test helper.
	return natsclient.NewKVStoreForTest(bucket, slog.Default())
}
