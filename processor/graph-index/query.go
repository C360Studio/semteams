// Package graphindex query handlers
package graphindex

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/nats-io/nats.go/jetstream"
)

// setupQueryHandlers sets up NATS request/reply subscriptions for query handlers
func (c *Component) setupQueryHandlers(ctx context.Context) error {
	// Subscribe to outgoing query
	if err := c.natsClient.SubscribeForRequests(ctx, "graph.index.query.outgoing", c.handleQueryOutgoingNATS); err != nil {
		return fmt.Errorf("subscribe outgoing query: %w", err)
	}

	// Subscribe to incoming query
	if err := c.natsClient.SubscribeForRequests(ctx, "graph.index.query.incoming", c.handleQueryIncomingNATS); err != nil {
		return fmt.Errorf("subscribe incoming query: %w", err)
	}

	// Subscribe to alias query
	if err := c.natsClient.SubscribeForRequests(ctx, "graph.index.query.alias", c.handleQueryAliasNATS); err != nil {
		return fmt.Errorf("subscribe alias query: %w", err)
	}

	// Subscribe to predicate query
	if err := c.natsClient.SubscribeForRequests(ctx, "graph.index.query.predicate", c.handleQueryPredicateNATS); err != nil {
		return fmt.Errorf("subscribe predicate query: %w", err)
	}

	c.logger.Info("query handlers registered",
		"subjects", []string{"graph.index.query.outgoing", "graph.index.query.incoming", "graph.index.query.alias", "graph.index.query.predicate"})

	return nil
}

// handleQueryOutgoingNATS handles outgoing relationship query requests via NATS request/reply
func (c *Component) handleQueryOutgoingNATS(_ context.Context, data []byte) ([]byte, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	var req struct {
		EntityID string `json:"entity_id"`
	}
	if err := json.Unmarshal(data, &req); err != nil {
		return nil, fmt.Errorf("invalid request: %w", err)
	}

	if req.EntityID == "" {
		return nil, fmt.Errorf("invalid request: empty entity_id")
	}

	entry, err := c.outgoingBucket.Get(ctx, req.EntityID)
	if err != nil {
		if err == jetstream.ErrKeyNotFound {
			return []byte("[]"), nil // Return empty array for not found
		}
		return nil, fmt.Errorf("internal error: %w", err)
	}

	var entries []OutgoingEntry
	if err := json.Unmarshal(entry.Value(), &entries); err != nil {
		return nil, fmt.Errorf("internal error: %w", err)
	}

	return json.Marshal(entries)
}

// handleQueryIncomingNATS handles incoming relationship query requests via NATS request/reply
func (c *Component) handleQueryIncomingNATS(_ context.Context, data []byte) ([]byte, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	var req struct {
		EntityID string `json:"entity_id"`
	}
	if err := json.Unmarshal(data, &req); err != nil {
		return nil, fmt.Errorf("invalid request: %w", err)
	}

	if req.EntityID == "" {
		return nil, fmt.Errorf("invalid request: empty entity_id")
	}

	entry, err := c.incomingBucket.Get(ctx, req.EntityID)
	if err != nil {
		if err == jetstream.ErrKeyNotFound {
			return []byte("[]"), nil // Return empty array for not found
		}
		return nil, fmt.Errorf("internal error: %w", err)
	}

	var entries []IncomingEntry
	if err := json.Unmarshal(entry.Value(), &entries); err != nil {
		return nil, fmt.Errorf("internal error: %w", err)
	}

	return json.Marshal(entries)
}

// handleQueryAliasNATS handles alias resolution query requests via NATS request/reply
func (c *Component) handleQueryAliasNATS(_ context.Context, data []byte) ([]byte, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	var req struct {
		Alias string `json:"alias"`
	}
	if err := json.Unmarshal(data, &req); err != nil {
		return nil, fmt.Errorf("invalid request: %w", err)
	}

	if req.Alias == "" {
		return nil, fmt.Errorf("invalid request: empty alias")
	}

	entry, err := c.aliasBucket.Get(ctx, req.Alias)
	if err != nil {
		if err == jetstream.ErrKeyNotFound {
			return nil, fmt.Errorf("not found: %s", req.Alias)
		}
		return nil, fmt.Errorf("internal error: %w", err)
	}

	response := map[string]string{
		"canonical_id": string(entry.Value()),
	}
	return json.Marshal(response)
}

// handleQueryPredicateNATS handles predicate entity query requests via NATS request/reply
func (c *Component) handleQueryPredicateNATS(_ context.Context, data []byte) ([]byte, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	var req struct {
		Predicate string `json:"predicate"`
	}
	if err := json.Unmarshal(data, &req); err != nil {
		return nil, fmt.Errorf("invalid request: %w", err)
	}

	if req.Predicate == "" {
		return nil, fmt.Errorf("invalid request: empty predicate")
	}

	entry, err := c.predicateBucket.Get(ctx, req.Predicate)
	if err != nil {
		if err == jetstream.ErrKeyNotFound {
			return json.Marshal(map[string][]string{"entities": {}})
		}
		return nil, fmt.Errorf("internal error: %w", err)
	}

	var indexEntry PredicateIndexEntry
	if err := json.Unmarshal(entry.Value(), &indexEntry); err != nil {
		return nil, fmt.Errorf("internal error: %w", err)
	}

	response := map[string][]string{
		"entities": indexEntry.Entities,
	}
	return json.Marshal(response)
}

// queryMsg is an interface for query request messages.
// This accommodates both real NATS messages and test mocks.
type queryMsg interface {
	Data() []byte
	Respond(data []byte) error
}

// handleQueryOutgoing handles outgoing relationship query requests
func (c *Component) handleQueryOutgoing(msg queryMsg) {
	// Create context with timeout for KV operation
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// Parse request
	var req struct {
		EntityID string `json:"entity_id"`
	}
	if err := json.Unmarshal(msg.Data(), &req); err != nil {
		c.respondError(msg, "invalid request")
		return
	}

	// Validate request
	if req.EntityID == "" {
		c.respondError(msg, "invalid request")
		return
	}

	// Get outgoing relationships from KV bucket
	entry, err := c.outgoingBucket.Get(ctx, req.EntityID)
	if err != nil {
		if err == jetstream.ErrKeyNotFound {
			// Return empty array for not found
			c.respondJSON(msg, []OutgoingEntry{})
			return
		}
		c.respondError(msg, "internal error")
		return
	}

	// Parse outgoing entries
	var entries []OutgoingEntry
	if err := json.Unmarshal(entry.Value(), &entries); err != nil {
		c.respondError(msg, "internal error")
		return
	}

	// Respond with entries array
	c.respondJSON(msg, entries)
}

// handleQueryIncoming handles incoming relationship query requests
func (c *Component) handleQueryIncoming(msg queryMsg) {
	// Create context with timeout for KV operation
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// Parse request
	var req struct {
		EntityID string `json:"entity_id"`
	}
	if err := json.Unmarshal(msg.Data(), &req); err != nil {
		c.respondError(msg, "invalid request")
		return
	}

	// Validate request
	if req.EntityID == "" {
		c.respondError(msg, "invalid request")
		return
	}

	// Get incoming relationships from KV bucket
	entry, err := c.incomingBucket.Get(ctx, req.EntityID)
	if err != nil {
		if err == jetstream.ErrKeyNotFound {
			// Return empty array for not found
			c.respondJSON(msg, []IncomingEntry{})
			return
		}
		c.respondError(msg, "internal error")
		return
	}

	// Parse incoming entries
	var entries []IncomingEntry
	if err := json.Unmarshal(entry.Value(), &entries); err != nil {
		c.respondError(msg, "internal error")
		return
	}

	// Respond with entries array
	c.respondJSON(msg, entries)
}

// handleQueryAlias handles alias resolution query requests
func (c *Component) handleQueryAlias(msg queryMsg) {
	// Create context with timeout for KV operation
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// Parse request
	var req struct {
		Alias string `json:"alias"`
	}
	if err := json.Unmarshal(msg.Data(), &req); err != nil {
		c.respondError(msg, "invalid request")
		return
	}

	// Validate request
	if req.Alias == "" {
		c.respondError(msg, "invalid request")
		return
	}

	// Get canonical entity ID from KV bucket
	entry, err := c.aliasBucket.Get(ctx, req.Alias)
	if err != nil {
		if err == jetstream.ErrKeyNotFound {
			c.respondError(msg, "not found")
			return
		}
		c.respondError(msg, "internal error")
		return
	}

	// Respond with canonical ID
	response := map[string]string{
		"canonical_id": string(entry.Value()),
	}
	c.respondJSON(msg, response)
}

// handleQueryPredicate handles predicate entity query requests
func (c *Component) handleQueryPredicate(msg queryMsg) {
	// Create context with timeout for KV operation
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// Parse request
	var req struct {
		Predicate string `json:"predicate"`
	}
	if err := json.Unmarshal(msg.Data(), &req); err != nil {
		c.respondError(msg, "invalid request")
		return
	}

	// Validate request
	if req.Predicate == "" {
		c.respondError(msg, "invalid request")
		return
	}

	// Get predicate index entry from KV bucket
	entry, err := c.predicateBucket.Get(ctx, req.Predicate)
	if err != nil {
		if err == jetstream.ErrKeyNotFound {
			// Return empty entities array for not found
			c.respondJSON(msg, map[string][]string{
				"entities": {},
			})
			return
		}
		c.respondError(msg, "internal error")
		return
	}

	// Parse predicate index entry
	var indexEntry PredicateIndexEntry
	if err := json.Unmarshal(entry.Value(), &indexEntry); err != nil {
		c.respondError(msg, "internal error")
		return
	}

	// Respond with entities array
	response := map[string][]string{
		"entities": indexEntry.Entities,
	}
	c.respondJSON(msg, response)
}

// respondError sends an error response
func (c *Component) respondError(msg queryMsg, errorMsg string) {
	response := map[string]string{"error": errorMsg}
	data, _ := json.Marshal(response)
	if err := msg.Respond(data); err != nil {
		c.logger.Error("failed to respond with error", "error", err)
	}
}

// respondJSON sends a JSON response
func (c *Component) respondJSON(msg queryMsg, payload any) {
	data, err := json.Marshal(payload)
	if err != nil {
		c.respondError(msg, "internal error")
		return
	}
	if err := msg.Respond(data); err != nil {
		c.logger.Error("failed to respond with JSON", "error", err)
	}
}
