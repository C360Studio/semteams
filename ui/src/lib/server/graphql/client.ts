// ---------------------------------------------------------------------------
// GraphQL Client — thin fetch wrapper for SemStreams graph-gateway
// ---------------------------------------------------------------------------

export interface GraphQLClientConfig {
  baseUrl: string;
  timeout?: number;
}

export interface GraphQLClient {
  query<T = unknown>(
    query: string,
    variables?: Record<string, unknown>,
  ): Promise<T>;
}

const DEFAULT_TIMEOUT = 10_000;

export function createGraphQLClient(
  config: GraphQLClientConfig,
): GraphQLClient {
  const { baseUrl, timeout = DEFAULT_TIMEOUT } = config;
  const endpoint = `${baseUrl}/graphql`;

  return {
    async query<T = unknown>(
      queryString: string,
      variables?: Record<string, unknown>,
    ): Promise<T> {
      const controller = new AbortController();
      const timerId = setTimeout(() => controller.abort(), timeout);

      let response: Response;
      try {
        const body: Record<string, unknown> = { query: queryString };
        if (variables !== undefined) {
          body.variables = variables;
        }

        response = await fetch(endpoint, {
          method: "POST",
          headers: { "Content-Type": "application/json" },
          body: JSON.stringify(body),
          signal: controller.signal,
        });
      } catch (err: unknown) {
        clearTimeout(timerId);
        // Re-throw as-is — preserves AbortError / network error messages
        throw err;
      }

      clearTimeout(timerId);

      if (!response.ok) {
        throw new Error(
          `GraphQL request failed: ${response.status} ${response.statusText}`,
        );
      }

      const json = (await response.json()) as {
        data?: T | null;
        errors?: Array<{ message: string }>;
      };

      if (json.errors && json.errors.length > 0) {
        throw new Error(json.errors.map((e) => e.message).join("; "));
      }

      if (json.data == null) {
        throw new Error("GraphQL response contained no data");
      }

      return json.data;
    },
  };
}
