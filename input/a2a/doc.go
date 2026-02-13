// Package a2a provides an input component that implements the A2A (Agent-to-Agent)
// protocol for receiving task delegations from external agents.
//
// # Overview
//
// The A2A adapter enables agent-to-agent communication following the A2A protocol
// specification. It receives task requests from external agents and converts them
// to SemStreams TaskMessages for processing by the agentic system.
//
// # Architecture
//
//	┌─────────────────┐     ┌──────────────────┐     ┌─────────────────┐
//	│  External Agent │────▶│   A2A Adapter    │────▶│  NATS/JetStream │
//	│  (A2A Client)   │     │  (SemStreams)    │     │  (Agent Dispatch)│
//	└─────────────────┘     └──────────────────┘     └─────────────────┘
//	                               │
//	                               ▼
//	                        ┌──────────────────┐
//	                        │   Agent Card     │
//	                        │  (Capabilities)  │
//	                        └──────────────────┘
//
// # Components
//
// The package consists of several key components:
//
//   - Component: The main LifecycleComponent that handles A2A requests
//   - TaskMapper: Translates between A2A tasks and SemStreams TaskMessages
//   - AgentCardGenerator: Generates A2A agent cards from OASF records
//   - Config: Configuration for transport, authentication, and endpoints
//
// # Configuration
//
// Example configuration for HTTP transport:
//
//	{
//	  "transport": "http",
//	  "listen_address": ":8080",
//	  "agent_card_path": "/.well-known/agent.json",
//	  "enable_authentication": true,
//	  "request_timeout": "30s",
//	  "max_concurrent_tasks": 10
//	}
//
// Example configuration for SLIM transport:
//
//	{
//	  "transport": "slim",
//	  "slim_group_id": "did:agntcy:group:tenant-123",
//	  "enable_authentication": true
//	}
//
// # Transport Options
//
// The adapter supports two transport mechanisms:
//
//   - HTTP: RESTful endpoints for task submission and agent card retrieval
//   - SLIM: MLS-encrypted group messaging via the SLIM bridge
//
// # A2A Protocol Support
//
// The adapter implements the following A2A protocol endpoints:
//
//   - GET /.well-known/agent.json: Returns the agent card
//   - POST /tasks/send: Submit a new task
//   - GET /tasks/get?id=<id>: Get task status
//   - POST /tasks/cancel: Cancel a running task
//
// # Agent Card Generation
//
// Agent cards are automatically generated from OASF records stored in the
// OASF_RECORDS KV bucket. The card includes:
//
//   - Agent name and description
//   - Capabilities derived from OASF skills
//   - Authentication methods (DID-based)
//   - Supported input/output modes
//
// # Task Flow
//
// Inbound (A2A → NATS):
//  1. External agent submits task via A2A endpoint
//  2. Adapter validates authentication (DID verification)
//  3. Task converted to agentic.TaskMessage
//  4. Published to agent.task.a2a.{task_id}
//
// Outbound (NATS → A2A):
//  1. Agent completes task, publishes to agent.complete.*
//  2. Adapter converts result to A2A TaskResult
//  3. Response sent back to requesting agent
//
// # Authentication
//
// When EnableAuthentication is true, the adapter verifies incoming requests
// using DID-based authentication. The requester's DID can be provided via:
//
//   - Authorization header
//   - X-Agent-DID header
//   - Signed message body (for SLIM transport)
//
// # Usage
//
// Register the component with the component registry:
//
//	import a2a "github.com/c360studio/semstreams/input/a2a"
//
//	func init() {
//	    a2a.Register(registry)
//	}
//
// # See Also
//
//   - input/slim: SLIM bridge for encrypted cross-organizational messaging
//   - output/directory-bridge: Registers agents with AGNTCY directories
//   - processor/oasf-generator: Generates OASF records for agent card
//   - agentic/identity: DID and verifiable credential management
//   - docs/concepts/23-a2a-protocol.md: A2A integration guide
package a2a
