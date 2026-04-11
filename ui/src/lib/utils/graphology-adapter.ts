/**
 * Graphology Adapter
 *
 * Syncs graphStore state (entities + relationships) into a graphology Graph
 * instance that Sigma.js consumes for rendering.
 *
 * The graphStore remains the source of truth. This adapter is a rendering bridge.
 */

import type Graph from "graphology";
import type { GraphEntity, GraphRelationship } from "$lib/types/graph";
import { getEntityColor, getPredicateColor } from "$lib/utils/entity-colors";

const DEFAULT_NODE_SIZE = 8;
const MIN_NODE_SIZE = 5;
const MAX_NODE_SIZE = 20;

/**
 * Calculate node size based on connection count.
 */
function getNodeSize(entity: GraphEntity): number {
  const connections = entity.outgoing.length + entity.incoming.length;
  const size = DEFAULT_NODE_SIZE + Math.sqrt(connections) * 2;
  return Math.min(Math.max(size, MIN_NODE_SIZE), MAX_NODE_SIZE);
}

/**
 * Full sync: clear the graphology graph and rebuild from store data.
 * Preserves existing node positions so the FA2 layout isn't lost on re-sync.
 */
export function syncStoreToGraph(
  graph: Graph,
  entities: GraphEntity[],
  relationships: GraphRelationship[],
): void {
  // Snapshot positions before clearing so FA2 layout is preserved
  const positions = new Map<string, { x: number; y: number }>();
  graph.forEachNode((id, attrs) => {
    positions.set(id, { x: attrs.x as number, y: attrs.y as number });
  });

  graph.clear();

  for (const entity of entities) {
    const existing = positions.get(entity.id);
    graph.addNode(entity.id, {
      label: entity.idParts.instance || entity.id,
      size: getNodeSize(entity),
      color: getEntityColor(entity.idParts),
      // "type" is reserved by Sigma as the WebGL program selector (only "circle"
      // is registered by default). Store entity type as a separate attribute.
      entityType: entity.idParts.type,
      domain: entity.idParts.domain,
      x: existing?.x ?? Math.random() * 100,
      y: existing?.y ?? Math.random() * 100,
    });
  }

  for (const rel of relationships) {
    if (graph.hasNode(rel.sourceId) && graph.hasNode(rel.targetId)) {
      const edgeKey = rel.id;
      if (!graph.hasEdge(edgeKey)) {
        graph.addEdgeWithKey(edgeKey, rel.sourceId, rel.targetId, {
          label: rel.predicate.split(".").pop() || rel.predicate,
          color: getPredicateColor(rel.predicate),
          size: Math.max(1, rel.confidence * 3),
          type: "arrow",
        });
      }
    }
  }
}

/**
 * Incremental add: add new nodes/edges without clearing existing ones.
 * Used for entity expansion.
 */
export function addToGraph(
  graph: Graph,
  entities: GraphEntity[],
  relationships: GraphRelationship[],
): void {
  for (const entity of entities) {
    if (!graph.hasNode(entity.id)) {
      // Position near existing neighbors if possible
      const { x, y } = getInitialPosition(graph, entity);
      graph.addNode(entity.id, {
        label: entity.idParts.instance || entity.id,
        size: getNodeSize(entity),
        color: getEntityColor(entity.idParts),
        entityType: entity.idParts.type,
        domain: entity.idParts.domain,
        x,
        y,
      });
    }
  }

  for (const rel of relationships) {
    if (
      graph.hasNode(rel.sourceId) &&
      graph.hasNode(rel.targetId) &&
      !graph.hasEdge(rel.id)
    ) {
      graph.addEdgeWithKey(rel.id, rel.sourceId, rel.targetId, {
        label: rel.predicate.split(".").pop() || rel.predicate,
        color: getPredicateColor(rel.predicate),
        size: Math.max(1, rel.confidence * 3),
        type: "arrow",
      });
    }
  }
}

/**
 * Get initial position for a new node, near its existing neighbors.
 */
function getInitialPosition(
  graph: Graph,
  entity: GraphEntity,
): { x: number; y: number } {
  const neighborIds = [
    ...entity.outgoing.map((r) => r.targetId),
    ...entity.incoming.map((r) => r.sourceId),
  ];

  let sumX = 0;
  let sumY = 0;
  let count = 0;

  for (const nId of neighborIds) {
    if (graph.hasNode(nId)) {
      const attrs = graph.getNodeAttributes(nId);
      sumX += attrs.x as number;
      sumY += attrs.y as number;
      count++;
    }
  }

  if (count > 0) {
    // Place near neighbors with jitter
    return {
      x: sumX / count + (Math.random() - 0.5) * 20,
      y: sumY / count + (Math.random() - 0.5) * 20,
    };
  }

  return { x: Math.random() * 100, y: Math.random() * 100 };
}
