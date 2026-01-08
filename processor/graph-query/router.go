package graphquery

import (
	"context"
	"encoding/json"
	"log/slog"
	"sync"
	"time"

	"github.com/c360/semstreams/component"
)

// discoveryTimeout is the timeout for on-demand capability discovery.
const discoveryTimeout = 2 * time.Second

// IntentRouter discovers query capabilities via NATS and routes by typed QueryIntent.
// Routes are discovered lazily on first use and cached for subsequent requests.
// Falls back to type-only subjects when no exact match is found.
type IntentRouter struct {
	natsClient natsRequester
	routes     map[component.QueryIntent]string // exact intent -> subject (cache)
	fallback   map[component.IntentType]string  // type-only fallback
	logger     *slog.Logger
	mu         sync.RWMutex
}

// NewIntentRouter creates a router with hardcoded fallback subjects.
func NewIntentRouter(natsClient natsRequester, logger *slog.Logger) *IntentRouter {
	return &IntentRouter{
		natsClient: natsClient,
		routes:     make(map[component.QueryIntent]string),
		fallback: map[component.IntentType]string{
			component.IntentTypeEntity:       "graph.ingest.query.entity",
			component.IntentTypeRelationship: "graph.index.query.outgoing",
			component.IntentTypeSpatial:      "graph.spatial.query.bounds",
			component.IntentTypeTemporal:     "graph.temporal.query.range",
			component.IntentTypeSemantic:     "graph.embedding.query.search",
			component.IntentTypeAggregate:    "graph.clustering.query.community",
			component.IntentTypeAnomaly:      "graph.anomalies.query.detect",
		},
		logger: logger,
	}
}

// capabilityEndpoints returns the known capability endpoints to query.
func capabilityEndpoints() []string {
	return []string{
		"graph.ingest.capabilities",
		"graph.index.capabilities",
		"graph.embedding.capabilities",
		"graph.clustering.capabilities",
		"graph.anomalies.capabilities",
		"graph.spatial.capabilities",
		"graph.temporal.capabilities",
	}
}

// Route returns the NATS subject for a given QueryIntent.
// Uses cached route if available, otherwise discovers on-demand.
// Falls back to type-only match if discovery fails.
func (r *IntentRouter) Route(ctx context.Context, intent component.QueryIntent) string {
	// Check cache first (fast path)
	r.mu.RLock()
	if subject, ok := r.routes[intent]; ok {
		r.mu.RUnlock()
		return subject
	}
	r.mu.RUnlock()

	// Not in cache - try to discover
	subject := r.discoverForIntent(ctx, intent)
	if subject != "" {
		return subject
	}

	// Fallback to type-only
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.fallback[intent.Type]
}

// discoverForIntent queries capability endpoints to find a route for the given intent.
// Caches all discovered routes for future use.
func (r *IntentRouter) discoverForIntent(ctx context.Context, intent component.QueryIntent) string {
	// Skip discovery if no NATS client (unit test scenario)
	if r.natsClient == nil {
		return ""
	}

	endpoints := capabilityEndpoints()

	for _, endpoint := range endpoints {
		resp, err := r.natsClient.Request(ctx, endpoint, []byte{}, discoveryTimeout)
		if err != nil {
			r.logger.Debug("capability endpoint unavailable", "endpoint", endpoint, "error", err)
			continue
		}

		var caps component.QueryCapabilities
		if err := json.Unmarshal(resp, &caps); err != nil {
			r.logger.Debug("failed to parse capabilities", "endpoint", endpoint, "error", err)
			continue
		}

		// Cache all discovered routes
		r.mu.Lock()
		for _, q := range caps.Queries {
			r.routes[q.Intent] = q.Subject
		}
		r.mu.Unlock()

		// Check if we found our target
		r.mu.RLock()
		if subject, ok := r.routes[intent]; ok {
			r.mu.RUnlock()
			r.logger.Debug("discovered route for intent", "intent", intent, "subject", subject)
			return subject
		}
		r.mu.RUnlock()
	}

	return ""
}

// RouteCount returns the number of cached routes.
func (r *IntentRouter) RouteCount() int {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return len(r.routes)
}
