// GraphQL data transformation
// Converts backend GraphQL responses to frontend graph structures

import type {
  GlobalSearchResult,
  GraphEntity,
  GraphRelationship,
  PathSearchResult,
  TripleProperty,
} from "$lib/types/graph";
import {
  createRelationshipId,
  isEntityReference,
  parseEntityId,
} from "$lib/types/graph";

/**
 * Transform backend PathSearchResult to array of GraphEntity.
 * Handles both triples (properties and relationships) and edges (bidirectional relationships).
 */
export function transformPathSearchResult(
  result: PathSearchResult,
): GraphEntity[] {
  // Create a map to build entities
  const entityMap = new Map<string, GraphEntity>();

  // Initialize entities from backend entities
  for (const backendEntity of result.entities) {
    const entity: GraphEntity = {
      id: backendEntity.id,
      idParts: parseEntityId(backendEntity.id),
      properties: [],
      outgoing: [],
      incoming: [],
    };
    entityMap.set(backendEntity.id, entity);
  }

  // Process triples to extract properties and relationships
  for (const backendEntity of result.entities) {
    const entity = entityMap.get(backendEntity.id);
    if (!entity) continue;

    for (const triple of backendEntity.triples) {
      if (isEntityReference(triple.object)) {
        // This is a relationship (object is an entity ID)
        const relationship: GraphRelationship = {
          id: createRelationshipId(
            triple.subject,
            triple.predicate,
            triple.object,
          ),
          sourceId: triple.subject,
          targetId: triple.object,
          predicate: triple.predicate,
          confidence: 1.0, // Default confidence
          timestamp: Date.now(), // Default timestamp
        };
        entity.outgoing.push(relationship);
      } else {
        // This is a property (object is a literal value)
        const property: TripleProperty = {
          predicate: triple.predicate,
          object: triple.object,
          confidence: 1.0, // Default confidence
          source: "unknown", // Default source
          timestamp: Date.now(), // Default timestamp
        };
        entity.properties.push(property);
      }
    }
  }

  // Process edges to create bidirectional relationships
  for (const edge of result.edges) {
    const sourceEntity = entityMap.get(edge.subject);
    const targetEntity = entityMap.get(edge.object);

    const relationship: GraphRelationship = {
      id: createRelationshipId(edge.subject, edge.predicate, edge.object),
      sourceId: edge.subject,
      targetId: edge.object,
      predicate: edge.predicate,
      confidence: 1.0, // Default confidence
      timestamp: Date.now(), // Default timestamp
    };

    // Add outgoing relationship to source entity
    if (sourceEntity) {
      sourceEntity.outgoing.push(relationship);
    }

    // Add incoming relationship to target entity
    if (targetEntity) {
      targetEntity.incoming.push(relationship);
    }
  }

  // Convert map to array
  return Array.from(entityMap.values());
}

/**
 * Transform a GlobalSearchResult (NLQ response) to an array of GraphEntity.
 *
 * Triple processing mirrors transformPathSearchResult:
 * - Object is a 6-part entity reference → outgoing relationship
 * - Otherwise → property
 *
 * Explicit SearchRelationship entries are processed after triples.
 * Duplicates (same relationship ID) are deduplicated.
 */
export function transformGlobalSearchResult(
  result: GlobalSearchResult,
): GraphEntity[] {
  const entityMap = new Map<string, GraphEntity>();

  // Initialise entities
  for (const backendEntity of result.entities) {
    const entity: GraphEntity = {
      id: backendEntity.id,
      idParts: parseEntityId(backendEntity.id),
      properties: [],
      outgoing: [],
      incoming: [],
    };
    entityMap.set(backendEntity.id, entity);
  }

  // Track relationship IDs per entity to deduplicate
  const outgoingIds = new Map<string, Set<string>>();
  const incomingIds = new Map<string, Set<string>>();

  for (const id of entityMap.keys()) {
    outgoingIds.set(id, new Set<string>());
    incomingIds.set(id, new Set<string>());
  }

  // Process triples for each entity
  for (const backendEntity of result.entities) {
    const entity = entityMap.get(backendEntity.id);
    if (!entity) continue;

    for (const triple of backendEntity.triples) {
      if (isEntityReference(triple.object)) {
        const relId = createRelationshipId(
          triple.subject,
          triple.predicate,
          triple.object,
        );
        const seen = outgoingIds.get(entity.id)!;
        if (!seen.has(relId)) {
          seen.add(relId);
          const relationship: GraphRelationship = {
            id: relId,
            sourceId: triple.subject,
            targetId: triple.object,
            predicate: triple.predicate,
            confidence: 1.0,
            timestamp: Date.now(),
          };
          entity.outgoing.push(relationship);
        }
      } else {
        const property: TripleProperty = {
          predicate: triple.predicate,
          object: triple.object,
          confidence: 1.0,
          source: "unknown",
          timestamp: Date.now(),
        };
        entity.properties.push(property);
      }
    }
  }

  // Process explicit relationships
  for (const rel of result.relationships) {
    const relId = createRelationshipId(rel.from, rel.predicate, rel.to);
    const relationship: GraphRelationship = {
      id: relId,
      sourceId: rel.from,
      targetId: rel.to,
      predicate: rel.predicate,
      confidence: 1.0,
      timestamp: Date.now(),
    };

    // Add outgoing to source entity if present and not already there
    const sourceEntity = entityMap.get(rel.from);
    if (sourceEntity) {
      const seen = outgoingIds.get(rel.from)!;
      if (!seen.has(relId)) {
        seen.add(relId);
        sourceEntity.outgoing.push(relationship);
      }
    }

    // Add incoming to target entity if present and not already there
    const targetEntity = entityMap.get(rel.to);
    if (targetEntity) {
      const seen = incomingIds.get(rel.to)!;
      if (!seen.has(relId)) {
        seen.add(relId);
        targetEntity.incoming.push(relationship);
      }
    }
  }

  return Array.from(entityMap.values());
}
