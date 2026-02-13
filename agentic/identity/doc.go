// Package identity provides DID-based cryptographic identity for AGNTCY integration.
//
// # Overview
//
// This package implements the identity layer for agents in the AGNTCY (Internet of Agents)
// ecosystem. It provides:
//
//   - DID (Decentralized Identifier) support following W3C DID Core specification
//   - Verifiable Credentials for capability and delegation attestation
//   - Identity providers for local and AGNTCY-based identity management
//
// # DID Support
//
// DIDs are self-sovereign identifiers that don't depend on centralized registries.
// This package supports multiple DID methods:
//
//   - did:key - Public key based DIDs (default for local provider)
//   - did:web - DNS-based DIDs
//   - did:agntcy - AGNTCY-specific DIDs
//
// Example:
//
//	did, err := identity.ParseDID("did:key:z6MkhaXgBZDvotDkL5257faiztiGiC2QtKLGpbnnEGta2doK")
//	if err != nil {
//	    log.Fatal(err)
//	}
//	fmt.Println(did.Method) // "key"
//
// # Verifiable Credentials
//
// Credentials follow the W3C Verifiable Credentials data model and are used to:
//
//   - Attest agent capabilities (AgentCapabilityCredential)
//   - Delegate authority between agents (AgentDelegationCredential)
//   - Verify agent identity (AgentIdentityCredential)
//
// Example:
//
//	cred, err := identity.NewAgentCapabilityCredential(
//	    "urn:uuid:123",
//	    issuerDID,
//	    agentDID,
//	    "code-review",
//	    0.95,
//	)
//
// # Identity Providers
//
// Identity providers manage the lifecycle of agent identities:
//
// LocalProvider: For development and single-node deployments
//
//	provider, _ := identity.NewLocalProvider(identity.ProviderConfig{})
//	identity, _ := provider.CreateIdentity(ctx, identity.CreateIdentityOptions{
//	    DisplayName: "Code Reviewer",
//	    InternalRole: "reviewer",
//	})
//
// AgntcyProvider: For AGNTCY service integration (requires AGNTCY SDK)
//
//	provider, _ := identity.NewAgntcyProvider(identity.ProviderConfig{
//	    AgntcyURL: "https://directory.agntcy.org",
//	})
//
// # Agent Identity
//
// AgentIdentity combines a DID with credentials and metadata:
//
//	identity := NewAgentIdentity(did, "Architect Agent")
//	identity.InternalRole = "architect"
//	identity.AddCredential(capabilityCredential)
//
// # Integration with LoopEntity
//
// LoopEntity includes an optional Identity field for AGNTCY integration:
//
//	loop := agentic.LoopEntity{
//	    ID: "loop-123",
//	    Identity: &identity.AgentIdentity{
//	        DID: did,
//	        DisplayName: "Architect",
//	    },
//	}
//
// # See Also
//
//   - ADR-019: AGNTCY Integration
//   - docs/concepts/21-did-identity.md
//   - W3C DID Core: https://www.w3.org/TR/did-core/
//   - W3C VC Data Model: https://www.w3.org/TR/vc-data-model/
package identity
