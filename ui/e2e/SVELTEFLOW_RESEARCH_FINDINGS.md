# SvelteFlow Event System Research Findings

**Date**: October 19, 2025
**Context**: Understanding proper SvelteFlow usage for node persistence and upcoming visual wiring feature

## Key Discoveries

### 1. SvelteFlow Uses Svelte Bindings, Not React Flow Callbacks

**React Flow pattern** (callback-based):

```typescript
<ReactFlow
  nodes={nodes}
  onNodesChange={handleNodesChange}  // Callback for changes
/>
```

**SvelteFlow pattern** (binding-based):

```typescript
<SvelteFlow
  bind:nodes={nodes}  // Two-way binding
  bind:edges={edges}
  bind:viewport={viewport}
/>
```

**Source**: `node_modules/@xyflow/svelte/dist/lib/container/SvelteFlow/SvelteFlow.svelte.d.ts:102`

```typescript
bindings(): "nodes" | "edges" | "viewport";
```

### 2. Available SvelteFlow Events

From `node_modules/@xyflow/svelte/dist/lib/types/events.d.ts`:

**Node Events**:

- `onnodeclick` - User clicks a node
- `onnodecontextmenu` - User right-clicks a node
- `onnodedrag` - User drags a node
- `onnodedragstart` - User starts dragging
- `onnodedragstop` - User stops dragging (we use this!)
- `onnodepointerenter/leave/move` - Pointer interactions

**Edge Events**:

- `onedgeclick`, `onedgecontextmenu`
- `onedgepointerenter/leave`

**Connection Events** (for visual wiring):

- `onconnect` - Connection created
- `onconnectstart` - Connection drag started
- `onconnectend` - Connection drag ended
- `onbeforeconnect` - Validation before connection
- `onreconnect` - Connection moved to different handle

**Other Events**:

- `onselectionchange` - Selection changes
- `ondelete` - Nodes/edges deleted
- `onbeforedelete` - Before deletion (can prevent)
- `oninit` - Component initialized

**IMPORTANT**: No `onnodeschange` event like React Flow!

### 3. Our Current Architecture Issue

**FlowCanvas.svelte:24-41** - `nodes` is `$derived`:

```typescript
const nodes = $derived<Node[]>(
  flow?.nodes?.map((node) => ({
    id: node.id,
    type: 'custom',
    position: node.position,
    data: { ... }
  })) || []
);
```

**Problem**: `$derived` creates **read-only computed state**. Cannot use `bind:nodes` on it because SvelteFlow would need to write to it.

**FlowCanvas.svelte:138** - One-way prop:

```svelte
<SvelteFlow
  {nodes}  <!-- One-way prop, not bound -->
  {edges}
  onnodedragstop={handleNodeDragStop}
/>
```

**FlowCanvas.svelte:56-66** - Manual position updates:

```typescript
function handleNodeDragStop(event: any) {
  const { id, position } = event.detail;
  const updatedNodes = flow.nodes.map((node) =>
    node.id === id ? { ...node, position } : node,
  );
  onNodesChange?.(updatedNodes);
}
```

### 4. Why E2E Tests Fail But Manual Testing Works

**The drag-and-drop flow**:

1. User drags from ComponentPalette → fires HTML5 drag events
2. `handleDrop` in FlowCanvas catches the HTML5 `drop` event
3. Creates new FlowNode, calls `onNodesChange`
4. Parent updates `flow.nodes`, save succeeds

**Playwright's dragTo() behavior**:

1. Playwright fires **mouse events** (mousedown/mousemove/mouseup)
2. XYFlow's internal handlers respond to mouse events
3. XYFlow adds node to its **internal DOM state** (visual only)
4. Our `handleDrop` expects HTML5 `drop` event → **never fires**
5. `onNodesChange` never called, parent state not updated
6. Node appears visually but `flow.nodes` stays empty
7. Save sends empty array to backend

**Why config panel tests pass**:

1. XYFlow creates DOM element for the visual node
2. Test clicks the DOM element → `onnodeclick` fires
3. Config panel opens successfully
4. But saving the flow still fails (0 nodes)

### 5. Accessibility Violation

**Current**: Drag-and-drop is the ONLY way to add components.

**WCAG 2.1 Level A violation**:

- Keyboard-only users cannot add components
- Screen reader users cannot navigate/add components
- Motor impairment users struggle with precise drag

**Constitutional violation**: Reference implementations must be accessible.

## Recommended Solution: Multi-Modal Component Addition

### Approach: Add Keyboard + Double-Click Alternatives

**Benefits**:

1. ✅ Fixes WCAG 2.1 Level A violation
2. ✅ Makes E2E tests work (use keyboard/click instead of drag)
3. ✅ Simpler than managing bidirectional binding conversions
4. ✅ Better UX overall (users can choose preferred interaction)

**Implementation**:

#### Option 1: Double-Click on Palette Item

```svelte
<!-- ComponentPalette.svelte -->
<button
  ondblclick={() => onAddComponent(componentType)}
  ondragstart={handleDragStart}
>
  {componentType.name}
</button>
```

Adds component to **canvas center** (or last clicked position).

#### Option 2: Keyboard Support

```svelte
<button
  onkeydown={(e) => {
    if (e.key === 'Enter' || e.key === ' ') {
      e.preventDefault();
      onAddComponent(componentType);
    }
  }}
  ondragstart={handleDragStart}
>
  {componentType.name}
</button>
```

#### Option 3: Context Menu

```svelte
<button
  oncontextmenu={(e) => {
    e.preventDefault();
    showContextMenu(e, componentType);
  }}
>
  {componentType.name}
</button>
```

Context menu shows: "Add to Canvas", "View Documentation"

### E2E Test Pattern

**After fix**, tests can use:

```typescript
// Option 1: Double-click
await page.locator('[data-component="UDP Input"]').dblclick();

// Option 2: Keyboard
await page.locator('[data-component="UDP Input"]').press("Enter");

// Both add to canvas center, no drag needed!
```

## For Visual Wiring Feature (Next)

### Connection Events to Use

**onconnect** - When user creates edge:

```typescript
onconnect={(params) => {
  const newConnection = {
    id: generateConnectionId(),
    source_node_id: params.source,
    target_node_id: params.target,
    source_port: params.sourceHandle,
    target_port: params.targetHandle
  };
  onConnectionsChange?.([...flow.connections, newConnection]);
}}
```

**onbeforeconnect** - Validate before allowing:

```typescript
onbeforeconnect={(params) => {
  // Validate connection is valid per FlowGraph rules
  return isValidConnection(params.source, params.target);
}}
```

**ondelete** - When user deletes edges:

```typescript
ondelete={({ nodes, edges }) => {
  // Update flow.connections
  const remainingConnections = flow.connections.filter(
    conn => !edges.find(e => e.id === conn.id)
  );
  onConnectionsChange?.(remainingConnections);
}}
```

### Controlled Pattern for Wiring

Since we need to sync between FlowConnection[] and XYFlow Edge[]:

**Option A: Keep $derived + manual event handlers** (current approach)

```typescript
const edges = $derived<Edge[]>(
  flow.connections.map(conn => ({ ... }))
);

<SvelteFlow
  {edges}  <!-- One-way prop -->
  onconnect={handleConnect}
  ondelete={handleDelete}
/>
```

**Option B: Use $state with bidirectional sync**

```typescript
let xyflowEdges = $state<Edge[]>([]);

// Sync from flow to xyflow
$effect(() => {
  xyflowEdges = flow.connections.map(conn => ({ ... }));
});

// Watch xyflow changes and sync back
$effect(() => {
  const connections = xyflowEdges.map(edge => ({ ... }));
  onConnectionsChange?.(connections);
});

<SvelteFlow bind:edges={xyflowEdges} />
```

**Recommendation**: Stick with **Option A** (manual event handlers) because:

1. Clearer data flow (easier to debug)
2. Avoids circular update issues with $effect
3. More explicit about when state changes
4. Consistent with current architecture

## Summary

**For Node Persistence**:

- Add multi-modal component addition (double-click + keyboard)
- Fixes accessibility, E2E tests, and UX simultaneously
- Keep current event-driven architecture

**For Visual Wiring**:

- Use `onconnect`, `onbeforeconnect`, `ondelete` events
- Keep $derived edges with manual event handlers
- Validate connections with FlowGraph patterns

**No Need for**:

- Switching to `bind:` directive (causes circular dependency issues)
- Complex bidirectional state sync
- Changing fundamental architecture

## Files Referenced

- `frontend/node_modules/@xyflow/svelte/dist/lib/container/SvelteFlow/SvelteFlow.svelte.d.ts` - Component props
- `frontend/node_modules/@xyflow/svelte/dist/lib/types/events.d.ts` - Event types
- `frontend/src/lib/components/FlowCanvas.svelte` - Current implementation
- `frontend/e2e/drag-and-drop.spec.ts` - Playwright drag limitations documented
- `frontend/e2e/component-config.spec.ts` - Shows clicking works but not state
