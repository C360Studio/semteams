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

	// Legacy string-based routing (deprecated, for backward compatibility)
	legacyRoutes   map[string]string // intent tag -> subject
	legacyFallback map[string]string // hardcoded fallback subjects

	logger *slog.Logger
	mu     sync.RWMutex
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
		legacyRoutes: make(map[string]string),
		legacyFallback: map[string]string{
			component.IntentTagEntity:       "graph.ingest.query.entity",
			component.IntentTagRelationship: "graph.index.query.outgoing",
			component.IntentTagSpatial:      "graph.spatial.query.bounds",
			component.IntentTagTemporal:     "graph.temporal.query.range",
			component.IntentTagSemantic:     "graph.embedding.query.search",
			component.IntentTagAggregate:    "graph.clustering.query.community",
			component.IntentTagAnomaly:      "graph.anomalies.query.detect",
			component.IntentTagAlias:        "graph.index.query.alias",
			component.IntentTagPrefix:       "graph.ingest.query.prefix",
			component.IntentTagBatch:        "graph.ingest.query.batch",
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

			// Also populate legacy routes for backward compatibility
			for _, tag := range q.IntentTags {
				r.legacyRoutes[tag] = q.Subject
			}
		}
	}

	r.logger.Info("capability discovery complete", "routes", len(r.routes), "discovered", discovered)
	return nil
}

// Route returns the NATS subject for a given intent.
// Accepts either component.QueryIntent (new API) or string (legacy API).
// For QueryIntent: returns exact match if available, otherwise falls back to type-only match.
// For string: returns discovered subject or fallback.
// Returns empty string if type/tag is unknown.
func (r *IntentRouter) Route(intent interface{}) string {
	r.mu.RLock()
	defer r.mu.RUnlock()

	// Handle typed QueryIntent (new API)
	if qi, ok := intent.(component.QueryIntent); ok {
		// Exact match first
		if subject, ok := r.routes[qi]; ok {
			return subject
		}

		// Fallback to type-only
		if subject, ok := r.fallback[qi.Type]; ok {
			return subject
		}

		return ""
	}

	// Handle legacy string-based intent tags
	if tag, ok := intent.(string); ok {
		if subject, ok := r.legacyRoutes[tag]; ok {
			return subject
		}
		return r.legacyFallback[tag]
	}

	// Unknown type
	return ""
}

// RouteByTag returns the NATS subject for an intent tag string (legacy API).
// DEPRECATED: Use Route(QueryIntent) instead.
// Returns discovered subject if available, otherwise falls back to hardcoded subject.
func (r *IntentRouter) RouteByTag(intentTag string) string {
	r.mu.RLock()
	defer r.mu.RUnlock()

	if subject, ok := r.legacyRoutes[intentTag]; ok {
		return subject
	}
	return r.legacyFallback[intentTag]
}

// HasDiscoveredRoute returns true if a discovered route exists for the intent tag (legacy API).
// DEPRECATED: Exists only for backward compatibility with old tests.
func (r *IntentRouter) HasDiscoveredRoute(intentTag string) bool {
	r.mu.RLock()
	defer r.mu.RUnlock()
	_, ok := r.legacyRoutes[intentTag]
	return ok
}

// RouteCount returns the number of discovered routes (legacy API).
// DEPRECATED: Exists only for backward compatibility with old tests.
func (r *IntentRouter) RouteCount() int {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return len(r.routes)
}
