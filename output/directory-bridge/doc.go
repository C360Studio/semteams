// Package directorybridge provides an output component that registers agents
// with AGNTCY directories using OASF (Open Agent Specification Framework) records.
//
// # Overview
//
// The directory-bridge component watches for OASF records in a NATS KV bucket
// and automatically registers/updates agents with AGNTCY directory services.
// It maintains registrations through periodic heartbeats and handles
// deregistration on shutdown.
//
// # Architecture
//
//	┌─────────────────┐     ┌──────────────────┐     ┌─────────────────┐
//	│  OASF_RECORDS   │────▶│  Directory       │────▶│  AGNTCY         │
//	│  KV Bucket      │     │  Bridge          │     │  Directory      │
//	└─────────────────┘     └──────────────────┘     └─────────────────┘
//	                               │
//	                               ▼
//	                        ┌──────────────────┐
//	                        │  Identity        │
//	                        │  Provider        │
//	                        └──────────────────┘
//
// # Components
//
// The package consists of several key components:
//
//   - Component: The main LifecycleComponent that watches KV and orchestrates registration
//   - DirectoryClient: HTTP client for communicating with AGNTCY directory APIs
//   - RegistrationManager: Manages the lifecycle of agent registrations including heartbeats
//   - Config: Configuration for directory URL, heartbeat intervals, and retry settings
//
// # Configuration
//
// Example configuration:
//
//	{
//	  "directory_url": "https://directory.agntcy.dev",
//	  "heartbeat_interval": "30s",
//	  "registration_ttl": "5m",
//	  "identity_provider": "local",
//	  "oasf_kv_bucket": "OASF_RECORDS",
//	  "retry_count": 3,
//	  "retry_delay": "1s"
//	}
//
// # Identity Integration
//
// The bridge uses the agentic/identity package to create or resolve DIDs
// for agents being registered. Supported identity providers:
//
//   - "local": Creates did:key identities locally
//   - "agntcy": Resolves identities through AGNTCY identity service (future)
//
// # NATS Topology
//
// Input:
//   - OASF_RECORDS KV bucket (watch): Receives OASF records from oasf-generator
//
// Output:
//   - directory.registration.* (optional): Emits registration events
//
// # Registration Lifecycle
//
//  1. KV watcher detects new/updated OASF record
//  2. Component parses record and extracts entity ID
//  3. RegistrationManager creates/retrieves DID identity
//  4. DirectoryClient sends registration request
//  5. Registration stored with expiration time
//  6. Heartbeat loop maintains registration before expiry
//  7. On shutdown, all agents are deregistered
//
// # Usage
//
// Register the component with the component registry:
//
//	import directorybridge "github.com/c360studio/semstreams/output/directory-bridge"
//
//	func init() {
//	    directorybridge.Register(registry)
//	}
//
// # See Also
//
//   - processor/oasf-generator: Generates OASF records from entity predicates
//   - agentic/identity: DID and verifiable credential management
//   - docs/architecture/adr-019-agntcy-integration.md: Architecture decision record
package directorybridge
