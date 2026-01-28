# SOFTWARE DESIGN DESCRIPTION

## Config Entity Processor

---

| Field | Value |
|-------|-------|
| **Document Identifier** | SDD-SEMSTREAMS-CEP-001 |
| **Document Title** | Software Design Description for Config Entity Processor |
| **Contract Number** | N/A (Internal Development) |
| **CDRL Sequence Number** | N/A |
| **Prepared For** | SemStreams Project |
| **Prepared By** | C360 Studio |
| **Version** | 0.1.0 |
| **Status** | DRAFT |
| **Classification** | UNCLASSIFIED |
| **Distribution** | Distribution A: Approved for public release; distribution unlimited |

---

## REVISION HISTORY

| Version | Date | Author | Description |
|---------|------|--------|-------------|
| 0.1.0 | 2025-01-27 | C360 Studio | Initial draft |

---

## DOCUMENT APPROVAL

| Role | Name | Signature | Date |
|------|------|-----------|------|
| Author | | | |
| Technical Lead | | | |
| Project Manager | | | |
| Quality Assurance | | | |

---

## TABLE OF CONTENTS

1. [Scope](#1-scope)
   1.1 [Identification](#11-identification)
   1.2 [System Overview](#12-system-overview)
   1.3 [Document Overview](#13-document-overview)
2. [Referenced Documents](#2-referenced-documents)
3. [Design Decisions](#3-design-decisions)
   3.1 [Design Principles](#31-design-principles)
   3.2 [Design Constraints](#32-design-constraints)
   3.3 [Design Goals](#33-design-goals)
   3.4 [Non-Goals](#34-non-goals)
4. [Software Architecture](#4-software-architecture)
   4.1 [Component Structure](#41-component-structure)
   4.2 [Component Relationships](#42-component-relationships)
   4.3 [Data Flow](#43-data-flow)
   4.4 [External Interfaces](#44-external-interfaces)
5. [Data Design](#5-data-design)
   5.1 [Data Model](#51-data-model)
   5.2 [Configuration Schema](#52-configuration-schema)
   5.3 [Entity Mapping Specification](#53-entity-mapping-specification)
   5.4 [Storage Design](#54-storage-design)
6. [Detailed Design](#6-detailed-design)
   6.1 [Sync Behavior](#61-sync-behavior)
   6.2 [Reconciliation Logic](#62-reconciliation-logic)
   6.3 [Security Controls](#63-security-controls)
7. [Interface Design](#7-interface-design)
   7.1 [API Contract](#71-api-contract)
   7.2 [Message Formats](#72-message-formats)
8. [Requirements Traceability](#8-requirements-traceability)
9. [Notes](#9-notes)
   9.1 [Use Cases](#91-use-cases)
   9.2 [Examples](#92-examples)
   9.3 [Future Considerations](#93-future-considerations)

Appendix A. [Acronyms and Abbreviations](#appendix-a-acronyms-and-abbreviations)
Appendix B. [Glossary](#appendix-b-glossary)
Appendix C. [Security Considerations](#appendix-c-security-considerations)

---

## 1. SCOPE

### 1.1 Identification

This Software Design Description (SDD) describes the design of the **Config Entity Processor** component, a software item within the SemStreams semantic stream processing framework.

| Attribute | Value |
|-----------|-------|
| Software Item Name | Config Entity Processor |
| Software Item Identifier | SEMSTREAMS-CEP |
| Version | 0.1.0 |
| Component Type | Optional Processor |
| Tier Requirement | Tier 0 (Structural/Deterministic) |

### 1.2 System Overview

SemStreams is a semantic stream processing framework designed for tactical environments and edge computing applications. The system processes real-time data from heterogeneous sources into knowledge graphs using RDF triples, with support for tiered operation (Tier 0: Deterministic, Tier 1: Statistical, Tier 2: Semantic).

The Config Entity Processor is an optional component that bridges operational configuration with the knowledge graph. It enables operators to define resources (agents, services, federation peers) in configuration files while automatically synchronizing queryable representations to the entity graph.

### 1.3 Document Overview

This SDD provides:

- Design decisions and rationale for the Config Entity Processor
- Software architecture and component structure
- Data models and schemas
- Interface specifications
- Requirements traceability
- Implementation guidance

This document is intended for:

- Software developers implementing the component
- System integrators deploying SemStreams
- Quality assurance personnel validating the design
- Operations personnel configuring the system

---

## 2. REFERENCED DOCUMENTS

### 2.1 Applicable Documents

The following documents form a part of this specification to the extent specified herein.

| Document ID | Title | Version |
|-------------|-------|---------|
| SDD-SEMSTREAMS-CORE-001 | SemStreams Core Architecture Specification | 1.0.0 |
| SDD-SEMSTREAMS-WFP-001 | Workflow Processor Specification | 1.0.0 |
| SDD-SEMSTREAMS-OPT-001 | SemStreams Optional Components Specification | 0.1.0 |

### 2.2 Reference Documents

The following documents provide background information and are referenced herein.

| Document ID | Title | Source |
|-------------|-------|--------|
| IEEE 1016-2009 | IEEE Standard for Information Technology—Systems Design—Software Design Descriptions | IEEE |
| DI-IPSC-81435A | Data Item Description: Software Design Description | DoD |
| NIST SP 800-53 | Security and Privacy Controls for Information Systems | NIST |

---

## 3. DESIGN DECISIONS

### 3.1 Design Principles

The following principles govern the design of the Config Entity Processor:

| ID | Principle | Description | Rationale |
|----|-----------|-------------|-----------|
| DP-01 | One-way sync | Config → Entities only, never reverse | Configuration is authoritative source of truth for operational parameters |
| DP-02 | Explicit mapping | Only designated `entity:` blocks synchronize | Prevent accidental credential exposure; maintain clear separation of concerns |
| DP-03 | Idempotent operations | Same configuration always produces same entities | Safe to re-run; predictable behavior; supports automated deployment |
| DP-04 | Reconciling | Tracks provenance; cleans up deletions | No orphan entities; maintains graph integrity |
| DP-05 | Optional component | Disabled by default; explicit opt-in required | Prevent graph pollution; only enable when queryable config is needed |

### 3.2 Design Constraints

| ID | Constraint | Description |
|----|------------|-------------|
| DC-01 | NATS dependency | Component requires NATS JetStream for storage and messaging |
| DC-02 | Entity bucket access | Requires write access to target entity bucket |
| DC-03 | File system access | Requires read access to configuration file paths |
| DC-04 | No credential sync | Credentials and secrets MUST NOT be synchronized to entity graph |

### 3.3 Design Goals

| ID | Goal | Priority | Metric |
|----|------|----------|--------|
| DG-01 | Single source of truth | High | Operators maintain one config file per resource type |
| DG-02 | Automatic synchronization | High | Changes detected and synced within 5 seconds |
| DG-03 | Queryable resources | High | All synced resources queryable via GraphQL/MCP |
| DG-04 | Minimal pollution | Medium | Config entities distinguishable via namespace/provenance |
| DG-05 | Hot reload | Medium | Configuration changes applied without restart |

### 3.4 Non-Goals

The following are explicitly excluded from this design:

| ID | Non-Goal | Rationale |
|----|----------|-----------|
| NG-01 | Bidirectional sync | Entity changes should not modify operational config |
| NG-02 | Secret management | Use dedicated secret management solutions |
| NG-03 | Config validation | Use JSON Schema or dedicated validators |
| NG-04 | Real-time critical | Eventual consistency (seconds) is acceptable |

---

## 4. SOFTWARE ARCHITECTURE

### 4.1 Component Structure

The Config Entity Processor is organized as follows:

```
processor/config-entity/
├── processor.go        # Main ConfigEntityProcessor component
├── watcher.go          # File and NATS KV watching
├── mapper.go           # Config → Entity mapping logic
├── reconciler.go       # Deletion reconciliation
├── schema.go           # Mapping schema definitions
└── register.go         # Component registration
```

| File | Responsibility |
|------|----------------|
| processor.go | Component lifecycle, message handling, coordination |
| watcher.go | File system monitoring (fsnotify), NATS KV watching |
| mapper.go | Configuration parsing, entity extraction, property mapping |
| reconciler.go | Startup reconciliation, deletion detection, orphan cleanup |
| schema.go | Mapping rule definitions, validation |
| register.go | Component registry integration |

### 4.2 Component Relationships

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                          Config Entity Processor                             │
│                                                                              │
│  Config Sources:                        Entity Targets:                      │
│  ┌──────────────────┐                   ┌──────────────────┐                │
│  │ JSON files  │                   │ ENTITY_STATES    │                │
│  │ (watched)        │───────────────────│ bucket           │                │
│  └──────────────────┘    entity:        └──────────────────┘                │
│                          section              │                              │
│  ┌──────────────────┐    extracted            │                              │
│  │ CONFIG_* buckets │───────────────────      │                              │
│  │ (NATS KV)        │                   ▼     ▼                              │
│  └──────────────────┘              ┌──────────────────┐                     │
│                                    │ Rules Engine     │                     │
│  Runtime Components:               │ Query Engine     │                     │
│  ┌──────────────────┐              │ Graph Gateway    │                     │
│  │ Agent Interface  │◄─────────────└──────────────────┘                     │
│  │ Service Manager  │  reads                                                 │
│  │ Federation       │  operational                                          │
│  └──────────────────┘  config directly                                      │
│                                                                              │
└─────────────────────────────────────────────────────────────────────────────┘
```

### 4.3 Data Flow

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                       Config Entity Processor Data Flow                      │
│                                                                              │
│  Inputs:                                                                     │
│  ┌──────────────────────┐  ┌──────────────────────┐                         │
│  │ File watcher         │  │ NATS KV watcher      │                         │
│  │ (fsnotify)           │  │ (bucket.Watch)       │                         │
│  └──────────┬───────────┘  └──────────┬───────────┘                         │
│             │                         │                                      │
│             └────────────┬────────────┘                                      │
│                          ▼                                                   │
│             ┌────────────────────────┐                                      │
│             │    Config Parser       │                                      │
│             │    (JSON)         │                                      │
│             └────────────┬───────────┘                                      │
│                          ▼                                                   │
│             ┌────────────────────────┐                                      │
│             │    Entity Mapper       │                                      │
│             │                        │                                      │
│             │  • Extract entity:     │                                      │
│             │  • Apply mappings      │                                      │
│             │  • Add provenance      │                                      │
│             └────────────┬───────────┘                                      │
│                          ▼                                                   │
│             ┌────────────────────────┐                                      │
│             │    Reconciler          │                                      │
│             │                        │                                      │
│             │  • Diff current/new    │                                      │
│             │  • Identify deletions  │                                      │
│             │  • Track provenance    │                                      │
│             └────────────┬───────────┘                                      │
│                          ▼                                                   │
│  Outputs:                                                                    │
│  ┌──────────────────────┐  ┌──────────────────────┐                         │
│  │ graph.entity.upsert  │  │ graph.entity.delete  │                         │
│  │ (new/updated)        │  │ (removed from config)│                         │
│  └──────────────────────┘  └──────────────────────┘                         │
│                                                                              │
└─────────────────────────────────────────────────────────────────────────────┘
```

### 4.4 External Interfaces

| Interface | Type | Direction | Description |
|-----------|------|-----------|-------------|
| Config files | File I/O | Input | JSON configuration files |
| CONFIG_* buckets | NATS KV | Input | Dynamic configuration storage |
| ENTITY_STATES bucket | NATS KV | Output | Target for synchronized entities |
| graph.entity.* subjects | NATS Pub/Sub | Output | Entity mutation messages |
| config.entity.* subjects | NATS Request/Reply | Bidirectional | Management API |

---

## 5. DATA DESIGN

### 5.1 Data Model

#### 5.1.1 Configuration Structure Convention

Configuration files follow a standard pattern separating operational and entity data:

```json
{
  "<resource_type>": {
    "<resource_id>": {
      
      "connection": {
        "endpoint": "...",
        "protocol": "..."
      },
      "auth": {
        "type": "...",
        "credentials_env": "..."
      },
      "operational": {
        "timeout": "...",
        "retry_policy": "..."
      },
      
      "entity": {
        "name": "Human readable name",
        "type": "specific_type",
        "properties": {
          "key": "value"
        }
      }
    }
  }
}
```

**Note**: Comments shown above for clarity; actual JSON files do not contain comments.

#### 5.1.2 Entity Output Structure

```
ConfigDerivedEntity
├── id: string              # Generated entity identifier
├── type: string            # Entity type (e.g., "agent", "service")
├── properties: map         # Properties from config entity: block
│   ├── <user-defined>      # Properties from configuration
│   ├── _config_source      # Provenance: source file/bucket
│   ├── _config_key         # Provenance: key within config
│   └── _synced_at          # ISO timestamp of last sync
```

#### 5.1.3 Entity ID Generation

Default pattern: `{namespace}.{resource_type}.{resource_id}`

| Config Key | Pattern | Generated ID |
|------------|---------|--------------|
| `agents.claude-code-primary` | `semmem.{type}.{id}` | `semmem.agents.claude-code-primary` |
| `services.shimmy-local` | `config.{type}.{id}` | `config.services.shimmy-local` |

### 5.2 Configuration Schema

#### 5.2.1 Processor Configuration

```json
{
  "type": "processor",
  "name": "config-entity",
  "enabled": false,
  
  "config": {
    "sources": [
      {
        "type": "file",
        "path": "config/agents.json",
        "watch": true
      },
      {
        "type": "file",
        "path": "config/services.json",
        "watch": true
      },
      {
        "type": "nats_kv",
        "bucket": "CONFIG_AGENTS",
        "watch": true
      }
    ],
    
    "target": {
      "bucket": "ENTITY_STATES",
      "subject": "graph.entity.upsert"
    },
    
    "mappings": [
      {
        "config_path": "agents.*",
        "entity_type": "agent",
        "entity_id_pattern": "config.agents.{key}",
        "property_path": "entity"
      },
      {
        "config_path": "services.*",
        "entity_type": "service",
        "entity_id_pattern": "config.services.{key}",
        "property_path": "entity"
      }
    ],
    
    "sync": {
      "reconcile_on_startup": true,
      "reconcile_interval": "5m",
      "delete_removed": true
    }
  }
}
```

**Notes**: 
- `enabled: false` enforces DC-05 (explicit opt-in)
- `target.bucket` can be `CONFIG_ENTITIES` for isolation
- Comments shown in documentation only; actual JSON files do not contain comments

#### 5.2.2 Mapping Rule Schema

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| config_path | string | Yes | JSONPath-like selector (e.g., "agents.*") |
| entity_type | string | Yes | Type for created entities |
| entity_id_pattern | string | Yes | Pattern for generating entity IDs |
| property_path | string | Yes | Path to entity properties in config |
| flatten | array | No | Property flattening rules |
| defaults | map | No | Default property values |
| exclude | array | No | Properties to exclude from sync |

### 5.3 Entity Mapping Specification

#### 5.3.1 Mapping Process

1. **Extract entity section** from configuration
2. **Generate entity ID** using configured pattern
3. **Build properties** from entity section
4. **Apply flattening** rules if configured
5. **Apply defaults** for missing properties
6. **Add provenance** metadata
7. **Validate** against security blocklist

#### 5.3.2 Security Blocklist

The following fields are NEVER synchronized to entities, regardless of mapping configuration:

| Pattern | Category | Rationale |
|---------|----------|-----------|
| `auth.*` | Credentials | Authentication configuration |
| `credentials.*` | Credentials | Credential storage |
| `*_secret` | Secrets | Secret values |
| `*_key` | API Keys | API key values (except capability keys) |
| `*_password` | Passwords | Password values |
| `*_token` | Tokens | Token values |
| `connection.endpoint` | Operational | Infrastructure detail |
| `operational.*` | Operational | Runtime parameters |

### 5.4 Storage Design

#### 5.4.1 Target Buckets

| Bucket | Purpose | Isolation Level |
|--------|---------|-----------------|
| ENTITY_STATES | Default target; shared with all entities | Low |
| CONFIG_ENTITIES | Optional; dedicated to config-derived entities | High |

#### 5.4.2 Provenance Tracking

Each synchronized entity includes provenance properties:

| Property | Type | Description |
|----------|------|-------------|
| _config_source | string | File path or NATS bucket key |
| _config_key | string | Key within configuration |
| _synced_at | string | ISO 8601 timestamp |

---

## 6. DETAILED DESIGN

### 6.1 Sync Behavior

#### 6.1.1 Startup Reconciliation

On processor startup, the following reconciliation occurs:

```
PROCEDURE: Startup Reconciliation
────────────────────────────────────────────────────────────────────────────────
1. LOAD current configuration from all sources
2. MAP configuration to expected entities
3. QUERY existing entities with matching provenance (_config_source)
4. COMPUTE diff:
   a. toCreate = expected NOT IN existing
   b. toUpdate = expected DIFFERENT FROM existing
   c. toDelete = existing NOT IN expected
5. APPLY changes:
   a. FOR EACH entity IN toCreate: PUBLISH graph.entity.upsert
   b. FOR EACH entity IN toUpdate: PUBLISH graph.entity.upsert
   c. FOR EACH entity IN toDelete: PUBLISH graph.entity.delete (if delete_removed)
6. LOG reconciliation summary
────────────────────────────────────────────────────────────────────────────────
```

#### 6.1.2 Change Detection

On configuration file change or NATS KV update:

```
PROCEDURE: Change Detection
────────────────────────────────────────────────────────────────────────────────
1. RECEIVE change notification (fsnotify or NATS watch)
2. PARSE updated configuration
3. MAP to entities
4. RETRIEVE previous state from cache
5. COMPUTE diff
6. APPLY changes
7. UPDATE cache with new state
────────────────────────────────────────────────────────────────────────────────
```

### 6.2 Reconciliation Logic

#### 6.2.1 Deletion Handling

When a configuration entry is removed:

| Configuration | Action if `delete_removed: true` | Action if `delete_removed: false` |
|---------------|----------------------------------|-----------------------------------|
| Entry removed | Publish `graph.entity.delete` | Set `status: decommissioned` |
| File deleted | Delete all entities from that source | Mark all as decommissioned |

#### 6.2.2 Conflict Resolution

| Scenario | Resolution |
|----------|------------|
| Same entity ID from multiple sources | Last-write-wins; log warning |
| Entity exists without provenance | Do not modify; log warning |
| Malformed configuration | Skip entry; log error |

### 6.3 Security Controls

#### 6.3.1 Credential Protection

| Control ID | Control | Implementation |
|------------|---------|----------------|
| SC-01 | Blocklist enforcement | Hardcoded patterns checked before sync |
| SC-02 | Warning on detection | Log WARNING if credential-like data in entity section |
| SC-03 | Mapping validation | Reject mappings that would expose operational config |

#### 6.3.2 Access Control

| Resource | Required Permission |
|----------|---------------------|
| Config files | File system read |
| CONFIG_* buckets | NATS KV read |
| ENTITY_STATES bucket | NATS KV write |
| graph.entity.* subjects | NATS publish |

---

## 7. INTERFACE DESIGN

### 7.1 API Contract

#### 7.1.1 Output Subjects

| Subject | Payload Type | Description |
|---------|--------------|-------------|
| `graph.entity.upsert` | EntityState | Create or update entity |
| `graph.entity.delete` | DeleteRequest | Delete entity by ID |
| `config.entity.synced` | SyncEvent | Notification of sync completion |

#### 7.1.2 Request/Reply Subjects

| Subject | Request | Response | Description |
|---------|---------|----------|-------------|
| `config.entity.sync` | SyncRequest | SyncResponse | Force sync of specific source |
| `config.entity.status` | Empty | StatusResponse | Get processor status |

### 7.2 Message Formats

#### 7.2.1 EntityState (Output)

```json
{
  "id": "semmem.agents.claude-code-primary",
  "type": "agent",
  "properties": {
    "name": "Claude Code (Primary)",
    "capabilities": {
      "languages": ["go", "typescript"]
    },
    "status": "available",
    "_config_source": "config/agents.json",
    "_config_key": "agents.claude-code-primary",
    "_synced_at": "2025-01-27T10:30:00Z"
  }
}
```

#### 7.2.2 SyncEvent (Notification)

```json
{
  "source": "config/agents.json",
  "timestamp": "2025-01-27T10:30:00Z",
  "created": 2,
  "updated": 1,
  "deleted": 0,
  "errors": 0
}
```

#### 7.2.3 StatusResponse

```json
{
  "sources": [
    {
      "path": "config/agents.json",
      "type": "file",
      "last_sync": "2025-01-27T10:30:00Z",
      "entity_count": 3,
      "status": "healthy"
    }
  ],
  "total_entities": 5,
  "uptime_seconds": 3600
}
```

---

## 8. REQUIREMENTS TRACEABILITY

### 8.1 Functional Requirements Traceability

| Requirement ID | Requirement | Design Element | Section |
|----------------|-------------|----------------|---------|
| FR-01 | Sync config to entities | Entity Mapper | 5.3 |
| FR-02 | Watch config changes | File/KV Watcher | 4.3 |
| FR-03 | Reconcile on startup | Reconciler | 6.1.1 |
| FR-04 | Handle deletions | Reconciliation Logic | 6.2.1 |
| FR-05 | Track provenance | Provenance Properties | 5.4.2 |

### 8.2 Non-Functional Requirements Traceability

| Requirement ID | Requirement | Design Element | Section |
|----------------|-------------|----------------|---------|
| NFR-01 | Sync within 5 seconds | File/KV Watch | 4.3 |
| NFR-02 | No credential exposure | Security Blocklist | 5.3.2 |
| NFR-03 | Idempotent operations | Mapping Process | 5.3.1 |
| NFR-04 | Optional component | Processor Config | 5.2.1 |

### 8.3 Security Requirements Traceability

| Requirement ID | Requirement | Control | Section |
|----------------|-------------|---------|---------|
| SR-01 | Protect credentials | SC-01 Blocklist | 6.3.1 |
| SR-02 | Warn on credential detection | SC-02 Warning | 6.3.1 |
| SR-03 | Enforce access control | Access Control | 6.3.2 |

---

## 9. NOTES

### 9.1 Use Cases

#### 9.1.1 UC-01: SemMem Agent Registry

**Actors**: System Operator, SemMem System

**Preconditions**: 
- Config Entity Processor enabled
- Agent configuration file exists

**Flow**:
1. Operator defines agents in `semmem/config/agents.json`
2. Processor watches file for changes
3. On change, processor extracts `entity:` sections
4. Processor syncs entities to ENTITY_STATES
5. SemMem queries graph for agent capabilities
6. Task matching uses agent entities for assignment

**Configuration** (`semmem/config/agents.json`):

```json
{
  "agents": {
    "claude-code-primary": {
      "connection": {
        "protocol": "poll",
        "poll_interval": "30s"
      },
      "auth": {
        "type": "api_key",
        "key_env": "CLAUDE_CODE_API_KEY"
      },
      "operational": {
        "timeout": "30m",
        "rate_limit": "10/hour"
      },
      
      "entity": {
        "name": "Claude Code (Primary)",
        "capabilities": {
          "languages": ["go", "typescript", "python"],
          "task_types": ["implement", "test", "refactor"],
          "context_window": 200000
        },
        "preferences": {
          "max_concurrent": 1,
          "preferred_size": "medium",
          "requires_design": true
        },
        "constraints": [
          "no_direct_db_migrations",
          "requires_approval_for_api_changes"
        ]
      }
    }
  }
}
```

**Resulting Entity**:

```json
{
  "id": "semmem.agents.claude-code-primary",
  "type": "agent",
  "properties": {
    "name": "Claude Code (Primary)",
    "capabilities": {
      "languages": ["go", "typescript", "python"],
      "task_types": ["implement", "test", "refactor"],
      "context_window": 200000
    },
    "preferences": {
      "max_concurrent": 1,
      "preferred_size": "medium",
      "requires_design": true
    },
    "constraints": [
      "no_direct_db_migrations",
      "requires_approval_for_api_changes"
    ],
    "status": "available",
    "_config_source": "semmem/config/agents.json",
    "_config_key": "agents.claude-code-primary",
    "_synced_at": "2025-01-27T10:30:00Z"
  }
}
```

#### 9.1.2 UC-02: Federation Peer Registry

**Configuration** (`config/federation.json`):

```json
{
  "peers": {
    "shore-primary": {
      "connection": {
        "endpoint": "nats://shore.example.com:4222",
        "mtls": {
          "cert_file": "/certs/edge.crt",
          "key_file": "/certs/edge.key"
        }
      },
      "sync": {
        "interval": "5m",
        "batch_size": 100
      },
      
      "entity": {
        "name": "Shore Primary",
        "region": "us-east",
        "tier": "shore",
        "capabilities": ["full_graph", "llm_inference"]
      }
    }
  }
}
```

#### 9.1.3 UC-03: Service Endpoint Registry

**Configuration** (`config/services.json`):

```json
{
  "services": {
    "shimmy-local": {
      "connection": {
        "endpoint": "http://localhost:8080",
        "timeout": "30s"
      },
      "auth": {
        "type": "none"
      },
      
      "entity": {
        "name": "Local Shimmy",
        "service_type": "llm",
        "models": ["qwen2.5-coder-7b", "qwen2.5-coder-14b"],
        "capabilities": ["completion", "chat"],
        "tier": 1,
        "cost": "low"
      }
    },

    "openai-fallback": {
      "connection": {
        "endpoint": "https://api.openai.com/v1"
      },
      "auth": {
        "type": "api_key",
        "key_env": "OPENAI_API_KEY"
      },
      
      "entity": {
        "name": "OpenAI Fallback",
        "service_type": "llm",
        "models": ["gpt-4o"],
        "capabilities": ["completion", "chat", "function_calling"],
        "tier": 2,
        "cost": "high"
      }
    }
  }
}
```

### 9.2 Examples

#### 9.2.1 Minimal Setup

`config/agents.json`:
```json
{
  "agents": {
    "my-agent": {
      "entity": {
        "name": "My Agent",
        "capabilities": ["code", "test"]
      }
    }
  }
}
```

Component configuration:
```json
{
  "type": "processor",
  "name": "config-entity",
  "enabled": true,
  "config": {
    "sources": [
      {
        "type": "file",
        "path": "config/agents.json"
      }
    ],
    "mappings": [
      {
        "config_path": "agents.*",
        "entity_type": "agent",
        "entity_id_pattern": "agents.{key}",
        "property_path": "entity"
      }
    ]
  }
}
```

#### 9.2.2 NATS KV as Config Source

```json
{
  "config": {
    "sources": [
      {
        "type": "nats_kv",
        "bucket": "CONFIG_AGENTS",
        "watch": true
      }
    ],
    "mappings": [
      {
        "config_path": "*",
        "entity_type": "agent",
        "entity_id_pattern": "agents.{key}",
        "property_path": "entity"
      }
    ]
  }
}
```

#### 9.2.3 Query Examples

```graphql
# Find agents capable of Go implementation
{
  entities(type: "agent", where: {
    "capabilities.languages": {contains: "go"},
    "capabilities.task_types": {contains: "implement"}
  }) {
    id
    properties
  }
}

# Find cheapest LLM service with chat capability
{
  entities(type: "service", where: {
    "service_type": "llm",
    "capabilities": {contains: "chat"}
  }, orderBy: {field: "cost", direction: ASC}, limit: 1) {
    id
    properties
  }
}

# List all federation peers in us-east region
{
  entities(type: "peer", where: {
    "region": "us-east"
  }) {
    id
    properties
  }
}
```

### 9.3 Future Considerations

| ID | Enhancement | Description | Complexity |
|----|-------------|-------------|------------|
| FC-01 | Reverse sync | Entity changes update config | High |
| FC-02 | Validation hooks | External validator before sync | Medium |
| FC-03 | CEL transformations | Expression-based property mapping | Medium |
| FC-04 | Multi-tenant | Namespace isolation per tenant | Medium |
| FC-05 | UI integration | Config entities in flow builder | Low |

---

## APPENDIX A. ACRONYMS AND ABBREVIATIONS

| Acronym | Definition |
|---------|------------|
| API | Application Programming Interface |
| CEL | Common Expression Language |
| CDRL | Contract Data Requirements List |
| DID | Data Item Description |
| DoD | Department of Defense |
| GraphQL | Graph Query Language |
| IEEE | Institute of Electrical and Electronics Engineers |
| ISO | International Organization for Standardization |
| JSON | JavaScript Object Notation |
| KV | Key-Value |
| MCP | Model Context Protocol |
| mTLS | Mutual Transport Layer Security |
| NATS | Neural Autonomic Transport System |
| NIST | National Institute of Standards and Technology |
| RDF | Resource Description Framework |
| SDD | Software Design Description |
| SemMem | Semantic Memory |
| SRS | Software Requirements Specification |

---

## APPENDIX B. GLOSSARY

| Term | Definition |
|------|------------|
| Config Entity | An entity derived from configuration data via this processor |
| Entity | A node in the SemStreams knowledge graph |
| Graph Pollution | Unintended mixing of operational and knowledge data in entity storage |
| Provenance | Metadata tracking the origin of an entity |
| Reconciliation | Process of synchronizing expected and actual state |
| Triple | A subject-predicate-object statement in the knowledge graph |

---

## APPENDIX C. SECURITY CONSIDERATIONS

### C.1 Threat Model

| Threat | Likelihood | Impact | Mitigation |
|--------|------------|--------|------------|
| Credential exposure via sync | Medium | High | Security blocklist (SC-01) |
| Unauthorized entity creation | Low | Medium | Access control (6.3.2) |
| Config injection attack | Low | High | Input validation, file permissions |
| Denial of service via watch | Low | Low | Rate limiting on reconciliation |

### C.2 Security Controls Mapping

| NIST SP 800-53 Control | Implementation |
|------------------------|----------------|
| AC-3 Access Enforcement | NATS auth for bucket/subject access |
| AU-3 Content of Audit Records | Provenance properties, sync events |
| SC-8 Transmission Confidentiality | NATS TLS |
| SC-28 Protection of Information at Rest | NATS JetStream encryption |

### C.3 Compliance Notes

For deployments requiring NIST 800-171 or CMMC compliance:

1. Enable NATS TLS for all connections
2. Configure NATS authentication
3. Use separate bucket (CONFIG_ENTITIES) for isolation
4. Enable audit logging for sync events
5. Review security blocklist against CUI categories

---

**END OF DOCUMENT**
