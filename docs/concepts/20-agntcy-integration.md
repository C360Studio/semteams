# AGNTCY Integration

This document describes how SemStreams integrates with the AGNTCY (Internet of Agents)
ecosystem to enable agent discoverability, cross-organizational communication, and
observability.

## Overview

AGNTCY provides infrastructure for agents to discover each other, communicate securely,
and interoperate across organizational boundaries. SemStreams implements four integration
points:

```
                        AGNTCY Ecosystem
    ┌─────────────────────────────────────────────────────────┐
    │                                                         │
    │   ┌─────────────┐    ┌─────────────┐    ┌───────────┐  │
    │   │  Directory  │    │    SLIM     │    │   OTEL    │  │
    │   │  Services   │    │   Network   │    │ Collectors│  │
    │   └──────▲──────┘    └──────▲──────┘    └─────▲─────┘  │
    │          │                  │                  │        │
    └──────────┼──────────────────┼──────────────────┼────────┘
               │                  │                  │
    ┌──────────┼──────────────────┼──────────────────┼────────┐
    │          │                  │                  │        │
    │   ┌──────┴──────┐    ┌──────┴──────┐    ┌─────┴─────┐  │
    │   │  Directory  │    │    SLIM     │    │   OTEL    │  │
    │   │   Bridge    │    │   Bridge    │    │  Exporter │  │
    │   └──────▲──────┘    └──────┬──────┘    └─────▲─────┘  │
    │          │                  │                  │        │
    │   ┌──────┴──────┐    ┌──────▼──────┐          │        │
    │   │    OASF     │    │     A2A     │          │        │
    │   │  Generator  │    │   Adapter   │──────────┘        │
    │   └─────────────┘    └─────────────┘                   │
    │                                                         │
    │                    SemStreams                           │
    └─────────────────────────────────────────────────────────┘
```

## Core Concepts

### OASF Records

The Open Agent Specification Framework (OASF) is the standard format for describing
agent capabilities in the AGNTCY ecosystem. SemStreams generates OASF records from
agent capability predicates stored in the knowledge graph.

Key aspects:
- **Skills**: What the agent can do (derived from `agent.capability.*` predicates)
- **Domains**: Areas of expertise (derived from `agent.intent.type`)
- **Metadata**: Version, authors, creation time

OASF records enable other agents and systems to understand what a SemStreams agent
can do without direct interaction.

See: `processor/oasf-generator/README.md`

### DID Identity

Decentralized Identifiers (DIDs) provide cryptographic identity for agents. Unlike
traditional identity systems, DIDs are self-sovereign—agents control their own
identity without relying on a central authority.

SemStreams supports three DID methods:

| Method | Use Case | Resolution |
|--------|----------|------------|
| `did:key` | Development, ephemeral agents | Self-contained in identifier |
| `did:web` | Production, organization-owned agents | HTTPS resolution |
| `did:agntcy` | AGNTCY network registration | AGNTCY directory resolution |

DIDs are paired with Verifiable Credentials to assert capabilities:
- **AgentCapabilityCredential**: Attests to specific skills
- **AgentDelegationCredential**: Grants authority from one agent to another
- **AgentIdentityCredential**: Binds DID to organizational identity

See: `agentic/identity/README.md`

### Directory Registration

The directory bridge registers agents with AGNTCY directories, making them
discoverable by other participants in the network.

Registration lifecycle:
1. **Detection**: Watch for new OASF records
2. **Identity**: Acquire or create DID for the agent
3. **Registration**: Submit agent card to directory
4. **Heartbeat**: Maintain registration with periodic updates
5. **Deregistration**: Clean removal when agent stops

Directories maintain an index of agent capabilities, enabling queries like
"find agents that can analyze code" or "find agents in the healthcare domain."

See: `output/directory-bridge/README.md`

### SLIM Messaging

SLIM (Secure Lightweight Instant Messaging) enables encrypted communication between
agents across organizational boundaries. Built on the MLS (Messaging Layer Security)
protocol, SLIM provides:

- **End-to-end encryption**: Messages readable only by group members
- **Forward secrecy**: Past messages protected even if keys are compromised
- **Group messaging**: Efficient multi-party communication
- **Key ratcheting**: Automatic key rotation for ongoing security

```
    Organization A              SLIM Network              Organization B
    ┌───────────┐              ┌───────────┐              ┌───────────┐
    │ SemStreams│◄────────────►│   Group   │◄────────────►│ SemStreams│
    │   Agent   │   Encrypted  │  Session  │   Encrypted  │   Agent   │
    └───────────┘              └───────────┘              └───────────┘
```

The SLIM bridge translates between SemStreams' internal NATS messaging and the
external SLIM protocol, enabling seamless cross-organizational workflows.

See: `input/slim/README.md`

### A2A Protocol

The Agent-to-Agent (A2A) protocol standardizes how agents delegate tasks to each
other. It defines:

- **Agent Cards**: JSON documents describing agent capabilities (derived from OASF)
- **Tasks**: Work items with lifecycle states (submitted → working → completed/failed)
- **Message Parts**: Text, files, and structured data within tasks
- **Artifacts**: Results produced by task execution

Task lifecycle:
```
    submitted ──► working ──► completed
                    │
                    └──────► failed
                    │
                    └──────► canceled
```

The A2A adapter exposes SemStreams agents via HTTP endpoints and handles the
translation between A2A tasks and internal TaskMessage types.

See: `input/a2a/README.md`

### OpenTelemetry Export

The OTEL exporter converts agent execution events into OpenTelemetry traces and
metrics, enabling observability across distributed agent systems.

Span hierarchy mirrors agent execution:
```
    Loop Span (root)
    ├── Task Span
    │   ├── Tool Span (code_search)
    │   └── Tool Span (file_edit)
    └── Task Span
        └── Tool Span (test_runner)
```

This hierarchy enables:
- **Trace visualization**: See complete agent execution in Jaeger/Zipkin
- **Performance analysis**: Identify slow tools or tasks
- **Error correlation**: Connect failures to specific execution points
- **Cross-agent tracing**: Follow work across agent boundaries

See: `output/otel/README.md`

## Integration Patterns

### Internal Agents (No AGNTCY)

For agents operating within a single SemStreams deployment:
- DIDs are optional (internal role-based identity suffices)
- SLIM is unnecessary (NATS provides internal messaging)
- Directory registration is skipped
- OTEL export remains valuable for observability

### Federated Agents

For agents participating in the AGNTCY network:
- DIDs required for cryptographic identity
- Directory registration enables discoverability
- SLIM or A2A enables cross-organizational communication
- OTEL provides distributed tracing across boundaries

### Hybrid Deployments

Most production deployments combine both patterns:
- Internal agents communicate via NATS
- Gateway agents have DIDs and directory registration
- External requests arrive via A2A, dispatch internally via NATS
- OTEL traces span both internal and external execution

## Security Model

AGNTCY integration introduces several security boundaries:

| Boundary | Protection | Mechanism |
|----------|------------|-----------|
| Identity | Authentication | DID signatures, verifiable credentials |
| Transport | Confidentiality | SLIM/MLS encryption, TLS for HTTP |
| Authorization | Access control | Capability credentials, allowed agent lists |
| Integrity | Tamper detection | Message signing, credential verification |

Trust is established through:
1. DID resolution (verify agent identity)
2. Credential verification (validate capabilities)
3. Directory attestation (confirm registration)
4. MLS group membership (authorize communication)

## Configuration Summary

| Component | Purpose | Key Settings |
|-----------|---------|--------------|
| `oasf-generator` | Generate capability records | `entity_kv_bucket`, `watch_pattern` |
| `directory-bridge` | Register with directories | `directory_url`, `heartbeat_interval` |
| `slim-bridge` | Secure external messaging | `endpoint`, `group_ids` |
| `a2a-adapter` | HTTP task interface | `listen_address`, `enable_authentication` |
| `otel-exporter` | Observability export | `endpoint`, `export_traces`, `sampling_rate` |

## References

- [OASF Specification](https://docs.agntcy.org/pages/syntaxes/oasf)
- [A2A Protocol](https://github.com/google/A2A)
- [W3C DID Specification](https://www.w3.org/TR/did-core/)
- [MLS Protocol (RFC 9420)](https://datatracker.ietf.org/doc/rfc9420/)
- [OpenTelemetry](https://opentelemetry.io/)
