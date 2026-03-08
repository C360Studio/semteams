// Package federation provides types for cross-service graph exchange in the sem* ecosystem.
//
// Federation types are an exchange format for graph entities between services (semsource,
// semspec, semdragon, etc.). Each EventPayload carries a single Entity with an ID and
// triples — implementing graph.Graphable so graph-ingest processes federation entities
// natively.
//
// Key types:
//   - [Entity]: graph entity with ID, triples, and provenance
//   - [Provenance]: origin record for an entity or event
//   - [Event]: graph mutation event (SEED, DELTA, RETRACT, HEARTBEAT)
//   - [EventPayload]: message bus transport wrapper for [Event], implements graph.Graphable
package federation
