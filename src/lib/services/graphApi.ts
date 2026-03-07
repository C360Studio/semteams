// GraphQL API client
// Handles GraphQL queries for knowledge graph operations

import type { BackendEntity, PathSearchResult } from "$lib/types/graph";

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
};
