/**
 * Knowledge Graph Store
 *
 * Manages state for the graph visualization tab.
 * Handles entities, relationships, communities, selection, and filters.
 *
 * Uses Svelte 5 runes ($state) with SvelteMap/SvelteSet for deep reactivity.
 * Consumers read state directly via getters — no .subscribe() needed.
 */

import { SvelteDate, SvelteMap, SvelteSet } from "svelte/reactivity";
import {
  type GraphEntity,
  type GraphRelationship,
  type GraphCommunity,
  type GraphFilters,
  DEFAULT_GRAPH_FILTERS,
  parseEntityId,
  createRelationshipId,
  type TripleProperty,
} from "$lib/types/graph";

// =============================================================================
// Store Implementation
// =============================================================================

function createGraphStore() {
  // ---------------------------------------------------------------------------
  // Reactive state — use SvelteMap/SvelteSet for deep reactivity on
  // collection mutations without needing to replace the entire reference.
  // ---------------------------------------------------------------------------
  let entities = new SvelteMap<string, GraphEntity>();
  let relationships = new SvelteMap<string, GraphRelationship>();
  let communities = new SvelteMap<string, GraphCommunity>();
  let selectedEntityId = $state<string | null>(null);
  let hoveredEntityId = $state<string | null>(null);
  let expandedEntityIds = new SvelteSet<string>();
  let filters = $state<GraphFilters>({ ...DEFAULT_GRAPH_FILTERS });
  let loading = $state(false);
  let error = $state<string | null>(null);
  let connected = $state(false);

  return {
    // -------------------------------------------------------------------------
    // State getters — consumers read these directly, no .subscribe() needed
    // -------------------------------------------------------------------------

    get entities() {
      return entities;
    },
    get relationships() {
      return relationships;
    },
    get communities() {
      return communities;
    },
    get selectedEntityId() {
      return selectedEntityId;
    },
    get hoveredEntityId() {
      return hoveredEntityId;
    },
    get expandedEntityIds() {
      return expandedEntityIds;
    },
    get filters() {
      return filters;
    },
    get loading() {
      return loading;
    },
    get error() {
      return error;
    },
    get connected() {
      return connected;
    },

    // ========================================================================
    // Connection State
    // ========================================================================

    setConnected(value: boolean) {
      connected = value;
      if (value) error = null;
    },

    setLoading(value: boolean) {
      loading = value;
    },

    setError(value: string | null) {
      error = value;
      // Clearing the error (null) at the start of a fetch should not affect
      // loading state — callers manage that separately. Only a real error
      // implies loading has ended.
      if (value !== null) {
        loading = false;
      }
    },

    // ========================================================================
    // Entity Management
    // ========================================================================

    /**
     * Add or update a single entity.
     */
    upsertEntity(entity: GraphEntity) {
      entities.set(entity.id, entity);

      for (const rel of entity.outgoing) {
        relationships.set(rel.id, rel);
      }
      for (const rel of entity.incoming) {
        relationships.set(rel.id, rel);
      }
    },

    /**
     * Add or update multiple entities at once.
     */
    upsertEntities(newEntities: GraphEntity[]) {
      for (const entity of newEntities) {
        entities.set(entity.id, entity);

        for (const rel of entity.outgoing) {
          relationships.set(rel.id, rel);
        }
        for (const rel of entity.incoming) {
          relationships.set(rel.id, rel);
        }
      }
    },

    /**
     * Remove an entity and its relationships.
     */
    removeEntity(entityId: string) {
      entities.delete(entityId);

      for (const [relId, rel] of relationships) {
        if (rel.sourceId === entityId || rel.targetId === entityId) {
          relationships.delete(relId);
        }
      }

      if (selectedEntityId === entityId) {
        selectedEntityId = null;
      }
    },

    /**
     * Clear all entities and relationships.
     */
    clearEntities() {
      entities.clear();
      relationships.clear();
      selectedEntityId = null;
      hoveredEntityId = null;
      expandedEntityIds.clear();
    },

    // ========================================================================
    // Community Management
    // ========================================================================

    /**
     * Update communities data.
     */
    updateCommunities(newCommunities: GraphCommunity[]) {
      communities.clear();
      for (const community of newCommunities) {
        communities.set(community.id, community);
      }
    },

    // ========================================================================
    // Selection State
    // ========================================================================

    /**
     * Select an entity.
     */
    selectEntity(entityId: string | null) {
      selectedEntityId = entityId;
    },

    /**
     * Set hovered entity (for highlighting).
     */
    setHoveredEntity(entityId: string | null) {
      hoveredEntityId = entityId;
    },

    /**
     * Mark an entity as expanded (neighbors loaded).
     */
    markExpanded(entityId: string) {
      expandedEntityIds.add(entityId);
    },

    /**
     * Check if an entity has been expanded.
     */
    isExpanded(entityId: string): boolean {
      return expandedEntityIds.has(entityId);
    },

    /**
     * Clear only the expanded entity tracking set.
     * Used on DataView mount to reset expansion state without clearing entities.
     */
    clearExpanded() {
      expandedEntityIds.clear();
    },

    // ========================================================================
    // Filters
    // ========================================================================

    /**
     * Update filters (merges partial update into existing filters).
     */
    setFilters(newFilters: Partial<GraphFilters>) {
      filters = { ...filters, ...newFilters };
    },

    /**
     * Reset filters to defaults.
     */
    resetFilters() {
      filters = { ...DEFAULT_GRAPH_FILTERS };
    },

    // ========================================================================
    // Derived Data Helpers
    // ========================================================================

    /**
     * Get filtered entities based on current filters.
     */
    getFilteredEntities(): GraphEntity[] {
      let result = Array.from(entities.values());

      // Search filter
      if (filters.search) {
        const searchLower = filters.search.toLowerCase();
        result = result.filter(
          (e) =>
            e.id.toLowerCase().includes(searchLower) ||
            e.idParts.instance.toLowerCase().includes(searchLower) ||
            e.idParts.type.toLowerCase().includes(searchLower),
        );
      }

      // Type filter
      if (filters.types.length > 0) {
        result = result.filter((e) => filters.types.includes(e.idParts.type));
      }

      // Domain filter
      if (filters.domains.length > 0) {
        result = result.filter((e) =>
          filters.domains.includes(e.idParts.domain),
        );
      }

      // Time range filter
      if (filters.timeRange) {
        const [start, end] = filters.timeRange;
        result = result.filter((e) => {
          return e.properties.some(
            (p) => p.timestamp >= start && p.timestamp <= end,
          );
        });
      }

      return result;
    },

    /**
     * Get filtered relationships based on current filters.
     */
    getFilteredRelationships(): GraphRelationship[] {
      const visibleEntityIds = new SvelteSet(
        this.getFilteredEntities().map((e) => e.id),
      );

      let result = Array.from(relationships.values()).filter(
        (r) =>
          visibleEntityIds.has(r.sourceId) && visibleEntityIds.has(r.targetId),
      );

      // Confidence filter
      if (filters.minConfidence > 0) {
        result = result.filter((r) => r.confidence >= filters.minConfidence);
      }

      // Time range filter
      if (filters.timeRange) {
        const [start, end] = filters.timeRange;
        result = result.filter(
          (r) => r.timestamp >= start && r.timestamp <= end,
        );
      }

      return result;
    },

    /**
     * Get unique entity types from current data.
     */
    getEntityTypes(): string[] {
      const types = new SvelteSet<string>();
      for (const entity of entities.values()) {
        types.add(entity.idParts.type);
      }
      return Array.from(types).sort();
    },

    /**
     * Get unique domains from current data.
     */
    getDomains(): string[] {
      const domains = new SvelteSet<string>();
      for (const entity of entities.values()) {
        domains.add(entity.idParts.domain);
      }
      return Array.from(domains).sort();
    },

    // ========================================================================
    // Reset
    // ========================================================================

    /**
     * Reset store to initial state.
     */
    reset() {
      entities.clear();
      relationships.clear();
      communities.clear();
      selectedEntityId = null;
      hoveredEntityId = null;
      expandedEntityIds.clear();
      filters = { ...DEFAULT_GRAPH_FILTERS };
      loading = false;
      error = null;
      connected = false;
    },
  };
}

// =============================================================================
// Entity Builder Helpers
// =============================================================================

/**
 * Build a GraphEntity from API response data.
 */
export function buildGraphEntity(data: {
  id: string;
  properties?: Array<{
    predicate: string;
    object: unknown;
    confidence: number;
    source?: string;
    timestamp: string | number;
  }>;
  outgoing?: Array<{
    predicate: string;
    targetId: string;
    confidence: number;
    timestamp?: string | number;
  }>;
  incoming?: Array<{
    predicate: string;
    sourceId: string;
    confidence: number;
    timestamp?: string | number;
  }>;
  community?: {
    id: string;
    label?: string;
    memberCount?: number;
  };
}): GraphEntity {
  const idParts = parseEntityId(data.id);

  const properties: TripleProperty[] = (data.properties || []).map((p) => ({
    predicate: p.predicate,
    object: p.object,
    confidence: p.confidence,
    source: p.source || "unknown",
    timestamp:
      typeof p.timestamp === "string"
        ? new SvelteDate(p.timestamp).getTime()
        : p.timestamp,
  }));

  const outgoing: GraphRelationship[] = (data.outgoing || []).map((r) => ({
    id: createRelationshipId(data.id, r.predicate, r.targetId),
    sourceId: data.id,
    targetId: r.targetId,
    predicate: r.predicate,
    confidence: r.confidence,
    timestamp: r.timestamp
      ? typeof r.timestamp === "string"
        ? new SvelteDate(r.timestamp).getTime()
        : r.timestamp
      : Date.now(),
  }));

  const incoming: GraphRelationship[] = (data.incoming || []).map((r) => ({
    id: createRelationshipId(r.sourceId, r.predicate, data.id),
    sourceId: r.sourceId,
    targetId: data.id,
    predicate: r.predicate,
    confidence: r.confidence,
    timestamp: r.timestamp
      ? typeof r.timestamp === "string"
        ? new SvelteDate(r.timestamp).getTime()
        : r.timestamp
      : Date.now(),
  }));

  const community: GraphCommunity | undefined = data.community
    ? {
        id: data.community.id,
        label: data.community.label,
        memberCount: data.community.memberCount || 0,
        color: "", // Will be assigned by visualization
      }
    : undefined;

  return {
    id: data.id,
    idParts,
    properties,
    outgoing,
    incoming,
    community,
  };
}

// =============================================================================
// Export Singleton
// =============================================================================

export const graphStore = createGraphStore();
