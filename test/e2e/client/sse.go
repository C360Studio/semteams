// Package client provides HTTP clients for SemStreams E2E tests
package client

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// SSEClient handles Server-Sent Events connections for KV bucket watching
type SSEClient struct {
	baseURL    string
	httpClient *http.Client
}

// NewSSEClient creates a new SSE client
func NewSSEClient(baseURL string) *SSEClient {
	return &SSEClient{
		baseURL: baseURL,
		httpClient: &http.Client{
			// No timeout - SSE connections are long-lived
			// Timeout is managed via context
			Timeout: 0,
		},
	}
}

// SSEEvent represents a parsed SSE event
type SSEEvent struct {
	Event string          // Event type (e.g., "connected", "kv_change", "error")
	ID    string          // Event ID for reconnection
	Data  json.RawMessage // Event payload
}

// KVChangeEvent represents a KV bucket change from SSE
type KVChangeEvent struct {
	Bucket    string          `json:"bucket"`
	Key       string          `json:"key"`
	Operation string          `json:"operation"` // "create", "update", "delete"
	Value     json.RawMessage `json:"value,omitempty"`
	Revision  uint64          `json:"revision"`
	Timestamp time.Time       `json:"timestamp"`
}

// KVWatchCondition is a function that evaluates KV events and returns true when satisfied
type KVWatchCondition func(events []KVChangeEvent) (satisfied bool, err error)

// KVWatchOpts configures KV watching behavior
type KVWatchOpts struct {
	Timeout time.Duration // Max wait time (default 60s)
	Pattern string        // Key pattern filter (default "*")
}

// DefaultKVWatchOpts returns sensible defaults
func DefaultKVWatchOpts() KVWatchOpts {
	return KVWatchOpts{
		Timeout: 60 * time.Second,
		Pattern: "*",
	}
}

// WatchKVBucket streams KV changes until the condition is satisfied or timeout
func (c *SSEClient) WatchKVBucket(
	ctx context.Context,
	bucket string,
	condition KVWatchCondition,
	opts KVWatchOpts,
) ([]KVChangeEvent, error) {
	// Apply defaults
	if opts.Timeout == 0 {
		opts.Timeout = DefaultKVWatchOpts().Timeout
	}
	if opts.Pattern == "" {
		opts.Pattern = "*"
	}

	// Build SSE endpoint URL
	endpoint := fmt.Sprintf("%s/message-logger/kv/%s/watch?pattern=%s",
		c.baseURL,
		url.PathEscape(bucket),
		url.QueryEscape(opts.Pattern),
	)

	// Create request with timeout context
	ctx, cancel := context.WithTimeout(ctx, opts.Timeout)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, fmt.Errorf("creating SSE request: %w", err)
	}
	req.Header.Set("Accept", "text/event-stream")

	// Execute request
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("SSE connection failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("SSE endpoint returned status %d", resp.StatusCode)
	}

	// Parse SSE stream
	return c.parseSSEStream(ctx, resp.Body, bucket, condition)
}

// parseSSEStream reads SSE events and evaluates condition
func (c *SSEClient) parseSSEStream(
	ctx context.Context,
	body io.Reader,
	bucket string,
	condition KVWatchCondition,
) ([]KVChangeEvent, error) {
	var collectedEvents []KVChangeEvent
	scanner := bufio.NewScanner(body)

	var currentEvent SSEEvent
	var dataLines []string

	for scanner.Scan() {
		select {
		case <-ctx.Done():
			return collectedEvents, ctx.Err()
		default:
		}

		line := scanner.Text()

		// Empty line = end of event
		if line == "" {
			if currentEvent.Event != "" && len(dataLines) > 0 {
				currentEvent.Data = json.RawMessage(strings.Join(dataLines, "\n"))

				// Process event
				if currentEvent.Event == "kv_change" {
					var kvEvent KVChangeEvent
					if err := json.Unmarshal(currentEvent.Data, &kvEvent); err == nil {
						// Ensure bucket is set
						if kvEvent.Bucket == "" {
							kvEvent.Bucket = bucket
						}
						collectedEvents = append(collectedEvents, kvEvent)

						// Check condition
						satisfied, err := condition(collectedEvents)
						if err != nil {
							return collectedEvents, err
						}
						if satisfied {
							return collectedEvents, nil
						}
					}
				}
			}
			// Reset for next event
			currentEvent = SSEEvent{}
			dataLines = nil
			continue
		}

		// Parse SSE fields
		if strings.HasPrefix(line, "event:") {
			currentEvent.Event = strings.TrimSpace(strings.TrimPrefix(line, "event:"))
		} else if strings.HasPrefix(line, "id:") {
			currentEvent.ID = strings.TrimSpace(strings.TrimPrefix(line, "id:"))
		} else if strings.HasPrefix(line, "data:") {
			dataLines = append(dataLines, strings.TrimPrefix(line, "data:"))
		}
		// Ignore retry: and comments (:)
	}

	if err := scanner.Err(); err != nil {
		return collectedEvents, fmt.Errorf("SSE stream error: %w", err)
	}

	return collectedEvents, nil
}

// Health checks if SSE endpoint is available
func (c *SSEClient) Health(ctx context.Context) error {
	// Quick check - try to connect and immediately disconnect
	ctx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()

	endpoint := fmt.Sprintf("%s/message-logger/kv/ENTITY_STATES/watch", c.baseURL)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Accept", "text/event-stream")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("SSE endpoint unreachable: %w", err)
	}
	resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("SSE endpoint returned status %d", resp.StatusCode)
	}

	return nil
}

// --- Pre-built Condition Functions ---

// CountReaches returns a condition that is satisfied when event count >= target
func CountReaches(target int) KVWatchCondition {
	return func(events []KVChangeEvent) (bool, error) {
		return len(events) >= target, nil
	}
}

// CountCreatesReaches returns a condition satisfied when create operations >= target
func CountCreatesReaches(target int) KVWatchCondition {
	return func(events []KVChangeEvent) (bool, error) {
		count := 0
		for _, e := range events {
			if e.Operation == "create" {
				count++
			}
		}
		return count >= target, nil
	}
}

// KeyExists returns a condition satisfied when a specific key appears
func KeyExists(key string) KVWatchCondition {
	return func(events []KVChangeEvent) (bool, error) {
		for _, e := range events {
			if e.Key == key {
				return true, nil
			}
		}
		return false, nil
	}
}

// KeysExist returns a condition satisfied when all specified keys appear
func KeysExist(keys []string) KVWatchCondition {
	return func(events []KVChangeEvent) (bool, error) {
		found := make(map[string]bool)
		for _, e := range events {
			for _, key := range keys {
				if e.Key == key {
					found[key] = true
				}
			}
		}
		return len(found) == len(keys), nil
	}
}

// KeyPrefixCount returns a condition satisfied when keys with prefix >= target
func KeyPrefixCount(prefix string, target int) KVWatchCondition {
	return func(events []KVChangeEvent) (bool, error) {
		count := 0
		seen := make(map[string]bool)
		for _, e := range events {
			if strings.HasPrefix(e.Key, prefix) && !seen[e.Key] {
				seen[e.Key] = true
				count++
			}
		}
		return count >= target, nil
	}
}

// KeySuffixCount returns a condition satisfied when keys with suffix >= target
func KeySuffixCount(suffix string, target int) KVWatchCondition {
	return func(events []KVChangeEvent) (bool, error) {
		count := 0
		seen := make(map[string]bool)
		for _, e := range events {
			if strings.HasSuffix(e.Key, suffix) && !seen[e.Key] {
				seen[e.Key] = true
				count++
			}
		}
		return count >= target, nil
	}
}

// AllMatch returns a condition satisfied when all collected events match predicate
func AllMatch(predicate func(KVChangeEvent) bool, minCount int) KVWatchCondition {
	return func(events []KVChangeEvent) (bool, error) {
		if len(events) < minCount {
			return false, nil
		}
		for _, e := range events {
			if !predicate(e) {
				return false, nil
			}
		}
		return true, nil
	}
}

// AnyMatch returns a condition satisfied when any event matches predicate
func AnyMatch(predicate func(KVChangeEvent) bool) KVWatchCondition {
	return func(events []KVChangeEvent) (bool, error) {
		for _, e := range events {
			if predicate(e) {
				return true, nil
			}
		}
		return false, nil
	}
}

// ValueFieldEquals returns a condition for checking a JSON field in event values
func ValueFieldEquals(field string, expectedValue interface{}) KVWatchCondition {
	return func(events []KVChangeEvent) (bool, error) {
		for _, e := range events {
			if len(e.Value) == 0 {
				continue
			}
			var valueMap map[string]interface{}
			if err := json.Unmarshal(e.Value, &valueMap); err != nil {
				continue
			}
			if valueMap[field] == expectedValue {
				return true, nil
			}
		}
		return false, nil
	}
}

// CombineAnd returns a condition satisfied when all conditions are satisfied
func CombineAnd(conditions ...KVWatchCondition) KVWatchCondition {
	return func(events []KVChangeEvent) (bool, error) {
		for _, cond := range conditions {
			satisfied, err := cond(events)
			if err != nil {
				return false, err
			}
			if !satisfied {
				return false, nil
			}
		}
		return true, nil
	}
}

// CombineOr returns a condition satisfied when any condition is satisfied
func CombineOr(conditions ...KVWatchCondition) KVWatchCondition {
	return func(events []KVChangeEvent) (bool, error) {
		for _, cond := range conditions {
			satisfied, err := cond(events)
			if err != nil {
				return false, err
			}
			if satisfied {
				return true, nil
			}
		}
		return false, nil
	}
}

// UniqueKeyCountReaches returns a condition satisfied when unique non-deleted keys >= target.
// This properly counts entities by tracking creates/updates and removing deletes,
// giving an accurate count of keys that currently exist in the bucket.
func UniqueKeyCountReaches(target int) KVWatchCondition {
	return func(events []KVChangeEvent) (bool, error) {
		return CountUniqueKeys(events) >= target, nil
	}
}

// CountUniqueKeys returns the count of unique non-deleted keys from events.
// It tracks creates/updates and removes deletes to give an accurate count
// of keys that currently exist. This is essential for SSE-based counting
// since NATS KV watch sends all existing keys first before streaming updates.
func CountUniqueKeys(events []KVChangeEvent) int {
	keys := make(map[string]bool)
	for _, e := range events {
		// Skip the initial_sync_complete marker event
		if e.Operation == "initial_sync_complete" {
			continue
		}
		if e.Operation == "delete" {
			delete(keys, e.Key)
		} else {
			// "create" or "update" - key exists
			keys[e.Key] = true
		}
	}
	return len(keys)
}

// SourceEntityCountReaches returns a condition satisfied when source (non-container)
// entity count >= target. Container entities have suffixes like .group, .group.container,
// or .group.container.level and are excluded from the count.
func SourceEntityCountReaches(target int) KVWatchCondition {
	return func(events []KVChangeEvent) (bool, error) {
		return CountSourceEntities(events) >= target, nil
	}
}

// CountSourceEntities returns the count of source entities (non-container) from events.
// Container entities (ending in .group, .group.container, .group.container.level) are excluded.
func CountSourceEntities(events []KVChangeEvent) int {
	keys := make(map[string]bool)
	for _, e := range events {
		if e.Operation == "initial_sync_complete" {
			continue
		}
		// Skip container entities
		if isContainerKey(e.Key) {
			continue
		}
		if e.Operation == "delete" {
			delete(keys, e.Key)
		} else {
			keys[e.Key] = true
		}
	}
	return len(keys)
}

// isContainerKey checks if a key represents a hierarchy container entity.
func isContainerKey(key string) bool {
	return strings.HasSuffix(key, ".group") ||
		strings.HasSuffix(key, ".group.container") ||
		strings.HasSuffix(key, ".group.container.level")
}
