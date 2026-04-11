// Task: NLQ Phase 1 — graphApi.globalSearch tests (alpha.17 schema)

import { describe, it, expect, beforeEach, vi } from "vitest";
import { graphApi, GraphApiError } from "./graphApi";

describe("graphApi.globalSearch", () => {
  const mockFetch = vi.fn();
  globalThis.fetch = mockFetch;

  beforeEach(() => {
    mockFetch.mockClear();
  });

  // ---------------------------------------------------------------------------
  // Fixture helpers — alpha.17 returns properly typed fields
  // ---------------------------------------------------------------------------

  const makeGlobalSearchData = () => ({
    entities: [
      {
        id: "c360.ops.robotics.gcs.drone.001",
        triples: [
          {
            subject: "c360.ops.robotics.gcs.drone.001",
            predicate: "core.property.name",
            object: "Drone 001",
          },
          {
            subject: "c360.ops.robotics.gcs.drone.001",
            predicate: "fleet.membership.current",
            object: "c360.ops.robotics.gcs.fleet.alpha",
          },
        ],
      },
    ],
    community_summaries: [
      {
        communityId: "community-1",
        text: "A cluster of drones in the west-coast fleet.",
        keywords: ["drone", "fleet", "west-coast"],
      },
    ],
    relationships: [
      {
        from: "c360.ops.robotics.gcs.drone.001",
        to: "c360.ops.robotics.gcs.fleet.alpha",
        predicate: "fleet.membership.current",
      },
    ],
    count: 1,
    duration_ms: 42,
  });

  const makeGqlResponse = (
    globalSearchData: unknown,
    extensions?: Record<string, unknown>,
  ) => ({
    data: {
      globalSearch: globalSearchData,
    },
    ...(extensions ? { extensions } : {}),
  });

  function mockOkFetch(body: unknown) {
    mockFetch.mockResolvedValueOnce({
      ok: true,
      json: async () => body,
    });
  }

  // ---------------------------------------------------------------------------
  // GraphQL query shape
  // ---------------------------------------------------------------------------

  describe("query shape", () => {
    it("should send a POST to /graphql with the correct GraphQL query", async () => {
      mockOkFetch(makeGqlResponse(makeGlobalSearchData()));

      await graphApi.globalSearch("show me all drones");

      expect(mockFetch).toHaveBeenCalledWith("/graphql", {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: expect.stringContaining("globalSearch"),
      });
    });

    it("should include query, level, and maxCommunities in variables", async () => {
      mockOkFetch(makeGqlResponse(makeGlobalSearchData()));

      await graphApi.globalSearch("show me all drones", 2, 10);

      const body = JSON.parse(mockFetch.mock.calls[0][1].body);
      expect(body.variables).toEqual({
        query: "show me all drones",
        level: 2,
        maxCommunities: 10,
      });
    });

    it("should request individual fields (entities, community_summaries, etc.)", async () => {
      mockOkFetch(makeGqlResponse(makeGlobalSearchData()));

      await graphApi.globalSearch("find fleet alpha");

      const body = JSON.parse(mockFetch.mock.calls[0][1].body);
      expect(body.query).toContain("entities");
      expect(body.query).toContain("community_summaries");
      expect(body.query).toContain("relationships");
      expect(body.query).toContain("count");
      expect(body.query).toContain("duration_ms");
    });

    it("should use default level and maxCommunities when not provided", async () => {
      mockOkFetch(makeGqlResponse(makeGlobalSearchData()));

      await graphApi.globalSearch("find all entities");

      const body = JSON.parse(mockFetch.mock.calls[0][1].body);
      expect(body.variables.query).toBe("find all entities");
      expect(body.variables).toHaveProperty("query");
    });
  });

  // ---------------------------------------------------------------------------
  // Response parsing — alpha.17 typed fields
  // ---------------------------------------------------------------------------

  describe("response parsing", () => {
    it("should return entities from the typed response", async () => {
      mockOkFetch(makeGqlResponse(makeGlobalSearchData()));

      const result = await graphApi.globalSearch("find drones");

      expect(result.entities).toHaveLength(1);
      expect(result.entities[0].id).toBe("c360.ops.robotics.gcs.drone.001");
      expect(result.entities[0].triples).toHaveLength(2);
    });

    it("should return community_summaries from the typed response", async () => {
      mockOkFetch(makeGqlResponse(makeGlobalSearchData()));

      const result = await graphApi.globalSearch("community overview");

      expect(result.communitySummaries).toHaveLength(1);
      expect(result.communitySummaries[0].communityId).toBe("community-1");
      expect(result.communitySummaries[0].text).toContain("west-coast fleet");
      expect(result.communitySummaries[0].keywords).toContain("drone");
    });

    it("should return relationships from the typed response", async () => {
      mockOkFetch(makeGqlResponse(makeGlobalSearchData()));

      const result = await graphApi.globalSearch("show relationships");

      expect(result.relationships).toHaveLength(1);
      expect(result.relationships[0].from).toBe(
        "c360.ops.robotics.gcs.drone.001",
      );
      expect(result.relationships[0].to).toBe(
        "c360.ops.robotics.gcs.fleet.alpha",
      );
      expect(result.relationships[0].predicate).toBe(
        "fleet.membership.current",
      );
    });

    it("should return count and durationMs from the typed response", async () => {
      mockOkFetch(makeGqlResponse(makeGlobalSearchData()));

      const result = await graphApi.globalSearch("count entities");

      expect(result.count).toBe(1);
      expect(result.durationMs).toBe(42);
    });

    it("should handle empty results gracefully", async () => {
      const emptyData = {
        entities: [],
        community_summaries: [],
        relationships: [],
        count: 0,
        duration_ms: 5,
      };
      mockOkFetch(makeGqlResponse(emptyData));

      const result = await graphApi.globalSearch("find nothing");

      expect(result.entities).toEqual([]);
      expect(result.communitySummaries).toEqual([]);
      expect(result.relationships).toEqual([]);
      expect(result.count).toBe(0);
    });

    it("should handle results with multiple entities", async () => {
      const multiEntityData = {
        entities: [
          { id: "c360.ops.robotics.gcs.drone.001", triples: [] },
          { id: "c360.ops.robotics.gcs.drone.002", triples: [] },
          { id: "c360.ops.robotics.gcs.fleet.alpha", triples: [] },
        ],
        community_summaries: [],
        relationships: [],
        count: 3,
        duration_ms: 120,
      };
      mockOkFetch(makeGqlResponse(multiEntityData));

      const result = await graphApi.globalSearch("fleet alpha and drones");

      expect(result.entities).toHaveLength(3);
      expect(result.count).toBe(3);
    });
  });

  // ---------------------------------------------------------------------------
  // Classification extensions (alpha.17)
  // ---------------------------------------------------------------------------

  describe("classification extensions", () => {
    it("should extract classification metadata from extensions", async () => {
      mockOkFetch(
        makeGqlResponse(makeGlobalSearchData(), {
          classification: {
            tier: 1,
            confidence: 0.92,
            intent: "entity_lookup",
          },
        }),
      );

      const result = await graphApi.globalSearch("find drones");

      expect(result.classification).toEqual({
        tier: 1,
        confidence: 0.92,
        intent: "entity_lookup",
      });
    });

    it("should return undefined classification when extensions absent", async () => {
      mockOkFetch(makeGqlResponse(makeGlobalSearchData()));

      const result = await graphApi.globalSearch("find drones");

      expect(result.classification).toBeUndefined();
    });

    it("should handle T0 keyword classification", async () => {
      mockOkFetch(
        makeGqlResponse(makeGlobalSearchData(), {
          classification: {
            tier: 0,
            confidence: 1.0,
            intent: "temporal_filter",
          },
        }),
      );

      const result = await graphApi.globalSearch("drones from last hour");

      expect(result.classification?.tier).toBe(0);
      expect(result.classification?.confidence).toBe(1.0);
    });

    it("should handle T3 LLM classification", async () => {
      mockOkFetch(
        makeGqlResponse(makeGlobalSearchData(), {
          classification: {
            tier: 3,
            confidence: 0.67,
            intent: "aggregation",
          },
        }),
      );

      const result = await graphApi.globalSearch(
        "how many drones are offline?",
      );

      expect(result.classification?.tier).toBe(3);
      expect(result.classification?.intent).toBe("aggregation");
    });
  });

  // ---------------------------------------------------------------------------
  // Default parameter behaviour
  // ---------------------------------------------------------------------------

  describe("default parameters", () => {
    it("should call successfully with only the query string", async () => {
      mockOkFetch(makeGqlResponse(makeGlobalSearchData()));

      const result = await graphApi.globalSearch("minimal call");

      expect(result).toBeDefined();
      expect(result.entities).toBeDefined();
    });

    it("should call with level overridden but maxCommunities defaulted", async () => {
      mockOkFetch(makeGqlResponse(makeGlobalSearchData()));

      await graphApi.globalSearch("level only", 3);

      const body = JSON.parse(mockFetch.mock.calls[0][1].body);
      expect(body.variables.query).toBe("level only");
      expect(body.variables.level).toBe(3);
    });
  });

  // ---------------------------------------------------------------------------
  // Error handling
  // ---------------------------------------------------------------------------

  describe("error handling", () => {
    it("should throw GraphApiError on network error (fetch throws)", async () => {
      mockFetch.mockRejectedValueOnce(new Error("Network unavailable"));

      await expect(graphApi.globalSearch("find drones")).rejects.toThrow(
        GraphApiError,
      );
    });

    it("should throw GraphApiError with statusCode 0 on network error", async () => {
      mockFetch.mockRejectedValueOnce(new Error("Failed to fetch"));

      try {
        await graphApi.globalSearch("find drones");
        expect.fail("Should have thrown");
      } catch (error) {
        expect(error).toBeInstanceOf(GraphApiError);
        expect((error as GraphApiError).statusCode).toBe(0);
      }
    });

    it("should throw GraphApiError on non-200 HTTP response", async () => {
      mockFetch.mockResolvedValueOnce({
        ok: false,
        status: 503,
        statusText: "Service Unavailable",
        json: async () => ({}),
      });

      try {
        await graphApi.globalSearch("find drones");
        expect.fail("Should have thrown");
      } catch (error) {
        expect(error).toBeInstanceOf(GraphApiError);
        expect((error as GraphApiError).statusCode).toBe(503);
        expect((error as GraphApiError).message).toContain("globalSearch");
      }
    });

    it("should throw GraphApiError on GraphQL errors array in response", async () => {
      mockFetch.mockResolvedValueOnce({
        ok: true,
        json: async () => ({
          errors: [
            {
              message: "NLQ classifier unavailable",
              path: ["globalSearch"],
            },
          ],
        }),
      });

      try {
        await graphApi.globalSearch("find drones");
        expect.fail("Should have thrown");
      } catch (error) {
        expect(error).toBeInstanceOf(GraphApiError);
        expect((error as GraphApiError).message).toContain(
          "NLQ classifier unavailable",
        );
      }
    });

    it("should throw GraphApiError on malformed JSON response", async () => {
      mockFetch.mockResolvedValueOnce({
        ok: true,
        json: async () => {
          throw new Error("Unexpected token");
        },
      });

      await expect(graphApi.globalSearch("find drones")).rejects.toThrow(
        GraphApiError,
      );
    });

    it("should throw GraphApiError when response has no data field", async () => {
      mockFetch.mockResolvedValueOnce({
        ok: true,
        json: async () => ({ errors: [] }),
      });

      await expect(graphApi.globalSearch("find drones")).rejects.toThrow(
        GraphApiError,
      );
    });
  });
});
