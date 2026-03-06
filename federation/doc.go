// Package federation provides types for cross-service graph exchange in the sem* ecosystem.
//
// Federation types are an exchange format for graph events between services (semsource,
// semspec, semdragon, etc.). They carry explicit entities, edges, and provenance chains
// for namespace-sovereign merge operations.
//
// This is distinct from [graph.EntityState], which is the internal storage format using
// triple-based representation in NATS KV. Use [ToEntityState] and [FromEntityState] for
// bidirectional conversion between the two representations.
//
// Key types:
//   - [Entity]: normalized graph entity with triples, edges, and provenance
//   - [Edge]: directed relationship between two entities
//   - [Provenance]: origin record for an entity or event
//   - [Event]: graph mutation event (SEED, DELTA, RETRACT, HEARTBEAT)
//   - [EventPayload]: message bus transport wrapper for [Event]
//   - [Store]: thread-safe in-memory store with content-based change detection
package federation
