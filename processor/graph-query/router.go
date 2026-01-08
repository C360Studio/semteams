package graphquery

import (
	"context"
	"encoding/json"
	"log/slog"
	"sync"
	"time"

	"github.com/c360/semstreams/component"
)

// IntentRouter discovers query capabilities via NATS and routes by intent tag.
// It queries known capability endpoints at startup to build a routing table,
// falling back to hardcoded subjects when discovery fails.
type IntentRouter struct {
	natsClient natsRequester
	routes     map[string]string // intent tag -> subject
	fallback   map[string]string // hardcoded fallback subjects
	logger     *slog.Logger
	mu         sync.RWMutex
}

// NewIntentRouter creates a router with hardcoded fallback subjects.
func NewIntentRouter(natsClient natsRequester, logger *slog.Logger) *IntentRouter {
	return &IntentRouter{
		natsClient: natsClient,
		routes:     make(map[string]string),
		fallback: map[string]string{
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
// Components that don't respond are skipped; their intent tags will use fallback subjects.
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

		// Index routes by intent tags
		for _, q := range caps.Queries {
			for _, tag := range q.IntentTags {
				r.routes[tag] = q.Subject
				discovered++
			}
		}
	}

	r.logger.Info("capability discovery complete", "routes", len(r.routes), "discovered", discovered)
	return nil
}

// Route returns the NATS subject for an intent tag.
// Returns discovered subject if available, otherwise falls back to hardcoded subject.
func (r *IntentRouter) Route(intentTag string) string {
	r.mu.RLock()
	defer r.mu.RUnlock()

	if subject, ok := r.routes[intentTag]; ok {
		return subject
	}
	return r.fallback[intentTag]
}

// HasDiscoveredRoute returns true if a discovered route exists for the intent tag.
func (r *IntentRouter) HasDiscoveredRoute(intentTag string) bool {
	r.mu.RLock()
	defer r.mu.RUnlock()
	_, ok := r.routes[intentTag]
	return ok
}

// RouteCount returns the number of discovered routes.
func (r *IntentRouter) RouteCount() int {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return len(r.routes)
}
