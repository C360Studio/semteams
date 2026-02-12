// Package executors provides tool executor implementations for the agentic-tools component.
package executors

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/c360studio/semstreams/agentic"
	"github.com/c360studio/semstreams/pkg/errs"
)

// KVGetter defines the minimal interface needed to query entities from a KV store.
// This allows for easier testing and decouples the executor from the full jetstream.KeyValue interface.
type KVGetter interface {
	Get(ctx context.Context, key string) (KVEntry, error)
}

// KVEntry defines the minimal interface needed to read an entry from the KV store.
type KVEntry interface {
	Value() []byte
	Revision() uint64
}

// ErrKeyNotFound is returned when a key is not found in the KV store.
var ErrKeyNotFound = errs.ErrKeyNotFound

// GraphQueryExecutor executes graph queries against the ENTITY_STATES KV bucket.
type GraphQueryExecutor struct {
	kvGetter KVGetter
}

// NewGraphQueryExecutor creates a new GraphQueryExecutor with the given KV getter.
func NewGraphQueryExecutor(kvGetter KVGetter) *GraphQueryExecutor {
	return &GraphQueryExecutor{
		kvGetter: kvGetter,
	}
}

// ListTools returns the tool definitions provided by this executor.
func (e *GraphQueryExecutor) ListTools() []agentic.ToolDefinition {
	return []agentic.ToolDefinition{
		{
			Name:        "query_entity",
			Description: "Query an entity from the knowledge graph by its ID. Returns the entity's properties, relationships, and metadata as JSON.",
			Parameters: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"entity_id": map[string]any{
						"type":        "string",
						"description": "The full entity ID to query (e.g., c360.logistics.environmental.sensor.temperature.temp-sensor-001)",
					},
				},
				"required": []string{"entity_id"},
			},
		},
	}
}

// Execute executes a tool call and returns the result.
func (e *GraphQueryExecutor) Execute(ctx context.Context, call agentic.ToolCall) (agentic.ToolResult, error) {
	switch call.Name {
	case "query_entity":
		return e.queryEntity(ctx, call)
	default:
		return agentic.ToolResult{
			CallID: call.ID,
			Error:  fmt.Sprintf("unknown tool: %s", call.Name),
		}, errs.WrapInvalid(fmt.Errorf("unknown tool: %s", call.Name), "GraphQueryExecutor", "Execute", "find tool")
	}
}

func (e *GraphQueryExecutor) queryEntity(ctx context.Context, call agentic.ToolCall) (agentic.ToolResult, error) {
	// Extract entity_id from arguments
	entityID, ok := call.Arguments["entity_id"].(string)
	if !ok || entityID == "" {
		return agentic.ToolResult{
			CallID: call.ID,
			Error:  "entity_id is required and must be a non-empty string",
		}, nil
	}

	// Query the KV bucket
	entry, err := e.kvGetter.Get(ctx, entityID)
	if err != nil {
		// Check if it's a not found error
		if err == ErrKeyNotFound || err.Error() == "nats: key not found" {
			return agentic.ToolResult{
				CallID: call.ID,
				Error:  fmt.Sprintf("entity not found: %s", entityID),
			}, nil
		}
		return agentic.ToolResult{
			CallID: call.ID,
			Error:  fmt.Sprintf("failed to query entity: %v", err),
		}, errs.WrapTransient(err, "GraphQueryExecutor", "queryEntity", "get entity from KV")
	}

	// Return the entity data as content
	// The value is stored as JSON, so we can return it directly
	content := string(entry.Value())

	// Optionally pretty-print if it's valid JSON
	var jsonData any
	if err := json.Unmarshal(entry.Value(), &jsonData); err == nil {
		if prettyJSON, err := json.MarshalIndent(jsonData, "", "  "); err == nil {
			content = string(prettyJSON)
		}
	}

	return agentic.ToolResult{
		CallID:  call.ID,
		Content: content,
		Metadata: map[string]any{
			"entity_id": entityID,
			"revision":  entry.Revision(),
		},
	}, nil
}

// JetStreamKVAdapter adapts a jetstream.KeyValue to our KVGetter interface.
type JetStreamKVAdapter struct {
	kv interface {
		Get(ctx context.Context, key string) (interface {
			Value() []byte
			Revision() uint64
		}, error)
	}
}

// NewJetStreamKVAdapter creates a new adapter for jetstream.KeyValue.
// Usage: NewJetStreamKVAdapter(kvBucket) where kvBucket is a jetstream.KeyValue
func NewJetStreamKVAdapter(kv any) *JetStreamKVAdapter {
	return &JetStreamKVAdapter{kv: kv.(interface {
		Get(ctx context.Context, key string) (interface {
			Value() []byte
			Revision() uint64
		}, error)
	})}
}

// Get implements KVGetter.
func (a *JetStreamKVAdapter) Get(ctx context.Context, key string) (KVEntry, error) {
	entry, err := a.kv.Get(ctx, key)
	if err != nil {
		// Convert jetstream not found error to our error
		if err.Error() == "nats: key not found" {
			return nil, ErrKeyNotFound
		}
		return nil, err
	}
	return &kvEntryAdapter{entry: entry}, nil
}

type kvEntryAdapter struct {
	entry interface {
		Value() []byte
		Revision() uint64
	}
}

func (e *kvEntryAdapter) Value() []byte    { return e.entry.Value() }
func (e *kvEntryAdapter) Revision() uint64 { return e.entry.Revision() }
