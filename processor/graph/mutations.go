package graph

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/nats-io/nats.go"

	gtypes "github.com/c360/semstreams/graph"
	"github.com/c360/semstreams/pkg/errs"
)

// Mutation subject patterns
const (
	// Entity mutations
	SubjectEntityCreate            = "graph.mutation.entity.create"
	SubjectEntityUpdate            = "graph.mutation.entity.update"
	SubjectEntityDelete            = "graph.mutation.entity.delete"
	SubjectEntityCreateWithTriples = "graph.mutation.entity.create-with-triples"
	SubjectEntityUpdateWithTriples = "graph.mutation.entity.update-with-triples"

	// Triple mutations
	SubjectTripleAdd    = "graph.mutation.triple.add"
	SubjectTripleRemove = "graph.mutation.triple.remove"

	// Default timeout for mutation operations
	DefaultMutationTimeout = 5 * time.Second
)

// setupMutationHandlers subscribes to all mutation subjects
func (p *Processor) setupMutationHandlers(ctx context.Context) error {
	// Check for cancellation before setup
	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
	}

	if p.natsClient == nil {
		return errs.WrapFatal(nil, "GraphProcessor", "setupMutationHandlers", "NATS client not initialized")
	}

	// Get raw NATS connection for request/reply pattern
	nc := p.natsClient.GetConnection()
	if nc == nil {
		return errs.WrapFatal(nil, "GraphProcessor", "setupMutationHandlers", "NATS connection not available")
	}

	// Entity mutations
	handlers := map[string]nats.MsgHandler{
		SubjectEntityCreate:            p.handleEntityCreate,
		SubjectEntityUpdate:            p.handleEntityUpdate,
		SubjectEntityDelete:            p.handleEntityDelete,
		SubjectEntityCreateWithTriples: p.handleEntityCreateWithTriples,
		SubjectEntityUpdateWithTriples: p.handleEntityUpdateWithTriples,
		SubjectTripleAdd:               p.handleTripleAdd,
		SubjectTripleRemove:            p.handleTripleRemove,
	}

	// Subscribe to each mutation subject using raw NATS for request/reply
	for subject, handler := range handlers {
		sub, err := nc.Subscribe(subject, handler)
		if err != nil {
			return errs.Wrap(err, "GraphProcessor", "setupMutationHandlers",
				fmt.Sprintf("failed to subscribe to %s", subject))
		}

		p.logger.Info("Subscribed to mutation subject",
			"subject", subject,
			"queue", sub.Queue,
		)
	}

	p.logger.Info("NATS mutation handlers initialized",
		"handlers", len(handlers),
	)

	return nil
}

// handleEntityCreate handles entity creation requests
func (p *Processor) handleEntityCreate(msg *nats.Msg) {
	// Check if processor is ready to handle requests
	if !p.IsReady() {
		p.respondWithError(msg,
			errs.WrapTransient(nil, "GraphProcessor", "handleEntityCreate", "processor not ready"),
			"", "")
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), DefaultMutationTimeout)
	defer cancel()

	// Parse request
	var req gtypes.CreateEntityRequest
	if err := json.Unmarshal(msg.Data, &req); err != nil {
		p.respondWithError(msg, err, req.TraceID, req.RequestID)
		return
	}

	// Validate request
	if req.Entity == nil {
		p.respondWithError(msg,
			errs.WrapInvalid(nil, "GraphProcessor", "handleEntityCreate", "entity is required"),
			req.TraceID, req.RequestID)
		return
	}

	// Create entity using DataManager
	entity, err := p.entityManager.CreateEntity(ctx, req.Entity)

	// Build response
	resp := gtypes.CreateEntityResponse{
		MutationResponse: gtypes.NewMutationResponse(err == nil, err, req.TraceID, req.RequestID),
		Entity:           entity,
	}

	// Send response
	p.respond(msg, resp)
}

// handleEntityUpdate handles entity update requests
func (p *Processor) handleEntityUpdate(msg *nats.Msg) {
	// Check if processor is ready to handle requests
	if !p.IsReady() {
		p.respondWithError(msg,
			errs.WrapTransient(nil, "GraphProcessor", "handleEntityUpdate", "processor not ready"),
			"", "")
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), DefaultMutationTimeout)
	defer cancel()

	// Parse request
	var req gtypes.UpdateEntityRequest
	if err := json.Unmarshal(msg.Data, &req); err != nil {
		p.respondWithError(msg, err, req.TraceID, req.RequestID)
		return
	}

	// Validate request
	if req.Entity == nil {
		p.respondWithError(msg,
			errs.WrapInvalid(nil, "GraphProcessor", "handleEntityUpdate", "entity is required"),
			req.TraceID, req.RequestID)
		return
	}

	// Update entity using DataManager
	entity, err := p.entityManager.UpdateEntity(ctx, req.Entity)

	// Build response
	resp := gtypes.UpdateEntityResponse{
		MutationResponse: gtypes.NewMutationResponse(err == nil, err, req.TraceID, req.RequestID),
		Entity:           entity,
	}
	if entity != nil {
		resp.Version = int64(entity.Version)
	}

	// Send response
	p.respond(msg, resp)
}

// handleEntityDelete handles entity deletion requests
func (p *Processor) handleEntityDelete(msg *nats.Msg) {
	// Check if processor is ready to handle requests
	if !p.IsReady() {
		p.respondWithError(msg,
			errs.WrapTransient(nil, "GraphProcessor", "handleEntityDelete", "processor not ready"),
			"", "")
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), DefaultMutationTimeout)
	defer cancel()

	// Parse request
	var req gtypes.DeleteEntityRequest
	if err := json.Unmarshal(msg.Data, &req); err != nil {
		p.respondWithError(msg, err, req.TraceID, req.RequestID)
		return
	}

	// Validate request
	if req.EntityID == "" {
		p.respondWithError(msg,
			errs.WrapInvalid(nil, "GraphProcessor", "handleEntityDelete", "entity ID is required"),
			req.TraceID, req.RequestID)
		return
	}

	// Delete entity using DataManager
	err := p.entityManager.DeleteEntity(ctx, req.EntityID)

	// Build response
	resp := gtypes.DeleteEntityResponse{
		MutationResponse: gtypes.NewMutationResponse(err == nil, err, req.TraceID, req.RequestID),
		Deleted:          err == nil,
	}

	// Send response
	p.respond(msg, resp)
}

// handleEntityCreateWithTriples handles atomic entity+triples creation
func (p *Processor) handleEntityCreateWithTriples(msg *nats.Msg) {
	// Check if processor is ready to handle requests
	if !p.IsReady() {
		p.respondWithError(msg,
			errs.WrapTransient(nil, "GraphProcessor", "handleEntityCreateWithTriples", "processor not ready"),
			"", "")
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), DefaultMutationTimeout)
	defer cancel()

	// Parse request
	var req gtypes.CreateEntityWithTriplesRequest
	if err := json.Unmarshal(msg.Data, &req); err != nil {
		p.respondWithError(msg, err, req.TraceID, req.RequestID)
		return
	}

	// Validate request
	if req.Entity == nil {
		p.respondWithError(msg,
			errs.WrapInvalid(nil, "GraphProcessor", "handleEntityCreateWithTriples", "entity is required"),
			req.TraceID, req.RequestID)
		return
	}

	// Create entity with triples atomically
	entity, err := p.entityManager.CreateEntityWithTriples(ctx, req.Entity, req.Triples)

	// Build response
	resp := gtypes.CreateEntityWithTriplesResponse{
		MutationResponse: gtypes.NewMutationResponse(err == nil, err, req.TraceID, req.RequestID),
		Entity:           entity,
		TriplesAdded:     len(req.Triples),
	}

	// Send response
	p.respond(msg, resp)
}

// handleEntityUpdateWithTriples handles atomic entity+triples update
func (p *Processor) handleEntityUpdateWithTriples(msg *nats.Msg) {
	// Check if processor is ready to handle requests
	if !p.IsReady() {
		p.respondWithError(msg,
			errs.WrapTransient(nil, "GraphProcessor", "handleEntityUpdateWithTriples", "processor not ready"),
			"", "")
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), DefaultMutationTimeout)
	defer cancel()

	// Parse request
	var req gtypes.UpdateEntityWithTriplesRequest
	if err := json.Unmarshal(msg.Data, &req); err != nil {
		p.respondWithError(msg, err, req.TraceID, req.RequestID)
		return
	}

	// Validate request
	if req.Entity == nil {
		p.respondWithError(msg,
			errs.WrapInvalid(nil, "GraphProcessor", "handleEntityUpdateWithTriples", "entity is required"),
			req.TraceID, req.RequestID)
		return
	}

	// Update entity with triples atomically
	entity, err := p.entityManager.UpdateEntityWithTriples(ctx, req.Entity, req.AddTriples, req.RemoveTriples)

	// Build response
	resp := gtypes.UpdateEntityWithTriplesResponse{
		MutationResponse: gtypes.NewMutationResponse(err == nil, err, req.TraceID, req.RequestID),
		Entity:           entity,
		TriplesAdded:     len(req.AddTriples),
		TriplesRemoved:   len(req.RemoveTriples),
	}
	if entity != nil {
		resp.Version = int64(entity.Version)
	}

	// Send response
	p.respond(msg, resp)
}

// handleTripleAdd handles triple addition requests
func (p *Processor) handleTripleAdd(msg *nats.Msg) {
	// Check if processor is ready to handle requests
	if !p.IsReady() {
		p.respondWithError(msg,
			errs.WrapTransient(nil, "GraphProcessor", "handleTripleAdd", "processor not ready"),
			"", "")
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), DefaultMutationTimeout)
	defer cancel()

	// Parse request
	var req gtypes.AddTripleRequest
	if err := json.Unmarshal(msg.Data, &req); err != nil {
		p.respondWithError(msg, err, req.TraceID, req.RequestID)
		return
	}

	// Validate request
	if req.Triple.Subject == "" || req.Triple.Predicate == "" {
		p.respondWithError(msg,
			errs.WrapInvalid(nil, "GraphProcessor", "handleTripleAdd",
				"subject and predicate are required"),
			req.TraceID, req.RequestID)
		return
	}

	// Add triple using TripleManager
	err := p.tripleManager.AddTriple(ctx, req.Triple)

	// Build response
	resp := gtypes.AddTripleResponse{
		MutationResponse: gtypes.NewMutationResponse(err == nil, err, req.TraceID, req.RequestID),
	}
	if err == nil {
		resp.Triple = &req.Triple
	}

	// Send response
	p.respond(msg, resp)
}

// handleTripleRemove handles triple removal requests
func (p *Processor) handleTripleRemove(msg *nats.Msg) {
	// Check if processor is ready to handle requests
	if !p.IsReady() {
		p.respondWithError(msg,
			errs.WrapTransient(nil, "GraphProcessor", "handleTripleRemove", "processor not ready"),
			"", "")
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), DefaultMutationTimeout)
	defer cancel()

	// Parse request
	var req gtypes.RemoveTripleRequest
	if err := json.Unmarshal(msg.Data, &req); err != nil {
		p.respondWithError(msg, err, req.TraceID, req.RequestID)
		return
	}

	// Validate request
	if req.Subject == "" || req.Predicate == "" {
		p.respondWithError(msg,
			errs.WrapInvalid(nil, "GraphProcessor", "handleTripleRemove",
				"subject and predicate are required"),
			req.TraceID, req.RequestID)
		return
	}

	// Remove triple using TripleManager
	err := p.tripleManager.RemoveTriple(ctx, req.Subject, req.Predicate)

	// Build response
	resp := gtypes.RemoveTripleResponse{
		MutationResponse: gtypes.NewMutationResponse(err == nil, err, req.TraceID, req.RequestID),
		Removed:          err == nil,
	}

	// Send response
	p.respond(msg, resp)
}

// Helper methods

// respond sends a JSON response to a NATS request
func (p *Processor) respond(msg *nats.Msg, response interface{}) {
	data, err := json.Marshal(response)
	if err != nil {
		p.logger.Error("Failed to marshal response",
			"error", err,
			"type", fmt.Sprintf("%T", response),
		)
		// Send error response
		errResp := gtypes.MutationResponse{
			Success:   false,
			Error:     fmt.Sprintf("internal error: failed to marshal response: %v", err),
			Timestamp: time.Now().UnixNano(),
		}
		if errData, err := json.Marshal(errResp); err == nil {
			msg.Respond(errData)
		}
		return
	}

	if err := msg.Respond(data); err != nil {
		p.logger.Error("Failed to send response",
			"error", err,
			"subject", msg.Subject,
		)
	}
}

// respondWithError sends an error response
func (p *Processor) respondWithError(msg *nats.Msg, err error, traceID, requestID string) {
	resp := gtypes.NewMutationResponse(false, err, traceID, requestID)
	p.respond(msg, resp)
}
