// Package query provides an agentic tool that wraps TrustGraph's GraphRAG API.
//
// This tool enables SemStreams agents to query TrustGraph's document-extracted
// knowledge graph using natural language questions.
//
// # Tool Definition
//
//	Name: trustgraph_query
//	Description: Query TrustGraph's document knowledge graph using natural language
//
//	Parameters:
//	  - query (required): Natural language question to ask the knowledge graph
//	  - collection (optional): Specific collection to query
//
// # Usage
//
// The tool is automatically registered when the package is imported.
// Import it in your application to make it available to agentic-tools:
//
//	import _ "github.com/c360studio/semstreams/bridge/trustgraph/query"
//
// # Configuration
//
// The tool reads configuration from environment variables or can be configured
// at registration time:
//
//	TRUSTGRAPH_ENDPOINT: TrustGraph API URL (default: http://localhost:8088)
//	TRUSTGRAPH_API_KEY: Optional API key for authentication
//	TRUSTGRAPH_FLOW_ID: GraphRAG flow ID (default: graph-rag)
//	TRUSTGRAPH_TIMEOUT: Query timeout (default: 120s)
//
// # Example
//
//	Agent: "What are the maintenance procedures for pump model X?"
//	→ agentic-loop invokes trustgraph_query tool
//	→ trustgraph-query executor POSTs to TrustGraph GraphRAG API
//	→ Returns structured response from TrustGraph
package query
