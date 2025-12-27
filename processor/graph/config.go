package graph

import (
	"log/slog"

	"github.com/c360/semstreams/component"
	"github.com/c360/semstreams/metric"
	"github.com/c360/semstreams/natsclient"
	"github.com/c360/semstreams/processor/graph/datamanager"
	graphqlgateway "github.com/c360/semstreams/processor/graph/gateway/graphql"
	mcpgateway "github.com/c360/semstreams/processor/graph/gateway/mcp"
	"github.com/c360/semstreams/processor/graph/indexmanager"
	"github.com/c360/semstreams/processor/graph/messagemanager"
	"github.com/c360/semstreams/processor/graph/querymanager"
)

// Config holds processor configuration
type Config struct {
	// Ports defines input/output port configuration (standard pattern)
	// When configured, InputSubjects is ignored and ports are used instead
	Ports *component.PortConfig `json:"ports,omitempty" schema:"type:ports,description:Port configuration for inputs and outputs,category:basic"`

	Workers int `json:"workers"       schema:"type:int,description:Number of worker goroutines,default:10,category:basic"`

	QueueSize int `json:"queue_size"    schema:"type:int,description:Worker queue size,default:10000,category:basic"`

	// Deprecated: Use Ports instead
	InputSubject string `json:"input_subject,omitempty" schema:"type:string,description:NATS subject to subscribe for input messages (deprecated: use ports),category:basic"`

	// Deprecated: Use Ports instead
	// InputSubjects supports multiple input subjects for multi-stream subscription.
	// Each subject is mapped to its stream using convention: subject "component.action.type" → stream "COMPONENT"
	InputSubjects []string `json:"input_subjects,omitempty" schema:"type:array,description:Multiple NATS subjects to subscribe (deprecated: use ports),category:basic"`

	// JetStream configuration for durable message consumption
	// Deprecated: Use Ports with type=jetstream instead
	StreamName     string   `json:"stream_name,omitempty"     schema:"type:string,description:JetStream stream name for durable consumption (deprecated: use ports),category:advanced"`
	StreamSubjects []string `json:"stream_subjects,omitempty" schema:"type:array,description:JetStream stream subjects (deprecated: use ports),category:advanced"`
	ConsumerName   string   `json:"consumer_name,omitempty"   schema:"type:string,description:JetStream consumer name (deprecated: use ports),category:advanced"`

	// Component configurations

	MessageHandler *messagemanager.Config `json:"message_handler,omitempty" schema:"type:object,description:Message handler configuration,category:advanced"`

	DataManager *datamanager.Config `json:"data_manager,omitempty"    schema:"type:object,description:Data manager configuration,category:advanced"`

	Indexer *indexmanager.Config `json:"indexer,omitempty"         schema:"type:object,description:Index manager configuration,category:advanced"`

	Querier *querymanager.Config `json:"querier,omitempty"         schema:"type:object,description:Query manager configuration,category:advanced"`

	// GraphAnalysis configures graph analysis features (community detection, structural indexing, anomaly detection)
	GraphAnalysis *AnalysisConfig `json:"graph_analysis,omitempty" schema:"type:object,description:Graph analysis configuration,category:advanced"`

	// Gateway configures optional HTTP gateway output ports
	Gateway *GatewayConfig `json:"gateway,omitempty" schema:"type:object,description:HTTP gateway configuration,category:gateway"`
}

// GatewayConfig configures the HTTP gateway output ports for the graph processor.
// These gateways provide HTTP access to graph query capabilities.
type GatewayConfig struct {
	// GraphQL configures the GraphQL gateway output port
	GraphQL *graphqlgateway.Config `json:"graphql,omitempty" schema:"type:object,description:GraphQL gateway settings"`

	// MCP configures the MCP gateway output port
	MCP *mcpgateway.Config `json:"mcp,omitempty" schema:"type:object,description:MCP gateway settings"`
}

// ProcessorDeps holds processor dependencies
type ProcessorDeps struct {
	Config          *Config
	NATSClient      *natsclient.Client
	MetricsRegistry *metric.MetricsRegistry
	Logger          *slog.Logger
}
