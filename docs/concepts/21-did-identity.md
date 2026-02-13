# DID-Based Identity

Decentralized Identifiers (DIDs) provide cryptographic identity for agents participating in the AGNTCY
Internet of Agents ecosystem.

## Overview

SemStreams implements W3C-compliant DID support for agent identity management. This enables:

- **Cryptographic verification** of agent identity across organizational boundaries
- **Verifiable credentials** for capability attestation and delegation
- **Federated discovery** through AGNTCY directories
- **Interoperability** with other AGNTCY-compatible agent systems

DIDs are **optional** — agents work without them. Add DIDs when you need cross-organizational agent
collaboration or compliance with identity verification requirements.

## What are DIDs?

A Decentralized Identifier (DID) is a globally unique identifier that doesn't depend on centralized
registries. Unlike email addresses or domain names, DIDs are:

- **Self-sovereign**: Owner controls the identifier without intermediaries
- **Cryptographically verifiable**: Backed by public key cryptography
- **Persistent**: Don't expire or require renewal
- **Resolvable**: Can be looked up to retrieve public keys and service endpoints

### DID Format

DIDs follow the W3C DID Core specification format:

```text
did:method:method-specific-id
│   │      │
│   │      └─ Unique identifier within the method
│   └──────── DID method (key, web, agntcy)
└──────────── Scheme (always "did")
```

**Examples:**

```text
did:key:z6MkhaXgBZDvotDkL5257faiztiGiC2QtKLGpbnnEGta2doK
did:web:example.com:agents:architect
did:agntcy:agent-architect-001
```

The method determines how the DID is created, resolved, and managed.

## Supported DID Methods

SemStreams supports three DID methods, each suited for different deployment scenarios:

### did:key - Public Key Based

**Best for**: Development, testing, single-node deployments

```text
did:key:z6MkhaXgBZDvotDkL5257faiztiGiC2QtKLGpbnnEGta2doK
        └────────────────────────────────────────────────┘
                      Multibase-encoded public key
```

- **Generation**: Derived directly from a public key (Ed25519)
- **Resolution**: Public key is embedded in the identifier
- **No external infrastructure**: Works offline
- **No key rotation**: If private key is lost, identity is lost

**When to use:**

- Local development and testing
- Single-organization deployments
- Prototyping AGNTCY integrations

**Example creation:**

```go
import "github.com/c360/semstreams/agentic/identity"

// Generate new did:key
provider, _ := identity.NewLocalProvider(identity.ProviderConfig{})
agentIdentity, _ := provider.CreateIdentity(ctx, identity.CreateIdentityOptions{
    DisplayName: "Architect Agent",
    Method:      identity.MethodKey,
})

fmt.Println(agentIdentity.DIDString())
// Output: did:key:z6Mk...
```

### did:web - DNS-Based

**Best for**: Organizations with existing web infrastructure

```text
did:web:example.com:agents:architect
        └──────────┘ └────┘ └────────┘
         Domain      Path    Agent ID
```

- **Resolution**: Uses HTTPS to fetch DID document from domain
- **Infrastructure**: Requires web server with TLS
- **Trust model**: Trust the domain (same as HTTPS)
- **Key rotation**: Supported by updating the hosted DID document

**When to use:**

- Organizations with existing domain infrastructure
- Enterprise deployments requiring familiar trust models
- Scenarios requiring key rotation capabilities

**DID Document location:**

```text
did:web:example.com:agents:architect
→ https://example.com/agents/architect/did.json
```

### did:agntcy - AGNTCY-Specific

**Best for**: Production AGNTCY integrations

```text
did:agntcy:agent-architect-001
           └──────────────────┘
              AGNTCY-managed ID
```

- **Management**: AGNTCY identity service handles lifecycle
- **Features**: Built-in credential issuance, revocation, rotation
- **Integration**: Native support for AGNTCY directories and SLIM messaging
- **Infrastructure**: Requires AGNTCY service deployment or subscription

**When to use:**

- Production multi-agent systems
- Cross-organizational agent collaboration
- Scenarios requiring revocation and audit trails

**Implementation status:** Stub implementation — requires AGNTCY SDK integration (see ADR-019).

## Verifiable Credentials

Verifiable Credentials (VCs) are cryptographically signed attestations about an agent. Think of them as
digital certificates that prove capabilities or authority.

### Credential Structure

Following the W3C Verifiable Credentials data model:

```json
{
  "@context": [
    "https://www.w3.org/2018/credentials/v1",
    "https://agntcy.org/credentials/v1"
  ],
  "id": "urn:uuid:123e4567-e89b-12d3-a456-426614174000",
  "type": ["VerifiableCredential", "AgentCapabilityCredential"],
  "issuer": "did:key:z6Mk...",
  "issuanceDate": "2024-01-15T10:30:00Z",
  "credentialSubject": {
    "id": "did:key:z6Mk...",
    "capability": "code-review",
    "confidence": 0.95
  },
  "proof": {
    "type": "Ed25519Signature2020",
    "created": "2024-01-15T10:30:00Z",
    "verificationMethod": "did:key:z6Mk...#key-1",
    "proofPurpose": "assertionMethod",
    "proofValue": "eyJhbGc..."
  }
}
```

### Credential Types

SemStreams defines three credential types for agent systems:

#### AgentCapabilityCredential

Attests that an agent has a specific capability.

```text
┌─────────────────────────────────────────────────────────┐
│ Issuer: Platform Identity Service                       │
│ Subject: did:key:z6Mk... (Architect Agent)             │
│ Claim: Has capability "software-design" (conf: 0.95)    │
│ Proof: Cryptographic signature by issuer                │
└─────────────────────────────────────────────────────────┘
```

**Subject fields:**

- `id`: Agent's DID
- `capability`: Capability name (e.g., "code-review", "data-analysis")
- `confidence`: Self-assessed confidence level (0.0-1.0)
- `scope`: Optional scope restriction (e.g., "golang-projects")

**Example creation:**

```go
cred, _ := identity.NewAgentCapabilityCredential(
    "urn:uuid:123",          // Credential ID
    issuerDID.String(),      // Issuer DID
    agentDID.String(),       // Agent DID
    "code-review",           // Capability
    0.95,                    // Confidence
)
```

#### AgentDelegationCredential

Delegates authority from one agent to another.

```text
┌─────────────────────────────────────────────────────────┐
│ Issuer: Platform Identity Service                       │
│ Subject: did:key:z6Mk... (Editor Agent)                │
│ Claim: Delegated by did:key:z6Mk... (Architect Agent)  │
│        Capabilities: ["file-write", "git-commit"]       │
│ Proof: Cryptographic signature by issuer                │
└─────────────────────────────────────────────────────────┘
```

**Subject fields:**

- `id`: Delegate agent's DID (receives authority)
- `delegator`: Delegating agent's DID (grants authority)
- `capabilities`: List of delegated capabilities
- `scope`: Optional scope restriction
- `validUntil`: Optional expiration timestamp

**Example creation:**

```go
cred, _ := identity.NewAgentDelegationCredential(
    "urn:uuid:456",          // Credential ID
    issuerDID.String(),      // Issuer DID
    delegateDID.String(),    // Delegate (receives authority)
    delegatorDID.String(),   // Delegator (grants authority)
    []string{"file-write"},  // Delegated capabilities
)
```

#### AgentIdentityCredential

Verifies the basic identity of an agent.

Used for bootstrapping trust when an agent first joins a system. Typically issued during agent
provisioning by the platform's identity service.

### Credential Lifecycle

```text
┌─────────────────────────────────────────────────────────────────┐
│                    Credential Lifecycle                          │
├─────────────────────────────────────────────────────────────────┤
│                                                                  │
│  1. Issuance                                                     │
│     Provider creates credential → signs with issuer key          │
│                                                                  │
│  2. Presentation                                                 │
│     Agent includes credentials when interacting with other       │
│     systems (SLIM handshake, A2A delegation)                     │
│                                                                  │
│  3. Verification                                                 │
│     Receiver resolves issuer DID → validates signature →         │
│     checks expiration/revocation                                 │
│                                                                  │
│  4. Expiration/Revocation                                        │
│     Credentials expire after validUntil timestamp, or can be     │
│     revoked via credentialStatus field                           │
│                                                                  │
└─────────────────────────────────────────────────────────────────┘
```

## Agent Identity

The `AgentIdentity` struct combines a DID with credentials and metadata:

```go
type AgentIdentity struct {
    DID          DID                    // Decentralized identifier
    DisplayName  string                 // Human-readable name
    Credentials  []VerifiableCredential // Held credentials
    InternalRole string                 // Local role (architect, editor, reviewer)
    Created      time.Time              // Identity creation timestamp
    Updated      time.Time              // Last update timestamp
    Metadata     map[string]any         // Additional metadata
}
```

### Creating an Identity

```go
import "github.com/c360/semstreams/agentic/identity"

// Initialize provider
provider, err := identity.NewLocalProvider(identity.ProviderConfig{})
if err != nil {
    log.Fatal(err)
}

// Create agent identity
agentIdentity, err := provider.CreateIdentity(ctx, identity.CreateIdentityOptions{
    DisplayName: "Code Reviewer",
    InternalRole: "reviewer",
    Method: identity.MethodKey,
    InitialCapabilities: []string{"code-review", "security-audit"},
})

// Identity now has:
// - A did:key DID
// - Two AgentCapabilityCredentials (code-review, security-audit)
// - Internal role metadata preserved for compatibility
```

### Capability Checking

```go
// Check if agent has a specific capability
if agentIdentity.HasCapability("code-review") {
    fmt.Println("Agent can perform code reviews")
}

// Get all capabilities
capabilities := agentIdentity.GetCapabilities()
fmt.Println("Agent capabilities:", capabilities)
// Output: Agent capabilities: [code-review security-audit]
```

### Credential Management

```go
// Add a new credential
delegationCred, _ := provider.IssueCredential(
    ctx,
    agentIdentity.DID,
    identity.TypeAgentDelegationCredential,
    claims,
)
agentIdentity.AddCredential(*delegationCred)

// Get credentials by type
capCreds := agentIdentity.GetCredentialsByType(identity.TypeAgentCapabilityCredential)

// Remove expired credentials
validCreds := agentIdentity.GetValidCredentials()
```

## Integration with LoopEntity

Agent loops optionally include an `Identity` field for AGNTCY integration:

```go
type LoopEntity struct {
    ID                 string
    State              LoopState
    Role               string
    // ... other fields ...

    // AGNTCY identity (optional)
    Identity *identity.AgentIdentity `json:"identity,omitempty"`
}
```

### When LoopEntity Has Identity

```text
┌────────────────────────────────────────────────────────────────┐
│                    Loop with Identity                          │
├────────────────────────────────────────────────────────────────┤
│                                                                 │
│  ┌──────────────┐                                              │
│  │  LoopEntity  │                                              │
│  ├──────────────┤                                              │
│  │ ID: loop_123 │                                              │
│  │ Role: arch   │                                              │
│  │              │                                              │
│  │ Identity     │                                              │
│  │  ├─ DID      │──► did:key:z6Mk...                          │
│  │  ├─ Display  │──► "Architect Agent"                        │
│  │  ├─ Creds    │──► [AgentCapabilityCredential, ...]         │
│  │  └─ Role     │──► "architect"                              │
│  └──────────────┘                                              │
│                                                                 │
│  External systems can:                                         │
│  • Verify agent identity cryptographically                     │
│  • Check agent capabilities via credentials                    │
│  • Delegate authority through credential chains                │
│  • Trace agent actions via DID audit trail                     │
│                                                                 │
└────────────────────────────────────────────────────────────────┘
```

### Creating a Loop with Identity

```go
// Create identity
provider, _ := identity.NewLocalProvider(identity.ProviderConfig{})
agentIdentity, _ := provider.CreateIdentity(ctx, identity.CreateIdentityOptions{
    DisplayName: "Architect Agent",
    InternalRole: "architect",
    InitialCapabilities: []string{"software-design"},
})

// Create loop with identity
loop := agentic.LoopEntity{
    ID:       "loop_123",
    State:    agentic.LoopStateExploring,
    Role:     "architect",
    Identity: agentIdentity,  // DID-based identity attached
}
```

### Identity in AGNTCY Workflows

When a loop has identity, it participates in AGNTCY workflows:

1. **OASF Record Generation**: Identity's DID and capabilities are included in OASF records
2. **Directory Registration**: Agent is registered in AGNTCY directories with DID
3. **SLIM Messaging**: DID is used for authentication in SLIM group membership
4. **A2A Protocol**: DID appears in A2A task delegation messages
5. **Audit Trail**: Agent actions are traced back to DID for compliance

## Identity Predicates

Agent identity information can be stored in the knowledge graph using standardized predicates:

### Core Identity Predicates

| Predicate | Object Type | Description |
|-----------|-------------|-------------|
| `agentic.identity.did` | String | Agent's DID string |
| `agentic.identity.display_name` | String | Human-readable name |
| `agentic.identity.method` | String | DID method (key, web, agntcy) |
| `agentic.identity.created` | ISO8601 | Identity creation timestamp |
| `agentic.identity.updated` | ISO8601 | Last update timestamp |

### Credential Predicates

| Predicate | Object Type | Description |
|-----------|-------------|-------------|
| `agentic.identity.credential` | Credential ID | Link to credential entity |
| `agentic.identity.capability` | String | Capability name (extracted from credential) |
| `agentic.identity.delegated_by` | DID | Delegator DID (from delegation credential) |

### Example Graph Representation

```text
Loop Entity: loop_123
├─ agentic.identity.did → "did:key:z6Mk..."
├─ agentic.identity.display_name → "Architect Agent"
├─ agentic.identity.method → "key"
├─ agentic.identity.capability → "software-design"
├─ agentic.identity.capability → "api-design"
└─ agentic.identity.created → "2024-01-15T10:30:00Z"

Credential Entity: urn:uuid:123
├─ credential.type → "AgentCapabilityCredential"
├─ credential.issuer → "did:key:z6Mk..."
├─ credential.subject → "did:key:z6Mk..."
├─ credential.capability → "software-design"
├─ credential.confidence → 0.95
└─ credential.issued → "2024-01-15T10:30:00Z"
```

This enables queries like:

- "Find all agents with capability 'code-review'"
- "List agents using did:web method"
- "Show agents delegated by architect agent"

## Provider Architecture

SemStreams uses the Provider pattern for identity lifecycle management:

```text
┌─────────────────────────────────────────────────────────────────┐
│                     Provider Interface                          │
├─────────────────────────────────────────────────────────────────┤
│                                                                  │
│  CreateIdentity(opts) → AgentIdentity                           │
│  ResolveIdentity(did) → AgentIdentity                           │
│  IssueCredential(subject, type, claims) → VerifiableCredential  │
│  VerifyCredential(cred) → bool                                  │
│  UpdateIdentity(identity) → error                               │
│  DeleteIdentity(did) → error                                    │
│                                                                  │
└─────────────────────────────────────────────────────────────────┘
         ▲                            ▲
         │                            │
         │                            │
    ┌────┴────────┐         ┌────────┴─────────┐
    │   Local     │         │    AGNTCY        │
    │  Provider   │         │   Provider       │
    ├─────────────┤         ├──────────────────┤
    │ • did:key   │         │ • did:agntcy     │
    │ • In-memory │         │ • AGNTCY service │
    │ • Dev/test  │         │ • Production     │
    └─────────────┘         └──────────────────┘
```

### LocalProvider

In-memory implementation for development and single-node deployments:

```go
provider, err := identity.NewLocalProvider(identity.ProviderConfig{
    KeyStorePath: "/var/lib/semstreams/keys",  // Optional persistent storage
})

// Creates did:key identities
// Stores keys and identities in memory (with optional persistence)
// Self-signs credentials using issuer key
```

**Characteristics:**

- **Key generation**: Ed25519 keypairs generated locally
- **Storage**: In-memory maps (optionally persisted to disk)
- **Credential signing**: Self-signed using provider's issuer key
- **Resolution**: Local lookup only
- **Trust model**: Trust the local provider's issuer key

**Best for:**

- Development and testing
- Single-node deployments
- Offline scenarios
- Prototyping before AGNTCY integration

### AgntcyProvider

Integration with AGNTCY identity service (stub implementation):

```go
provider, err := identity.NewAgntcyProvider(identity.ProviderConfig{
    AgntcyURL: "https://identity.agntcy.org",
    IssuerDID: "did:agntcy:platform-issuer",
})

// Creates did:agntcy identities
// Delegates to AGNTCY service for lifecycle
// Supports credential revocation and rotation
```

**Characteristics:**

- **Key management**: AGNTCY service manages keys
- **Storage**: Federated across AGNTCY network
- **Credential signing**: AGNTCY identity service signs
- **Resolution**: DID resolution via AGNTCY protocol
- **Trust model**: Trust AGNTCY federation governance

**Implementation status:** Stub — requires AGNTCY SDK integration (Phase 2, see ADR-019).

### Provider Factory

Select provider based on configuration:

```go
config := identity.ProviderConfig{
    ProviderType: "local",  // or "agntcy"
}

provider, err := identity.DefaultProviderFactory(config)
```

## Security Considerations

### Key Management

**Private key security is critical** — compromised keys mean compromised identity.

**LocalProvider considerations:**

- Private keys stored in-memory (ephemeral) or on disk (persistent)
- Disk storage should use encrypted filesystem
- No key rotation support — lost key = lost identity
- Suitable for development, not production multi-agent deployments

**AgntcyProvider considerations:**

- Keys managed by AGNTCY identity service
- Hardware security module (HSM) support
- Key rotation capabilities
- Revocation and audit trails

**Best practice:** Use LocalProvider for development, AgntcyProvider for production.

### Trust Boundaries

```text
┌─────────────────────────────────────────────────────────────────┐
│                      Trust Model                                │
├─────────────────────────────────────────────────────────────────┤
│                                                                  │
│  Internal (same SemStreams instance)                            │
│  ──────────────────────────────────────────                     │
│  • Agent loops trust each other implicitly                      │
│  • Internal role field sufficient for authorization             │
│  • DIDs are optional enhancement                                │
│                                                                  │
│  External (cross-organization via AGNTCY)                       │
│  ──────────────────────────────────────────                     │
│  • Agents verify DIDs cryptographically                         │
│  • Credentials checked before accepting task delegation         │
│  • No implicit trust — must present valid credentials           │
│  • DIDs are required for authentication                         │
│                                                                  │
└─────────────────────────────────────────────────────────────────┘
```

SemStreams' internal trust model (SOP checks, adversarial reviewers, runtime risk monitoring) remains
unchanged. DIDs add an external trust layer for cross-organizational collaboration.

### Credential Verification

Always verify credentials before acting on delegated authority:

```go
// Verify credential signature and validity
valid, err := provider.VerifyCredential(ctx, credential)
if err != nil || !valid {
    return fmt.Errorf("invalid credential")
}

// Check expiration
if credential.IsExpired() {
    return fmt.Errorf("credential expired")
}

// Extract and validate claims
var subject identity.AgentCapabilitySubject
if err := credential.GetSubject(&subject); err != nil {
    return fmt.Errorf("invalid subject: %w", err)
}

// Check capability matches required capability
if subject.Capability != requiredCapability {
    return fmt.Errorf("insufficient capability")
}
```

### Revocation

W3C VC standard supports credential revocation via `credentialStatus` field:

```json
{
  "credentialStatus": {
    "id": "https://identity.agntcy.org/status/123",
    "type": "StatusList2021Entry"
  }
}
```

**LocalProvider**: Does not support revocation (credentials are trusted until expiration).

**AgntcyProvider**: Supports revocation via AGNTCY identity service (when implemented).

**Best practice**: Set short expiration times for sensitive capabilities when revocation is unavailable.

## Examples

### Complete Agent Identity Setup

```go
package main

import (
    "context"
    "fmt"
    "log"

    "github.com/c360/semstreams/agentic"
    "github.com/c360/semstreams/agentic/identity"
)

func main() {
    ctx := context.Background()

    // 1. Initialize provider
    provider, err := identity.NewLocalProvider(identity.ProviderConfig{})
    if err != nil {
        log.Fatal(err)
    }

    // 2. Create agent identity
    agentIdentity, err := provider.CreateIdentity(ctx, identity.CreateIdentityOptions{
        DisplayName: "Architect Agent",
        InternalRole: "architect",
        Method: identity.MethodKey,
        InitialCapabilities: []string{
            "software-design",
            "api-design",
        },
    })
    if err != nil {
        log.Fatal(err)
    }

    fmt.Printf("Created identity: %s\n", agentIdentity.DIDString())
    fmt.Printf("Capabilities: %v\n", agentIdentity.GetCapabilities())

    // 3. Create loop with identity
    loop := agentic.LoopEntity{
        ID:       "loop_architect_001",
        State:    agentic.LoopStateExploring,
        Role:     "architect",
        Identity: agentIdentity,
    }

    // 4. Verify capability before operation
    if !loop.Identity.HasCapability("software-design") {
        log.Fatal("Agent lacks required capability")
    }

    fmt.Println("Agent authorized for software design tasks")
}
```

### Delegation Between Agents

```go
// Architect delegates file-write capability to editor
architectIdentity := architectLoop.Identity
editorIdentity := editorLoop.Identity

// Issue delegation credential
delegationCred, err := provider.IssueCredential(
    ctx,
    editorIdentity.DID,
    identity.TypeAgentDelegationCredential,
    identity.AgentDelegationSubject{
        ID:           editorIdentity.DID.String(),
        Delegator:    architectIdentity.DID.String(),
        Capabilities: []string{"file-write", "git-commit"},
        Scope:        "src/",
    },
)
if err != nil {
    log.Fatal(err)
}

// Editor receives and stores credential
editorIdentity.AddCredential(*delegationCred)

// When editor attempts file write, verify delegation
valid, err := provider.VerifyCredential(ctx, delegationCred)
if err != nil || !valid {
    log.Fatal("Invalid delegation")
}

fmt.Println("Editor authorized via architect delegation")
```

### Resolving External Agent Identity

```go
// Parse DID from external message
externalDID, err := identity.ParseDID("did:key:z6MkhaXgBZDvotDkL5257faiztiGiC2QtKLGpbnnEGta2doK")
if err != nil {
    log.Fatal(err)
}

// Resolve identity (local or via AGNTCY)
externalIdentity, err := provider.ResolveIdentity(ctx, *externalDID)
if err != nil {
    log.Printf("Could not resolve identity: %v", err)
    // Reject task delegation from unknown agent
    return
}

// Verify capabilities before accepting task
if !externalIdentity.HasCapability("code-review") {
    log.Fatal("External agent lacks required capability")
}

fmt.Printf("Accepting task from %s\n", externalIdentity.DisplayName)
```

## When to Use DIDs

**Use DIDs when:**

- Integrating with AGNTCY directories and SLIM messaging
- Building multi-organization agent systems
- Compliance requires verifiable identity audit trails
- Agents need to present cryptographic proofs of capabilities

**Skip DIDs when:**

- All agents are within same SemStreams instance (internal role field sufficient)
- Building prototypes or single-node systems
- Identity management overhead isn't justified by collaboration needs

DIDs are a foundation for the Internet of Agents, but they're optional within SemStreams unless
cross-organizational agent collaboration is required.

## See Also

- [Agentic Systems](./11-agentic-systems.md) — Core agent architecture
- [OASF Integration](./20-oasf-integration.md) — Agent capability records
- [ADR-019: AGNTCY Integration](../architecture/adr-019-agntcy-integration.md) — Integration strategy
- [W3C DID Core Specification](https://www.w3.org/TR/did-core/)
- [W3C Verifiable Credentials Data Model](https://www.w3.org/TR/vc-data-model/)
