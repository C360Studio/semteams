# OASF Integration

This document describes how SemStreams integrates with the Open Agent Specification Framework (OASF) for AGNTCY directory registration.

## Overview

The OASF generator component watches for agent entity changes and generates OASF records that describe agent capabilities in a standardized format. These records can then be used by the directory-bridge component to register agents with AGNTCY directories.

## OASF Record Structure

OASF (Open Agent Specification Framework) is the standard format for describing agent capabilities in the AGNTCY ecosystem.

```json
{
  "name": "agent-architect",
  "version": "1.0.0",
  "schema_version": "1.0.0",
  "authors": ["system"],
  "created_at": "2024-01-15T10:30:00Z",
  "description": "Designs software architecture and system components",
  "skills": [
    {
      "id": "software-design",
      "name": "Software Design",
      "description": "Creates software architecture diagrams and designs",
      "confidence": 0.95,
      "permissions": ["file_system_read", "file_system_write"]
    }
  ],
  "domains": [
    {
      "name": "software-architecture",
      "description": "Software architecture and design patterns"
    }
  ],
  "extensions": {
    "semstreams_entity_id": "acme.ops.agentic.system.agent.architect",
    "source": "semstreams"
  }
}
```

## Predicate Mapping

The OASF generator maps SemStreams predicates to OASF fields:

| SemStreams Predicate | OASF Field | Description |
|---------------------|------------|-------------|
| `agent.capability.name` | `skills[].name` | Human-readable capability name |
| `agent.capability.description` | `skills[].description` | Detailed capability description |
| `agent.capability.expression` | `skills[].id` | Unique skill identifier |
| `agent.capability.confidence` | `skills[].confidence` | Self-assessed confidence (0.0-1.0) |
| `agent.capability.permission` | `skills[].permissions[]` | Required permissions |
| `agent.intent.goal` | `description` | Agent's primary objective |
| `agent.intent.type` | `domains[].name` | Domain categorization |
| `agent.action.type` | `extensions.action_types[]` | Supported action types |

## NATS Topology

```
┌─────────────────────┐
│  ENTITY_STATES KV   │──── KV Watch ────┐
└─────────────────────┘                   │
                                          ▼
                              ┌───────────────────────┐
                              │    OASF Generator     │
                              │   (processor)         │
                              └───────────────────────┘
                                     │         │
                    ┌────────────────┘         └────────────────┐
                    ▼                                            ▼
         ┌─────────────────────┐                    ┌───────────────────────┐
         │  OASF_RECORDS KV    │                    │ oasf.record.generated.*│
         │   (storage)         │                    │   (JetStream)          │
         └─────────────────────┘                    └───────────────────────┘
```

### Input Sources

| Subject/Bucket | Type | Purpose |
|---------------|------|---------|
| `ENTITY_STATES` | KV Watch | Watch for agent entity changes |
| `oasf.generate.request` | NATS Request | On-demand generation requests |

### Output Destinations

| Subject/Bucket | Type | Purpose |
|---------------|------|---------|
| `OASF_RECORDS` | KV Write | Store generated OASF records |
| `oasf.record.generated.*` | JetStream | Publish generation events |

## Configuration

```yaml
components:
  - name: oasf-gen
    type: oasf-generator
    config:
      # KV buckets
      entity_kv_bucket: ENTITY_STATES
      oasf_kv_bucket: OASF_RECORDS

      # Watch configuration
      watch_pattern: "*.agent.*"  # Only watch agent entities
      generation_debounce: "1s"   # Debounce rapid changes

      # Record defaults
      default_agent_version: "1.0.0"
      default_authors:
        - "system"

      # Extensions
      include_extensions: true
```

### Configuration Options

| Option | Type | Default | Description |
|--------|------|---------|-------------|
| `entity_kv_bucket` | string | `ENTITY_STATES` | KV bucket to watch for entity changes |
| `oasf_kv_bucket` | string | `OASF_RECORDS` | KV bucket to store OASF records |
| `watch_pattern` | string | `*` | Key pattern for KV watch |
| `generation_debounce` | duration | `1s` | Debounce duration for generation |
| `default_agent_version` | string | `1.0.0` | Default version for OASF records |
| `default_authors` | []string | `[]` | Default authors list |
| `include_extensions` | bool | `true` | Include SemStreams extensions |

## Usage Example

### Creating Agent Capabilities

Store capability triples for an agent entity:

```go
// Create capability triples
triples := []message.Triple{
    {
        Subject:   "acme.ops.agentic.system.agent.architect",
        Predicate: agentic.CapabilityName,
        Object:    "Software Design",
        Source:    "system",
        Timestamp: time.Now(),
    },
    {
        Subject:   "acme.ops.agentic.system.agent.architect",
        Predicate: agentic.CapabilityDescription,
        Object:    "Creates software architecture diagrams",
        Source:    "system",
        Timestamp: time.Now(),
    },
    {
        Subject:   "acme.ops.agentic.system.agent.architect",
        Predicate: agentic.CapabilityExpression,
        Object:    "software-design",
        Source:    "system",
        Timestamp: time.Now(),
    },
    {
        Subject:   "acme.ops.agentic.system.agent.architect",
        Predicate: agentic.CapabilityConfidence,
        Object:    0.95,
        Source:    "system",
        Timestamp: time.Now(),
    },
}

// Store in entity state (triggers OASF generation)
entityState := EntityState{
    ID:        "acme.ops.agentic.system.agent.architect",
    Triples:   triples,
    UpdatedAt: time.Now(),
}
data, _ := json.Marshal(entityState)
kv.Put(ctx, entityState.ID, data)
```

### Retrieving OASF Records

```go
// Get OASF record from KV
oasfKV, _ := client.GetKVBucket(ctx, "OASF_RECORDS")
entry, _ := oasfKV.Get(ctx, "acme.ops.agentic.system.agent.architect")

var record OASFRecord
json.Unmarshal(entry.Value(), &record)

fmt.Printf("Agent: %s\n", record.Name)
fmt.Printf("Skills: %d\n", len(record.Skills))
```

## Integration with Directory Bridge

The OASF records generated by this component are consumed by the `directory-bridge` output component for AGNTCY directory registration:

```yaml
components:
  - name: oasf-gen
    type: oasf-generator
    config:
      entity_kv_bucket: ENTITY_STATES
      oasf_kv_bucket: OASF_RECORDS

  - name: dir-bridge
    type: directory-bridge
    config:
      oasf_kv_bucket: OASF_RECORDS
      directory_url: "https://directory.agntcy.org"
      heartbeat_interval: "30s"
```

## Metrics

The OASF generator exposes Prometheus metrics:

| Metric | Type | Description |
|--------|------|-------------|
| `oasf_generator_records_generated_total` | Counter | Total OASF records generated |
| `oasf_generator_records_failed_total` | Counter | Total generation failures |
| `oasf_generator_generation_duration_seconds` | Histogram | Generation duration |
| `oasf_generator_skills_generated_total` | Counter | Total skills generated |
| `oasf_generator_domains_generated_total` | Counter | Total domains generated |
| `oasf_generator_entity_changes_total` | Counter | Entity changes processed |

## See Also

- [ADR-019: AGNTCY Integration](../architecture/adr-019-agntcy-integration.md)
- [Predicate Registry Guide](13-payload-registry.md)
- [OASF Specification](https://docs.agntcy.org/pages/syntaxes/oasf)
