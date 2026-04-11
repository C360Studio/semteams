/**
 * D3-based layout utilities for flow and graph visualization
 *
 * Provides hierarchical left-to-right layout for:
 * - Flow nodes (rectangular, with ports)
 * - Graph entities (circular, for knowledge graphs)
 *
 * Uses topological sort for column assignment, ensuring no edge crossings.
 */

import * as d3 from "d3";
import type { FlowNode, FlowConnection } from "$lib/types/flow";
import type {
  GraphEntity,
  GraphLayoutNode,
  GraphLayoutEdge,
  GraphRelationship,
} from "$lib/types/graph";
import { getEntityColor } from "$lib/utils/entity-colors";

/** Node with computed layout position */
export interface LayoutNode {
  id: string;
  component: string;
  type: string;
  name: string;
  x: number;
  y: number;
  width: number;
  height: number;
  config: Record<string, unknown>;
  /** Original node data */
  original: FlowNode;
}

/** Edge with computed path data */
export interface LayoutEdge {
  id: string;
  sourceNodeId: string;
  sourcePort: string;
  targetNodeId: string;
  targetPort: string;
  /** Source position (output port) */
  sourceX: number;
  sourceY: number;
  /** Target position (input port) */
  targetX: number;
  targetY: number;
  /** Original connection data */
  original: FlowConnection;
}

/** Layout configuration */
export interface LayoutConfig {
  /** Node dimensions */
  nodeWidth: number;
  nodeHeight: number;
  /** Spacing between nodes */
  horizontalSpacing: number;
  verticalSpacing: number;
  /** Canvas padding */
  padding: number;
}

const DEFAULT_CONFIG: LayoutConfig = {
  nodeWidth: 200,
  nodeHeight: 80,
  horizontalSpacing: 100,
  verticalSpacing: 60,
  padding: 50,
};

/**
 * Calculate layout positions for flow nodes
 *
 * Uses a simple left-to-right hierarchical layout based on connections.
 * Nodes with no incoming connections are placed in the first column,
 * and subsequent nodes are placed based on their dependencies.
 */
export function layoutNodes(
  nodes: FlowNode[],
  connections: FlowConnection[],
  config: Partial<LayoutConfig> = {},
): LayoutNode[] {
  const cfg = { ...DEFAULT_CONFIG, ...config };

  if (nodes.length === 0) {
    return [];
  }

  // Build adjacency info
  const incomingEdges = new Map<string, string[]>();
  const outgoingEdges = new Map<string, string[]>();

  for (const node of nodes) {
    incomingEdges.set(node.id, []);
    outgoingEdges.set(node.id, []);
  }

  for (const conn of connections) {
    const incoming = incomingEdges.get(conn.target_node_id);
    if (incoming) {
      incoming.push(conn.source_node_id);
    }
    const outgoing = outgoingEdges.get(conn.source_node_id);
    if (outgoing) {
      outgoing.push(conn.target_node_id);
    }
  }

  // Assign columns using topological sort
  const columns = new Map<string, number>();
  const visited = new Set<string>();

  function assignColumn(nodeId: string): number {
    if (columns.has(nodeId)) {
      return columns.get(nodeId)!;
    }

    if (visited.has(nodeId)) {
      // Cycle detected, assign to column 0
      return 0;
    }

    visited.add(nodeId);
    const incoming = incomingEdges.get(nodeId) || [];

    if (incoming.length === 0) {
      columns.set(nodeId, 0);
      return 0;
    }

    const maxParentCol = Math.max(...incoming.map((id) => assignColumn(id)));
    const col = maxParentCol + 1;
    columns.set(nodeId, col);
    return col;
  }

  for (const node of nodes) {
    assignColumn(node.id);
  }

  // Group nodes by column
  const columnGroups = new Map<number, FlowNode[]>();
  for (const node of nodes) {
    const col = columns.get(node.id) || 0;
    if (!columnGroups.has(col)) {
      columnGroups.set(col, []);
    }
    columnGroups.get(col)!.push(node);
  }

  // Calculate positions
  const layoutNodes: LayoutNode[] = [];

  for (const [col, colNodes] of columnGroups) {
    const x = cfg.padding + col * (cfg.nodeWidth + cfg.horizontalSpacing);

    colNodes.forEach((node, rowIndex) => {
      const y = cfg.padding + rowIndex * (cfg.nodeHeight + cfg.verticalSpacing);

      layoutNodes.push({
        id: node.id,
        component: node.component,
        type: node.type,
        name: node.name,
        x,
        y,
        width: cfg.nodeWidth,
        height: cfg.nodeHeight,
        config: node.config,
        original: node,
      });
    });
  }

  return layoutNodes;
}

/**
 * Calculate edge positions based on node layout
 */
export function layoutEdges(
  connections: FlowConnection[],
  layoutNodes: LayoutNode[],
): LayoutEdge[] {
  const nodeMap = new Map<string, LayoutNode>();
  for (const node of layoutNodes) {
    nodeMap.set(node.id, node);
  }

  return connections
    .map((conn) => {
      const sourceNode = nodeMap.get(conn.source_node_id);
      const targetNode = nodeMap.get(conn.target_node_id);

      if (!sourceNode || !targetNode) {
        return null;
      }

      // Source: right side of source node
      const sourceX = sourceNode.x + sourceNode.width;
      const sourceY = sourceNode.y + sourceNode.height / 2;

      // Target: left side of target node
      const targetX = targetNode.x;
      const targetY = targetNode.y + targetNode.height / 2;

      return {
        id: conn.id,
        sourceNodeId: conn.source_node_id,
        sourcePort: conn.source_port,
        targetNodeId: conn.target_node_id,
        targetPort: conn.target_port,
        sourceX,
        sourceY,
        targetX,
        targetY,
        original: conn,
      };
    })
    .filter((edge): edge is LayoutEdge => edge !== null);
}

/**
 * Calculate canvas dimensions to fit all nodes
 */
export function calculateCanvasBounds(
  layoutNodes: LayoutNode[],
  padding: number = 50,
): { width: number; height: number } {
  if (layoutNodes.length === 0) {
    return { width: 800, height: 600 };
  }

  let maxX = 0;
  let maxY = 0;

  for (const node of layoutNodes) {
    maxX = Math.max(maxX, node.x + node.width);
    maxY = Math.max(maxY, node.y + node.height);
  }

  return {
    width: maxX + padding,
    height: maxY + padding,
  };
}

/**
 * Create a D3 zoom behavior for the canvas
 *
 * Returns a zoom behavior that can be applied to an SVG element.
 * The transform is provided via callback for Svelte reactivity.
 */
export function createZoomBehavior(
  onTransform: (transform: d3.ZoomTransform) => void,
  options: {
    minZoom?: number;
    maxZoom?: number;
  } = {},
): d3.ZoomBehavior<SVGSVGElement, unknown> {
  const { minZoom = 0.1, maxZoom = 2 } = options;

  return d3
    .zoom<SVGSVGElement, unknown>()
    .scaleExtent([minZoom, maxZoom])
    .on("zoom", (event) => {
      onTransform(event.transform);
    });
}

/**
 * Apply zoom behavior to an SVG element
 */
export function applyZoom(
  svgElement: SVGSVGElement,
  zoomBehavior: d3.ZoomBehavior<SVGSVGElement, unknown>,
): void {
  d3.select(svgElement).call(zoomBehavior);
}

/**
 * Reset zoom to fit content
 */
export function fitToContent(
  svgElement: SVGSVGElement,
  zoomBehavior: d3.ZoomBehavior<SVGSVGElement, unknown>,
  bounds: { width: number; height: number },
  viewportWidth: number,
  viewportHeight: number,
  padding: number = 50,
): void {
  const scale = Math.min(
    (viewportWidth - padding * 2) / bounds.width,
    (viewportHeight - padding * 2) / bounds.height,
    1, // Don't zoom in beyond 100%
  );

  const translateX = (viewportWidth - bounds.width * scale) / 2;
  const translateY = (viewportHeight - bounds.height * scale) / 2;

  const transform = d3.zoomIdentity
    .translate(translateX, translateY)
    .scale(scale);
  const selection = d3.select(svgElement);

  // Check if we're in a test environment where transitions don't work
  // In jsdom, transition().duration is not a function
  const transition = selection.transition();
  if (typeof transition.duration === "function") {
    // Browser environment - use animated transition
    transition.duration(300).call(zoomBehavior.transform, transform);
  } else {
    // Test environment - apply transform directly without animation
    selection.call(zoomBehavior.transform, transform);
  }
}

// =============================================================================
// Graph Layout (Knowledge Graph Entities)
// =============================================================================

/** Configuration for graph entity layout */
export interface GraphLayoutConfig {
  /** Node width (rectangular nodes) */
  nodeWidth: number;
  /** Node height (rectangular nodes) */
  nodeHeight: number;
  /** Spacing between nodes horizontally */
  horizontalSpacing: number;
  /** Spacing between nodes vertically */
  verticalSpacing: number;
  /** Canvas padding */
  padding: number;
}

const DEFAULT_GRAPH_CONFIG: GraphLayoutConfig = {
  nodeWidth: 180,
  nodeHeight: 60,
  horizontalSpacing: 80,
  verticalSpacing: 40,
  padding: 60,
};

/**
 * Calculate node dimensions based on entity importance.
 * More connected entities get slightly larger nodes.
 */
function calculateNodeDimensions(
  entity: GraphEntity,
  baseWidth: number,
  baseHeight: number,
): { width: number; height: number } {
  const connectionCount = entity.outgoing.length + entity.incoming.length;
  // Scale factor grows with connections (capped at 1.2x base)
  const scale = Math.min(1 + Math.sqrt(connectionCount) * 0.05, 1.2);
  return {
    width: Math.round(baseWidth * scale),
    height: Math.round(baseHeight * scale),
  };
}

/**
 * Barycenter crossing minimization (Sugiyama Phase 2).
 *
 * Reorders nodes within each column to minimize edge crossings by:
 * 1. Sweeping left-to-right: order nodes by average position of their incoming neighbors
 * 2. Sweeping right-to-left: order nodes by average position of their outgoing neighbors
 * 3. Repeating for several iterations to converge on a good ordering
 *
 * @param columnGroups - Map of column index to entities in that column (modified in place)
 * @param incomingEdges - Map of entity ID to array of source entity IDs
 * @param outgoingEdges - Map of entity ID to array of target entity IDs
 * @param iterations - Number of barycenter sweeps (default 4)
 */
function orderNodesWithinColumns(
  columnGroups: Map<number, GraphEntity[]>,
  incomingEdges: Map<string, string[]>,
  outgoingEdges: Map<string, string[]>,
  iterations: number = 4,
): void {
  const columns = Array.from(columnGroups.keys()).sort((a, b) => a - b);

  if (columns.length < 2) {
    return; // Nothing to optimize with 0 or 1 columns
  }

  // Track barycenter values for sorting
  const barycenters = new Map<string, number>();

  for (let iter = 0; iter < iterations; iter++) {
    // Sweep left-to-right (use incoming edges)
    for (let i = 1; i < columns.length; i++) {
      const col = columns[i];
      const prevCol = columns[i - 1];
      const prevNodes = columnGroups.get(prevCol) || [];
      const prevPositions = new Map(prevNodes.map((n, idx) => [n.id, idx]));

      const nodes = columnGroups.get(col) || [];
      for (const node of nodes) {
        const parents = incomingEdges.get(node.id) || [];
        const positions = parents
          .map((p) => prevPositions.get(p))
          .filter((p): p is number => p !== undefined);

        if (positions.length > 0) {
          barycenters.set(
            node.id,
            positions.reduce((a, b) => a + b, 0) / positions.length,
          );
        } else {
          // No incoming edges from previous column - keep current position
          barycenters.set(node.id, nodes.indexOf(node));
        }
      }
      nodes.sort(
        (a, b) => (barycenters.get(a.id) ?? 0) - (barycenters.get(b.id) ?? 0),
      );
    }

    // Sweep right-to-left (use outgoing edges)
    for (let i = columns.length - 2; i >= 0; i--) {
      const col = columns[i];
      const nextCol = columns[i + 1];
      const nextNodes = columnGroups.get(nextCol) || [];
      const nextPositions = new Map(nextNodes.map((n, idx) => [n.id, idx]));

      const nodes = columnGroups.get(col) || [];
      for (const node of nodes) {
        const children = outgoingEdges.get(node.id) || [];
        const positions = children
          .map((c) => nextPositions.get(c))
          .filter((p): p is number => p !== undefined);

        if (positions.length > 0) {
          barycenters.set(
            node.id,
            positions.reduce((a, b) => a + b, 0) / positions.length,
          );
        } else {
          // No outgoing edges to next column - keep current position
          barycenters.set(node.id, nodes.indexOf(node));
        }
      }
      nodes.sort(
        (a, b) => (barycenters.get(a.id) ?? 0) - (barycenters.get(b.id) ?? 0),
      );
    }
  }
}

/**
 * Layout graph entities using hierarchical left-to-right layout.
 *
 * Uses the same topological sort algorithm as flow layout:
 * - Nodes with no incoming relationships are placed in column 0
 * - Subsequent nodes are placed in columns based on their dependencies
 * - Nodes are stacked vertically within each column
 */
export function layoutGraphEntities(
  entities: GraphEntity[],
  config: Partial<GraphLayoutConfig> = {},
): GraphLayoutNode[] {
  const cfg = { ...DEFAULT_GRAPH_CONFIG, ...config };

  if (entities.length === 0) {
    return [];
  }

  // Build adjacency info from entity relationships
  const incomingEdges = new Map<string, string[]>();
  const outgoingEdges = new Map<string, string[]>();

  for (const entity of entities) {
    incomingEdges.set(entity.id, []);
    outgoingEdges.set(entity.id, []);
  }

  // Build edges from entity relationships
  for (const entity of entities) {
    for (const rel of entity.outgoing) {
      const outgoing = outgoingEdges.get(entity.id);
      if (outgoing) {
        outgoing.push(rel.targetId);
      }
      const incoming = incomingEdges.get(rel.targetId);
      if (incoming) {
        incoming.push(entity.id);
      }
    }
  }

  // Assign columns using topological sort
  const columns = new Map<string, number>();
  const visited = new Set<string>();

  function assignColumn(entityId: string): number {
    if (columns.has(entityId)) {
      return columns.get(entityId)!;
    }

    if (visited.has(entityId)) {
      // Cycle detected, assign to column 0
      return 0;
    }

    visited.add(entityId);
    const incoming = incomingEdges.get(entityId) || [];

    if (incoming.length === 0) {
      columns.set(entityId, 0);
      return 0;
    }

    const maxParentCol = Math.max(...incoming.map((id) => assignColumn(id)));
    const col = maxParentCol + 1;
    columns.set(entityId, col);
    return col;
  }

  for (const entity of entities) {
    assignColumn(entity.id);
  }

  // Group entities by column
  const columnGroups = new Map<number, GraphEntity[]>();
  for (const entity of entities) {
    const col = columns.get(entity.id) || 0;
    if (!columnGroups.has(col)) {
      columnGroups.set(col, []);
    }
    columnGroups.get(col)!.push(entity);
  }

  // Apply barycenter crossing minimization to reorder nodes within columns
  orderNodesWithinColumns(columnGroups, incomingEdges, outgoingEdges);

  // Calculate positions
  const layoutNodes: GraphLayoutNode[] = [];
  const nodeSpacing = cfg.nodeHeight + cfg.verticalSpacing;
  const columnSpacing = cfg.nodeWidth + cfg.horizontalSpacing;

  for (const [col, colEntities] of columnGroups) {
    const x = cfg.padding + col * columnSpacing;

    colEntities.forEach((entity, rowIndex) => {
      const y = cfg.padding + rowIndex * nodeSpacing;
      const { width, height } = calculateNodeDimensions(
        entity,
        cfg.nodeWidth,
        cfg.nodeHeight,
      );
      // Radius used for bounds calculation (half of diagonal)
      const radius = Math.max(width, height) / 2;

      layoutNodes.push({
        id: entity.id,
        entity,
        x,
        y,
        width,
        height,
        radius,
        color: getEntityColor(entity.idParts),
        communityColor: entity.community?.color,
        expanded: false,
        pinned: false,
      });
    });
  }

  return layoutNodes;
}

/**
 * Layout graph edges based on node positions.
 *
 * Edges connect from right side of source node to left side of target node.
 */
export function layoutGraphEdges(
  relationships: GraphRelationship[],
  layoutNodes: GraphLayoutNode[],
): GraphLayoutEdge[] {
  const nodeMap = new Map<string, GraphLayoutNode>();
  for (const node of layoutNodes) {
    nodeMap.set(node.id, node);
  }

  return relationships
    .map((rel) => {
      const sourceNode = nodeMap.get(rel.sourceId);
      const targetNode = nodeMap.get(rel.targetId);

      if (!sourceNode || !targetNode) {
        return null;
      }

      return {
        id: rel.id,
        relationship: rel,
        source: sourceNode,
        target: targetNode,
        opacity: rel.confidence,
      };
    })
    .filter((edge): edge is GraphLayoutEdge => edge !== null);
}

/**
 * Calculate bounds for graph layout
 */
export function calculateGraphBounds(
  layoutNodes: GraphLayoutNode[],
  padding: number = 50,
): {
  minX: number;
  minY: number;
  maxX: number;
  maxY: number;
  width: number;
  height: number;
} {
  if (layoutNodes.length === 0) {
    return { minX: 0, minY: 0, maxX: 800, maxY: 600, width: 800, height: 600 };
  }

  let minX = Infinity,
    minY = Infinity;
  let maxX = -Infinity,
    maxY = -Infinity;

  for (const node of layoutNodes) {
    // Use width/height for rectangle-based nodes
    minX = Math.min(minX, node.x);
    minY = Math.min(minY, node.y);
    maxX = Math.max(maxX, node.x + node.width);
    maxY = Math.max(maxY, node.y + node.height);
  }

  return {
    minX: minX - padding,
    minY: minY - padding,
    maxX: maxX + padding,
    maxY: maxY + padding,
    width: maxX - minX + padding * 2,
    height: maxY - minY + padding * 2,
  };
}
