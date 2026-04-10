// ---------------------------------------------------------------------------
// Tool Executors — executes AI tool calls against the GraphQL client
// ---------------------------------------------------------------------------

import type {
  MessageAttachment,
  SearchResultAttachment,
  EntityDetailAttachment,
  ErrorAttachment,
  HealthAttachment,
  FlowStatusAttachment,
} from "$lib/types/chat";
import type { GraphQLClient } from "$lib/server/graphql/client";

// ---------------------------------------------------------------------------
// Public types
// ---------------------------------------------------------------------------

export interface ToolExecutorContext {
  graphqlClient: GraphQLClient;
  backendUrl?: string;
}

export interface ToolResult {
  attachments: MessageAttachment[];
  textSummary: string;
}

// ---------------------------------------------------------------------------
// GraphQL query shapes
// ---------------------------------------------------------------------------

interface GraphQLTriple {
  subject: string;
  predicate: string;
  object: unknown;
}

interface GraphQLEntity {
  id: string;
  triples: GraphQLTriple[];
}

interface GlobalSearchResult {
  globalSearch: {
    entities: GraphQLEntity[];
    count: number;
    duration_ms: number;
    community_summaries: unknown[];
    relationships: unknown[];
  };
}

interface EntityLookupResult {
  entity: GraphQLEntity | null | undefined;
}

// ---------------------------------------------------------------------------
// Entity ID parsing helpers
// ID format: c360.<org>.<domain>.<subdomain>.<type>.<instance>
//   index:    0    1     2        3           4      5
// ---------------------------------------------------------------------------

function parseEntityId(id: string): {
  label: string;
  type: string;
  domain: string;
} {
  const parts = id.split(".");
  // domain is index 2, type is index 4, label is last segment
  const domain = parts[2] ?? "unknown";
  const type = parts[4] ?? "unknown";
  const label = parts[parts.length - 1] ?? id;
  return { label, type, domain };
}

function makeErrorAttachment(code: string, message: string): ErrorAttachment {
  return { kind: "error", code, message };
}

// ---------------------------------------------------------------------------
// executeGraphSearch
// ---------------------------------------------------------------------------

const GLOBAL_SEARCH_QUERY = `
  query GlobalSearch($query: String!, $limit: Int) {
    globalSearch(query: $query, limit: $limit) {
      entities {
        id
        triples {
          subject
          predicate
          object
        }
      }
      count
      duration_ms
      community_summaries
      relationships
    }
  }
`;

const DEFAULT_SEARCH_LIMIT = 10;

export async function executeGraphSearch(
  params: { query: string; depth?: number; limit?: number },
  context: ToolExecutorContext,
): Promise<ToolResult> {
  const { query, limit = DEFAULT_SEARCH_LIMIT } = params;

  try {
    const data = await context.graphqlClient.query<GlobalSearchResult>(
      GLOBAL_SEARCH_QUERY,
      { query, limit },
    );

    const { entities, count } = data.globalSearch;

    const results = entities.map((entity) => {
      const { label, type, domain } = parseEntityId(entity.id);
      return { id: entity.id, label, type, domain };
    });

    const attachment: SearchResultAttachment = {
      kind: "search-result",
      query,
      results,
      totalCount: count,
    };

    const textSummary =
      count === 0
        ? `No results found for '${query}'`
        : `Found ${count} result${count === 1 ? "" : "s"} for '${query}'`;

    return { attachments: [attachment], textSummary };
  } catch (err: unknown) {
    const message = err instanceof Error ? err.message : String(err);
    return {
      attachments: [makeErrorAttachment("GRAPH_SEARCH_ERROR", message)],
      textSummary: `Search failed: ${message}`,
    };
  }
}

// ---------------------------------------------------------------------------
// executeEntityLookup
// ---------------------------------------------------------------------------

const ENTITY_LOOKUP_QUERY = `
  query EntityLookup($id: String!) {
    entity(id: $id) {
      id
      triples {
        subject
        predicate
        object
      }
    }
  }
`;

export async function executeEntityLookup(
  params: { entityId: string },
  context: ToolExecutorContext,
): Promise<ToolResult> {
  const { entityId } = params;

  try {
    const data = await context.graphqlClient.query<EntityLookupResult>(
      ENTITY_LOOKUP_QUERY,
      { id: entityId },
    );

    const entity = data.entity;

    if (entity == null) {
      return {
        attachments: [
          makeErrorAttachment(
            "ENTITY_NOT_FOUND",
            `Entity not found: ${entityId}`,
          ),
        ],
        textSummary: `Entity '${entityId}' was not found`,
      };
    }

    const { label, type, domain } = parseEntityId(entity.id);

    // Split triples into properties (same entity as subject) and relationships (different subject)
    const properties = entity.triples
      .filter(
        (t) =>
          t.subject === entity.id ||
          typeof t.object !== "string" ||
          !t.object.includes("."),
      )
      .map((t) => ({ predicate: t.predicate, value: t.object }));

    const relationships = entity.triples
      .filter(
        (t) =>
          typeof t.object === "string" &&
          t.object !== entity.id &&
          t.object.includes(".") &&
          t.object !== t.subject,
      )
      .map((t) => ({ predicate: t.predicate, targetId: String(t.object) }));

    const attachment: EntityDetailAttachment = {
      kind: "entity-detail",
      entity: {
        id: entity.id,
        label,
        type,
        domain,
        properties,
        relationships,
      },
    };

    const textSummary = `Entity '${label}' (${type}): ${properties.length} properties, ${relationships.length} relationships`;

    return { attachments: [attachment], textSummary };
  } catch (err: unknown) {
    const message = err instanceof Error ? err.message : String(err);
    return {
      attachments: [makeErrorAttachment("ENTITY_LOOKUP_ERROR", message)],
      textSummary: `Entity lookup failed: ${message}`,
    };
  }
}

// ---------------------------------------------------------------------------
// executeComponentHealth
// ---------------------------------------------------------------------------

function mapHealthStatus(
  backendStatus: string,
): "healthy" | "degraded" | "unhealthy" | "unknown" {
  switch (backendStatus) {
    case "healthy":
      return "healthy";
    case "degraded":
      return "degraded";
    case "error":
    case "unhealthy":
      return "unhealthy";
    default:
      return "unknown";
  }
}

export async function executeComponentHealth(
  params: { componentName: string },
  context: ToolExecutorContext,
): Promise<ToolResult> {
  const { componentName } = params;

  if (!context.backendUrl) {
    return {
      attachments: [
        makeErrorAttachment("CONFIG_ERROR", "backendUrl is not configured"),
      ],
      textSummary: "Health check failed: backendUrl is not configured",
    };
  }

  try {
    const encodedName = encodeURIComponent(componentName ?? "");
    const baseUrl = context.backendUrl.replace(/\/$/, "");
    const response = await fetch(`${baseUrl}/health/${encodedName}`);

    if (response.status === 404) {
      return {
        attachments: [
          makeErrorAttachment(
            "COMPONENT_NOT_FOUND",
            `Component '${componentName}' not found`,
          ),
        ],
        textSummary: `Component '${componentName}' was not found`,
      };
    }

    if (!response.ok) {
      return {
        attachments: [
          makeErrorAttachment(
            "HEALTH_ERROR",
            `Health check failed with status ${response.status}`,
          ),
        ],
        textSummary: `Health check for '${componentName}' failed`,
      };
    }

    const data = (await response.json()) as {
      name?: string;
      status?: string;
      message?: string | null;
      metrics?: Record<string, number>;
      lastCheck?: string;
    };

    const status = mapHealthStatus(data.status ?? "unknown");

    const attachment: HealthAttachment = {
      kind: "health",
      componentName,
      status,
      ...(data.message != null ? { message: data.message } : {}),
      ...(data.metrics != null ? { metrics: data.metrics } : {}),
      ...(data.lastCheck != null ? { lastCheck: data.lastCheck } : {}),
    };

    return {
      attachments: [attachment],
      textSummary: `Component '${componentName}' is ${status}`,
    };
  } catch (err: unknown) {
    const message = err instanceof Error ? err.message : String(err);
    return {
      attachments: [makeErrorAttachment("HEALTH_FETCH_ERROR", message)],
      textSummary: `Health check for '${componentName}' failed: ${message}`,
    };
  }
}

// ---------------------------------------------------------------------------
// executeFlowStatus
// ---------------------------------------------------------------------------

export async function executeFlowStatus(
  params: { flowId: string },
  context: ToolExecutorContext,
): Promise<ToolResult> {
  const { flowId } = params;

  if (!context.backendUrl) {
    return {
      attachments: [
        makeErrorAttachment("CONFIG_ERROR", "backendUrl is not configured"),
      ],
      textSummary: "Flow status check failed: backendUrl is not configured",
    };
  }

  try {
    const encodedFlowId = encodeURIComponent(flowId ?? "");
    const baseUrl = context.backendUrl.replace(/\/$/, "");
    const response = await fetch(
      `${baseUrl}/flowbuilder/flows/${encodedFlowId}`,
    );

    if (response.status === 404) {
      return {
        attachments: [
          makeErrorAttachment("FLOW_NOT_FOUND", `Flow '${flowId}' not found`),
        ],
        textSummary: `Flow '${flowId}' was not found`,
      };
    }

    if (!response.ok) {
      return {
        attachments: [
          makeErrorAttachment(
            "FLOW_STATUS_ERROR",
            `Flow status check failed with status ${response.status}`,
          ),
        ],
        textSummary: `Flow status check for '${flowId}' failed`,
      };
    }

    const data = (await response.json()) as {
      id?: string;
      name?: string;
      state?: string;
      nodes?: unknown[];
      connections?: unknown[];
      warnings?: string[];
    };

    const flowName = data.name ?? flowId;
    const state = data.state ?? "unknown";
    const nodeCount = data.nodes?.length ?? 0;
    const connectionCount = data.connections?.length ?? 0;

    const attachment: FlowStatusAttachment = {
      kind: "flow-status",
      flowId,
      flowName,
      state,
      nodeCount,
      connectionCount,
      ...(data.warnings != null ? { warnings: data.warnings } : {}),
    };

    return {
      attachments: [attachment],
      textSummary: `Flow '${flowName}' is ${state} with ${nodeCount} node${nodeCount === 1 ? "" : "s"}`,
    };
  } catch (err: unknown) {
    const message = err instanceof Error ? err.message : String(err);
    return {
      attachments: [makeErrorAttachment("FLOW_STATUS_FETCH_ERROR", message)],
      textSummary: `Flow status check for '${flowId}' failed: ${message}`,
    };
  }
}
