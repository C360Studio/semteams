# TrustGraph Bridge Specification

**Version**: 1.0.0  
**Status**: Draft  
**Last Updated**: 2026-02-13

---

## Table of Contents

1. [Overview](#1-overview)
2. [Design Principles](#2-design-principles)
3. [TrustGraph Background](#3-trustgraph-background)
4. [Architecture](#4-architecture)
5. [Vocabulary Translation Layer](#5-vocabulary-translation-layer)
6. [Component: trustgraph-input](#6-component-trustgraph-input)
7. [Component: trustgraph-output](#7-component-trustgraph-output)
8. [Component: trustgraph-query (Optional)](#8-component-trustgraph-query-optional)
9. [Configuration](#9-configuration)
10. [Data Model](#10-data-model)
11. [Error Handling & Resilience](#11-error-handling--resilience)
12. [Testing Strategy](#12-testing-strategy)
13. [Deployment Patterns](#13-deployment-patterns)
14. [Examples](#14-examples)

---

## 1. Overview

### 1.1 Purpose

The TrustGraph Bridge provides bidirectional interoperability between SemStreams and TrustGraph. TrustGraph excels at LLM-powered document-to-knowledge-graph extraction and GraphRAG retrieval. SemStreams excels at real-time operational data processing at the edge. The bridge enables clients to use both systems together — TrustGraph for their document corpus, SemStreams for their operational/sensor data — with knowledge flowing between both.

### 1.2 Scope

**In Scope:**
- Import TrustGraph RDF triples into SemStreams entity graph
- Export SemStreams entities as RDF triples to TrustGraph knowledge cores
- Vocabulary translation between SemStreams dotted notation and RDF URIs
- GraphRAG query delegation from SemStreams agentic system to TrustGraph
- REST API integration (not Pulsar-level bridging)

**Out of Scope:**
- Apache Pulsar message bus bridging (too much complexity for minimal gain)
- Shared graph storage (each system owns its store)
- TrustGraph deployment/management (client's responsibility)
- SPARQL query translation (use TrustGraph's own query APIs)
- TrustGraph document ingestion pipeline (client uses TrustGraph tools directly)

### 1.3 Relationship to Other Components

```
┌───────────────────────────────────────────────────────────────────────┐
│                        SemStreams                                     │
│                                                                       │
│  Sensors/Telemetry ──► graph-ingest ──► ENTITY_STATES                │
│                                              │                        │
│                                              ▼                        │
│                                        graph-index                    │
│                                              │                        │
│  trustgraph-input ──────────────────► entity subjects                │
│       ▲ (REST poll/webhook)                                          │
│       │                                                               │
│       │                           trustgraph-output ◄── ENTITY_STATES│
│       │                                │ (REST push)                  │
│       │                                ▼                              │
│  ┌────┴────────────────────────────────┴──────────┐                  │
│  │         vocab/trustgraph (translation)          │                  │
│  └────┬────────────────────────────────┬──────────┘                  │
│       │                                │                              │
│  trustgraph-query ──► agentic tool                                   │
│       │                                                               │
└───────┼────────────────────────────────┼──────────────────────────────┘
        │         REST/HTTP              │
        ▼                                ▼
┌───────────────────────────────────────────────────────────────────────┐
│                        TrustGraph                                     │
│                                                                       │
│  /api/v1/triples-query    /api/v1/knowledge    /api/v1/graph-rag     │
│         │                        │                     │              │
│         ▼                        ▼                     ▼              │
│  Triples Store            Knowledge Cores        GraphRAG Engine     │
│  (Cassandra/Neo4j)        (triples+embeddings)   (LLM + graph)      │
│                                                                       │
│  Apache Pulsar (internal transport — we don't touch this)            │
└───────────────────────────────────────────────────────────────────────┘
```

### 1.4 Customer Context

The target client is a TrustGraph user who wants to bring document-extracted knowledge into their SemStreams operational graph, and optionally push operational data back to TrustGraph for agent context. They are comfortable running TrustGraph's Docker stack and using its CLI tools for document ingestion. They need SemStreams components that speak TrustGraph's REST APIs.

---

## 2. Design Principles

### 2.1 Core Principles

| Principle | Description | Rationale |
|-----------|-------------|-----------|
| **REST at boundaries** | Integrate via TrustGraph REST APIs, not Pulsar topics | Both systems are event-driven internally; REST is the stable external contract |
| **Dual vocabulary** | Translate at system boundaries, not in storage | Each system keeps its efficient internal representation |
| **Composable** | Three independent components, deploy what you need | Client may only need input, only output, or all three |
| **Non-invasive** | Zero changes to TrustGraph deployment | Client's existing TrustGraph setup works unchanged |
| **Tiered** | Works at Tier 0 (no LLM needed for bridge itself) | GraphRAG query tool is Tier 2 but input/output are Tier 0 |

### 2.2 Non-Goals

- **Not a TrustGraph replacement**: We complement their document→KG pipeline
- **Not a message bus bridge**: Pulsar↔NATS bridging would be a complexity sink
- **Not an RDF store**: SemStreams stores triples in its own format; translation happens in flight
- **Not an ontology engine**: We translate predicates via configurable mappings, not OWL reasoning

---

## 3. TrustGraph Background

Understanding TrustGraph's architecture is required context for this spec.

### 3.1 What TrustGraph Is

TrustGraph is an open-source (Apache 2.0), Python-based, Docker-containerized platform for building knowledge graphs from documents using LLMs. It uses Apache Pulsar for internal messaging, pluggable graph databases (Cassandra default, Neo4j, Memgraph, FalkorDB), and pluggable vector stores (Qdrant default, Pinecone, Milvus).

**Their sweet spot**: Ingest PDFs/documents → LLM extracts entities and relationships → builds RDF knowledge graph → provides GraphRAG retrieval over that graph.

### 3.2 TrustGraph Triple Format

TrustGraph uses RDF triples with a compact JSON encoding:

```json
{
  "s": { "v": "http://trustgraph.ai/e/sensor-042", "e": true },
  "p": { "v": "http://www.w3.org/ns/sosa/observes", "e": true },
  "o": { "v": "http://trustgraph.ai/e/temperature", "e": true }
}
```

Where:
- `v` = value (URI string or literal)
- `e` = is_entity (true = URI/entity, false = literal value)

**Literal values** use `e: false`:
```json
{
  "s": { "v": "http://trustgraph.ai/e/sensor-042", "e": true },
  "p": { "v": "http://www.w3.org/2000/01/rdf-schema#label", "e": true },
  "o": { "v": "Temperature Sensor 042", "e": false }
}
```

### 3.3 TrustGraph REST APIs

All APIs are served by TrustGraph's REST gateway (default port 8088).

**Triples Query** — `POST /api/v1/triples-query`
```json
// Request
{
  "service": "triples-query",
  "request": {
    "s": { "v": "http://trustgraph.ai/e/some-entity", "e": true },
    "limit": 100
  }
}

// Response
{
  "response": {
    "response": [
      {
        "s": { "v": "...", "e": true },
        "p": { "v": "...", "e": true },
        "o": { "v": "...", "e": true|false }
      }
    ]
  },
  "complete": true
}
```

**Knowledge API** — `POST /api/v1/knowledge`
```json
// Store triples into a knowledge core
{
  "service": "knowledge",
  "request": {
    "operation": "put-kg-core-triples",
    "id": "core-id",
    "user": "semstreams",
    "collection": "operational-data",
    "triples": [
      { "s": {...}, "p": {...}, "o": {...} }
    ]
  }
}
```

**Graph RAG** — `POST /api/v1/graph-rag`
```json
// Request
{
  "service": "graph-rag",
  "flow": "flow-id",
  "request": {
    "query": "What sensors are in the affected zone?"
  }
}

// Response
{
  "response": {
    "response": "Based on the knowledge graph, sensors X, Y, Z are..."
  },
  "complete": true
}
```

### 3.4 TrustGraph Concepts

| Concept | Description | SemStreams Equivalent |
|---------|-------------|----------------------|
| Knowledge Core | Isolated collection of triples + embeddings | Entity namespace (org.platform prefix) |
| Collection | Grouping of knowledge cores | Not directly — use entity ID hierarchy |
| Flow | Processing pipeline configuration | SemStreams Flow |
| Context Graph | AI-optimized subgraph for retrieval | PathRAG result |

---

## 4. Architecture

### 4.1 Package Structure

```
bridge/trustgraph/
├── vocab/
│   ├── translator.go       # EntityID ↔ URI translation
│   ├── translator_test.go
│   ├── predicates.go       # Predicate mapping registry
│   ├── predicates_test.go
│   ├── namespaces.go       # URI namespace definitions
│   └── namespaces_test.go
├── client/
│   ├── client.go           # TrustGraph REST API client
│   ├── client_test.go
│   ├── triples.go          # Triples query operations
│   ├── knowledge.go        # Knowledge core operations
│   ├── graphrag.go         # GraphRAG query operations
│   └── types.go            # API request/response types
├── input/
│   ├── component.go        # trustgraph-input component
│   ├── component_test.go
│   ├── factory.go          # Component registration
│   ├── factory_test.go
│   └── doc.go
├── output/
│   ├── component.go        # trustgraph-output component
│   ├── component_test.go
│   ├── factory.go          # Component registration
│   ├── factory_test.go
│   └── doc.go
└── query/
    ├── component.go        # trustgraph-query processor
    ├── component_test.go
    ├── factory.go          # Component registration
    ├── factory_test.go
    └── doc.go
```

### 4.2 Dependency Flow

```
bridge/trustgraph/vocab/       ← No SemStreams dependencies beyond message.Triple
bridge/trustgraph/client/      ← Only net/http, vocab
bridge/trustgraph/input/       ← client, vocab, component, natsclient
bridge/trustgraph/output/      ← client, vocab, component, natsclient
bridge/trustgraph/query/       ← client, component, natsclient, agentic
```

The `vocab` and `client` packages have minimal dependencies and can be tested independently.

---

## 5. Vocabulary Translation Layer

This is the critical shared package. It translates between SemStreams dotted notation and RDF URIs.

### 5.1 Entity ID ↔ URI Translation

**SemStreams → URI:**
```
acme.ops.environmental.sensor.temperature.sensor-042
  ↓
http://acme.org/ops/environmental/sensor/temperature/sensor-042
```

**Translation rules:**
1. First segment (org) becomes the domain: `acme` → `http://acme.org/`
2. Remaining segments become the path, dots replaced with slashes
3. Configurable base URI override per org (e.g., `acme` → `https://data.acme-corp.com/`)

**URI → SemStreams:**
```
http://trustgraph.ai/e/space-station-modules
  ↓
trustgraph.default.knowledge.entity.concept.space-station-modules
```

**Translation rules:**
1. Domain becomes org segment: `trustgraph.ai` → `trustgraph`
2. Path segments mapped to entity ID parts
3. If URI has fewer than 6 path segments, fill with configurable defaults
4. URI path separators (`/`) become dots
5. Hyphens preserved (valid in entity IDs)

### 5.2 Translator Interface

```go
// vocab/translator.go

// Translator converts between SemStreams entity IDs and RDF URIs.
type Translator struct {
    // OrgMappings maps org segment to base URI
    // e.g., "acme" → "https://data.acme-corp.com/"
    OrgMappings map[string]string

    // URIMappings maps URI domain to org segment + defaults
    // e.g., "trustgraph.ai" → OrgMapping{Org: "trustgraph", Defaults: {...}}
    URIMappings map[string]URIMapping

    // PredicateMap maps SemStreams predicates to RDF predicate URIs
    PredicateMap *PredicateMap

    // DefaultOrg for URIs with unmapped domains
    DefaultOrg string

    // DefaultURIBase for entities with unmapped orgs
    DefaultURIBase string
}

type URIMapping struct {
    Org      string // SemStreams org segment
    Platform string // Default platform (if URI has < 6 segments)
    Domain   string // Default domain
    System   string // Default system
    Type     string // Default type
}

// EntityIDToURI converts a SemStreams 6-part entity ID to an RDF URI.
func (t *Translator) EntityIDToURI(entityID string) string

// URIToEntityID converts an RDF URI to a SemStreams 6-part entity ID.
func (t *Translator) URIToEntityID(uri string) string

// TripleToRDF converts a SemStreams message.Triple to a TrustGraph triple.
func (t *Translator) TripleToRDF(triple message.Triple) TGTriple

// RDFToTriple converts a TrustGraph triple to a SemStreams message.Triple.
func (t *Translator) RDFToTriple(tgTriple TGTriple, source string) message.Triple
```

### 5.3 Predicate Mapping

Predicate translation uses a two-tier approach:

**Tier 1 — Exact match table (loaded from config):**
```json
{
  "predicate_mappings": {
    "sensor.measurement.celsius": "http://www.w3.org/ns/sosa/hasSimpleResult",
    "sensor.classification.type": "http://www.w3.org/1999/02/22-rdf-syntax-ns#type",
    "geo.location.zone": "http://www.w3.org/ns/sosa/isHostedBy",
    "sensor.observation.time": "http://www.w3.org/ns/sosa/resultTime",
    "relation.part_of": "http://purl.obolibrary.org/obo/BFO_0000050",
    "relation.member_of": "http://purl.obolibrary.org/obo/BFO_0000129"
  }
}
```

**Tier 2 — Structural fallback (for unmapped predicates):**
```
sensor.measurement.celsius
  ↓
http://{org-base}/predicate/sensor/measurement/celsius
```

Reverse mapping works the same way — check the exact match table first, fall back to structural conversion.

### 5.4 Predicate Map Interface

```go
// vocab/predicates.go

// PredicateMap manages bidirectional predicate translation.
type PredicateMap struct {
    // Forward: SemStreams predicate → RDF URI
    toRDF map[string]string
    // Reverse: RDF URI → SemStreams predicate
    fromRDF map[string]string
    // DefaultBase for unmapped predicates
    defaultBase string
}

// NewPredicateMap creates a map from configuration.
func NewPredicateMap(mappings map[string]string, defaultBase string) *PredicateMap

// ToRDF converts a SemStreams predicate to an RDF predicate URI.
func (m *PredicateMap) ToRDF(predicate string) string

// FromRDF converts an RDF predicate URI to a SemStreams predicate.
func (m *PredicateMap) FromRDF(uri string) string
```

### 5.5 Well-Known Namespace Prefixes

```go
// vocab/namespaces.go

const (
    RDF     = "http://www.w3.org/1999/02/22-rdf-syntax-ns#"
    RDFS    = "http://www.w3.org/2000/01/rdf-schema#"
    SOSA    = "http://www.w3.org/ns/sosa/"
    SSN     = "http://www.w3.org/ns/ssn/"
    BFO     = "http://purl.obolibrary.org/obo/"
    GEO     = "http://www.opengis.net/ont/geosparql#"
    SCHEMA  = "http://schema.org/"
    TG      = "http://trustgraph.ai/e/"
)
```

---

## 6. Component: trustgraph-input

### 6.1 Purpose

Pulls triples from TrustGraph's knowledge graph and emits them as SemStreams entity messages. This is the highest-value component — it brings document-extracted knowledge into the operational graph.

### 6.2 Component Metadata

```go
component.Metadata{
    Name:        "trustgraph-input",
    Type:        "input",
    Description: "Imports entities from TrustGraph knowledge graph via REST API",
    Version:     "1.0.0",
}
```

### 6.3 Configuration

```go
type Config struct {
    // TrustGraph REST API endpoint
    Endpoint string `schema:"type:string,desc:TrustGraph API base URL,default:http://localhost:8088"`

    // Authentication (optional — TrustGraph may have auth configured)
    APIKey    string `schema:"type:string,desc:API key for TrustGraph (optional)"`
    APIKeyEnv string `schema:"type:string,desc:Env var containing API key"`

    // Polling configuration
    PollInterval string `schema:"type:string,desc:Polling interval (e.g. 30s 5m),default:60s"`

    // What to import
    Collections []string `schema:"type:array,desc:TrustGraph collections to import from"`
    KGCoreIDs   []string `schema:"type:array,desc:Specific knowledge core IDs to import"`

    // Optional: query filter
    SubjectFilter string `schema:"type:string,desc:URI prefix filter for subjects"`
    PredicateFilter []string `schema:"type:array,desc:Predicate URIs to include (empty = all)"`
    Limit         int    `schema:"type:int,desc:Max triples per poll,default:1000,min:1,max:10000"`

    // Vocabulary translation
    Vocab VocabConfig `schema:"type:object,desc:Vocabulary translation settings"`

    // Entity source identifier
    Source string `schema:"type:string,desc:Source identifier for imported triples,default:trustgraph"`

    // Ports
    Ports *component.PortsConfig `schema:"type:ports,desc:Output port configuration"`
}

type VocabConfig struct {
    OrgMappings       map[string]string     `json:"org_mappings"`
    URIMappings       map[string]URIMapping  `json:"uri_mappings"`
    PredicateMappings map[string]string      `json:"predicate_mappings"`
    DefaultOrg        string                `json:"default_org"`
    DefaultURIBase    string                `json:"default_uri_base"`
}
```

### 6.4 Ports

```go
// Output: entity messages for graph-ingest
OutputPort{
    Name:      "entity",
    Direction: component.DirectionOutput,
    Required:  true,
    Config: component.NATSPort{
        Subject: "entity.>",  // Publish to entity subjects based on entity type
    },
}
```

### 6.5 Processing Logic

```
On each poll interval:
  1. Query TrustGraph Triples Query API for triples matching filters
  2. Group triples by subject URI (each unique subject = one entity)
  3. For each subject group:
     a. Translate subject URI → SemStreams entity ID via Translator
     b. Translate each triple (predicate URI → SemStreams predicate, object handling)
     c. Build EntityMessage with translated triples
     d. Publish to output port subject based on entity type segment
  4. Track last-seen state to enable incremental polling (if TrustGraph supports it)
```

### 6.6 Change Detection

TrustGraph doesn't natively support change feeds via REST. Two strategies:

**Strategy A — Full poll with dedup (default):** Query all matching triples each interval. Compare against a local hash of previously imported triples (stored in NATS KV bucket `TRUSTGRAPH_SYNC`). Only emit entities that have changed.

```
NATS KV: TRUSTGRAPH_SYNC
├── hash:{entity_id}  → SHA256 of sorted triple set
└── last_poll          → Timestamp of last successful poll
```

**Strategy B — Collection-based (if client creates new knowledge cores):** Poll the Knowledge API for knowledge cores modified after the last sync timestamp. Only import from changed cores.

### 6.7 Entity Assembly

TrustGraph triples for a single entity are assembled into a SemStreams entity:

```go
// For subject URI: http://trustgraph.ai/e/supply-chain-risk
// With triples:
//   (supply-chain-risk, rdf:type, Concept)
//   (supply-chain-risk, rdfs:label, "Supply Chain Risk Assessment")
//   (supply-chain-risk, relates-to, cyber-threat-landscape)

// Produces EntityMessage:
EntityMessage{
    EntityID: "trustgraph.default.knowledge.entity.concept.supply-chain-risk",
    Triples: []Triple{
        {
            Subject:   "trustgraph.default.knowledge.entity.concept.supply-chain-risk",
            Predicate: "entity.classification.type",
            Object:    "concept",
            Source:    "trustgraph",
            Timestamp: time.Now(),
            Confidence: 1.0,
        },
        {
            Subject:   "trustgraph.default.knowledge.entity.concept.supply-chain-risk",
            Predicate: "entity.metadata.label",
            Object:    "Supply Chain Risk Assessment",
            Source:    "trustgraph",
            Timestamp: time.Now(),
            Confidence: 1.0,
        },
        {
            Subject:   "trustgraph.default.knowledge.entity.concept.supply-chain-risk",
            Predicate: "relation.relates_to",
            Object:    "trustgraph.default.knowledge.entity.concept.cyber-threat-landscape",
            Source:    "trustgraph",
            Timestamp: time.Now(),
            Confidence: 1.0,
        },
    },
}
```

---

## 7. Component: trustgraph-output

### 7.1 Purpose

Watches SemStreams entity state changes and exports them as RDF triples to TrustGraph knowledge cores. This enables operational data (sensor readings, telemetry, events) to be available in TrustGraph's GraphRAG queries.

### 7.2 Component Metadata

```go
component.Metadata{
    Name:        "trustgraph-output",
    Type:        "output",
    Description: "Exports SemStreams entities to TrustGraph knowledge cores via REST API",
    Version:     "1.0.0",
}
```

### 7.3 Configuration

```go
type Config struct {
    // TrustGraph REST API endpoint
    Endpoint string `schema:"type:string,desc:TrustGraph API base URL,default:http://localhost:8088"`

    // Authentication
    APIKey    string `schema:"type:string,desc:API key for TrustGraph (optional)"`
    APIKeyEnv string `schema:"type:string,desc:Env var containing API key"`

    // Target knowledge core
    KGCoreID   string `schema:"type:string,desc:Knowledge core ID to write to,default:semstreams-operational"`
    User       string `schema:"type:string,desc:TrustGraph user for knowledge core,default:semstreams"`
    Collection string `schema:"type:string,desc:TrustGraph collection name,default:operational"`

    // Batching
    BatchSize    int    `schema:"type:int,desc:Triples per batch,default:100,min:1,max:5000"`
    FlushInterval string `schema:"type:string,desc:Max time before flush,default:5s"`

    // Filtering — which entities to export
    EntityPrefixes []string `schema:"type:array,desc:Entity ID prefixes to export (empty = all)"`
    ExcludeSources []string `schema:"type:array,desc:Source names to exclude (prevents re-export loops)"`

    // Vocabulary translation
    Vocab VocabConfig `schema:"type:object,desc:Vocabulary translation settings"`

    // Ports
    Ports *component.PortsConfig `schema:"type:ports,desc:Input port configuration"`
}
```

### 7.4 Ports

```go
// Input: entity state changes
InputPort{
    Name:      "entity",
    Direction: component.DirectionInput,
    Required:  true,
    Config: component.NATSPort{
        Subject: "entity.>",  // Subscribe to entity subjects
    },
}
```

### 7.5 Processing Logic

```
On entity message received:
  1. Check entity ID against EntityPrefixes filter (skip if not matched)
  2. Check source against ExcludeSources (skip if source = "trustgraph" to prevent loops)
  3. Translate entity ID → URI via Translator
  4. Translate each triple:
     a. Subject entity ID → URI
     b. Predicate → RDF predicate URI via PredicateMap
     c. Object: if it's an entity reference → URI; if literal → literal with e:false
  5. Add translated triples to batch buffer
  6. When batch is full OR flush interval fires:
     a. POST batch to TrustGraph Knowledge API
     b. On success: clear batch, update sync state
     c. On failure: retry with backoff, use buffer package
```

### 7.6 Loop Prevention

**Critical**: When `trustgraph-input` and `trustgraph-output` are both deployed, we must prevent re-export loops where TrustGraph data imported into SemStreams gets exported back to TrustGraph.

**Mechanism**: The `Source` field on triples. `trustgraph-input` stamps all imported triples with `Source: "trustgraph"`. `trustgraph-output` defaults to excluding entities where all triples have `Source: "trustgraph"` via `ExcludeSources`.

---

## 8. Component: trustgraph-query (Optional)

### 8.1 Purpose

A Processor component that wraps TrustGraph's GraphRAG API as a tool for SemStreams' agentic system. Agents running in SemStreams can ask natural language questions over TrustGraph's document-extracted knowledge.

### 8.2 Component Metadata

```go
component.Metadata{
    Name:        "trustgraph-query",
    Type:        "processor",
    Description: "Provides TrustGraph GraphRAG as an agentic tool",
    Version:     "1.0.0",
}
```

### 8.3 Configuration

```go
type Config struct {
    // TrustGraph REST API endpoint
    Endpoint string `schema:"type:string,desc:TrustGraph API base URL,default:http://localhost:8088"`

    // Authentication
    APIKey    string `schema:"type:string,desc:API key for TrustGraph (optional)"`
    APIKeyEnv string `schema:"type:string,desc:Env var containing API key"`

    // Default flow for GraphRAG queries
    FlowID     string `schema:"type:string,desc:TrustGraph flow ID for GraphRAG,default:graph-rag"`
    Collection string `schema:"type:string,desc:Default collection to query"`

    // Timeout for GraphRAG queries (LLM-backed, can be slow)
    Timeout string `schema:"type:string,desc:Query timeout,default:120s"`

    // Ports
    Ports *component.PortsConfig `schema:"type:ports,desc:Input and output ports"`
}
```

### 8.4 Ports

```go
// Input: tool execution requests from agentic-tools
InputPort{
    Name:      "tool.execute",
    Direction: component.DirectionInput,
    Required:  true,
    Config: component.NATSPort{
        Subject: "tool.execute.trustgraph-query",
    },
}

// Output: tool execution results
OutputPort{
    Name:      "tool.result",
    Direction: component.DirectionOutput,
    Required:  true,
    Config: component.NATSPort{
        Subject: "tool.result.trustgraph-query",
    },
}
```

### 8.5 Tool Definition

Register as an agentic tool so the agentic-loop can discover and invoke it:

```go
agentic.ToolDefinition{
    Name:        "trustgraph_query",
    Description: "Query TrustGraph's document knowledge graph using natural language. Use for questions about documents, reports, procedures, or extracted knowledge that isn't in the operational sensor data.",
    Parameters: map[string]agentic.ToolParameter{
        "query": {
            Type:        "string",
            Description: "Natural language question to ask the knowledge graph",
            Required:    true,
        },
        "collection": {
            Type:        "string",
            Description: "Knowledge collection to query (optional, uses default if not specified)",
            Required:    false,
        },
    },
}
```

### 8.6 Processing Logic

```
On tool execution request:
  1. Extract "query" and optional "collection" from tool parameters
  2. POST to TrustGraph /api/v1/graph-rag:
     {
       "service": "graph-rag",
       "flow": config.FlowID,
       "request": { "query": query }
     }
  3. Wait for response (up to Timeout)
  4. Return tool result with GraphRAG response text
  5. On error/timeout: return tool error result
```

---

## 9. Configuration

### 9.1 Static Config Example (Headless Deployment)

```json
{
  "components": {
    "tg-import": {
      "name": "trustgraph-input",
      "config": {
        "endpoint": "http://trustgraph:8088",
        "poll_interval": "60s",
        "collections": ["intelligence"],
        "source": "trustgraph",
        "vocab": {
          "uri_mappings": {
            "trustgraph.ai": {
              "org": "client",
              "platform": "intel",
              "domain": "knowledge",
              "system": "trustgraph",
              "type": "entity"
            }
          },
          "predicate_mappings": {
            "entity.classification.type": "http://www.w3.org/1999/02/22-rdf-syntax-ns#type",
            "entity.metadata.label": "http://www.w3.org/2000/01/rdf-schema#label",
            "sensor.measurement.celsius": "http://www.w3.org/ns/sosa/hasSimpleResult",
            "relation.part_of": "http://purl.obolibrary.org/obo/BFO_0000050"
          },
          "default_org": "external"
        },
        "ports": {
          "outputs": {
            "entity": { "subject": "entity.>" }
          }
        }
      }
    },
    "tg-export": {
      "name": "trustgraph-output",
      "config": {
        "endpoint": "http://trustgraph:8088",
        "kg_core_id": "semstreams-ops",
        "collection": "operational",
        "batch_size": 50,
        "flush_interval": "10s",
        "entity_prefixes": ["acme.ops."],
        "exclude_sources": ["trustgraph"],
        "vocab": {
          "org_mappings": {
            "acme": "https://data.acme-corp.com/"
          },
          "predicate_mappings": {
            "sensor.measurement.celsius": "http://www.w3.org/ns/sosa/hasSimpleResult",
            "geo.location.zone": "http://www.w3.org/ns/sosa/isHostedBy"
          },
          "default_uri_base": "http://semstreams.local/e/"
        },
        "ports": {
          "inputs": {
            "entity": { "subject": "entity.>" }
          }
        }
      }
    },
    "tg-query": {
      "name": "trustgraph-query",
      "config": {
        "endpoint": "http://trustgraph:8088",
        "flow_id": "graph-rag",
        "collection": "intelligence",
        "timeout": "120s",
        "ports": {
          "inputs": {
            "tool.execute": { "subject": "tool.execute.trustgraph-query" }
          },
          "outputs": {
            "tool.result": { "subject": "tool.result.trustgraph-query" }
          }
        }
      }
    }
  }
}
```

### 9.2 Shared Vocab Config

When both input and output are deployed, the vocab config should be consistent. Recommended pattern: define vocab in a shared config section and reference from both components.

---

## 10. Data Model

### 10.1 TrustGraph API Types

```go
// client/types.go

// TGTriple is TrustGraph's compact triple representation.
type TGTriple struct {
    S TGValue `json:"s"`
    P TGValue `json:"p"`
    O TGValue `json:"o"`
}

// TGValue is a compact value with entity flag.
type TGValue struct {
    V string `json:"v"` // Value (URI or literal)
    E bool   `json:"e"` // Is entity (true = URI, false = literal)
}

// TriplesQueryRequest is the request body for triples-query.
type TriplesQueryRequest struct {
    ID      string                 `json:"id,omitempty"`
    Service string                 `json:"service"`
    Flow    string                 `json:"flow,omitempty"`
    Request TriplesQueryParams     `json:"request"`
}

type TriplesQueryParams struct {
    S     *TGValue `json:"s,omitempty"`
    P     *TGValue `json:"p,omitempty"`
    O     *TGValue `json:"o,omitempty"`
    Limit int      `json:"limit,omitempty"`
}

// TriplesQueryResponse is the response from triples-query.
type TriplesQueryResponse struct {
    ID       string `json:"id,omitempty"`
    Response struct {
        Response []TGTriple `json:"response"`
    } `json:"response"`
    Complete bool `json:"complete"`
}

// GraphRAGRequest is the request body for graph-rag.
type GraphRAGRequest struct {
    ID      string         `json:"id,omitempty"`
    Service string         `json:"service"`
    Flow    string         `json:"flow,omitempty"`
    Request GraphRAGParams `json:"request"`
}

type GraphRAGParams struct {
    Query      string `json:"query"`
    Collection string `json:"collection,omitempty"`
}

// GraphRAGResponse is the response from graph-rag.
type GraphRAGResponse struct {
    ID       string `json:"id,omitempty"`
    Response struct {
        Response string `json:"response"`
    } `json:"response"`
    Complete bool `json:"complete"`
}

// KnowledgePutRequest stores triples in a knowledge core.
type KnowledgePutRequest struct {
    ID      string              `json:"id,omitempty"`
    Service string              `json:"service"`
    Request KnowledgePutParams  `json:"request"`
}

type KnowledgePutParams struct {
    Operation  string     `json:"operation"` // "put-kg-core-triples"
    ID         string     `json:"id"`
    User       string     `json:"user"`
    Collection string     `json:"collection"`
    Triples    []TGTriple `json:"triples"`
}
```

### 10.2 Sync State (NATS KV)

```
NATS KV: TRUSTGRAPH_SYNC
├── input:last_poll                → ISO8601 timestamp
├── input:hash:{entity_id}         → SHA256 of triple set (for dedup)
├── output:last_export             → ISO8601 timestamp
└── output:batch:{batch_id}        → Pending batch (for recovery)
```

---

## 11. Error Handling & Resilience

### 11.1 REST Client Resilience

Use the established `errors` and `worker` packages. The TrustGraph client should implement:

| Scenario | Handling |
|----------|----------|
| TrustGraph unavailable | Exponential backoff retry, log warning, continue polling |
| HTTP 429 (rate limit) | Respect Retry-After header, reduce batch size |
| HTTP 5xx | Retry with backoff (max 3 attempts per request) |
| HTTP 4xx | Log error, skip batch, don't retry (likely config issue) |
| Timeout | Use configured timeout, retry once |
| Network error | Backoff retry, mark TrustGraph as unhealthy in metrics |

### 11.2 Data Integrity

- **Idempotent imports**: Entity hash comparison prevents duplicate processing
- **Idempotent exports**: TrustGraph knowledge core PUT is idempotent (same triples = no change)
- **Loop prevention**: Source-based filtering prevents re-export
- **Batch recovery**: Pending export batches stored in KV, retried on restart

### 11.3 Health Checks

Each component exposes health via standard component health interface:

```go
func (c *Component) Health() component.HealthStatus {
    // Check: can we reach TrustGraph?
    // Check: last successful poll/export within expected interval?
    // Check: error rate below threshold?
}
```

---

## 12. Testing Strategy

### 12.1 Unit Tests

**vocab package** (no external dependencies):
- Entity ID → URI translation (various segment counts, special characters)
- URI → Entity ID translation (TrustGraph URIs, custom domain URIs)
- Predicate mapping (exact match, fallback, reverse)
- Edge cases: URIs with ports, trailing slashes, encoded characters

**client package** (HTTP mocking):
- Request serialization matches TrustGraph API format
- Response deserialization handles all triple types
- Error handling for various HTTP status codes
- Timeout behavior

**Component tests** (following existing patterns):
- Meta(), InputPorts(), OutputPorts() for each component
- Config validation (required fields, defaults)
- Factory registration

### 12.2 Integration Tests

Use testcontainers if a TrustGraph Docker image is practical, otherwise use a mock HTTP server:

```go
// Mock TrustGraph REST API for integration tests
type MockTrustGraphServer struct {
    triples    []TGTriple
    knowledgeCores map[string][]TGTriple
    server     *httptest.Server
}

func (m *MockTrustGraphServer) HandleTriplesQuery(w http.ResponseWriter, r *http.Request) { ... }
func (m *MockTrustGraphServer) HandleKnowledgePut(w http.ResponseWriter, r *http.Request) { ... }
func (m *MockTrustGraphServer) HandleGraphRAG(w http.ResponseWriter, r *http.Request) { ... }
```

**Integration test scenarios:**
1. Import triples from mock TrustGraph → verify entities appear on NATS subjects
2. Publish entities on NATS → verify triples appear in mock TrustGraph
3. Round-trip: export entity → import it back → verify equivalence (after translation)
4. Loop prevention: import from TrustGraph → verify NOT re-exported
5. Batch flush: verify batches sent at size limit and interval

### 12.3 Test Coverage Targets

- `vocab/`: 100% of public API (translation correctness is critical)
- `client/`: 100% of public API
- Components: 100% of public API behaviors (following existing component test patterns)

---

## 13. Deployment Patterns

### 13.1 Import Only (Most Common)

Client uses TrustGraph for document extraction, SemStreams for operational reasoning.

```json
{
  "components": {
    "tg-import": { "name": "trustgraph-input", "config": { ... } },
    "graph-ingest": { "name": "graph-ingest", "config": { ... } },
    "graph-index": { "name": "graph-index", "config": { ... } },
    "graph-gateway": { "name": "graph-gateway", "config": { ... } }
  }
}
```

### 13.2 Bidirectional Sync

Both systems share knowledge. Operational data flows to TrustGraph; document knowledge flows to SemStreams.

Deploy both `trustgraph-input` and `trustgraph-output` with matching vocab configs and loop prevention enabled.

### 13.3 GraphRAG Tool Only

Client wants their SemStreams agents to query TrustGraph knowledge without importing the full graph.

Deploy only `trustgraph-query` alongside the agentic system components.

### 13.4 Docker Compose Additions

TrustGraph runs as its own Docker Compose stack. SemStreams connects via network:

```yaml
# In SemStreams docker-compose.yml
services:
  semstreams:
    environment:
      - TRUSTGRAPH_ENDPOINT=http://trustgraph-api:8088

# Network bridge to TrustGraph stack
networks:
  trustgraph-bridge:
    external: true
    name: trustgraph_default
```

---

## 14. Examples

### 14.1 Document Knowledge → Operational Context

**Scenario**: Client ingests cybersecurity threat reports into TrustGraph. SemStreams monitors network sensors. The bridge imports threat entities so SemStreams rules can correlate sensor anomalies with known threats.

```
TrustGraph: PDF report → LLM extraction → knowledge graph
  "CVE-2024-1234 affects Apache 2.4, severity critical"

Bridge imports:
  Entity: client.intel.knowledge.trustgraph.vulnerability.cve-2024-1234
  Triples:
    - affects → client.intel.knowledge.trustgraph.software.apache-2-4
    - severity → "critical"
    - classification.type → "vulnerability"

SemStreams rule:
  IF entity.type == "network_service"
  AND entity.software_version MATCHES "apache-2.4.*"
  AND EXISTS relationship TO vulnerability WITH severity == "critical"
  THEN trigger workflow "vulnerability-alert"
```

### 14.2 Sensor Data → Document Context

**Scenario**: Operational sensor data exported to TrustGraph so agents querying documents can also see current operational state.

```
SemStreams: temperature sensor reading
  Entity: acme.ops.environmental.sensor.temperature.zone-7
  Triple: sensor.measurement.celsius = 45.2

Bridge exports to TrustGraph:
  s: https://data.acme-corp.com/ops/environmental/sensor/temperature/zone-7
  p: http://www.w3.org/ns/sosa/hasSimpleResult
  o: "45.2" (literal)

TrustGraph agent can now answer:
  "What is the current temperature in zone 7?"
  → Combines document SOPs with live sensor data
```

### 14.3 Agent Query Delegation

**Scenario**: SemStreams agent needs information from documents it doesn't have in the operational graph.

```
Agent: "What are the maintenance procedures for pump model X?"
  → agentic-loop checks available tools
  → Invokes trustgraph_query tool
  → trustgraph-query POSTs to TrustGraph GraphRAG API
  → TrustGraph searches document-extracted knowledge graph
  → Returns structured maintenance procedure summary
  → Agent incorporates into response
```

---

## Appendix A: Implementation Priority

| Phase | Component | Effort | Value |
|-------|-----------|--------|-------|
| 1 | `vocab/` package | Medium | Foundation — blocks everything else |
| 2 | `client/` package | Medium | Shared HTTP client |
| 3 | `trustgraph-input` | Medium | Highest customer value |
| 4 | `trustgraph-output` | Low (reuses vocab + client) | Bidirectional sync |
| 5 | `trustgraph-query` | Low | Agent tool — nice to have |

**Phase 1+2 estimated**: ~2-3 days of agent-assisted development  
**Phase 3**: ~1-2 days  
**Phase 4+5**: ~1 day each

---

## Appendix B: Positioning Notes

This bridge validates the SemStreams three-axis framework. TrustGraph and SemStreams sit at different positions:

| Axis | TrustGraph | SemStreams |
|------|-----------|-----------|
| **Expertise** | Assumes ML/data engineering team | Scales from field operator to ML engineer |
| **Compute** | Cloud/data center (Docker + Pulsar + graph DB + vector DB) | Edge to cloud (single binary + NATS) |
| **Output** | Exploratory (GraphRAG, agent queries) | Deterministic to exploratory (tiered) |

The bridge story: "Use the right tool at the right position on each axis. TrustGraph for back-office document intelligence. SemStreams for operational edge processing. Bridge knowledge between them."

This is a **complementary** pitch, not competitive. TrustGraph fans stay TrustGraph fans — they just get operational data capabilities they didn't have before.

---

## Appendix C: TrustGraph API Reference Links

- Triples Query: https://docs.trustgraph.ai/reference/apis/api-triples-query.html
- Knowledge API: https://docs.trustgraph.ai/reference/apis/api-knowledge.html
- Graph RAG: https://docs.trustgraph.ai/reference/apis/api-graph-rag.html
- Agent API: https://github.com/trustgraph-ai/trustgraph/blob/master/docs/apis/api-agent.md
- Python schemas: https://github.com/trustgraph-ai/trustgraph/tree/master/trustgraph-base/trustgraph/schema
- GitHub: https://github.com/trustgraph-ai/trustgraph
