# ADR-019: AGNTCY / Internet of Agents Integration

## Status

Proposed

## Context

AGNTCY is a Linux Foundation project (backed by Cisco, Dell, Google Cloud, Oracle, Red Hat, and 75+ companies) that provides inter-agent infrastructure: discovery, identity, messaging, and observability for AI agents across organizational boundaries. It was donated to the Linux Foundation in July 2025 and integrates with both Google's A2A protocol and Anthropic's MCP.

### Why This Matters for SemStreams

Using a military analogy: AGNTCY solves the **inter-platoon communication problem** — how independent units coordinate across organizational boundaries. SemStreams solves the **intra-platoon coordination problem** — how a team of agents with different capabilities executes missions under a shared command structure.

These are complementary, not competing. Building AGNTCY compatibility reinforces our positioning:

- **Government clients**: "Built on standards-aligned ontologies (BFO/CCO/PROV-O), AGNTCY-ready with DID identity, discoverable through open directories."
- **Ecosystem**: "Build your agent on semstreams, get Internet of Agents participation out of the box."
- **Differentiation**: "Anyone can adopt AGNTCY protocols. Our persistent semantic knowledge graph and validation-with-retry architecture is what makes agents actually reliable."

### Key Insight

This is a **semstreams concern, not a semspec concern**. Almost every integration point maps to infrastructure-layer capabilities that any semstreams-based agent should inherit. Building AGNTCY compatibility at the semstreams layer means every domain-specific agent built on the platform gets Internet of Agents participation for free.

### AGNTCY Core Components

| Component | What It Does |
|-----------|--------------|
| **OASF** | Open Agent Schema Framework. Standardized descriptions of agent capabilities. OCI-based, supports A2A agent cards and MCP server descriptions. |
| **Agent Directory** | Federated discovery service. Agents register capabilities; other agents query to find collaborators. |
| **SLIM** | Secure Low-Latency Interactive Messaging. gRPC-based pub/sub with MLS encryption, quantum-safe security. Has an IETF Internet-Draft. |
| **Identity** | Decentralized identity using DIDs and Verifiable Credentials. Cryptographic agent identity verification across organizational boundaries. |
| **Observability** | Telemetry collectors, evaluation tools, OpenTelemetry integration. Can transport OTEL data over SLIM. |
| **A2A Support** | Agent-to-Agent protocol (Google, now Linux Foundation). Request/response task delegation across systems. |

### Current SemStreams Architecture Fit

Our existing adapter pattern (`input/cli`, `input/slack`, `input/discord`) provides the template for AGNTCY integration. Each external protocol becomes an input/output adapter that bridges to internal NATS subjects:

```text
EXTERNAL (Current)              INTERNAL
  CLI    ──► input/cli   ──┐
  Slack  ──► input/slack ──┤  NATS subjects ──► agentic-loop
  Web    ──► input/web   ──┘
```

AGNTCY adapters follow the same pattern — they are peers of existing input/output adapters connecting to the same NATS subject topology.

## Decision

Integrate AGNTCY capabilities through infrastructure adapters at the semstreams layer. No changes required to agentic-loop, agentic-model, agentic-tools, graph-ingest, or graph-index.

### Integration Architecture

```text
EXTERNAL (AGNTCY / Internet of Agents)
  SLIM Network ──► input/slim ──┐
  A2A Protocol ──► input/a2a  ──┤  NATS subjects    ┌──────────────┐
  OASF Directory ◄── dir-bridge ┤ ◄──────────────── │  agentic-    │
  DID Identity ◄──── id-bridge  ┤                   │    loop      │
  OTEL Export  ◄──── otel-out  ─┘                   └──────────────┘

INTERNAL (Existing SemStreams)
  CLI    ──► input/cli   ──┐
  Slack  ──► input/slack ──┤  Same NATS subjects
  Web    ──► input/web   ──┘
```

### Component Specifications

#### 1. OASF Record Generator

**Purpose**: Generate OASF-compatible agent capability descriptions from our existing agentic vocabulary in the knowledge graph.

**SemStreams Mapping**: Our `agentic.capability.*` and `agentic.intent.*` predicates already describe agent capabilities semantically. OASF records are a serialization format for data we already have. Since our vocabulary has IRI mappings to W3C standards (PROV-O, BFO, CCO), the mapping to OASF schema is largely mechanical.

**Implementation**: A processor component that subscribes to agentic entity change events and generates/updates OASF records. Can be triggered on-demand or run as a continuous sync.

| Input | Output |
|-------|--------|
| Agentic entities with `agentic.capability.*`, `agentic.intent.*`, `agentic.action.*` predicates | OASF records conforming to Open Agentic Schema Framework specification |

**Priority**: HIGH — Low effort, high value. Foundation for everything else. Government clients will specifically ask about standards compliance and discoverability.

#### 2. Directory Bridge

**Purpose**: Register and maintain our agents' presence in AGNTCY Agent Directories so external systems can discover them.

**Implementation**: Consumes OASF records generated by the OASF component and pushes them to one or more AGNTCY directory instances via their API. Handles periodic heartbeat/refresh to maintain registration. Supports federated directory topology where organizations run their own directories that sync.

**Pattern**: Output processor that publishes to AGNTCY directory API on entity change events.

**Priority**: MEDIUM — Dependent on OASF record generator. Pairs naturally with it.

#### 3. SLIM Bridge (input/slim)

**Purpose**: Bidirectional bridge between AGNTCY's SLIM messaging network and our internal NATS pub/sub.

**SLIM Characteristics**:

- Built on gRPC with pub/sub extensions (not native NATS)
- MLS (Message Layer Security) end-to-end encryption with quantum-safe options
- Group-based communication model (agents join secure groups)
- DID-based hierarchical channel naming
- Supports SLIM-RPC for A2A and MCP-over-SLIM natively

**Implementation**: A semstreams processor component running the SLIM SDK as a client. It joins SLIM groups, translates incoming SLIM messages to NATS `UserMessage` format on `user.message.slim.*` subjects, and publishes `agent.complete.*` responses back through SLIM. Follows the same adapter pattern as `input/cli` and `input/discord`.

**Key Consideration**: SLIM has its own session management (MLS key ratcheting, group membership). The bridge component needs to manage SLIM session lifecycle independently from NATS connections. These are two separate connection lifecycles that the bridge coordinates.

**Priority**: MEDIUM — Important for cross-organizational agent collaboration, but only needed when we have external agents to talk to. OASF/directory work is a prerequisite.

#### 4. Identity Integration

**Purpose**: Add DID-based verifiable credentials to our agent identity model so external systems can cryptographically verify our agents.

**What AGNTCY Provides**:

- Identity service for creating and managing agent DIDs
- Verifiable Credential issuance (Agent Badges, MCP Server Badges)
- Identity Provider integration (connect existing org identity)
- Policy engine for agentic service authorization

**Implementation**: Extend the existing agent role/permission model in agentic-loop to include DID identifiers. When agents are instantiated, they receive a DID and associated verifiable credentials. These are presented during SLIM handshakes and A2A interactions.

Our internal trust model (SOP checks, adversarial reviewers, runtime risk monitoring) remains unchanged; DIDs add an external trust layer on top.

**Priority**: MEDIUM-HIGH for government clients. DID/VC is increasingly a procurement requirement. Can be implemented incrementally starting with basic DID assignment.

#### 5. A2A Protocol Adapter

**Purpose**: Enable task delegation to/from external agents using the A2A (Agent-to-Agent) protocol.

**SemStreams Mapping**: A2A task delegation maps to `agent.task.*` and `agent.complete.*` patterns. An A2A adapter translates between A2A protocol messages and our internal NATS subjects.

**Implementation**: Protocol adapter in the router layer. Receives A2A task requests, translates to internal task format, routes through agentic-loop, and responds with A2A-formatted results.

**Priority**: MEDIUM — Part of Phase 3 with SLIM bridge.

#### 6. OpenTelemetry Export

**Purpose**: Export telemetry data in OpenTelemetry format, optionally transported over SLIM for cross-system observability.

**Implementation**: An output processor that formats our existing observability data (loop state, task progress, agent activity, token usage) as OTEL spans and metrics. Can export to standard OTEL collectors or via SLIM's OTEL transport for integrated multi-agent observability.

**Priority**: LOW — Nice-to-have. We already have solid observability. This becomes important when participating in multi-agent workflows where a central orchestrator needs visibility across all agents.

### Recommended Phasing

Aligned with current roadmap priorities. None of this blocks the UI redesign or sources/knowledge management work.

| Phase | Components | Outcome | Effort |
|-------|------------|---------|--------|
| **Phase 1** | OASF Record Generator | Our agents are describable in the standard format. Foundation for everything else. Immediate value for standards-compliance conversations with government clients. | Small — Serialization of existing data |
| **Phase 2** | Directory Bridge + Basic DID Identity | Our agents are discoverable and verifiable by external systems. Can participate in the Internet of Agents ecosystem. | Medium — New component + AGNTCY SDK integration |
| **Phase 3** | SLIM Bridge + A2A Adapter | Full cross-organizational agent communication. External agents can delegate tasks to semstreams agents and vice versa. | Medium-Large — Two new adapters + session management |
| **Phase 4** | OTEL Export + Advanced Identity Policies | Full observability integration and fine-grained authorization policies for multi-org deployments. | Small-Medium — Export adapters |

## Consequences

### Positive

- **Standards Compliance**: OASF records provide immediate proof of standards alignment for government procurement
- **Ecosystem Participation**: Agents built on semstreams automatically participate in the Internet of Agents
- **Federated Discovery**: External systems can find and invoke our agents through standard directories
- **Cryptographic Identity**: DID-based identity satisfies enterprise security requirements
- **Cross-Org Collaboration**: SLIM enables secure agent-to-agent communication across organizational boundaries
- **Adapter Pattern Consistency**: Integration follows established input/output adapter patterns
- **No Core Changes**: agentic-loop, agentic-model, and graph components remain unchanged

### Negative

- **SDK Dependencies**: SLIM SDK and AGNTCY libraries add external dependencies
- **Session Complexity**: SLIM's MLS session management adds lifecycle coordination complexity
- **Standards Evolution**: AGNTCY is a new project; specifications may evolve
- **Testing Complexity**: Integration tests require SLIM/directory infrastructure

### What NOT to Build

AGNTCY has Linux Foundation governance, Cisco engineering, and 75+ company backing. Do not rebuild:

| Component | Reason |
|-----------|--------|
| SLIM messaging nodes or infrastructure | Use their SDK as a client only |
| Agent Directory service | Register with theirs, don't build our own discovery service |
| DID/VC issuance infrastructure | Use their identity service or a standard DID library |
| OASF schema definitions | Conform to their schema, extend through their contribution process if needed |

**Our value is in the semantic knowledge graph, spec-driven workflows, and disciplined agent coordination. AGNTCY provides the plumbing to expose that value to the broader ecosystem. Use their plumbing.**

## Implementation Requirements

### Phase 1: OASF Record Generator

**New Component**: `processor/oasf-generator/`

```go
type OASFGenerator struct {
    component.Base
    graphClient  *graph.Client
    oasfTemplate *template.Template
}

func (g *OASFGenerator) Process(ctx context.Context, msg message.Message) error {
    // 1. Extract agentic entity from message
    // 2. Query graph for capability/intent/action predicates
    // 3. Map predicates to OASF schema fields
    // 4. Generate OASF record JSON
    // 5. Publish to output port
}
```

**Files to Create**:

| File | Purpose |
|------|---------|
| `processor/oasf-generator/component.go` | Component implementation |
| `processor/oasf-generator/mapper.go` | Predicate-to-OASF field mapping |
| `processor/oasf-generator/templates/` | OASF JSON templates |
| `processor/oasf-generator/component_test.go` | Unit tests |

### Phase 2: Directory Bridge + Identity

**New Components**: `output/directory-bridge/`, `agentic/identity/`

**Identity Extension**: Add DID field to agent entity schema:

```go
type AgentEntity struct {
    // Existing fields...
    DID         string            `json:"did,omitempty"`
    Credentials []json.RawMessage `json:"credentials,omitempty"`
}
```

### Phase 3: SLIM Bridge + A2A

**New Components**: `input/slim/`, `input/a2a/`

**SLIM Bridge Pattern**:

```go
type SLIMBridge struct {
    component.Base
    slimClient   *slim.Client
    natsClient   *natsclient.Client
    groupManager *GroupManager  // MLS session lifecycle
}

func (b *SLIMBridge) Start(ctx context.Context) error {
    // 1. Initialize SLIM connection
    // 2. Join configured groups
    // 3. Start message pump: SLIM → NATS
    // 4. Subscribe to agent.complete.* for reverse flow
}
```

### Phase 4: OTEL Export

**New Component**: `output/otel/`

Standard OTEL exporter implementation using existing observability data.

## Open Questions

1. **SLIM Group Topology**: Should we join one global group or create per-tenant groups? Affects authorization model.

2. **DID Provider**: Use AGNTCY's identity service or integrate with existing enterprise identity (Azure AD, Okta)?

3. **OASF Extension Points**: Our vocabulary may have concepts not in base OASF. What's the extension/contribution process?

4. **A2A Task Mapping**: How do A2A task types map to our agent roles? Is there a standard taxonomy?

5. **Federation Strategy**: Should we run our own directory that syncs, or just register with public directories?

## References

- [AGNTCY Documentation](https://docs.agntcy.org)
- [AGNTCY GitHub](https://github.com/agntcy)
- [SLIM IETF Draft](https://datatracker.ietf.org/doc/draft-mpsb-agntcy-slim/)
- [OASF Schema](https://schema.oasf.outshift.com)
- [Linux Foundation Announcement (July 2025)](https://www.linuxfoundation.org/press/linux-foundation-welcomes-the-agntcy-project)
- [A2A Protocol (Linux Foundation)](https://github.com/a2aproject/A2A)
- [ADR-001: Pragmatic Semantic Web](./adr-001-pragmatic-semantic-web.md) — Vocabulary standards alignment
- [ADR-016: Agentic Governance Layer](./adr-016-agentic-governance-layer.md) — Internal trust model
- [ADR-018: Agentic Workflow Orchestration](./adr-018-agentic-workflow-orchestration.md) — Internal orchestration patterns
