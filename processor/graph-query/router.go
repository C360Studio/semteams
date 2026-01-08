package graphquery

import (
	"context"
	"encoding/json"
	"log/slog"
	"sync"
	"time"

	"github.com/c360/semstreams/component"
)

// IntentRouter discovers query capabilities via NATS and routes by typed QueryIntent.
// It queries known capability endpoints at startup to build a routing table,
// falling back to type-only subjects when no exact match is found.
type IntentRouter struct {
	natsClient natsRequester
	routes     map[component.QueryIntent]string // exact intent -> subject
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

// DiscoverCapabilities queries all known capability endpoints and builds the routing table.
// Components that don't respond are skipped; their intents will use fallback subjects.
func (r *IntentRouter) DiscoverCapabilities(ctx context.Context, timeout time.Duration) error {
	endpoints := capabilityEndpoints()

	r.mu.Lock()
	defer r.mu.Unlock()

	discovered := 0
	for _, endpoint := range endpoints {
		resp, err := r.natsClient.Request(ctx, endpoint, []byte{}, timeout)
		if err != nil {
			r.logger.Debug("capability endpoint unavailable", "endpoint", endpoint, "error", err)
			continue // Component not available - skip
		}

		var caps component.QueryCapabilities
		if err := json.Unmarshal(resp, &caps); err != nil {
			r.logger.Debug("failed to parse capabilities", "endpoint", endpoint, "error", err)
			continue
		}

		// Index routes by typed QueryIntent
		for _, q := range caps.Queries {
			r.routes[q.Intent] = q.Subject
			discovered++
		}
	}

	r.logger.Info("capability discovery complete", "routes", len(r.routes), "discovered", discovered)
	return nil
}

// Route returns the NATS subject for a given QueryIntent.
// Returns exact match if available, otherwise falls back to type-only match.
// Returns empty string if type is unknown.
func (r *IntentRouter) Route(intent component.QueryIntent) string {
	r.mu.RLock()
	defer r.mu.RUnlock()

	// Exact match first
	if subject, ok := r.routes[intent]; ok {
		return subject
	}

	// Fallback to type-only
	if subject, ok := r.fallback[intent.Type]; ok {
		return subject
	}

	return ""
}

// RouteCount returns the number of discovered routes.
func (r *IntentRouter) RouteCount() int {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return len(r.routes)
}
