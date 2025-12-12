# Subject Naming Convention

This document describes the NATS subject naming convention used in SemStreams.

## Pattern: `component.action.type`

| Position | Meaning | Examples |
|----------|---------|----------|
| 1st | Component name | `file`, `document`, `objectstore`, `graph`, `rule`, `sensor` |
| 2nd | Action/verb | `read`, `processed`, `stored`, `indexed`, `triggered` |
| 3rd | Data type | `entity`, `sensor`, `alert`, `document` |

## Common Subjects

| Subject | Description |
|---------|-------------|
| `document.processed.entity` | Document processor output (Graphable) |
| `sensor.processed.entity` | IoT sensor processor output (Graphable) |
| `objectstore.stored.entity` | ObjectStore output (Storable = Graphable + StorageRef) |
| `rule.triggered.alert` | Rule processor triggered alerts |
| `graph.indexed.entity` | Graph processor indexed entities |

## Wildcard Patterns

| Pattern | Matches | Use Case |
|---------|---------|----------|
| `objectstore.>` | All objectstore events | Monitor objectstore activity |
| `*.stored.*` | All stored items | Storage audit across components |
| `*.*.entity` | All entity events | Entity tracking across pipeline |
| `*.processed.*` | All processor outputs | Monitor processing stage |
| `document.>` | All document processor events | Document processor monitoring |
| `graph.>` | All graph processor events | Graph monitoring |

## Data Flow Example

```
file.read.document           # File input reads documents
    |
document.processed.entity    # Document processor transforms to Graphable
    |
objectstore.stored.entity    # ObjectStore stores + emits Storable
    |
graph.indexed.entity         # Graph indexes the Storable entity
```

## JetStream Stream Configuration

Graph processor JetStream streams should capture multiple subject patterns:

```json
{
  "stream_subjects": ["objectstore.stored.>", "*.processed.entity"]
}
```

This allows the graph to:
1. Process stored entities from ObjectStore
2. Directly process entities that bypass ObjectStore

## Interface Types

Subjects often include interface hints for type safety:

| Interface | Description |
|-----------|-------------|
| `content.document.v1` | Document content entities |
| `iot.sensor.v1` | IoT sensor readings |
| `core.json.v1` | Generic JSON payloads |

Example port configuration:

```json
{
  "outputs": [{
    "name": "doc_out",
    "subject": "document.processed.entity",
    "type": "nats",
    "interface": "content.document.v1"
  }]
}
```
