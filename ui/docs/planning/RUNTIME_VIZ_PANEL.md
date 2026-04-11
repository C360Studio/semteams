# Runtime Visualization Panel - Design Planning

## Context

**Goal**: Add runtime flow visualization for debugging purposes

**Current Setup**:

- Bottom status bar with Deploy/Start/Stop controls
- Shows runtime state: `not_deployed`, `deployed_stopped`, `running`, `error`
- No visibility into actual runtime behavior once deployed

**Need**: When flow is running, users need to see:

- Component health status (running, error, stopped)
- Message throughput (msgs/sec per component)
- Error messages and stack traces
- Connection activity (which edges are active)
- Resource usage (memory, CPU per component)
- Real-time logs

---

## Current Layout Analysis

```
┌──────────┬────────────────────────────────┬──────────┐
│ Palette  │ Header (name, save status)     │ Config   │
│ Sidebar  ├────────────────────────────────┤ Panel    │
│ 250px    │ Canvas (XYFlow)                │ (auto)   │
│ fixed    │                                │          │
│          │                                │          │
│          ├────────────────────────────────┤          │
│          │ StatusBar (Deploy/Start/Stop)  │          │
└──────────┴────────────────────────────────┴──────────┘
```

**Layout code:**

```css
.editor-layout {
  display: grid;
  grid-template-columns: 250px 1fr auto;
  height: 100vh;
  overflow: hidden;
}
```

**Current panels:**

- **Left**: Component palette (fixed 250px)
- **Center**: Canvas + header + status bar
- **Right**: Config panel (auto width, conditional)

---

## Option 1: Bottom Slide-Up Panel ⭐ (RECOMMENDED)

### Visual Concept

```
┌──────────┬────────────────────────────────┬──────────┐
│ Palette  │ Header                         │ Config   │
│ Sidebar  ├────────────────────────────────┤ Panel    │
│          │ Canvas (reduced height)        │          │
│          │                                │          │
│          ├────────────────────────────────┤          │
│          │ StatusBar (+ toggle button)    │          │
│          ├────────────────────────────────┤          │
│          │ Runtime Panel (slides up)      │          │
│          │ [Tabs: Logs | Metrics | Health]│          │
│          │                                │          │
└──────────┴────────────────────────────────┴──────────┘
```

### Implementation Approach

**HTML Structure:**

```svelte
<div class="canvas-area">
    <header class="editor-header">...</header>

    <div class="canvas-container" style="height: {canvasHeight};">
        <FlowCanvas ... />
    </div>

    <StatusBar
        onToggleRuntimePanel={toggleRuntimePanel}
        showRuntimePanelButton={runtimeState.state === 'running'}
    />

    {#if showRuntimePanel}
        <RuntimePanel
            height={runtimePanelHeight}
            onResize={handleRuntimePanelResize}
            onClose={closeRuntimePanel}
        />
    {/if}
</div>
```

**CSS (resizable panel):**

```css
.canvas-area {
  display: flex;
  flex-direction: column;
  height: 100vh;
  position: relative;
}

.runtime-panel {
  position: relative;
  height: var(--runtime-panel-height, 300px);
  min-height: 150px;
  max-height: 60vh;
  border-top: 1px solid var(--ui-border-emphasis);
  background: var(--ui-surface-primary);
  resize: vertical;
  overflow: auto;
}

.runtime-panel-resize-handle {
  position: absolute;
  top: 0;
  left: 0;
  right: 0;
  height: 4px;
  cursor: ns-resize;
  background: var(--ui-border-subtle);
}

.runtime-panel-resize-handle:hover {
  background: var(--ui-interactive-primary);
}
```

### Pros ✅

1. **Natural association** - Runtime controls in status bar, runtime viz below it
2. **Doesn't compete with config panel** - Right panel stays for component config
3. **Standard pattern** - Matches VSCode bottom panel, Chrome DevTools console
4. **Better for logs** - Horizontal space perfect for log lines
5. **Canvas stays wide** - Full width for flow visualization
6. **Independent toggling** - Can show/hide without affecting config panel
7. **Easy implementation** - Just toggle visibility + height adjustment
8. **Keyboard shortcut friendly** - Ctrl+` like VSCode terminal

### Cons ⚠️

1. **Reduces canvas vertical space** - Canvas height shrinks when panel open
2. **Scrolling complexity** - Need to handle overflow correctly
3. **Mobile/small screens** - Might be too cramped

### User Experience

**Opening the panel:**

```
User clicks "Debug" button in status bar
  → Panel slides up from bottom (300px default)
  → Canvas height reduces proportionally
  → Panel shows tabs: Logs, Metrics, Health
```

**Resizing:**

```
User drags resize handle at top of panel
  → Panel height adjusts (150px min, 60vh max)
  → Canvas height adjusts inversely
  → Height preference saved to localStorage
```

**Closing:**

```
User clicks X or presses Esc
  → Panel slides down
  → Canvas expands to full height
```

### Content Tabs

**Tab 1: Logs** (default)

```
┌────────────────────────────────────────────┐
│ [Filter] [▼ Level] [Clear]                │
├────────────────────────────────────────────┤
│ 14:23:01 [udp-source] INFO: Listening...   │
│ 14:23:02 [processor] DEBUG: Received msg   │
│ 14:23:03 [processor] ERROR: Parse failed   │
│   at processor.go:45                       │
│   invalid JSON syntax                      │
└────────────────────────────────────────────┘
```

**Tab 2: Metrics**

```
┌────────────────────────────────────────────┐
│ Component         Msg/sec   Errors   CPU   │
├────────────────────────────────────────────┤
│ udp-source        1,234     0        5%    │
│ json-processor    1,230     4        12%   │
│ nats-sink         1,226     0        3%    │
└────────────────────────────────────────────┘
```

**Tab 3: Health**

```
┌────────────────────────────────────────────┐
│ Component         Status      Uptime       │
├────────────────────────────────────────────┤
│ ● udp-source      Running     00:15:32     │
│ ● json-processor  Running     00:15:31     │
│ ⚠ nats-sink       Degraded    00:15:30     │
│   └─ Slow acks (>100ms)                    │
└────────────────────────────────────────────┘
```

---

## Option 2: Right Panel Tab

### Visual Concept

```
┌──────────┬────────────────────┬──────────────┐
│ Palette  │ Header             │ Right Panel  │
│ Sidebar  ├────────────────────┤ (resizable)  │
│          │ Canvas             │              │
│          │                    │ ┌──────────┐ │
│          │                    │ │Config│Dbg│ │
│          │                    │ ├──────────┤ │
│          │                    │ │          │ │
│          │                    │ │ Content  │ │
│          ├────────────────────┤ │          │ │
│          │ StatusBar          │ └──────────┘ │
└──────────┴────────────────────┴──────────────┘
```

### Implementation Approach

**HTML Structure:**

```svelte
{#if selectedComponent || showRuntimePanel}
    <aside class="right-panel" style="width: {rightPanelWidth}px;">
        <div class="panel-tabs">
            <button
                class:active={activeTab === 'config'}
                onclick={() => activeTab = 'config'}
            >Config</button>
            <button
                class:active={activeTab === 'runtime'}
                onclick={() => activeTab = 'runtime'}
                disabled={runtimeState.state !== 'running'}
            >Debug</button>
        </div>

        <div class="panel-content">
            {#if activeTab === 'config'}
                <ConfigPanel ... />
            {:else if activeTab === 'runtime'}
                <RuntimePanel ... />
            {/if}
        </div>

        <div class="resize-handle" onmousedown={startResize}></div>
    </aside>
{/if}
```

**CSS (resizable sidebar):**

```css
.right-panel {
  min-width: 300px;
  max-width: 50vw;
  width: var(--right-panel-width, 400px);
  border-left: 1px solid var(--ui-border-subtle);
  position: relative;
  display: flex;
  flex-direction: column;
}

.resize-handle {
  position: absolute;
  left: 0;
  top: 0;
  bottom: 0;
  width: 4px;
  cursor: ew-resize;
  background: var(--ui-border-subtle);
}

.resize-handle:hover {
  background: var(--ui-interactive-primary);
}

.panel-tabs {
  display: flex;
  border-bottom: 1px solid var(--ui-border-subtle);
  background: var(--ui-surface-secondary);
}

.panel-tabs button {
  flex: 1;
  padding: 0.75rem;
  border: none;
  background: transparent;
  cursor: pointer;
}

.panel-tabs button.active {
  background: var(--ui-surface-primary);
  border-bottom: 2px solid var(--ui-interactive-primary);
}
```

### Pros ✅

1. **More vertical space** - Runtime info gets full height
2. **Unified panel** - Single resizable panel for all tools
3. **Familiar pattern** - Similar to browser DevTools docked right
4. **Persistent width** - User sets width once, applies to both tabs
5. **Canvas stays tall** - Full vertical space for flow

### Cons ⚠️

1. **Competes with config panel** - Config and runtime share same space
2. **Narrower for logs** - Horizontal space constrained
3. **Context switching** - Need to switch tabs to see config vs runtime
4. **Requires resizable implementation** - More complex than bottom panel
5. **Canvas width reduced** - Flow canvas gets narrower
6. **Config panel displaced** - Current config UX changes significantly

---

## Detailed Comparison

| Factor                  | Bottom Panel               | Right Panel Tab          |
| ----------------------- | -------------------------- | ------------------------ |
| **Canvas Impact**       | Reduces height             | Reduces width            |
| **Log Readability**     | ✅ Better (wider)          | ⚠️ Narrower              |
| **Config Access**       | ✅ Independent             | ⚠️ Must switch tabs      |
| **Runtime Metrics**     | ⚠️ Needs horizontal scroll | ✅ More vertical space   |
| **Standard Pattern**    | ✅ VSCode, Chrome          | ⚠️ Less common           |
| **Implementation**      | ✅ Simpler                 | ⚠️ More complex          |
| **Mobile/Small Screen** | ⚠️ Very cramped            | ⚠️ Cramped               |
| **Keyboard Shortcuts**  | ✅ Natural (Ctrl+`)        | ⚠️ Need new pattern      |
| **Resizing**            | ✅ Vertical drag           | ⚠️ Horizontal drag       |
| **Multiple Data Views** | ✅ Tabs fit well           | ⚠️ Nested tabs confusing |

---

## Recommendation: Bottom Panel

### Why Bottom Panel Wins

1. **Runtime = Bottom Convention**: Industry standard
   - VSCode: Terminal, Debug Console, Problems → bottom
   - Chrome DevTools: Console → bottom (or right, but bottom is default)
   - IntelliJ: Run, Debug → bottom
   - VS: Output, Error List → bottom

2. **Logs Need Width**: Runtime debugging is log-heavy
   - Stack traces need horizontal space
   - Long log lines common (timestamps, levels, messages)
   - Metrics tables benefit from width

3. **Config Panel Independence**: Users need both simultaneously
   - Edit component config while watching runtime behavior
   - See how config changes affect metrics
   - No context switching required

4. **Natural Information Hierarchy**:

   ```
   Top:    Flow structure (canvas)
   Bottom: Flow runtime (debugging)
   Right:  Component details (config)
   ```

5. **Canvas Shape**: Flows are typically wider than tall
   - Losing height less impactful than losing width
   - Most flows spread horizontally (left to right data flow)

### Recommended Implementation Plan

**Phase 1: Basic Panel** (MVP)

- [ ] Add toggle button to StatusBar
- [ ] Create RuntimePanel component (slides up from bottom)
- [ ] Canvas height adjustment logic
- [ ] Show placeholder content

**Phase 2: Logs Tab**

- [ ] WebSocket connection to backend logs
- [ ] Real-time log streaming
- [ ] Log filtering (level, component)
- [ ] Auto-scroll toggle

**Phase 3: Metrics Tab**

- [ ] Poll backend for metrics data
- [ ] Component throughput table
- [ ] Error rate visualization
- [ ] CPU/Memory usage (if available)

**Phase 4: Health Tab**

- [ ] Component health status
- [ ] Connection health (edge activity)
- [ ] Uptime tracking
- [ ] Alert/warning indicators

**Phase 5: Polish**

- [ ] Resizable panel with drag handle
- [ ] Persist panel height to localStorage
- [ ] Keyboard shortcuts (Ctrl+` toggle, Esc close)
- [ ] Panel animation (slide up/down)

---

## Alternative: Hybrid Approach

**If you want both eventually:**

1. **Bottom panel** for logs (primary use case)
   - Quick access via Ctrl+`
   - Shows during runtime
   - Logs, errors, output

2. **Right panel tab** for detailed metrics
   - "Performance" tab next to "Config"
   - Shows metrics, graphs, flame charts
   - Optional, power-user feature

This gives:

- Quick log access (bottom)
- Detailed analysis (right)
- Config stays accessible (right)

---

## Implementation Details

### StatusBar Integration

**Add toggle button:**

```svelte
<!-- StatusBar.svelte -->
<div class="button-section">
    {#if runtimeState.state === 'running'}
        <button
            onclick={() => onToggleRuntimePanel?.()}
            class="debug-button"
            aria-label="Toggle runtime panel"
            title="Show runtime debugging info (Ctrl+`)"
        >
            {showRuntimePanel ? '▼' : '▲'} Debug
        </button>
    {/if}

    <!-- Existing Deploy/Start/Stop buttons -->
</div>
```

### RuntimePanel Component

**Create new component:**

```svelte
<!-- RuntimePanel.svelte -->
<script lang="ts">
    let { height, onResize, onClose } = $props();
    let activeTab = $state('logs');
</script>

<div class="runtime-panel" style="height: {height}px;">
    <!-- Resize handle -->
    <div class="resize-handle"></div>

    <!-- Tabs -->
    <div class="panel-header">
        <div class="panel-tabs">
            <button class:active={activeTab === 'logs'}
                onclick={() => activeTab = 'logs'}>
                Logs
            </button>
            <button class:active={activeTab === 'metrics'}
                onclick={() => activeTab = 'metrics'}>
                Metrics
            </button>
            <button class:active={activeTab === 'health'}
                onclick={() => activeTab = 'health'}>
                Health
            </button>
        </div>
        <button class="close-button" onclick={onClose}>✕</button>
    </div>

    <!-- Content -->
    <div class="panel-content">
        {#if activeTab === 'logs'}
            <RuntimeLogs />
        {:else if activeTab === 'metrics'}
            <RuntimeMetrics />
        {:else if activeTab === 'health'}
            <RuntimeHealth />
        {/if}
    </div>
</div>
```

### Canvas Height Adjustment

**In +page.svelte:**

```svelte
<script>
    let showRuntimePanel = $state(false);
    let runtimePanelHeight = $state(300);

    const canvasHeight = $derived(
        showRuntimePanel
            ? `calc(100vh - ${headerHeight}px - ${statusBarHeight}px - ${runtimePanelHeight}px)`
            : `calc(100vh - ${headerHeight}px - ${statusBarHeight}px)`
    );
</script>

<div class="canvas-container" style="height: {canvasHeight};">
    <FlowCanvas ... />
</div>
```

---

## Data Requirements

### Backend API Needs

**Runtime Logs Endpoint:**

```
GET /flows/{id}/runtime/logs
  ?since=timestamp
  &level=debug|info|warn|error
  &component=component-id

Returns: Server-Sent Events (SSE) stream
```

**Runtime Metrics Endpoint:**

```
GET /flows/{id}/runtime/metrics

Returns: {
  components: [{
    id: string,
    name: string,
    messages_per_sec: number,
    errors_per_sec: number,
    cpu_percent: number,
    memory_mb: number,
    status: 'running' | 'degraded' | 'error'
  }],
  connections: [{
    id: string,
    from: string,
    to: string,
    messages_per_sec: number,
    active: boolean
  }]
}
```

**Runtime Health Endpoint:**

```
GET /flows/{id}/runtime/health

Returns: {
  overall_status: 'healthy' | 'degraded' | 'error',
  components: [{
    id: string,
    status: 'running' | 'stopped' | 'error',
    uptime_seconds: number,
    last_error: string?,
    warnings: string[]
  }]
}
```

### WebSocket vs Polling

**For Logs**: WebSocket or SSE (real-time)
**For Metrics**: Poll every 1-2 seconds
**For Health**: Poll every 5 seconds

---

## Summary

**Choose Bottom Panel because:**

- ✅ Industry standard for runtime/debugging info
- ✅ Logs need horizontal space
- ✅ Doesn't interfere with config panel
- ✅ Simpler implementation
- ✅ Better keyboard shortcut pattern (Ctrl+`)
- ✅ Natural information hierarchy

**Implementation timeline:**

- Week 1: Basic panel structure, toggle, resize
- Week 2: Logs tab with real-time streaming
- Week 3: Metrics tab with polling
- Week 4: Health tab + polish

**Next steps:**

1. Confirm bottom panel approach
2. Design backend API endpoints for runtime data
3. Create RuntimePanel component
4. Implement basic toggle + resize
5. Add tabs one by one (Logs first)

The bottom panel approach gives us a professional, familiar debugging experience that matches user expectations from other development tools.
