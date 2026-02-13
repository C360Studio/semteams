// Package slim provides an input component that bridges SLIM (Secure Lightweight
// Instant Messaging) groups to SemStreams using MLS (Messaging Layer Security).
//
// # Overview
//
// The SLIM bridge enables cross-organizational agent communication by connecting
// to SLIM groups and translating messages to/from the SemStreams message format.
// SLIM provides end-to-end encrypted group messaging using the MLS protocol.
//
// # Architecture
//
//	┌─────────────────┐     ┌──────────────────┐     ┌─────────────────┐
//	│  SLIM Groups    │────▶│  SLIM Bridge     │────▶│  NATS/JetStream │
//	│  (MLS encrypted)│     │  (SemStreams)    │     │  (Agent Dispatch)│
//	└─────────────────┘     └──────────────────┘     └─────────────────┘
//	                               │
//	                               ▼
//	                        ┌──────────────────┐
//	                        │  Session Manager │
//	                        │  (MLS lifecycle) │
//	                        └──────────────────┘
//
// # Components
//
// The package consists of several key components:
//
//   - Component: The main LifecycleComponent that bridges SLIM to NATS
//   - SessionManager: Manages MLS group sessions and key ratcheting
//   - MessageMapper: Translates between SLIM and SemStreams message formats
//   - SLIMClient: Interface for SLIM protocol operations (stub for SDK)
//
// # Configuration
//
// Example configuration:
//
//	{
//	  "slim_endpoint": "wss://slim.agntcy.dev",
//	  "group_ids": ["did:agntcy:group:tenant-123"],
//	  "key_ratchet_interval": "1h",
//	  "reconnect_interval": "5s",
//	  "max_reconnect_attempts": 10,
//	  "identity_provider": "local"
//	}
//
// # Message Flow
//
// Inbound (SLIM → NATS):
//   - User messages → user.message.slim.{group_id}
//   - Task delegations → agent.task.slim.{group_id}
//
// Outbound (NATS → SLIM):
//   - Agent responses → SLIM group via SendResponse()
//   - Task results → SLIM group via SendTaskResult()
//
// # Session Lifecycle
//
//  1. Component starts and connects to SLIM service
//  2. Joins configured groups, establishing MLS sessions
//  3. Receives encrypted messages, decrypts, and publishes to NATS
//  4. Periodically ratchets MLS keys for forward secrecy
//  5. On shutdown, gracefully leaves all groups
//
// # MLS Key Ratcheting
//
// The session manager periodically ratchets MLS keys to maintain forward secrecy.
// This ensures that compromise of current keys does not expose past messages.
// The ratchet interval is configurable (default: 1 hour).
//
// # Security Considerations
//
//   - All SLIM messages are end-to-end encrypted using MLS
//   - Agent DIDs are used for authentication
//   - Key material is managed by the SLIM SDK
//   - No plaintext message content leaves the bridge
//
// # Usage
//
// Register the component with the component registry:
//
//	import slim "github.com/c360studio/semstreams/input/slim"
//
//	func init() {
//	    slim.Register(registry)
//	}
//
// # Implementation Status
//
// This package provides the infrastructure for SLIM integration. The actual
// SLIM protocol implementation requires the AGNTCY SLIM SDK, which provides:
//
//   - MLS protocol implementation
//   - Group management
//   - Key exchange and ratcheting
//   - Message encryption/decryption
//
// The SLIMClient interface defines the required operations and can be
// implemented when the SDK becomes available.
//
// # See Also
//
//   - input/a2a: A2A protocol adapter for agent-to-agent communication
//   - output/directory-bridge: Registers agents with AGNTCY directories
//   - agentic/identity: DID and verifiable credential management
//   - docs/concepts/22-slim-messaging.md: SLIM integration guide
package slim
