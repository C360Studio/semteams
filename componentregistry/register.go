// Package componentregistry provides component registration for SemStreams framework.
// This package registers both protocol-level and semantic-level components.
package componentregistry

import (
	"errors"

	"github.com/c360/semstreams/component"
	"github.com/c360/semstreams/examples/processors/document"
	iotsensor "github.com/c360/semstreams/examples/processors/iot_sensor"
	graphgateway "github.com/c360/semstreams/gateway/graph-gateway"
	gatewayhttp "github.com/c360/semstreams/gateway/http"
	fileinput "github.com/c360/semstreams/input/file"
	"github.com/c360/semstreams/input/udp"
	websocketinput "github.com/c360/semstreams/input/websocket"
	"github.com/c360/semstreams/output/file"
	"github.com/c360/semstreams/output/httppost"
	"github.com/c360/semstreams/output/websocket"
	pkgerrs "github.com/c360/semstreams/pkg/errs"
	agenticloop "github.com/c360/semstreams/processor/agentic-loop"
	agenticmodel "github.com/c360/semstreams/processor/agentic-model"
	agentictools "github.com/c360/semstreams/processor/agentic-tools"
	graphclustering "github.com/c360/semstreams/processor/graph-clustering"
	graphembedding "github.com/c360/semstreams/processor/graph-embedding"
	graphindex "github.com/c360/semstreams/processor/graph-index"
	graphindexspatial "github.com/c360/semstreams/processor/graph-index-spatial"
	graphindextemporal "github.com/c360/semstreams/processor/graph-index-temporal"
	graphingest "github.com/c360/semstreams/processor/graph-ingest"
	graphquery "github.com/c360/semstreams/processor/graph-query"
	jsonfilter "github.com/c360/semstreams/processor/json_filter"
	jsongeneric "github.com/c360/semstreams/processor/json_generic"
	jsonmap "github.com/c360/semstreams/processor/json_map"
	"github.com/c360/semstreams/processor/rule"
	"github.com/c360/semstreams/storage/objectstore"
)

// Register registers all SemStreams framework components with the provided registry.
// This includes protocol-layer, semantic-layer, and example domain components:
//
// Protocol Layer (network/data agnostic):
//   - UDP input (network protocol)
//   - WebSocket input (federation)
//   - JSONGeneric processor (Plain JSON wrapper)
//   - JSONFilter processor (field-based filtering)
//   - JSONMap processor (field transformation)
//   - ObjectStore storage (NATS JetStream)
//   - File output (file system)
//   - HTTP POST output (webhooks)
//   - WebSocket output (broadcasting)
//   - HTTP gateway (bidirectional HTTP ↔ NATS request/reply)
//
// Semantic Layer - Graph Components (modular architecture):
//
//	Core (all tiers):
//	- graph-ingest (entity/triple CRUD, hierarchy inference)
//	- graph-index (OUTGOING, INCOMING, ALIAS, PREDICATE indexes)
//	- graph-gateway (GraphQL + MCP HTTP servers)
//
//	Statistical/Semantic tier:
//	- graph-embedding (vector embedding generation)
//	- graph-clustering (community detection, structural analysis, anomaly detection, LLM enhancement)
//
//	Optional indexes:
//	- graph-index-spatial (geospatial indexing)
//	- graph-index-temporal (time-based indexing)
//
// Semantic Layer - Rule Processing:
//   - Rule processor (rule-based transformations)
//
// Agentic Layer - LLM-powered autonomous agents:
//   - agentic-model (OpenAI-compatible LLM endpoint caller)
//   - agentic-tools (tool execution dispatcher)
//   - agentic-loop (state machine orchestrator with trajectory capture)
//
// Domain Layer (example processors):
//   - IoT sensor processor (JSON sensor data → Graphable SensorReading)
//   - Document processor (document processing)
//
// Note: Domain-specific components (MAVLink, robotics, etc.) are registered
// in separate modules like streamkit-robotics.
func Register(registry *component.Registry) error {
	// CRITICAL: Nil registry is a programming error (fatal), not invalid input
	if registry == nil {
		return pkgerrs.WrapFatal(
			errors.New("registry cannot be nil"),
			"ComponentRegistry", "Register", "registry validation")
	}

	if err := registerProtocolLayer(registry); err != nil {
		return err
	}

	if err := registerSemanticLayer(registry); err != nil {
		return err
	}

	if err := registerAgenticLayer(registry); err != nil {
		return err
	}

	return registerDomainLayer(registry)
}

// registerProtocolLayer registers protocol-layer components (inputs, processors, outputs, gateways).
func registerProtocolLayer(registry *component.Registry) error {
	// Network Inputs
	if err := udp.Register(registry); err != nil {
		return pkgerrs.WrapInvalid(err, "ComponentRegistry", "Register", "UDP input component registration")
	}

	if err := websocketinput.Register(registry); err != nil {
		return pkgerrs.WrapInvalid(err, "ComponentRegistry", "Register", "WebSocket input component registration")
	}

	if err := fileinput.Register(registry); err != nil {
		return pkgerrs.WrapInvalid(err, "ComponentRegistry", "Register", "File input component registration")
	}

	// Processors
	if err := jsongeneric.Register(registry); err != nil {
		return pkgerrs.WrapInvalid(err, "ComponentRegistry", "Register", "JSONGeneric processor component registration")
	}

	if err := jsonfilter.Register(registry); err != nil {
		return pkgerrs.WrapInvalid(err, "ComponentRegistry", "Register", "JSONFilter processor component registration")
	}

	if err := jsonmap.Register(registry); err != nil {
		return pkgerrs.WrapInvalid(err, "ComponentRegistry", "Register", "JSONMap processor component registration")
	}

	// Storage
	if err := objectstore.Register(registry); err != nil {
		return pkgerrs.WrapInvalid(err, "ComponentRegistry", "Register", "ObjectStore storage component registration")
	}

	// Outputs
	if err := file.Register(registry); err != nil {
		return pkgerrs.WrapInvalid(err, "ComponentRegistry", "Register", "File output component registration")
	}

	if err := httppost.Register(registry); err != nil {
		return pkgerrs.WrapInvalid(err, "ComponentRegistry", "Register", "HTTP POST output component registration")
	}

	if err := websocket.Register(registry); err != nil {
		return pkgerrs.WrapInvalid(err, "ComponentRegistry", "Register", "WebSocket output component registration")
	}

	// Gateways
	if err := gatewayhttp.Register(registry); err != nil {
		return pkgerrs.WrapInvalid(err, "ComponentRegistry", "Register", "HTTP gateway component registration")
	}

	return nil
}

// registerSemanticLayer registers semantic-layer components (graph and rule processors).
func registerSemanticLayer(registry *component.Registry) error {
	// Graph Components - Core (required for all tiers)
	if err := graphingest.Register(registry); err != nil {
		return pkgerrs.WrapInvalid(err, "ComponentRegistry", "Register", "graph-ingest component registration")
	}

	if err := graphindex.Register(registry); err != nil {
		return pkgerrs.WrapInvalid(err, "ComponentRegistry", "Register", "graph-index component registration")
	}

	if err := graphgateway.Register(registry); err != nil {
		return pkgerrs.WrapInvalid(err, "ComponentRegistry", "Register", "graph-gateway component registration")
	}

	// Query coordinator (orchestrates queries across components)
	if err := graphquery.Register(registry); err != nil {
		return pkgerrs.WrapInvalid(err, "ComponentRegistry", "Register", "graph-query component registration")
	}

	// Statistical/Semantic tier components (enabled via config)
	if err := graphembedding.Register(registry); err != nil {
		return pkgerrs.WrapInvalid(err, "ComponentRegistry", "Register", "graph-embedding component registration")
	}

	if err := graphclustering.Register(registry); err != nil {
		return pkgerrs.WrapInvalid(err, "ComponentRegistry", "Register", "graph-clustering component registration")
	}

	// Optional index components (enabled via config)
	if err := graphindexspatial.Register(registry); err != nil {
		return pkgerrs.WrapInvalid(err, "ComponentRegistry", "Register", "graph-index-spatial component registration")
	}

	if err := graphindextemporal.Register(registry); err != nil {
		return pkgerrs.WrapInvalid(err, "ComponentRegistry", "Register", "graph-index-temporal component registration")
	}

	// Rule processor
	if err := rule.Register(registry); err != nil {
		return pkgerrs.WrapInvalid(err, "ComponentRegistry", "Register", "Rule processor component registration")
	}

	return nil
}

// registerAgenticLayer registers agentic-layer components (LLM-powered autonomous agents).
func registerAgenticLayer(registry *component.Registry) error {
	if err := agenticmodel.Register(registry); err != nil {
		return pkgerrs.WrapInvalid(err, "ComponentRegistry", "Register", "agentic-model component registration")
	}

	if err := agentictools.Register(registry); err != nil {
		return pkgerrs.WrapInvalid(err, "ComponentRegistry", "Register", "agentic-tools component registration")
	}

	if err := agenticloop.Register(registry); err != nil {
		return pkgerrs.WrapInvalid(err, "ComponentRegistry", "Register", "agentic-loop component registration")
	}

	return nil
}

// registerDomainLayer registers domain-layer example processors.
func registerDomainLayer(registry *component.Registry) error {
	if err := iotsensor.Register(registry); err != nil {
		return pkgerrs.WrapInvalid(err, "ComponentRegistry", "Register", "IoT sensor processor component registration")
	}

	if err := document.Register(registry); err != nil {
		return pkgerrs.WrapInvalid(err, "ComponentRegistry", "Register", "Document processor component registration")
	}

	return nil
}
