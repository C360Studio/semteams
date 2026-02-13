# SLIM Messaging

SLIM (Secure Lightweight Instant Messaging) is the secure messaging protocol for cross-organizational agent communication
in the AGNTCY ecosystem. It provides end-to-end encrypted group messaging with quantum-safe security options.

## Overview

SLIM enables agents from different organizations to communicate securely over untrusted networks. It combines:

- **MLS** (Messaging Layer Security) for end-to-end encryption
- **Group sessions** for multi-party communication
- **Key ratcheting** for forward secrecy
- **DID-based identity** for cryptographic verification

SLIM is built on gRPC with pub/sub extensions and has an IETF Internet-Draft specification.

## Why SLIM Matters for SemStreams

SemStreams solves **intra-organizational coordination** — how a team of agents with different capabilities executes
missions under a shared command structure. SLIM extends this to **inter-organizational coordination** — how agents
across organizational boundaries discover, communicate, and delegate tasks securely.

```text
┌───────────────────────────────────────────────────────────────┐
│                    Coordination Layers                         │
├───────────────────────────────────────────────────────────────┤
│                                                                │
│   Intra-Organization (SemStreams)                              │
│   ┌────────────────────────────────────────────────────┐      │
│   │  NATS JetStream                                    │      │
│   │  ┌────────┐   ┌────────┐   ┌────────┐            │      │
│   │  │ Agent  │──►│ Agent  │──►│ Agent  │            │      │
│   │  │   A    │   │   B    │   │   C    │            │      │
│   │  └────────┘   └────────┘   └────────┘            │      │
│   │  Shared trust, low latency, rich orchestration    │      │
│   └────────────────────────────────────────────────────┘      │
│                         │                                      │
│                         │ SLIM Bridge                          │
│                         ▼                                      │
│   Inter-Organization (SLIM)                                    │
│   ┌────────────────────────────────────────────────────┐      │
│   │  SLIM Group (MLS encrypted)                        │      │
│   │  ┌────────┐         ┌────────┐                    │      │
│   │  │ Org A  │◄───────►│ Org B  │                    │      │
│   │  │ Agents │         │ Agents │                    │      │
│   │  └────────┘         └────────┘                    │      │
│   │  Zero trust, E2E encryption, task delegation      │      │
│   └────────────────────────────────────────────────────┘      │
│                                                                │
└───────────────────────────────────────────────────────────────┘
```

**Internal use cases:**
- Agent coordination within a single deployment
- Shared knowledge graph access
- Tool invocation and state management

**SLIM-bridged use cases:**
- Cross-organizational task delegation
- Federated agent discovery
- Secure multi-party workflows
- Government/contractor collaboration

## MLS (Messaging Layer Security)

MLS is an IETF standard (RFC 9420) for end-to-end encrypted group messaging. It solves the problem of adding
end-to-end encryption to multi-party conversations efficiently.

### Why Traditional Encryption Fails for Groups

**Signal-style pairwise encryption** (N members = N(N-1)/2 keys) becomes unmanageable at scale:

```text
3 members:  3 pairwise keys
10 members: 45 pairwise keys
100 members: 4,950 pairwise keys
```

Sending a message requires N encryptions (one per member), and adding/removing members requires key renegotiation
with every participant.

### The MLS Solution

MLS uses a **tree-based key agreement** protocol where all members share a single symmetric group key derived from
a tree of asymmetric key pairs:

```text
┌─────────────────────────────────────────────────────────────┐
│                    MLS Key Tree                              │
├─────────────────────────────────────────────────────────────┤
│                                                              │
│                        Root                                  │
│                    (group key)                               │
│                          │                                   │
│                  ┌───────┴───────┐                           │
│                  │               │                           │
│              Branch A         Branch B                       │
│                  │               │                           │
│            ┌─────┴─────┐   ┌────┴────┐                      │
│            │           │   │         │                       │
│         Agent 1    Agent 2  Agent 3  Agent 4                 │
│                                                              │
│   Send Message:  1 encryption (symmetric with group key)    │
│   Add Member:    Update only affected branch paths          │
│   Remove Member: Ratchet group key (forward secrecy)        │
│                                                              │
└─────────────────────────────────────────────────────────────┘
```

**Benefits:**

| Operation | MLS Complexity | Pairwise Complexity |
|-----------|---------------|---------------------|
| Send message | O(1) - single encryption | O(N) - encrypt for each member |
| Add member | O(log N) - update path | O(N) - renegotiate with all |
| Remove member | O(log N) - update path | O(N) - renegotiate with all |

### MLS Protocol Phases

```text
┌─────────────────────────────────────────────────────────────┐
│                    MLS Lifecycle                             │
├─────────────────────────────────────────────────────────────┤
│                                                              │
│  1. KeyPackage Generation                                    │
│     Agent creates public/private key pair                    │
│     Publishes KeyPackage to discovery                        │
│                                                              │
│  2. Group Initialization                                     │
│     First member creates group                               │
│     Generates initial group key                              │
│                                                              │
│  3. Member Addition                                          │
│     Existing member invites new member                       │
│     New member derives group key from tree path              │
│     All members update local tree state                      │
│                                                              │
│  4. Message Exchange                                         │
│     Encrypt with current group key                           │
│     All members can decrypt with their derived key           │
│                                                              │
│  5. Key Ratcheting (periodic)                                │
│     Generate new group key                                   │
│     Old key cannot decrypt future messages (forward secrecy) │
│                                                              │
│  6. Member Removal                                           │
│     Update tree to exclude member's path                     │
│     Ratchet group key (removed member cannot decrypt)        │
│                                                              │
└─────────────────────────────────────────────────────────────┘
```

### Forward Secrecy via Key Ratcheting

Forward secrecy ensures that compromise of current keys does not expose past messages. MLS achieves this through
**key ratcheting** — periodically generating new group keys and destroying old ones.

```text
Time:        T0        T1        T2        T3
Keys:       Key₀  →   Key₁  →   Key₂  →   Key₃
Messages:   [M1 M2]   [M3]      [M4 M5 M6] [M7]

Compromise at T3:
  ✓ Can decrypt:  M7 (Key₃)
  ✗ Cannot decrypt: M1-M6 (keys destroyed)
```

**SemStreams SLIM bridge** ratchets keys on a configurable interval (default: 1 hour). Each ratchet:

1. Generates new group key
2. Securely distributes to all members via tree paths
3. Destroys old key material
4. Transitions session through "rekeying" state

See `session_manager.go` for implementation details.

## Group Session Management

SLIM groups are persistent multi-party communication channels identified by DID-based names:

```text
did:agntcy:group:tenant-acme-ops
did:agntcy:group:project-alpha-team
did:agntcy:group:consortium-defense-logistics
```

### Session States

Each SLIM group session progresses through well-defined states:

```text
┌─────────────────────────────────────────────────────────────┐
│                    Session State Machine                     │
├─────────────────────────────────────────────────────────────┤
│                                                              │
│   ┌──────────┐    MLS handshake    ┌────────────┐          │
│   │ joining  │────────────────────► │   active   │          │
│   └──────────┘                      └─────┬──────┘          │
│                                           │                  │
│                      ratchet trigger      ▼                  │
│                                     ┌────────────┐           │
│                                     │  rekeying  │           │
│                                     └─────┬──────┘           │
│                                           │                  │
│                      ratchet complete     ▼                  │
│                                     ┌────────────┐           │
│                                     │   active   │           │
│                                     └─────┬──────┘           │
│                                           │                  │
│                      leave/error          ▼                  │
│                                     ┌────────────┐           │
│                                     │  leaving/  │           │
│                                     │   error    │           │
│                                     └────────────┘           │
│                                    (terminal states)         │
│                                                              │
└─────────────────────────────────────────────────────────────┘
```

| State | Terminal | Description | Can Send? |
|-------|----------|-------------|-----------|
| `joining` | No | Establishing MLS session | No |
| `active` | No | Ready for message exchange | Yes |
| `rekeying` | No | Ratcheting group keys | No (buffered) |
| `leaving` | Yes | Gracefully departing group | No |
| `error` | Yes | Session failed | No |

**State tracking enables:**

- **Buffering**: Queue messages during rekeying, send when active
- **Observability**: Know where each session is in its lifecycle
- **Recovery**: Detect and handle error states appropriately

### Session Metadata

The SessionManager tracks metadata for each group:

```go
type GroupSession struct {
    GroupID        string       // DID-based group identifier
    State          SessionState // Current state
    JoinedAt       time.Time    // Session establishment time
    LastActive     time.Time    // Last message sent/received
    LastKeyRatchet time.Time    // Last key ratcheting
    MemberCount    int          // Group member count
    ErrorMessage   string       // Error details if state is error
}
```

This metadata supports:

- **Stale session detection**: Identify inactive groups for cleanup
- **Ratchet scheduling**: Determine when keys need rotation
- **Debugging**: Understand session history and failures

See `session_manager.go` for implementation.

## Message Formats and Mapping

SLIM messages follow a structured JSON format that maps to SemStreams internal message types.

### User Messages

User-initiated messages for conversational interactions:

```json
{
  "type": "user",
  "content": "Analyze the security posture of our API gateway",
  "attachments": [
    {
      "name": "config.yaml",
      "mime_type": "text/yaml",
      "data": "base64-encoded-content",
      "size": 1024
    }
  ],
  "metadata": {
    "priority": "high"
  },
  "reply_to": "msg_abc123",
  "thread_id": "thread_xyz789"
}
```

**Mapping to SemStreams UserMessage:**

| SLIM Field | SemStreams Field | Notes |
|------------|------------------|-------|
| `content` | `Content` | Message text |
| `attachments[]` | `Attachments[]` | File attachments |
| `metadata` | `Metadata` | Stored as `slim_*` keys |
| Group ID | `ChannelID` | From SLIM envelope |
| Sender DID | `UserID` | Agent DID from MLS |
| - | `ChannelType` | Always "slim" |

### Task Delegations

Cross-organizational task requests using the A2A (Agent-to-Agent) pattern:

```json
{
  "type": "task",
  "task_id": "task_def456",
  "prompt": "Review this codebase for security vulnerabilities",
  "role": "security-analyst",
  "model": "gpt-4",
  "requesting_agent_did": "did:agntcy:agent:org-a:analyst-001",
  "target_capabilities": ["security-analysis", "code-review"],
  "priority": "high",
  "deadline": "2024-01-16T10:00:00Z",
  "context": {
    "repository": "https://github.com/org/repo",
    "branch": "main"
  }
}
```

**Mapping to SemStreams TaskMessage:**

| SLIM Field | SemStreams Field | Notes |
|------------|------------------|-------|
| `task_id` | `TaskID` | Task identifier |
| `prompt` | `Prompt` | Task description |
| `role` | `Role` | Agent role for execution |
| `model` | `Model` | LLM model preference |
| `requesting_agent_did` | `UserID` | Requesting agent DID |
| Group ID | `ChannelID` | From SLIM envelope |
| - | `ChannelType` | Always "slim" |

### Response Messages

Replies to user messages or task delegations:

```json
{
  "type": "response",
  "in_reply_to": "msg_abc123",
  "status": "success",
  "content": "Analysis complete. Found 3 high-severity issues...",
  "metadata": {
    "execution_time_ms": 5432,
    "tokens_used": 1250
  }
}
```

**Mapping from SemStreams UserResponse:**

| SemStreams Field | SLIM Field | Notes |
|------------------|------------|-------|
| `Content` | `content` | Response text |
| `InReplyTo` | `in_reply_to` | Original message ID |
| `Type` | `status` | "error" type maps to status "error" |

### Task Results

Completion notifications for delegated tasks:

```json
{
  "type": "task_result",
  "in_reply_to": "task_def456",
  "status": "success",
  "content": "Security analysis results:\n\n1. SQL injection...",
  "metadata": {
    "completed_at": "2024-01-15T14:30:00Z"
  }
}
```

See `message_mapper.go` for complete mapping implementation.

## Integration with SemStreams Messaging

The SLIM bridge acts as a protocol adapter between SLIM groups and SemStreams NATS subjects:

```text
┌─────────────────────────────────────────────────────────────┐
│                    SLIM Bridge Architecture                  │
├─────────────────────────────────────────────────────────────┤
│                                                              │
│  External (SLIM)                                             │
│  ┌─────────────────────────────────────┐                    │
│  │  SLIM Groups (MLS encrypted)        │                    │
│  │  - did:agntcy:group:tenant-123      │                    │
│  │  - did:agntcy:group:project-alpha   │                    │
│  └──────────────┬──────────────────────┘                    │
│                 │                                            │
│                 │ gRPC + MLS                                 │
│                 ▼                                            │
│  ┌─────────────────────────────────────┐                    │
│  │     SLIM Bridge Component            │                   │
│  │                                      │                   │
│  │  ┌──────────────────────────────┐   │                   │
│  │  │  Session Manager             │   │                   │
│  │  │  - Join/leave groups         │   │                   │
│  │  │  - Key ratcheting            │   │                   │
│  │  │  - Session lifecycle         │   │                   │
│  │  └──────────────────────────────┘   │                   │
│  │                                      │                   │
│  │  ┌──────────────────────────────┐   │                   │
│  │  │  Message Mapper              │   │                   │
│  │  │  - SLIM ↔ SemStreams         │   │                   │
│  │  │  - Format translation        │   │                   │
│  │  └──────────────────────────────┘   │                   │
│  │                                      │                   │
│  └──────────────┬───────────────────────┘                   │
│                 │                                            │
│                 │ NATS JetStream                             │
│                 ▼                                            │
│  Internal (NATS)                                             │
│  ┌─────────────────────────────────────┐                    │
│  │  JetStream Subjects                 │                    │
│  │  - user.message.slim.{group_id}     │                    │
│  │  - agent.task.slim.{group_id}       │                    │
│  │  - agent.complete.slim.*            │                    │
│  └─────────────────────────────────────┘                    │
│                                                              │
└─────────────────────────────────────────────────────────────┘
```

### Inbound Flow (SLIM → NATS)

Messages received from SLIM groups are decrypted, mapped, and published to NATS:

```text
1. SLIM message arrives (MLS encrypted)
2. SessionManager decrypts using group key
3. MessageMapper determines type (user/task/response)
4. Map to SemStreams message format
5. Publish to appropriate NATS subject:
   - User messages → user.message.slim.{group_id}
   - Task delegations → agent.task.slim.{group_id}
6. agentic-dispatch routes to appropriate agent
7. agentic-loop executes task
```

### Outbound Flow (NATS → SLIM)

Agent responses are mapped back to SLIM format and encrypted for the group:

```text
1. Agent completes task
2. agentic-loop publishes agent.complete.slim.{group_id}
3. SLIM bridge listens for completions
4. MessageMapper converts to SLIM response format
5. SessionManager encrypts with group key
6. Send via SLIM protocol to group
```

### Subject Naming Convention

SLIM subjects include the sanitized group ID to enable per-group routing:

```text
Group: did:agntcy:group:tenant-acme

Sanitized: did-agntcy-group-tenant-acme
           (colons and dots replaced with dashes)

Subjects:
  user.message.slim.did-agntcy-group-tenant-acme
  agent.task.slim.did-agntcy-group-tenant-acme
  agent.complete.slim.did-agntcy-group-tenant-acme
```

This enables:

- **Per-group subscriptions**: Subscribe only to specific groups
- **Group isolation**: Messages from different groups don't interfere
- **Wildcard subscriptions**: `user.message.slim.*` for all SLIM groups

## Security Model

SLIM provides defense-in-depth security across multiple layers:

### Transport Security

```text
┌─────────────────────────────────────────────────────────────┐
│                    Security Layers                           │
├─────────────────────────────────────────────────────────────┤
│                                                              │
│  Layer 1: TLS (Transport)                                    │
│    WSS (WebSocket Secure) or gRPC over TLS                   │
│    Server authentication, transport encryption               │
│                                                              │
│  Layer 2: MLS (End-to-End)                                   │
│    Encrypted at sender, decrypted at receiver                │
│    Server cannot read message content                        │
│    Quantum-safe algorithms available (ML-KEM, ML-DSA)        │
│                                                              │
│  Layer 3: DID Authentication                                 │
│    Cryptographic verification of agent identity              │
│    Decentralized (no central CA required)                    │
│    Verifiable credentials for capabilities                   │
│                                                              │
│  Layer 4: Application Controls                               │
│    Tool allowlists, rate limiting, PII filtering             │
│    SemStreams agentic-governance integration                 │
│                                                              │
└─────────────────────────────────────────────────────────────┘
```

### Threat Model

| Threat | Defense | Provided By |
|--------|---------|-------------|
| **Network eavesdropping** | TLS encryption | gRPC/WSS |
| **Server compromise** | End-to-end MLS | MLS protocol |
| **Key compromise** | Forward secrecy via ratcheting | MLS protocol |
| **Identity spoofing** | DID cryptographic verification | AGNTCY identity |
| **Replay attacks** | Message IDs, timestamps | Protocol layer |
| **Quantum attacks** | ML-KEM, ML-DSA algorithms | MLS extensions |
| **Malicious agents** | Tool allowlists, governance | SemStreams |

### Key Material Lifecycle

```text
KeyPackage Creation → Group Join → Message Exchange → Key Ratchet → Group Leave
       │                   │               │               │              │
       │                   │               │               │              │
   Private key       Derive group      Use group       New group      Destroy
   generated          key from tree     key for         key, old       all keys
                                      encryption      destroyed
```

**Critical security properties:**

- **Key destruction**: Old keys are securely erased after ratcheting
- **Tree state consistency**: All members must agree on tree structure
- **Authenticated updates**: Only authorized members can add/remove participants
- **No plaintext storage**: SLIM bridge never persists unencrypted message content

## Use Cases for Cross-Organizational Communication

### Government/Contractor Collaboration

A government agency delegates analysis tasks to contractor agents securely:

```text
┌────────────────────────────────────────────────────────────┐
│  Government Agency (Org A)                                  │
│  ┌─────────────────┐                                        │
│  │ Analyst Agent   │ ────► "Analyze threat intel report"   │
│  └─────────────────┘                                        │
└──────────────┬─────────────────────────────────────────────┘
               │
               │ SLIM Group: did:agntcy:group:project-falcon
               │ (MLS encrypted, DID-authenticated)
               ▼
┌────────────────────────────────────────────────────────────┐
│  Contractor (Org B)                                         │
│  ┌─────────────────┐                                        │
│  │ Research Agent  │ ◄──── Receives task, analyzes, responds│
│  └─────────────────┘                                        │
└────────────────────────────────────────────────────────────┘
```

**Security requirements met:**

- End-to-end encryption (no server access to content)
- Cryptographic agent identity verification
- Audit trail of all interactions
- Forward secrecy (compromise doesn't expose history)

### Multi-Organization Incident Response

Multiple organizations coordinate during a security incident:

```text
SLIM Group: did:agntcy:group:incident-response-team

Members:
  - Security Operations Center (SOC) agents
  - Threat intelligence analysts
  - Forensics specialists
  - External consultants

Workflow:
  1. SOC agent detects anomaly
  2. Publishes alert to SLIM group
  3. Threat intel agent correlates with known campaigns
  4. Forensics agent analyzes affected systems
  5. Consultant agent recommends mitigations
  6. All results aggregated in shared context
```

**Advantages over traditional channels:**

- **Machine-actionable**: Agents can parse and act on messages
- **Cryptographically secure**: End-to-end encrypted
- **Federated**: No central authority required
- **Auditable**: Complete interaction history

### Federated Agent Marketplace

Agents discover and hire specialized agents from other organizations:

```text
Agent A (needs code review):
  1. Queries AGNTCY directory for agents with "code-review" capability
  2. Discovers Agent B from different organization
  3. Creates SLIM group for task collaboration
  4. Delegates code review task via SLIM
  5. Agent B performs review, returns results
  6. Payment/reputation update via smart contract
```

**SLIM enables:**

- Secure task delegation across trust boundaries
- Verifiable credentials proving agent capabilities
- Reputation tracking via persistent DIDs

### Supply Chain Coordination

Logistics agents coordinate across supplier, manufacturer, and distributor:

```text
SLIM Group: did:agntcy:group:order-12345

Timeline:
  T0: Supplier agent confirms raw materials available
  T1: Manufacturer agent schedules production
  T2: Quality agent validates output
  T3: Logistics agent coordinates shipping
  T4: Distributor agent confirms receipt

Each agent publishes status updates to shared SLIM group.
All participants have real-time visibility with cryptographic authenticity.
```

## Configuration

The SLIM bridge component is configured like any other SemStreams component:

```yaml
components:
  - name: slim-bridge
    type: slim
    config:
      # SLIM service endpoint
      slim_endpoint: "wss://slim.agntcy.dev"

      # Groups to join on startup
      group_ids:
        - "did:agntcy:group:tenant-acme"
        - "did:agntcy:group:project-alpha"

      # Key ratcheting interval (forward secrecy)
      key_ratchet_interval: "1h"

      # Connection management
      reconnect_interval: "5s"
      max_reconnect_attempts: 10

      # Identity provider
      identity_provider: "local"  # or "azure-ad", "okta"

      # Message buffering
      message_buffer_size: 1000

      # Output ports
      ports:
        outputs:
          - name: user_messages
            type: jetstream
            stream_name: USER_MESSAGES
            subject: "user.message.slim.>"
          - name: task_messages
            type: jetstream
            stream_name: AGENT_TASKS
            subject: "agent.task.slim.>"
```

### Configuration Options

| Option | Type | Default | Description |
|--------|------|---------|-------------|
| `slim_endpoint` | string | Required | WebSocket/gRPC endpoint for SLIM service |
| `group_ids` | []string | Required | SLIM groups to join on startup |
| `key_ratchet_interval` | duration | `1h` | Interval for MLS key ratcheting |
| `reconnect_interval` | duration | `5s` | Delay between reconnection attempts |
| `max_reconnect_attempts` | int | `10` | Maximum reconnection attempts before failure |
| `identity_provider` | string | `local` | DID identity provider (local, azure-ad, okta) |
| `message_buffer_size` | int | `1000` | Message buffer size for processing queue |

## Performance Considerations

### Message Throughput

SLIM is optimized for conversational messaging, not high-throughput data pipelines:

| Metric | SLIM | NATS JetStream |
|--------|------|----------------|
| Latency (P50) | ~100ms | ~10ms |
| Throughput | ~100 msg/sec | ~100k msg/sec |
| Max message size | ~1MB | ~1MB |
| Group size | ~1000 members | N/A |

**Use SLIM for:**
- Cross-organizational task delegation
- Secure command and control
- Federated agent collaboration

**Use NATS for:**
- High-throughput data pipelines
- Intra-organizational coordination
- Millisecond-latency requirements

### Key Ratcheting Overhead

Key ratcheting pauses message sending while new keys are distributed:

```text
Ratchet Process:
  1. Generate new group key: ~10ms
  2. Distribute to N members: ~N × 20ms
  3. Members update local state: ~50ms

Total: ~(N × 20ms) + 60ms

For 10 members: ~260ms
For 100 members: ~2.06s
```

**Mitigation strategies:**

- Schedule ratcheting during low-traffic periods
- Buffer messages during ratcheting (sent when active)
- Adjust ratchet interval based on group activity

See `session_manager.go` for buffering implementation.

## Operational Monitoring

The SLIM bridge exposes operational metrics and session state:

### Health Status

```go
status := component.Health()
// Healthy: true if connected and sessions active
// ErrorCount: Failed message/connection attempts
// Uptime: Time since component started
```

### Session Metrics

```go
sessions := component.GetSessions()
for _, session := range sessions {
    fmt.Printf("Group: %s\n", session.GroupID)
    fmt.Printf("State: %s\n", session.State)
    fmt.Printf("Members: %d\n", session.MemberCount)
    fmt.Printf("Last Active: %s\n", session.LastActive)
    fmt.Printf("Last Ratchet: %s\n", session.LastKeyRatchet)
}
```

### Prometheus Metrics

| Metric | Type | Description |
|--------|------|-------------|
| `slim_bridge_messages_received_total` | Counter | Messages received from SLIM groups |
| `slim_bridge_messages_sent_total` | Counter | Messages sent to SLIM groups |
| `slim_bridge_errors_total` | Counter | Message processing errors |
| `slim_bridge_sessions_active` | Gauge | Active SLIM sessions |
| `slim_bridge_ratchet_duration_seconds` | Histogram | Key ratcheting duration |

## Limitations and Future Work

### Current Implementation Status

The `input/slim` package provides the **infrastructure** for SLIM integration. Full functionality requires:

1. **AGNTCY SLIM SDK** — MLS protocol implementation, group management, encryption
2. **DID integration** — Agent identity verification and credential management
3. **Production SLIM service** — Hosted SLIM nodes for group coordination

The current implementation includes:

- Session manager with lifecycle tracking
- Message mapper for SLIM ↔ SemStreams translation
- Component structure for NATS integration
- Stub interfaces for SDK integration

### Known Limitations

| Limitation | Impact | Mitigation |
|------------|--------|------------|
| No message ordering guarantees | Out-of-order delivery possible | Use thread_id/reply_to for sequencing |
| Group size limits (~1000) | Cannot scale to massive groups | Use multiple groups or direct delegation |
| Ratcheting blocks sends | Brief unavailability during ratchet | Buffer messages, send when active |
| MLS requires all members online | Cannot add member if offline | Use invitation system with delayed join |

### Future Enhancements

**Phase 1 (Current)**: Infrastructure and stub implementation

**Phase 2**: Full SLIM SDK integration
- MLS protocol implementation
- Group join/leave with KeyPackage exchange
- Message encryption/decryption
- Key ratcheting automation

**Phase 3**: Advanced features
- Message receipts and read status
- Typing indicators for agent activity
- File transfer optimization (ObjectStore integration)
- Group administration capabilities

**Phase 4**: Production hardening
- Multi-region SLIM node failover
- Persistent session recovery
- Advanced monitoring and alerting
- Performance optimization for large groups

## References

### SLIM and AGNTCY

- [SLIM IETF Draft](https://datatracker.ietf.org/doc/draft-mpsb-agntcy-slim/)
- [AGNTCY Documentation](https://docs.agntcy.org)
- [AGNTCY GitHub](https://github.com/agntcy)
- [Linux Foundation Announcement](https://www.linuxfoundation.org/press/linux-foundation-welcomes-the-agntcy-project)

### MLS Protocol

- [RFC 9420: MLS Protocol](https://datatracker.ietf.org/doc/rfc9420/)
- [MLS Architecture](https://messaginglayersecurity.rocks/)

### SemStreams Integration

- [ADR-019: AGNTCY Integration](../architecture/adr-019-agntcy-integration.md)
- [OASF Integration](20-oasf-integration.md)
- [Agentic Systems](11-agentic-systems.md)

### Implementation

- `input/slim/component.go` — SLIM bridge component
- `input/slim/session_manager.go` — MLS session lifecycle
- `input/slim/message_mapper.go` — SLIM ↔ SemStreams translation
