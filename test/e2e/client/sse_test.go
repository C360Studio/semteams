package client

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestKVWatchConditions(t *testing.T) {
	// Helper to create test events
	makeEvent := func(key, op string) KVChangeEvent {
		return KVChangeEvent{
			Bucket:    "TEST_BUCKET",
			Key:       key,
			Operation: op,
			Revision:  1,
			Timestamp: time.Now(),
		}
	}

	makeEventWithValue := func(key, op string, value map[string]interface{}) KVChangeEvent {
		valueJSON, _ := json.Marshal(value)
		return KVChangeEvent{
			Bucket:    "TEST_BUCKET",
			Key:       key,
			Operation: op,
			Value:     valueJSON,
			Revision:  1,
			Timestamp: time.Now(),
		}
	}

	t.Run("CountReaches", func(t *testing.T) {
		cond := CountReaches(3)

		// Not enough events
		satisfied, err := cond([]KVChangeEvent{makeEvent("a", "create")})
		require.NoError(t, err)
		assert.False(t, satisfied)

		// Exactly enough
		satisfied, err = cond([]KVChangeEvent{
			makeEvent("a", "create"),
			makeEvent("b", "create"),
			makeEvent("c", "create"),
		})
		require.NoError(t, err)
		assert.True(t, satisfied)

		// More than enough
		satisfied, err = cond([]KVChangeEvent{
			makeEvent("a", "create"),
			makeEvent("b", "create"),
			makeEvent("c", "create"),
			makeEvent("d", "create"),
		})
		require.NoError(t, err)
		assert.True(t, satisfied)
	})

	t.Run("CountCreatesReaches", func(t *testing.T) {
		cond := CountCreatesReaches(2)

		// Mixed operations - not enough creates
		satisfied, err := cond([]KVChangeEvent{
			makeEvent("a", "create"),
			makeEvent("b", "update"),
			makeEvent("c", "delete"),
		})
		require.NoError(t, err)
		assert.False(t, satisfied)

		// Enough creates
		satisfied, err = cond([]KVChangeEvent{
			makeEvent("a", "create"),
			makeEvent("b", "update"),
			makeEvent("c", "create"),
		})
		require.NoError(t, err)
		assert.True(t, satisfied)
	})

	t.Run("KeyExists", func(t *testing.T) {
		cond := KeyExists("target.key")

		// Key not present
		satisfied, err := cond([]KVChangeEvent{
			makeEvent("other.key", "create"),
		})
		require.NoError(t, err)
		assert.False(t, satisfied)

		// Key present
		satisfied, err = cond([]KVChangeEvent{
			makeEvent("other.key", "create"),
			makeEvent("target.key", "create"),
		})
		require.NoError(t, err)
		assert.True(t, satisfied)
	})

	t.Run("KeysExist", func(t *testing.T) {
		cond := KeysExist([]string{"key.a", "key.b"})

		// Only one key
		satisfied, err := cond([]KVChangeEvent{
			makeEvent("key.a", "create"),
		})
		require.NoError(t, err)
		assert.False(t, satisfied)

		// Both keys
		satisfied, err = cond([]KVChangeEvent{
			makeEvent("key.a", "create"),
			makeEvent("key.b", "create"),
		})
		require.NoError(t, err)
		assert.True(t, satisfied)

		// Both keys with extras
		satisfied, err = cond([]KVChangeEvent{
			makeEvent("key.c", "create"),
			makeEvent("key.a", "create"),
			makeEvent("key.d", "create"),
			makeEvent("key.b", "create"),
		})
		require.NoError(t, err)
		assert.True(t, satisfied)
	})

	t.Run("KeyPrefixCount", func(t *testing.T) {
		cond := KeyPrefixCount("sensor.", 2)

		// Not enough with prefix
		satisfied, err := cond([]KVChangeEvent{
			makeEvent("sensor.temp", "create"),
			makeEvent("device.motor", "create"),
		})
		require.NoError(t, err)
		assert.False(t, satisfied)

		// Enough with prefix
		satisfied, err = cond([]KVChangeEvent{
			makeEvent("sensor.temp", "create"),
			makeEvent("sensor.humidity", "create"),
		})
		require.NoError(t, err)
		assert.True(t, satisfied)

		// Duplicate keys should not double-count
		satisfied, err = cond([]KVChangeEvent{
			makeEvent("sensor.temp", "create"),
			makeEvent("sensor.temp", "update"),
		})
		require.NoError(t, err)
		assert.False(t, satisfied, "duplicate keys should not double-count")
	})

	t.Run("KeySuffixCount", func(t *testing.T) {
		cond := KeySuffixCount(".group", 2)

		// Not enough with suffix
		satisfied, err := cond([]KVChangeEvent{
			makeEvent("temp.group", "create"),
			makeEvent("temp.sensor", "create"),
		})
		require.NoError(t, err)
		assert.False(t, satisfied)

		// Enough with suffix
		satisfied, err = cond([]KVChangeEvent{
			makeEvent("temp.group", "create"),
			makeEvent("humidity.group", "create"),
		})
		require.NoError(t, err)
		assert.True(t, satisfied)
	})

	t.Run("AllMatch", func(t *testing.T) {
		isCreate := func(e KVChangeEvent) bool { return e.Operation == "create" }
		cond := AllMatch(isCreate, 2)

		// Not enough events
		satisfied, err := cond([]KVChangeEvent{
			makeEvent("a", "create"),
		})
		require.NoError(t, err)
		assert.False(t, satisfied)

		// Enough but not all match
		satisfied, err = cond([]KVChangeEvent{
			makeEvent("a", "create"),
			makeEvent("b", "update"),
		})
		require.NoError(t, err)
		assert.False(t, satisfied)

		// All match
		satisfied, err = cond([]KVChangeEvent{
			makeEvent("a", "create"),
			makeEvent("b", "create"),
		})
		require.NoError(t, err)
		assert.True(t, satisfied)
	})

	t.Run("AnyMatch", func(t *testing.T) {
		isDelete := func(e KVChangeEvent) bool { return e.Operation == "delete" }
		cond := AnyMatch(isDelete)

		// No match
		satisfied, err := cond([]KVChangeEvent{
			makeEvent("a", "create"),
			makeEvent("b", "update"),
		})
		require.NoError(t, err)
		assert.False(t, satisfied)

		// Has match
		satisfied, err = cond([]KVChangeEvent{
			makeEvent("a", "create"),
			makeEvent("b", "delete"),
		})
		require.NoError(t, err)
		assert.True(t, satisfied)
	})

	t.Run("ValueFieldEquals", func(t *testing.T) {
		cond := ValueFieldEquals("status", "active")

		// No matching value
		satisfied, err := cond([]KVChangeEvent{
			makeEventWithValue("a", "create", map[string]interface{}{"status": "inactive"}),
		})
		require.NoError(t, err)
		assert.False(t, satisfied)

		// Has matching value
		satisfied, err = cond([]KVChangeEvent{
			makeEventWithValue("a", "create", map[string]interface{}{"status": "inactive"}),
			makeEventWithValue("b", "create", map[string]interface{}{"status": "active"}),
		})
		require.NoError(t, err)
		assert.True(t, satisfied)

		// Event without value should not match
		satisfied, err = cond([]KVChangeEvent{
			makeEvent("a", "delete"),
		})
		require.NoError(t, err)
		assert.False(t, satisfied)
	})

	t.Run("CombineAnd", func(t *testing.T) {
		cond := CombineAnd(
			CountReaches(2),
			KeyExists("target"),
		)

		// Only first condition satisfied
		satisfied, err := cond([]KVChangeEvent{
			makeEvent("a", "create"),
			makeEvent("b", "create"),
		})
		require.NoError(t, err)
		assert.False(t, satisfied)

		// Only second condition satisfied
		satisfied, err = cond([]KVChangeEvent{
			makeEvent("target", "create"),
		})
		require.NoError(t, err)
		assert.False(t, satisfied)

		// Both conditions satisfied
		satisfied, err = cond([]KVChangeEvent{
			makeEvent("a", "create"),
			makeEvent("target", "create"),
		})
		require.NoError(t, err)
		assert.True(t, satisfied)
	})

	t.Run("CombineOr", func(t *testing.T) {
		cond := CombineOr(
			CountReaches(10),
			KeyExists("early-exit"),
		)

		// Neither satisfied
		satisfied, err := cond([]KVChangeEvent{
			makeEvent("a", "create"),
		})
		require.NoError(t, err)
		assert.False(t, satisfied)

		// Second satisfied (early exit)
		satisfied, err = cond([]KVChangeEvent{
			makeEvent("early-exit", "create"),
		})
		require.NoError(t, err)
		assert.True(t, satisfied)
	})
}

func TestDefaultKVWatchOpts(t *testing.T) {
	opts := DefaultKVWatchOpts()
	assert.Equal(t, 60*time.Second, opts.Timeout)
	assert.Equal(t, "*", opts.Pattern)
}

func TestNewSSEClient(t *testing.T) {
	client := NewSSEClient("http://localhost:8080")
	assert.NotNil(t, client)
	assert.Equal(t, "http://localhost:8080", client.baseURL)
	assert.NotNil(t, client.httpClient)
}
