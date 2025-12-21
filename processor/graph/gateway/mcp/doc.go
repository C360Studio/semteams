// Package mcp provides an MCP (Model Context Protocol) gateway as an output port
// of the graph processor.
//
// This package exposes graph query capabilities via the MCP protocol, enabling
// AI agents and LLM-based tools to query the semantic graph. The gateway uses
// Server-Sent Events (SSE) for transport and wraps the GraphQL executor to
// provide a single "graphql" tool for query execution.
//
// # Architecture
//
// The MCP gateway is an output port of the graph processor, not an independent component.
// This design reflects the true architectural dependency: the gateway requires in-process
// access to the GraphQL executor (which uses QueryManager) and cannot function without
// the graph processor running.
//
//	┌─────────────────────────────────────────┐
//	│           Graph Processor               │
//	│  ┌─────────────┐  ┌─────────────────┐   │
//	│  │ QueryManager│──│ GraphQL Gateway │───┼──► HTTP :8080/graphql
//	│  └─────────────┘  └─────────────────┘   │
//	│        │          ┌─────────────────┐   │
//	│        └──────────│   MCP Gateway   │───┼──► HTTP :8081/mcp (SSE)
//	│                   └─────────────────┘   │
//	└─────────────────────────────────────────┘
//
// # Usage
//
// The gateway is configured through the graph processor's configuration:
//
//	{
//	  "components": {
//	    "graph": {
//	      "type": "processor",
//	      "name": "graph-processor",
//	      "config": {
//	        "gateway": {
//	          "mcp": {
//	            "enabled": true,
//	            "bind_address": ":8081",
//	            "path": "/mcp"
//	          }
//	        }
//	      }
//	    }
//	  }
//	}
//
// # MCP Tools
//
// The gateway exposes a single "graphql" tool:
//
//	Tool: graphql
//	Arguments:
//	  - query (string, required): GraphQL query string
//	  - variables (object, optional): GraphQL variables
//
// Example tool call:
//
//	{
//	  "tool": "graphql",
//	  "arguments": {
//	    "query": "{ entity(id: \"robot-1\") { id type properties } }"
//	  }
//	}
//
// # Transport
//
// The gateway uses Server-Sent Events (SSE) for the MCP transport layer.
// Clients connect to the SSE endpoint and receive tool responses as events.
//
// # Rate Limiting
//
// The gateway includes built-in rate limiting (10 requests/second with burst of 20)
// to prevent abuse and ensure fair resource allocation.
//
// # Lifecycle
//
// The gateway lifecycle is managed by the graph processor:
//  1. Graph processor starts QueryManager
//  2. Graph processor creates GraphQL Resolver and Executor
//  3. Graph processor creates and starts MCP Server with Executor
//  4. On shutdown, MCP gateway stops before GraphQL gateway and QueryManager
package mcp
