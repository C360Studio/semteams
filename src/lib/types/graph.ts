/**
 * Knowledge Graph Types
 *
 * Types for visualizing the semantic knowledge graph built by semstreams.
 * The graph uses RDF-like triples (Subject-Predicate-Object) where:
 * - Entities have 6-part IDs: org.platform.domain.system.type.instance
 * - Relationships are triples where the object references another entity
 * - Properties are triples where the object is a literal value
 */

// =============================================================================
// Entity Types
// =============================================================================

/**
 * Parsed components of a 6-part entity ID.
 * Format: org.platform.domain.system.type.instance
 * Example: "c360.ops.robotics.gcs.drone.001"
 */
export interface EntityIdParts {
  org: string;
  platform: string;
  domain: string;
  system: string;
  type: string;
  instance: string;
}

/**
 * A triple property representing a fact about an entity.
 * When object is a literal value, it's a property.
 * When object is another entity ID, it becomes a relationship.
 */
export interface TripleProperty {
  predicate: string; // 3-part dotted notation: domain.category.property
  object: unknown; // Literal value (number, string, boolean) or entity ID reference
  confidence: number; // 0.0 - 1.0
  source: string; // Origin component that created this fact
  timestamp: number; // Unix milliseconds
}

/**
 * A relationship between two entities (edge in the graph).
 * Created from triples where the object references another entity.
 */
export interface GraphRelationship {
  id: string; // Unique relationship ID (source:predicate:target)
  sourceId: string; // Source entity ID
  targetId: string; // Target entity ID
  predicate: string; // Relationship type (e.g., "fleet.membership.current")
  confidence: number; // 0.0 - 1.0
  timestamp: number; // Unix milliseconds
}

/**
 * Community cluster that entities can belong to.
 * Detected by graph-clustering component using LPA algorithm.
 */
export interface GraphCommunity {
  id: string;
  label?: string; // LLM-generated summary (if T2 enabled)
  memberCount: number;
  color: string; // Assigned visualization color
}

/**
 * A graph entity (node in the graph).
 */
export interface GraphEntity {
  id: string; // Full 6-part entity ID
  idParts: EntityIdParts; // Parsed ID components
  properties: TripleProperty[]; // Literal-valued triples
  outgoing: GraphRelationship[]; // Relationships where this entity is source
  incoming: GraphRelationship[]; // Relationships where this entity is target
  community?: GraphCommunity; // Community membership (if detected)
}

// =============================================================================
// Layout Types (Hierarchical Layout)
// =============================================================================

/**
 * Node for graph visualization layout.
 * Contains entity data and computed visual properties.
 */
export interface GraphLayoutNode {
  id: string;
  entity: GraphEntity;
  x: number;
  y: number;
  width: number; // Node width (rectangle)
  height: number; // Node height (rectangle)
  radius: number; // Equivalent radius for bounds calculation (max of width/height / 2)
  color: string; // Accent color (by type)
  communityColor?: string; // Border color (by community)
  expanded: boolean; // Whether neighbors have been loaded
  pinned: boolean; // Whether position is fixed by user
}

/**
 * Edge for graph visualization layout.
 * Contains relationship data and references to source/target nodes.
 */
export interface GraphLayoutEdge {
  id: string;
  relationship: GraphRelationship;
  source: GraphLayoutNode;
  target: GraphLayoutNode;
  opacity: number; // Based on confidence (0.0 - 1.0)
}

// =============================================================================
// Filter Types
// =============================================================================

/**
 * Filters for the graph visualization.
 */
export interface GraphFilters {
  search: string; // Entity ID/name search
  types: string[]; // Entity types to show (from 6-part ID)
  domains: string[]; // Domains to show (from 6-part ID)
  minConfidence: number; // Hide edges below this confidence (0.0 - 1.0)
  timeRange: [number, number] | null; // Unix ms range, null = all time
  communities: string[]; // Community IDs to highlight
  showProperties: boolean; // Show property values in tooltips
}

/**
 * Default filter values.
 */
export const DEFAULT_GRAPH_FILTERS: GraphFilters = {
  search: "",
  types: [],
  domains: [],
  minConfidence: 0,
  timeRange: null,
  communities: [],
  showProperties: true,
};

// =============================================================================
// Store State Types
// =============================================================================

/**
 * State for the graph store.
 */
export interface GraphStoreState {
  // Data
  entities: Map<string, GraphEntity>;
  relationships: Map<string, GraphRelationship>;
  communities: Map<string, GraphCommunity>;

  // Selection
  selectedEntityId: string | null;
  hoveredEntityId: string | null;
  expandedEntityIds: Set<string>;

  // Filters
  filters: GraphFilters;

  // Loading state
  loading: boolean;
  error: string | null;

  // Connection state
  connected: boolean;
}

/**
 * Default store state.
 */
export const DEFAULT_GRAPH_STORE_STATE: GraphStoreState = {
  entities: new Map(),
  relationships: new Map(),
  communities: new Map(),
  selectedEntityId: null,
  hoveredEntityId: null,
  expandedEntityIds: new Set(),
  filters: DEFAULT_GRAPH_FILTERS,
  loading: false,
  error: null,
  connected: false,
};

// =============================================================================
// API Response Types
// =============================================================================

/**
 * Backend triple structure (raw from GraphQL API).
 */
export interface BackendTriple {
  subject: string;
  predicate: string;
  object: unknown;
}

/**
 * Backend entity structure (raw from GraphQL API).
 */
export interface BackendEntity {
  id: string;
  triples: BackendTriple[];
}

/**
 * Backend edge structure (raw from GraphQL API).
 */
export interface BackendEdge {
  subject: string;
  predicate: string;
  object: string;
}

/**
 * GraphQL pathSearch query result.
 */
export interface PathSearchResult {
  entities: BackendEntity[];
  edges: BackendEdge[];
}

/**
 * Community summary returned by a global (NLQ) search.
 */
export interface CommunitySummary {
  communityId: string;
  text: string;
  keywords: string[];
}

/**
 * Explicit relationship returned by a global (NLQ) search.
 */
export interface SearchRelationship {
  from: string;
  to: string;
  predicate: string;
}

/**
 * Parsed result from the globalSearch GraphQL operation.
 */
export interface GlobalSearchResult {
  entities: BackendEntity[];
  communitySummaries: CommunitySummary[];
  relationships: SearchRelationship[];
  count: number;
  durationMs: number;
  classification?: ClassificationMeta;
}

/**
 * Whether search results replace or merge with existing graph data.
 */
export type SearchMode = "replace" | "merge";

/**
 * NLQ classification metadata returned via GraphQL extensions.
 * Available in semstreams alpha.17+.
 */
export interface ClassificationMeta {
  tier: number;
  confidence: number;
  intent: string;
}

/**
 * GraphQL response for entity query.
 */
export interface EntityQueryResponse {
  entity: {
    id: string;
    properties: Array<{
      predicate: string;
      object: unknown;
      confidence: number;
      timestamp: string;
    }>;
    outgoing: Array<{
      predicate: string;
      targetId: string;
      confidence: number;
    }>;
    incoming: Array<{
      predicate: string;
      sourceId: string;
      confidence: number;
    }>;
    community?: {
      id: string;
      label?: string;
    };
  };
}

/**
 * GraphQL response for entities list query.
 */
export interface EntitiesQueryResponse {
  entities: EntityQueryResponse["entity"][];
}

// =============================================================================
// Utility Functions
// =============================================================================

/**
 * Parse a 6-part entity ID into its components.
 */
export function parseEntityId(id: string): EntityIdParts {
  const parts = id.split(".");
  if (parts.length !== 6) {
    // Return partial/unknown for malformed IDs
    return {
      org: parts[0] || "unknown",
      platform: parts[1] || "unknown",
      domain: parts[2] || "unknown",
      system: parts[3] || "unknown",
      type: parts[4] || "unknown",
      instance: parts[5] || "unknown",
    };
  }
  return {
    org: parts[0],
    platform: parts[1],
    domain: parts[2],
    system: parts[3],
    type: parts[4],
    instance: parts[5],
  };
}

/**
 * Generate a unique relationship ID.
 */
export function createRelationshipId(
  sourceId: string,
  predicate: string,
  targetId: string,
): string {
  return `${sourceId}:${predicate}:${targetId}`;
}

/**
 * Check if a triple's object is an entity reference (vs literal value).
 */
export function isEntityReference(object: unknown): object is string {
  if (typeof object !== "string") return false;
  // Entity IDs have 6 dot-separated parts
  const parts = object.split(".");
  return parts.length === 6;
}

/**
 * Get display label for an entity (uses instance part of ID).
 */
export function getEntityLabel(entity: GraphEntity): string {
  return entity.idParts.instance || entity.id;
}

/**
 * Get short type label for an entity.
 */
export function getEntityTypeLabel(entity: GraphEntity): string {
  return entity.idParts.type || "unknown";
}
