# A2A Protocol Integration

The Agent-to-Agent (A2A) protocol enables standardized communication between AI agents across organizational
boundaries. SemStreams implements A2A as an input adapter, allowing external agents to delegate tasks to
SemStreams-based agents.

## What is A2A?

A2A (Agent-to-Agent) is an open protocol specification for cross-agent communication, now maintained by the
Linux Foundation as part of the AGNTCY project. It provides a standardized way for agents to:

- Discover each other's capabilities through agent cards
- Delegate tasks across system boundaries
- Track task lifecycle and status
- Exchange artifacts and results

```text
┌─────────────────────────────────────────────────────────────────────┐
│                      A2A Protocol Flow                               │
├─────────────────────────────────────────────────────────────────────┤
│                                                                      │
│   External Agent                  SemStreams Agent                  │
│   ──────────────                  ────────────────                  │
│                                                                      │
│   1. Discover capabilities                                          │
│      GET /.well-known/agent.json ──────────▶ Return agent card     │
│                                                                      │
│   2. Submit task                                                    │
│      POST /tasks/send ─────────────────────▶ Accept task           │
│                             (submitted)                             │
│                                                                      │
│   3. Check status                                                   │
│      GET /tasks/get?id=123 ────────────────▶ Status: working       │
│                                                                      │
│   4. Retrieve result                                                │
│      GET /tasks/get?id=123 ────────────────▶ Status: completed     │
│                             (artifacts)                             │
│                                                                      │
└─────────────────────────────────────────────────────────────────────┘
```

### A2A Specification Overview

The A2A protocol defines:

| Component | Purpose |
|-----------|---------|
| **Agent Cards** | Machine-readable capability descriptions |
| **Task Messages** | Structured input with role/parts/metadata |
| **Status Updates** | Lifecycle tracking (submitted, working, completed, failed, canceled) |
| **Artifacts** | Structured output with multiple parts |
| **Message Parts** | Typed content (text, file, data) |

A2A is transport-agnostic — it can run over HTTP REST endpoints or MLS-encrypted SLIM messaging.

## Agent Cards

Agent cards are the discovery mechanism for A2A. They describe what an agent can do, what authentication
it requires, and how to communicate with it.

### Agent Card Structure

```json
{
  "name": "SemStreams Knowledge Agent",
  "description": "Query and reason over semantic knowledge graphs",
  "url": "https://agent.example.com",
  "version": "1.0",
  "provider": {
    "organization": "ACME Corp",
    "url": "https://acme.com"
  },
  "capabilities": [
    {
      "name": "knowledge-query",
      "description": "Query semantic knowledge graph"
    },
    {
      "name": "reasoning",
      "description": "Perform logical inference over graph"
    }
  ],
  "authentication": {
    "schemes": ["did"],
    "credentials": {
      "did": "did:agntcy:agent:acme-knowledge-001"
    }
  },
  "defaultInputModes": ["text"],
  "defaultOutputModes": ["text", "data"],
  "skills": [
    {
      "id": "graph-query",
      "name": "Graph Query",
      "description": "Execute SPARQL-like queries",
      "inputSchema": {...},
      "outputSchema": {...}
    }
  ]
}
```

### Fields

| Field | Required | Description |
|-------|----------|-------------|
| `name` | Yes | Human-readable agent name |
| `description` | Yes | What the agent does |
| `url` | Yes | A2A endpoint base URL |
| `version` | Yes | Agent card schema version |
| `capabilities` | Yes | High-level capability list |
| `defaultInputModes` | Yes | Supported input types (text, file, data) |
| `defaultOutputModes` | Yes | Supported output types |
| `provider` | No | Organization operating the agent |
| `authentication` | No | Required auth schemes and credentials |
| `skills` | No | Detailed skill definitions with schemas |

### Capability Discovery

External agents discover capabilities through the well-known endpoint:

```bash
curl https://agent.example.com/.well-known/agent.json
```

This returns the agent card JSON. The external agent uses this to determine:

- What tasks this agent can handle
- What authentication is required
- What input/output formats are supported
- What specific skills are available

## Task Lifecycle

A2A tasks progress through a well-defined state machine.

### Task States

```text
┌─────────────────────────────────────────────────────────────────────┐
│                         Task Lifecycle                               │
├─────────────────────────────────────────────────────────────────────┤
│                                                                      │
│   ┌───────────┐                                                     │
│   │ submitted │  Initial state when task accepted                   │
│   └─────┬─────┘                                                     │
│         │                                                            │
│         ▼                                                            │
│   ┌───────────┐                                                     │
│   │  working  │  Agent processing the task                          │
│   └─────┬─────┘                                                     │
│         │                                                            │
│    ┌────┴────┐                                                      │
│    ▼         ▼                                                      │
│ ┌─────────┐ ┌─────────┐                                            │
│ │completed│ │ failed  │  Terminal states                            │
│ └─────────┘ └─────────┘                                            │
│                                                                      │
│   ┌───────────┐                                                     │
│   │ canceled  │  User-initiated cancellation (any time)             │
│   └───────────┘                                                     │
│                                                                      │
└─────────────────────────────────────────────────────────────────────┘
```

| State | Terminal | Description |
|-------|----------|-------------|
| `submitted` | No | Task accepted, queued for processing |
| `working` | No | Agent actively processing |
| `completed` | Yes | Successfully finished with artifacts |
| `failed` | Yes | Failed with error message |
| `canceled` | Yes | Canceled by requester |

### Task Message Structure

```json
{
  "id": "task-abc-123",
  "sessionId": "session-xyz",
  "status": {
    "state": "submitted",
    "message": "Task accepted for processing",
    "timestamp": "2026-02-13T12:00:00Z"
  },
  "message": {
    "role": "user",
    "parts": [
      {
        "type": "text",
        "text": "Analyze this code for security issues"
      },
      {
        "type": "file",
        "file": {
          "name": "main.go",
          "mimeType": "text/plain",
          "bytes": "cGFja2FnZSBtYWluCi4uLg=="
        }
      }
    ],
    "metadata": {
      "priority": "high",
      "timeout": "300s"
    }
  },
  "history": [
    {
      "role": "agent",
      "parts": [{"type": "text", "text": "Previous interaction..."}]
    }
  ],
  "metadata": {
    "requester": "did:agntcy:agent:external-001",
    "capabilities": ["code-analysis"]
  }
}
```

### Message Parts

A2A supports heterogeneous message content through typed parts:

| Part Type | Fields | Use Case |
|-----------|--------|----------|
| `text` | `text` | Natural language prompts, responses |
| `file` | `name`, `mimeType`, `uri` or `bytes` | File attachments, code, documents |
| `data` | `data` (JSON) | Structured data, API payloads |

Multiple parts can be combined in a single message to provide rich context.

### Artifacts

Completed tasks return artifacts — structured outputs from the agent:

```json
{
  "taskId": "task-abc-123",
  "status": {
    "state": "completed",
    "timestamp": "2026-02-13T12:05:00Z"
  },
  "artifacts": [
    {
      "name": "analysis-report",
      "description": "Security analysis findings",
      "parts": [
        {
          "type": "text",
          "text": "Found 3 security issues:\n1. SQL injection risk..."
        },
        {
          "type": "data",
          "data": {
            "issues": [
              {"severity": "high", "line": 42, "type": "sql-injection"}
            ]
          }
        }
      ],
      "index": 0,
      "metadata": {
        "format": "security-report-v1"
      }
    }
  ]
}
```

Artifacts can contain multiple parts, enabling agents to return both human-readable text and
machine-processable structured data.

## Task Mapping to SemStreams

The A2A adapter translates between A2A protocol messages and SemStreams internal message types.

### Inbound: A2A Task to TaskMessage

```text
┌──────────────────────────────────────────────────────────────────┐
│                    A2A Task → TaskMessage                         │
├──────────────────────────────────────────────────────────────────┤
│                                                                   │
│   A2A Task                        SemStreams TaskMessage         │
│   ─────────                       ───────────────────            │
│                                                                   │
│   id: "task-123"          ────▶   task_id: "task-123"           │
│   sessionId: "sess-xyz"   ────▶   channel_id: "sess-xyz"        │
│   message.parts[].text    ────▶   prompt: "..."                 │
│   metadata.role           ────▶   role: "architect"             │
│   metadata.model          ────▶   model: "gpt-4"                │
│   (extracted from headers)────▶   user_id: "did:agntcy:..."     │
│                           ────▶   channel_type: "a2a"           │
│                                                                   │
└──────────────────────────────────────────────────────────────────┘
```

### Extraction Logic

**Prompt**: Extracted from all `text` parts in the message, concatenated with newlines.

**Role**: Mapped from metadata or capabilities:

| Metadata | SemStreams Role |
|----------|----------------|
| `metadata.role = "architect"` | `architect` |
| `capabilities = ["design", "planning"]` | `architect` |
| `capabilities = ["code", "implementation"]` | `editor` |
| (default) | `general` |

**Model**: Extracted from `metadata.model` or defaults to `"default"`.

**Channel ID**: Uses A2A `sessionId` to group related tasks.

**User ID**: Extracted from authentication headers (DID or bearer token).

### Outbound: Agent Response to A2A Result

```text
┌──────────────────────────────────────────────────────────────────┐
│                  TaskMessage Result → A2A Result                  │
├──────────────────────────────────────────────────────────────────┤
│                                                                   │
│   agent.complete.* event      A2A TaskResult                     │
│   ────────────────────        ───────────                        │
│                                                                   │
│   task_id: "task-123"  ────▶  taskId: "task-123"                │
│   result: "..."        ────▶  artifacts[0].parts[0].text         │
│   error: nil           ────▶  status.state: "completed"          │
│   error: "timeout"     ────▶  status.state: "failed"             │
│                               error: "timeout"                    │
│                                                                   │
└──────────────────────────────────────────────────────────────────┘
```

## Transport Options

A2A is transport-agnostic. SemStreams supports two transport mechanisms.

### HTTP Transport

RESTful endpoints for synchronous task submission and status queries.

**Endpoints**:

| Method | Path | Purpose |
|--------|------|---------|
| GET | `/.well-known/agent.json` | Return agent card |
| POST | `/tasks/send` | Submit new task |
| GET | `/tasks/get?id={id}` | Query task status |
| POST | `/tasks/cancel` | Cancel running task |

**Example: Submit Task**

```bash
curl -X POST https://agent.example.com/tasks/send \
  -H "Authorization: Bearer did:agntcy:agent:requester-001" \
  -H "Content-Type: application/json" \
  -d '{
    "id": "task-abc-123",
    "sessionId": "session-xyz",
    "message": {
      "role": "user",
      "parts": [
        {"type": "text", "text": "Review this code"}
      ]
    }
  }'
```

**Response**:

```json
{
  "id": "task-abc-123",
  "status": {
    "state": "submitted",
    "message": "Task accepted for processing",
    "timestamp": "2026-02-13T12:00:00Z"
  }
}
```

### SLIM Transport

MLS-encrypted group messaging for cross-organizational communication.

SLIM (Secure Low-Latency Interactive Messaging) provides:

- End-to-end encryption using MLS (Message Layer Security)
- Quantum-safe cryptography options
- Group-based communication model
- DID-based hierarchical channels

**Architecture**:

```text
┌─────────────────────────────────────────────────────────────────────┐
│                    SLIM Transport Architecture                       │
├─────────────────────────────────────────────────────────────────────┤
│                                                                      │
│   External Agent                SLIM Network            SemStreams  │
│   ──────────────                ────────────            ──────────  │
│                                                                      │
│   ┌──────────────┐             ┌──────────┐           ┌──────────┐ │
│   │ SLIM Client  │────────────▶│  Group   │◀──────────│  A2A     │ │
│   │ (External)   │             │  Channel │           │  Adapter │ │
│   └──────────────┘             └──────────┘           └────┬─────┘ │
│                                     ▲                       │       │
│                                     │ MLS encrypted         │       │
│                                     │                       ▼       │
│                                     │                  ┌──────────┐ │
│                                     └──────────────────│  NATS    │ │
│                                                        │JetStream │ │
│                                                        └──────────┘ │
│                                                                      │
└─────────────────────────────────────────────────────────────────────┘
```

**Configuration**:

```yaml
transport: "slim"
slim_group_id: "did:agntcy:group:tenant-123"
enable_authentication: true
```

When using SLIM transport, the A2A adapter joins the specified group and listens for A2A protocol
messages. Task submissions and responses flow through the encrypted SLIM channel.

## Authentication with DIDs

A2A supports decentralized identity using DIDs (Decentralized Identifiers) for cryptographic verification
of agent identity across organizational boundaries.

### DID-Based Authentication

```text
┌─────────────────────────────────────────────────────────────────────┐
│                  DID Authentication Flow                             │
├─────────────────────────────────────────────────────────────────────┤
│                                                                      │
│   Requester Agent              SemStreams Agent                     │
│   ───────────────              ───────────────                      │
│                                                                      │
│   1. Include DID in request                                         │
│      Authorization: did:agntcy:agent:requester-001                  │
│                                     │                                │
│                                     ▼                                │
│   2. Verify DID signature ──────────────────▶ Cryptographic check  │
│                                                                      │
│   3. Authorize based on identity ────────────▶ Policy evaluation   │
│                                                                      │
│   4. Track task by DID ──────────────────────▶ Audit trail         │
│                                                                      │
└─────────────────────────────────────────────────────────────────────┘
```

### DID Headers

The A2A adapter accepts DIDs through multiple headers:

| Header | Format | Use Case |
|--------|--------|----------|
| `Authorization` | `Bearer did:agntcy:agent:id` | Standard OAuth-style header |
| `X-Agent-DID` | `did:agntcy:agent:id` | Explicit DID header |
| (SLIM envelope) | Signed message | For SLIM transport |

### Authentication Configuration

```yaml
enable_authentication: true
```

When enabled, requests without valid authentication are rejected with `401 Unauthorized`.

### Production DID Verification

**Note**: The current implementation performs header-based DID extraction without cryptographic
signature verification. Production deployments should integrate with AGNTCY's identity service
or a DID resolver library for full verification:

1. Extract DID from headers
2. Resolve DID document from DID registry
3. Verify request signature using public key from DID document
4. Check verifiable credentials and policies

See [ADR-019: AGNTCY Integration](../architecture/adr-019-agntcy-integration.md) for identity
integration roadmap.

## Integration with SemStreams

The A2A adapter integrates with SemStreams through the standard NATS subject topology.

### Message Flow

```text
External A2A Request
      │
      ▼
┌─────────────────┐
│  A2A Adapter    │ (input/a2a)
│  (HTTP/SLIM)    │
└────────┬────────┘
         │ agent.task.a2a.{task_id}
         ▼
┌─────────────────┐
│ agentic-dispatch│
│                 │
└────────┬────────┘
         │ agent.task.{task_id}
         ▼
┌─────────────────┐
│  agentic-loop   │
│                 │
└────────┬────────┘
         │ agent.complete.*
         ▼
┌─────────────────┐
│  A2A Adapter    │
│  (response)     │
└────────┬────────┘
         │
         ▼
External A2A Response
```

### NATS Subjects

| Subject Pattern | Direction | Purpose |
|----------------|-----------|---------|
| `agent.task.a2a.{task_id}` | Inbound | A2A tasks to agent dispatch |
| `agent.complete.*` | Outbound | Completion events to A2A adapter |
| `agent.signal.{loop_id}` | Control | Cancel/pause signals |

### Agent Card Generation

Agent cards are generated from OASF records stored in the `OASF_RECORDS` KV bucket:

```text
┌─────────────────────┐
│  OASF_RECORDS KV    │──── Read ────┐
│  (OASF records)     │               │
└─────────────────────┘               ▼
                           ┌───────────────────────┐
                           │ AgentCardGenerator    │
                           │ (input/a2a)           │
                           └───────────────────────┘
                                     │
                                     ▼
                           ┌───────────────────────┐
                           │  Agent Card (JSON)    │
                           │  Served at            │
                           │  /.well-known/agent.json│
                           └───────────────────────┘
```

The `AgentCardGenerator` reads OASF skills and domains, maps them to A2A capabilities and skills,
and generates the agent card JSON structure.

See [OASF Integration](./20-oasf-integration.md) for OASF record generation from agent entities.

## Use Cases for Agent Interoperability

A2A enables agents to collaborate across organizational and system boundaries.

### Cross-Organizational Task Delegation

An external planning agent delegates implementation tasks to a SemStreams coding agent:

```text
┌────────────────────┐                        ┌────────────────────┐
│   Planning Agent   │                        │  SemStreams Agent  │
│   (External Org)   │                        │   (ACME Corp)      │
├────────────────────┤                        ├────────────────────┤
│                    │                        │                    │
│ 1. Create plan     │                        │                    │
│    for feature     │                        │                    │
│                    │                        │                    │
│ 2. Delegate        │                        │                    │
│    implementation  │──▶ A2A task submit ──▶│ 3. Receive task    │
│                    │                        │                    │
│                    │                        │ 4. Execute         │
│                    │                        │    with tools      │
│                    │                        │                    │
│                    │◀── A2A artifacts ──────│ 5. Return result   │
│                    │                        │                    │
│ 6. Integrate       │                        │                    │
│    result          │                        │                    │
│                    │                        │                    │
└────────────────────┘                        └────────────────────┘
```

### Multi-Agent Collaboration

Multiple specialized agents work together through A2A coordination:

```text
┌─────────────┐     A2A     ┌─────────────┐     A2A     ┌─────────────┐
│   Security  │◀───────────▶│  Knowledge  │◀───────────▶│   Coding    │
│   Agent     │             │    Agent    │             │   Agent     │
└─────────────┘             │ (SemStreams)│             └─────────────┘
                            └──────┬──────┘
                                   │ A2A
                                   ▼
                            ┌─────────────┐
                            │   Testing   │
                            │   Agent     │
                            └─────────────┘
```

The SemStreams knowledge agent acts as a coordinator, querying its semantic graph to route
sub-tasks to appropriate specialist agents.

### Federated Agent Directory

Agents register with AGNTCY directories and discover each other through standard queries:

```text
┌────────────────────┐       ┌────────────────────┐       ┌────────────────────┐
│  Agent A           │       │  AGNTCY Directory  │       │  Agent B           │
│  (registers)       │       │  (discovery)       │       │  (discovers)       │
├────────────────────┤       ├────────────────────┤       ├────────────────────┤
│                    │       │                    │       │                    │
│ POST /register     │──────▶│ Store agent card   │       │                    │
│ (OASF + A2A card)  │       │                    │       │                    │
│                    │       │                    │       │                    │
│                    │       │                    │◀──────│ GET /search        │
│                    │       │                    │       │ ?capability=...    │
│                    │       │                    │       │                    │
│                    │       │ Return matches ────────────▶│                    │
│                    │       │ (A2A endpoints)    │       │                    │
│                    │       │                    │       │                    │
│                    │◀──────────── A2A task submit ──────│ Invoke Agent A     │
│                    │       │                    │       │                    │
│                    │       │                    │       │                    │
└────────────────────┘       └────────────────────┘       └────────────────────┘
```

### Internet of Agents

A2A + AGNTCY enables a federated network of discoverable, interoperable agents:

- **Discovery**: Agents find each other through directories
- **Delegation**: Agents delegate tasks through A2A protocol
- **Identity**: Agents verify each other with DIDs and verifiable credentials
- **Security**: Communication encrypted via SLIM with MLS
- **Observability**: OpenTelemetry integration for multi-agent workflows

SemStreams agents participate in this ecosystem while maintaining their internal semantic
knowledge graph and disciplined coordination patterns.

## Configuration

The A2A adapter is configured as an input component:

```yaml
components:
  - name: a2a-adapter
    type: a2a-adapter
    config:
      # Transport
      transport: "http"              # "http" or "slim"
      listen_address: ":8080"        # HTTP listen address
      agent_card_path: "/.well-known/agent.json"

      # SLIM configuration (for transport: "slim")
      slim_group_id: "did:agntcy:group:tenant-123"

      # Authentication
      enable_authentication: true

      # Timeouts
      request_timeout: "30s"
      max_concurrent_tasks: 10

      # Ports (internal NATS routing)
      ports:
        outputs:
          - name: "tasks"
            type: "jetstream"
            subject: "agent.task.a2a.>"
            stream_name: "AGENT_TASKS"
```

### Configuration Options

| Option | Type | Default | Description |
|--------|------|---------|-------------|
| `transport` | string | `http` | Transport mechanism (`http` or `slim`) |
| `listen_address` | string | `:8080` | HTTP server bind address |
| `agent_card_path` | string | `/.well-known/agent.json` | Agent card endpoint path |
| `slim_group_id` | string | `""` | SLIM group DID (for SLIM transport) |
| `enable_authentication` | bool | `false` | Require DID authentication |
| `request_timeout` | duration | `30s` | Maximum request processing time |
| `max_concurrent_tasks` | int | `10` | Maximum concurrent task limit |

## See Also

- [ADR-019: AGNTCY Integration](../architecture/adr-019-agntcy-integration.md) — Integration architecture
- [OASF Integration](./20-oasf-integration.md) — Agent card generation from OASF records
- [Agentic Systems](./11-agentic-systems.md) — Internal agent orchestration
- [A2A Protocol Specification](https://github.com/a2aproject/A2A) — Official protocol docs
- [AGNTCY Documentation](https://docs.agntcy.org) — AGNTCY ecosystem overview
