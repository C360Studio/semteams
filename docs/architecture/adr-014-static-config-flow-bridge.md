# ADR-014: Static Config to Flow Bridge

## Status

Proposed

## Context

SemStreams supports two operational modes:

1. **Headless Mode**: Load components from static JSON config file, run without UI
2. **UI Mode**: Visual flow builder where users design, deploy, and manage flows

The system uses two separate NATS KV buckets for persistence:

| Bucket | Contents | Used By |
|--------|----------|---------|
| `semstreams_config` | Component configurations, service configs | ComponentManager (runtime) |
| `semstreams_flows` | Visual flow definitions (canvas layout, connections) | FlowService (UI API) |

**The Problem**: When a static config file is loaded, components run but are **invisible to the UI** because:

- Static config → `semstreams_config` KV → ComponentManager starts components
- UI reads from `semstreams_flows` KV → empty, no flows to display
- **No bridge exists** between the two buckets

This breaks the intended workflow where users can:
1. Start with a static config for headless deployment
2. Later connect UI to view/modify the running flow
3. Have UI changes persist across restarts

```
CURRENT (broken for UI visibility):
┌─────────────┐     ┌──────────────────┐     ┌────────────────┐
│ config.json │ ──► │ semstreams_config│ ──► │ ComponentMgr   │
└─────────────┘     │     KV bucket    │     │ (runs them)    │
                    └──────────────────┘     └────────────────┘
                              │
                              ╳ NO CONNECTION
                              │
                    ┌──────────────────┐     ┌────────────────┐
                    │ semstreams_flows │ ◄── │ FlowService    │
                    │     KV bucket    │     │ (UI reads)     │
                    └──────────────────┘     └────────────────┘
```

## Decision

On startup, if static config contains components but no flows exist in KV:
1. Convert component configurations to a Flow entity
2. Persist the Flow to `semstreams_flows` KV bucket
3. Mark the flow as `running` since components are already active

Apply the same **KV wins** precedence pattern used by config.Manager:
- **First boot**: Static config → create Flow in KV
- **Subsequent boots**: KV Flow wins (preserves UI customizations)

```
INTENDED (UI can see static configs):
┌─────────────┐     ┌──────────────────┐     ┌────────────────┐
│ config.json │ ──► │ semstreams_config│ ──► │ ComponentMgr   │
└─────────────┘     │     KV bucket    │     │ (runs them)    │
                    └──────────────────┘     └────────────────┘
                              │
                    ┌─────────▼────────┐
                    │  Config→Flow     │  ◄── NEW: Converter
                    │  Bridge          │
                    └─────────┬────────┘
                              │
                    ┌─────────▼────────┐     ┌────────────────┐
                    │ semstreams_flows │ ◄── │ FlowService    │
                    │     KV bucket    │     │ (UI reads)     │
                    └──────────────────┘     └────────────────┘
```

### Conversion Logic

Each ComponentConfig becomes a FlowNode:

```go
// flowstore/converter.go
func FromComponentConfigs(name string, configs map[string]types.ComponentConfig) (*Flow, error) {
    flow := &Flow{
        ID:           uuid.New().String(),
        Name:         name,
        Version:      1,
        RuntimeState: RuntimeStateRunning,
        DeployedAt:   timePtr(time.Now()),
        StartedAt:    timePtr(time.Now()),
        CreatedAt:    time.Now(),
        UpdatedAt:    time.Now(),
    }

    for key, cfg := range configs {
        node := FlowNode{
            ID:     key,
            Type:   cfg.Name,  // Component factory name (e.g., "udp", "graph-processor")
            Name:   key,
            Config: cfg.Config,
            Position: calculatePosition(len(flow.Nodes)),  // Auto-layout
        }
        flow.Nodes = append(flow.Nodes, node)
    }

    // Derive connections from port subject matching
    flow.Connections = deriveConnections(flow.Nodes)

    return flow, nil
}
```

### Startup Integration

In `cmd/semstreams/main.go`, after FlowStore is available:

```go
// Static config is starting point. KV flow wins if it exists.
flows, _ := flowStore.List(ctx)
if len(flows) == 0 && len(cfg.Components) > 0 {
    // First boot: no flows in KV, create from static config
    defaultFlow, err := flowstore.FromComponentConfigs("default", cfg.Components)
    if err == nil {
        flowStore.Create(ctx, defaultFlow)
        logger.Info("Created default flow from static config",
            "components", len(cfg.Components))
    }
} else if len(flows) > 0 {
    // Subsequent boot: KV flow exists, use it (may have UI customizations)
    logger.Info("Using existing flow from KV", "flows", len(flows))
}
```

### Precedence Rules

Following the established config.Manager pattern:

| Scenario | Behavior |
|----------|----------|
| First boot, static config has components | Create flow from config → KV |
| First boot, no components in config | No flow created (minimal config) |
| Subsequent boot, flow exists in KV | Use KV flow (ignore static config) |
| Subsequent boot, flow deleted from KV | Re-create from static config |

This ensures:
- UI customizations persist across restarts
- Users can "reset" to static config by deleting the flow
- Headless deployments work unchanged

### Flow State for Static Configs

Static configs are already running when semstreams starts. The created flow reflects this:

```go
flow.RuntimeState = RuntimeStateRunning  // Already running
flow.DeployedAt   = &startupTime         // Deployment = startup
flow.StartedAt    = &startupTime         // Start = startup
```

This allows UI to show accurate state and offer appropriate controls (Stop, not Start).

## Consequences

### Positive

- **UI Visibility**: Static configs now visible in flow builder
- **Unified Model**: Both headless and UI modes use same Flow abstraction
- **Precedence Clarity**: Same "KV wins" pattern as config (predictable)
- **Non-Breaking**: Headless deployments continue working unchanged
- **User Customization**: UI changes persist across restarts

### Negative

- **Conversion Complexity**: Port subject matching for connections requires FlowGraph analysis
- **Position Layout**: Auto-generated node positions may not be optimal (users can rearrange in UI)
- **One Default Flow**: Multiple static configs would need separate handling

### Neutral

- **Migration Path**: Existing deployments get flow on first restart
- **Flow Naming**: Default flow named "default" (simple, predictable)

## Key Files

| File | Purpose |
|------|---------|
| `flowstore/converter.go` | Config→Flow conversion logic (NEW) |
| `flowstore/converter_test.go` | Converter unit tests (NEW) |
| `cmd/semstreams/main.go` | Startup integration |
| `flowstore/flow.go` | Flow, FlowNode, FlowConnection types |
| `component/flowgraph/` | Port subject matching for connections |
| `config/manager.go` | Reference for "KV wins" precedence pattern |

## References

- [FlowStore README](../../flowstore/README.md) - Flow persistence details
- [FlowEngine README](../../engine/README.md) - Flow deployment lifecycle
- [Configuration Guide](../basics/06-configuration.md) - Static config structure
