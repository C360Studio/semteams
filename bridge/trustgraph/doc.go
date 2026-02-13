// Package trustgraph provides bidirectional integration between SemStreams and
// TrustGraph, enabling knowledge sharing between operational and document-extracted
// knowledge graphs.
//
// # Overview
//
// TrustGraph is a document processing pipeline that extracts knowledge graphs from
// unstructured documents using LLMs. This bridge enables:
//
//   - Import: Bring document-extracted entities into SemStreams for operational use
//   - Export: Push operational data to TrustGraph for inclusion in GraphRAG queries
//   - Query: Enable agents to query TrustGraph's knowledge via the GraphRAG tool
//
// # Architecture
//
//	┌─────────────────────────────────────────────────────────────────┐
//	│                        TrustGraph                               │
//	│  ┌────────────┐   ┌────────────┐   ┌────────────┐              │
//	│  │  Pulsar    │   │   Graph    │   │  Vector    │              │
//	│  │  Topics    │   │    DB      │   │    DB      │              │
//	│  └─────▲──────┘   └─────▲──────┘   └─────▲──────┘              │
//	│        │                │                │                      │
//	│  ┌─────┴────────────────┴────────────────┴─────┐               │
//	│  │              REST API Gateway                │               │
//	│  │  /triples-query  /knowledge  /graph-rag     │               │
//	│  └─────────────────────▲────────────────────────┘               │
//	└────────────────────────┼────────────────────────────────────────┘
//	                         │
//	    ┌────────────────────┼────────────────────┐
//	    │                    │    SemStreams      │
//	    │  ┌─────────────────┼─────────────────┐  │
//	    │  │      trustgraph bridge            │  │
//	    │  │                 │                 │  │
//	    │  │ ┌───────┐  ┌────┴────┐  ┌──────┐ │  │
//	    │  │ │ input │  │  query  │  │output│ │  │
//	    │  │ └───┬───┘  └────┬────┘  └──┬───┘ │  │
//	    │  │     │           │          │     │  │
//	    │  └─────┼───────────┼──────────┼─────┘  │
//	    │        │           │          │        │
//	    │        ▼           ▼          ▲        │
//	    │   entity.>    tool.result  entity.>    │
//	    │     (NATS)      (NATS)      (NATS)     │
//	    └────────────────────────────────────────┘
//
// # Subpackages
//
//   - [client]: HTTP client for TrustGraph REST APIs
//   - [input]: Component that polls triples and emits entities
//   - [output]: Component that exports entities as triples
//   - [query]: Agentic tool for GraphRAG queries
//   - [vocab]: Vocabulary translation between entity IDs and RDF URIs
//
// # Loop Prevention
//
// When both input and output components are deployed, loop prevention ensures
// that entities imported from TrustGraph are not re-exported back. This is
// achieved by:
//
//  1. Input component sets Source="trustgraph" on all imported entities
//  2. Output component excludes entities where Source matches exclude_sources
//
// # Vocabulary Translation
//
// SemStreams uses 6-part dotted entity IDs while TrustGraph uses RDF URIs.
// The vocab package handles bidirectional translation:
//
//	acme.ops.robotics.gcs.drone.001  ↔  http://acme.org/ops/robotics/gcs/drone/001
//
// Configuration controls the org-to-URI mapping and predicate translation.
//
// # Getting Started
//
// See the README.md for configuration examples and deployment patterns.
package trustgraph
