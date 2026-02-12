// Package graphindex query handlers
package graphindex

import (
	"context"
	"encoding/json"
	"log/slog"
	"time"

	"github.com/c360studio/semstreams/graph"
	"github.com/c360studio/semstreams/pkg/errs"
	"github.com/nats-io/nats.go/jetstream"
)

// setupQueryHandlers sets up NATS request/reply subscriptions for query handlers
func (c *Component) setupQueryHandlers(ctx context.Context) error {
	// Subscribe to outgoing query
	sub, err := c.natsClient.SubscribeForRequests(ctx, "graph.index.query.outgoing", c.handleQueryOutgoingNATS)
	if err != nil {
		return errs.Wrap(err, "Component", "setupQueryHandlers", "subscribe outgoing query")
	}
	c.querySubscriptions = append(c.querySubscriptions, sub)

	// Subscribe to incoming query
	sub, err = c.natsClient.SubscribeForRequests(ctx, "graph.index.query.incoming", c.handleQueryIncomingNATS)
	if err != nil {
		return errs.Wrap(err, "Component", "setupQueryHandlers", "subscribe incoming query")
	}
	c.querySubscriptions = append(c.querySubscriptions, sub)

	// Subscribe to alias query
	sub, err = c.natsClient.SubscribeForRequests(ctx, "graph.index.query.alias", c.handleQueryAliasNATS)
	if err != nil {
		return errs.Wrap(err, "Component", "setupQueryHandlers", "subscribe alias query")
	}
	c.querySubscriptions = append(c.querySubscriptions, sub)

	// Subscribe to predicate query
	sub, err = c.natsClient.SubscribeForRequests(ctx, "graph.index.query.predicate", c.handleQueryPredicateNATS)
	if err != nil {
		return errs.Wrap(err, "Component", "setupQueryHandlers", "subscribe predicate query")
	}
	c.querySubscriptions = append(c.querySubscriptions, sub)

	c.logger.Info("query handlers registered",
		slog.Any("subjects", []string{"graph.index.query.outgoing", "graph.index.query.incoming", "graph.index.query.alias", "graph.index.query.predicate"}))

	return nil
}

// handleQueryOutgoingNATS handles outgoing relationship query requests via NATS request/reply
func (c *Component) handleQueryOutgoingNATS(ctx context.Context, data []byte) ([]byte, error) {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	var req struct {
		EntityID string `json:"entity_id"`
	}
	if err := json.Unmarshal(data, &req); err != nil {
		return json.Marshal(graph.NewQueryError[graph.OutgoingRelationshipsData]("invalid request"))
	}

	if req.EntityID == "" {
		return json.Marshal(graph.NewQueryError[graph.OutgoingRelationshipsData]("invalid request: empty entity_id"))
	}

	entry, err := c.outgoingBucket.Get(ctx, req.EntityID)
	if err != nil {
		if err == jetstream.ErrKeyNotFound {
			return json.Marshal(graph.NewQueryResponse(graph.OutgoingRelationshipsData{
				Relationships: []graph.OutgoingEntry{},
			}))
		}
		return json.Marshal(graph.NewQueryError[graph.OutgoingRelationshipsData]("internal error"))
	}

	var entries []graph.OutgoingEntry
	if err := json.Unmarshal(entry.Value(), &entries); err != nil {
		return json.Marshal(graph.NewQueryError[graph.OutgoingRelationshipsData]("internal error"))
	}

	return json.Marshal(graph.NewQueryResponse(graph.OutgoingRelationshipsData{
		Relationships: entries,
	}))
}

// handleQueryIncomingNATS handles incoming relationship query requests via NATS request/reply
func (c *Component) handleQueryIncomingNATS(ctx context.Context, data []byte) ([]byte, error) {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	var req struct {
		EntityID string `json:"entity_id"`
	}
	if err := json.Unmarshal(data, &req); err != nil {
		return json.Marshal(graph.NewQueryError[graph.IncomingRelationshipsData]("invalid request"))
	}

	if req.EntityID == "" {
		return json.Marshal(graph.NewQueryError[graph.IncomingRelationshipsData]("invalid request: empty entity_id"))
	}

	entry, err := c.incomingBucket.Get(ctx, req.EntityID)
	if err != nil {
		if err == jetstream.ErrKeyNotFound {
			return json.Marshal(graph.NewQueryResponse(graph.IncomingRelationshipsData{
				Relationships: []graph.IncomingEntry{},
			}))
		}
		return json.Marshal(graph.NewQueryError[graph.IncomingRelationshipsData]("internal error"))
	}

	var entries []graph.IncomingEntry
	if err := json.Unmarshal(entry.Value(), &entries); err != nil {
		return json.Marshal(graph.NewQueryError[graph.IncomingRelationshipsData]("internal error"))
	}

	return json.Marshal(graph.NewQueryResponse(graph.IncomingRelationshipsData{
		Relationships: entries,
	}))
}

// handleQueryAliasNATS handles alias resolution query requests via NATS request/reply
func (c *Component) handleQueryAliasNATS(ctx context.Context, data []byte) ([]byte, error) {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	var req struct {
		Alias string `json:"alias"`
	}
	if err := json.Unmarshal(data, &req); err != nil {
		return json.Marshal(graph.NewQueryError[graph.AliasData]("invalid request"))
	}

	if req.Alias == "" {
		return json.Marshal(graph.NewQueryError[graph.AliasData]("invalid request: empty alias"))
	}

	entry, err := c.aliasBucket.Get(ctx, req.Alias)
	if err != nil {
		if err == jetstream.ErrKeyNotFound {
			return json.Marshal(graph.NewQueryResponse(graph.AliasData{
				CanonicalID: nil,
			}))
		}
		return json.Marshal(graph.NewQueryError[graph.AliasData]("internal error"))
	}

	canonicalID := string(entry.Value())
	return json.Marshal(graph.NewQueryResponse(graph.AliasData{
		CanonicalID: &canonicalID,
	}))
}

// handleQueryPredicateNATS handles predicate entity query requests via NATS request/reply
func (c *Component) handleQueryPredicateNATS(ctx context.Context, data []byte) ([]byte, error) {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	var req struct {
		Predicate string `json:"predicate"`
	}
	if err := json.Unmarshal(data, &req); err != nil {
		return json.Marshal(graph.NewQueryError[graph.PredicateData]("invalid request"))
	}

	if req.Predicate == "" {
		return json.Marshal(graph.NewQueryError[graph.PredicateData]("invalid request: empty predicate"))
	}

	entry, err := c.predicateBucket.Get(ctx, req.Predicate)
	if err != nil {
		if err == jetstream.ErrKeyNotFound {
			return json.Marshal(graph.NewQueryResponse(graph.PredicateData{
				Entities: []string{},
			}))
		}
		return json.Marshal(graph.NewQueryError[graph.PredicateData]("internal error"))
	}

	var indexEntry graph.PredicateIndexEntry
	if err := json.Unmarshal(entry.Value(), &indexEntry); err != nil {
		return json.Marshal(graph.NewQueryError[graph.PredicateData]("internal error"))
	}

	return json.Marshal(graph.NewQueryResponse(graph.PredicateData{
		Entities: indexEntry.Entities,
	}))
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
			c.respondJSON(msg, []graph.OutgoingEntry{})
			return
		}
		c.respondError(msg, "internal error")
		return
	}

	// Parse outgoing entries
	var entries []graph.OutgoingEntry
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
			c.respondJSON(msg, []graph.IncomingEntry{})
			return
		}
		c.respondError(msg, "internal error")
		return
	}

	// Parse incoming entries
	var entries []graph.IncomingEntry
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
	var indexEntry graph.PredicateIndexEntry
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
