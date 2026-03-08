// GraphQL API client
// Handles GraphQL queries for knowledge graph operations

import type {
  BackendEntity,
  ClassificationMeta,
  GlobalSearchResult,
  PathSearchResult,
} from "$lib/types/graph";

const GRAPHQL_ENDPOINT = "/graphql";

export class GraphApiError extends Error {
  constructor(
    message: string,
    public statusCode: number,
    public details?: unknown,
  ) {
    super(message);
    this.name = "GraphApiError";
  }
}

interface GraphQLRequest {
  query: string;
  variables: Record<string, unknown>;
}

interface GraphQLResponse<T> {
  data?: T;
  errors?: Array<{
    message: string;
    path?: string[];
  }>;
  extensions?: Record<string, unknown>;
}

async function executeQuery<T>(
  query: string,
  variables: Record<string, unknown>,
  operationName: string,
): Promise<T> {
  const request: GraphQLRequest = {
    query,
    variables,
  };

  let response: Response;
  try {
    response = await fetch(GRAPHQL_ENDPOINT, {
      method: "POST",
      headers: {
        "Content-Type": "application/json",
      },
      body: JSON.stringify(request),
    });
  } catch (error) {
    throw new GraphApiError(
      `Network error during ${operationName}: ${error instanceof Error ? error.message : "Unknown error"}`,
      0,
      { originalError: error },
    );
  }

  if (!response.ok) {
    throw new GraphApiError(
      `${operationName} failed: ${response.statusText}`,
      response.status,
    );
  }

  let data: GraphQLResponse<T>;
  try {
    data = await response.json();
  } catch (error) {
    throw new GraphApiError(
      `Invalid JSON response from ${operationName}`,
      500,
      { originalError: error },
    );
  }

  if (data.errors && data.errors.length > 0) {
    throw new GraphApiError(data.errors[0].message, 200, {
      errors: data.errors,
    });
  }

  if (!data.data) {
    throw new GraphApiError(`No data in ${operationName} response`, 500);
  }

  return data.data;
}

interface QueryResultWithExtensions<T> {
  data: T;
  extensions?: Record<string, unknown>;
}

async function executeQueryWithExtensions<T>(
  query: string,
  variables: Record<string, unknown>,
  operationName: string,
): Promise<QueryResultWithExtensions<T>> {
  const request: GraphQLRequest = {
    query,
    variables,
  };

  let response: Response;
  try {
    response = await fetch(GRAPHQL_ENDPOINT, {
      method: "POST",
      headers: {
        "Content-Type": "application/json",
      },
      body: JSON.stringify(request),
    });
  } catch (error) {
    throw new GraphApiError(
      `Network error during ${operationName}: ${error instanceof Error ? error.message : "Unknown error"}`,
      0,
      { originalError: error },
    );
  }

  if (!response.ok) {
    throw new GraphApiError(
      `${operationName} failed: ${response.statusText}`,
      response.status,
    );
  }

  let parsed: GraphQLResponse<T>;
  try {
    parsed = await response.json();
  } catch (error) {
    throw new GraphApiError(
      `Invalid JSON response from ${operationName}`,
      500,
      { originalError: error },
    );
  }

  if (parsed.errors && parsed.errors.length > 0) {
    throw new GraphApiError(parsed.errors[0].message, 200, {
      errors: parsed.errors,
    });
  }

  if (!parsed.data) {
    throw new GraphApiError(`No data in ${operationName} response`, 500);
  }

  return { data: parsed.data, extensions: parsed.extensions };
}

export const graphApi = {
  /**
   * Execute pathSearch query to find entities and edges within a depth.
   */
  async pathSearch(
    startEntity: string,
    maxDepth: number = 3,
    maxNodes: number = 100,
  ): Promise<PathSearchResult> {
    const query = `
      query PathSearch($startEntity: String!, $maxDepth: Int!, $maxNodes: Int!) {
        pathSearch(startEntity: $startEntity, maxDepth: $maxDepth, maxNodes: $maxNodes) {
          entities {
            id
            triples {
              subject
              predicate
              object
            }
          }
          edges {
            subject
            predicate
            object
          }
        }
      }
    `;

    const variables = {
      startEntity,
      maxDepth,
      maxNodes,
    };

    const data = await executeQuery<{ pathSearch: PathSearchResult }>(
      query,
      variables,
      "pathSearch",
    );

    return data.pathSearch;
  },

  /**
   * Get a single entity by ID.
   */
  async getEntity(id: string): Promise<BackendEntity> {
    const query = `
      query GetEntity($id: String!) {
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

    const variables = { id };

    const data = await executeQuery<{ entity: BackendEntity | null }>(
      query,
      variables,
      "getEntity",
    );

    if (!data.entity) {
      throw new GraphApiError(`Entity ${id} not found`, 404);
    }

    return data.entity;
  },

  /**
   * Get entities matching a prefix.
   */
  async getEntitiesByPrefix(
    prefix: string,
    limit: number = 50,
  ): Promise<BackendEntity[]> {
    const query = `
      query GetEntitiesByPrefix($prefix: String!, $limit: Int!) {
        entitiesByPrefix(prefix: $prefix, limit: $limit) {
          id
          triples {
            subject
            predicate
            object
          }
        }
      }
    `;

    const variables = { prefix, limit };

    const data = await executeQuery<{ entitiesByPrefix: BackendEntity[] }>(
      query,
      variables,
      "getEntitiesByPrefix",
    );

    return data.entitiesByPrefix;
  },

  /**
   * Execute globalSearch NLQ query.
   * Uses alpha.17+ schema with properly typed fields and classification extensions.
   */
  async globalSearch(
    query: string,
    level?: number,
    maxCommunities?: number,
  ): Promise<GlobalSearchResult> {
    const gqlQuery = `
      query GlobalSearch($query: String!, $level: Int, $maxCommunities: Int) {
        globalSearch(query: $query, level: $level, maxCommunities: $maxCommunities) {
          entities {
            id
            triples {
              subject
              predicate
              object
            }
          }
          community_summaries {
            communityId
            text
            keywords
          }
          relationships {
            from
            to
            predicate
          }
          count
          duration_ms
        }
      }
    `;

    const variables: Record<string, unknown> = { query };
    if (level !== undefined) {
      variables.level = level;
    }
    if (maxCommunities !== undefined) {
      variables.maxCommunities = maxCommunities;
    }

    interface GlobalSearchGqlResponse {
      globalSearch: {
        entities: BackendEntity[];
        community_summaries: Array<{
          communityId: string;
          text: string;
          keywords: string[];
        }>;
        relationships: Array<{
          from: string;
          to: string;
          predicate: string;
        }>;
        count: number;
        duration_ms: number;
      };
    }

    const { data, extensions } =
      await executeQueryWithExtensions<GlobalSearchGqlResponse>(
        gqlQuery,
        variables,
        "globalSearch",
      );

    const gs = data.globalSearch;

    // Extract classification metadata from GraphQL extensions (alpha.17+)
    let classification: ClassificationMeta | undefined;
    if (extensions?.classification) {
      const c = extensions.classification as Record<string, unknown>;
      classification = {
        tier: (c.tier as number) ?? 0,
        confidence: (c.confidence as number) ?? 0,
        intent: (c.intent as string) ?? "",
      };
    }

    return {
      entities: gs.entities ?? [],
      communitySummaries: gs.community_summaries ?? [],
      relationships: gs.relationships ?? [],
      count: gs.count ?? 0,
      durationMs: gs.duration_ms ?? 0,
      classification,
    };
  },
};
