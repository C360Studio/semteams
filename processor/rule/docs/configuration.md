# Configuration

Complete configuration reference for the rule processor.

## Config Structure

```go
type Config struct {
    Ports                  *component.PortConfig
    RulesFiles             []string
    InlineRules            []Definition
    MessageCache           cache.Config
    BufferWindowSize       string
    AlertCooldownPeriod    string
    EnableGraphIntegration bool
    EntityWatchPatterns    []string
    Consumer               ConsumerConfig
}
```

## Basic Configuration

### rules_files

Paths to JSON files containing rule definitions.

```json
{
  "rules_files": [
    "/etc/semstreams/rules/alerts.json",
    "/etc/semstreams/rules/relationships.json"
  ]
}
```

**Type:** `[]string`
**Default:** `[]`

### inline_rules

Rule definitions embedded directly in config (alternative to files).

```json
{
  "inline_rules": [
    {
      "id": "battery-low",
      "conditions": [
        {"field": "drone.telemetry.battery", "operator": "lt", "value": 20}
      ],
      "on_enter": [
        {"type": "publish", "subject": "alerts.battery"}
      ]
    }
  ]
}
```

**Type:** `[]Definition`
**Default:** `[]`

### enable_graph_integration

When true, `add_triple` and `remove_triple` actions modify the graph.

```json
{
  "enable_graph_integration": true
}
```

**Type:** `bool`
**Default:** `true`

When disabled, triple actions are logged but not executed. Useful for testing.

## Advanced Configuration

### entity_watch_patterns

NATS wildcard patterns for entity IDs to watch in `ENTITY_STATES` bucket.

```json
{
  "entity_watch_patterns": [
    "acme.*.robotics.*.drone.*",
    "acme.*.environmental.*.sensor.*"
  ]
}
```

**Type:** `[]string`
**Default:** `[]`

See [Entity Watching](06-entity-watching.md) for pattern syntax.

### buffer_window_size

Time window for message buffering and analysis.

```json
{
  "buffer_window_size": "10m"
}
```

**Type:** `string` (duration)
**Default:** `"10m"`

Format: Go duration string (e.g., `"30s"`, `"5m"`, `"1h"`)

### alert_cooldown_period

Minimum time between repeated alerts for the same entity-rule combination.

```json
{
  "alert_cooldown_period": "2m"
}
```

**Type:** `string` (duration)
**Default:** `"2m"`

Prevents alert spam when conditions flap rapidly.

## Port Configuration

### ports.inputs

Define input sources for rule evaluation.

```json
{
  "ports": {
    "inputs": [
      {
        "name": "entity_states",
        "type": "kv-watch",
        "required": true,
        "description": "Watch entity state changes"
      },
      {
        "name": "predicate_index",
        "type": "kv-watch",
        "required": false,
        "description": "Watch predicate index changes"
      }
    ]
  }
}
```

### ports.outputs

Define output destinations for rule actions.

```json
{
  "ports": {
    "outputs": [
      {
        "name": "control_commands",
        "type": "nats",
        "subject": "control.*.commands",
        "required": false,
        "description": "Control commands based on rules"
      }
    ]
  }
}
```

## Consumer Configuration

Internal JetStream consumer settings.

```json
{
  "consumer": {
    "enabled": true,
    "ack_wait_seconds": 30,
    "max_deliver": 3,
    "replay_policy": "instant"
  }
}
```

| Field | Type | Default | Description |
|-------|------|---------|-------------|
| `enabled` | bool | `true` | Enable JetStream consumer |
| `ack_wait_seconds` | int | `30` | Acknowledgment timeout |
| `max_deliver` | int | `3` | Max delivery attempts |
| `replay_policy` | string | `"instant"` | `"instant"` or `"original"` |

## Message Cache Configuration

Internal message caching for windowed analysis.

```json
{
  "message_cache": {
    "enabled": true,
    "strategy": "ttl",
    "max_size": 1000,
    "ttl": "30s",
    "cleanup_interval": "15s",
    "stats_interval": "30s"
  }
}
```

| Field | Type | Default | Description |
|-------|------|---------|-------------|
| `enabled` | bool | `true` | Enable message caching |
| `strategy` | string | `"ttl"` | Cache eviction strategy |
| `max_size` | int | `1000` | Maximum cached messages |
| `ttl` | duration | `"30s"` | Time-to-live for entries |
| `cleanup_interval` | duration | `"15s"` | Cache cleanup frequency |
| `stats_interval` | duration | `"30s"` | Stats logging frequency |

## Default Configuration

```go
func DefaultConfig() Config {
    return Config{
        Ports: &component.PortConfig{
            Inputs: []component.PortDefinition{
                {Name: "entity_states", Type: "kv-watch", Required: true},
                {Name: "predicate_index", Type: "kv-watch", Required: false},
            },
            Outputs: []component.PortDefinition{
                {Name: "control_commands", Type: "nats", Subject: "control.*.commands"},
            },
        },
        MessageCache: cache.Config{
            Enabled:         true,
            Strategy:        cache.StrategyTTL,
            MaxSize:         1000,
            TTL:             30 * time.Second,
            CleanupInterval: 15 * time.Second,
            StatsInterval:   30 * time.Second,
        },
        BufferWindowSize:       "10m",
        AlertCooldownPeriod:    "2m",
        EnableGraphIntegration: true,
        Consumer: {
            Enabled:        true,
            AckWaitSeconds: 30,
            MaxDeliver:     3,
            ReplayPolicy:   "instant",
        },
    }
}
```

## Runtime Configuration

Some settings can be updated at runtime without restart.

### Dynamically Updateable

| Setting | Hot Reload | Notes |
|---------|------------|-------|
| `enable_graph_integration` | Yes | Takes effect on next action |
| `rules` (individual) | Yes | Add/update/remove rules |
| `entity_watch_patterns` | No | Requires restart |

### ApplyConfigUpdate

```go
changes := map[string]any{
    "enable_graph_integration": false,
    "rules": map[string]any{
        "battery-low": map[string]any{
            "type": "expression",
            "conditions": [...],
        },
    },
}

err := processor.ApplyConfigUpdate(changes)
```

### GetRuntimeConfig

```go
config := processor.GetRuntimeConfig()
// Returns:
// {
//   "buffer_window_size": "10m",
//   "alert_cooldown_period": "2m",
//   "enable_graph_integration": true,
//   "entity_watch_patterns": [...],
//   "rules": {...},
//   "rule_count": 5,
//   "is_running": true
// }
```

## Complete Example

```json
{
  "ports": {
    "inputs": [
      {"name": "entity_states", "type": "kv-watch", "required": true}
    ],
    "outputs": [
      {"name": "alerts", "type": "nats", "subject": "alerts.>"}
    ]
  },

  "rules_files": [
    "/etc/semstreams/rules/alerts.json",
    "/etc/semstreams/rules/fleet.json"
  ],

  "entity_watch_patterns": [
    "acme.*.robotics.*.drone.*",
    "acme.*.environmental.*.sensor.*"
  ],

  "buffer_window_size": "10m",
  "alert_cooldown_period": "5m",
  "enable_graph_integration": true,

  "consumer": {
    "enabled": true,
    "ack_wait_seconds": 30,
    "max_deliver": 3,
    "replay_policy": "instant"
  }
}
```

## Environment Variables

Configuration can be overridden via environment variables:

| Variable | Config Field |
|----------|--------------|
| `SEMSTREAMS_RULES_FILES` | `rules_files` (comma-separated) |
| `SEMSTREAMS_ENTITY_WATCH_PATTERNS` | `entity_watch_patterns` (comma-separated) |
| `SEMSTREAMS_ENABLE_GRAPH_INTEGRATION` | `enable_graph_integration` |
| `SEMSTREAMS_ALERT_COOLDOWN` | `alert_cooldown_period` |

## Validation

Configuration is validated on load:

- `rules_files` paths must exist and be readable
- `inline_rules` must have valid structure
- `buffer_window_size` must be valid Go duration
- `alert_cooldown_period` must be valid Go duration
- `entity_watch_patterns` must be valid NATS wildcards

Invalid configuration logs warnings but doesn't prevent startup (graceful degradation).

## Next Steps

- [Custom Rules](08-custom-rules.md) - Extending the rule system
- [Operations](09-operations.md) - Monitoring and debugging
- [Examples](10-examples.md) - Working configurations
