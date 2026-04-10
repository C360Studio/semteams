import { describe, it, expect } from "vitest";
import { executeGraphSearch, executeEntityLookup } from "./toolExecutors";
import type { ToolExecutorContext } from "./toolExecutors";
import type { ErrorAttachment } from "$lib/types/chat";

// ---------------------------------------------------------------------------
// Mock GraphQL client factory
// ---------------------------------------------------------------------------

function makeContext(
  queryImpl: (
    query: string,
    variables?: Record<string, unknown>,
  ) => Promise<unknown>,
): ToolExecutorContext {
  return {
    graphqlClient: {
      query: queryImpl as ToolExecutorContext["graphqlClient"]["query"],
    },
  };
}

// ---------------------------------------------------------------------------
// executeGraphSearch — success cases
// ---------------------------------------------------------------------------

describe("executeGraphSearch — returns SearchResultAttachment", () => {
  it("returns attachment with kind='search-result'", async () => {
    const ctx = makeContext(async () => ({
      globalSearch: {
        entities: [
          { id: "c360.ops.robotics.gcs.drone.001", triples: [] },
          { id: "c360.ops.robotics.gcs.drone.002", triples: [] },
        ],
        count: 2,
        duration_ms: 15,
        community_summaries: [],
        relationships: [],
      },
    }));

    const result = await executeGraphSearch({ query: "drones" }, ctx);

    expect(result.attachments).toHaveLength(1);
    expect(result.attachments[0].kind).toBe("search-result");
  });

  it("attachment contains the original search query", async () => {
    const ctx = makeContext(async () => ({
      globalSearch: {
        entities: [],
        count: 0,
        duration_ms: 5,
        community_summaries: [],
        relationships: [],
      },
    }));

    const result = await executeGraphSearch({ query: "border sensors" }, ctx);
    const attachment = result.attachments[0] as { kind: string; query: string };

    expect(attachment.query).toBe("border sensors");
  });

  it("maps GraphQL entities to results array with id, label, type, domain", async () => {
    const ctx = makeContext(async () => ({
      globalSearch: {
        entities: [
          { id: "c360.ops.robotics.gcs.drone.001", triples: [] },
          { id: "c360.ops.robotics.gcs.drone.002", triples: [] },
        ],
        count: 2,
        duration_ms: 20,
        community_summaries: [],
        relationships: [],
      },
    }));

    const result = await executeGraphSearch({ query: "drones" }, ctx);
    const attachment = result.attachments[0] as {
      kind: string;
      results: Array<{
        id: string;
        label: string;
        type: string;
        domain: string;
      }>;
    };

    expect(attachment.results).toHaveLength(2);
    expect(attachment.results[0].id).toBe("c360.ops.robotics.gcs.drone.001");
    // label is derived from the entity ID (instance part)
    expect(attachment.results[0].label).toBeTruthy();
    // type comes from the 5th ID segment
    expect(attachment.results[0].type).toBe("drone");
    // domain from the 3rd ID segment
    expect(attachment.results[0].domain).toBe("robotics");
  });

  it("attachment includes totalCount matching GraphQL count", async () => {
    const ctx = makeContext(async () => ({
      globalSearch: {
        entities: [{ id: "c360.ops.robotics.gcs.drone.001", triples: [] }],
        count: 42,
        duration_ms: 10,
        community_summaries: [],
        relationships: [],
      },
    }));

    const result = await executeGraphSearch({ query: "drones" }, ctx);
    const attachment = result.attachments[0] as {
      kind: string;
      totalCount: number;
    };

    expect(attachment.totalCount).toBe(42);
  });

  it("textSummary mentions result count and query for non-zero results", async () => {
    const ctx = makeContext(async () => ({
      globalSearch: {
        entities: [
          { id: "c360.ops.robotics.gcs.drone.001", triples: [] },
          { id: "c360.ops.robotics.gcs.drone.002", triples: [] },
          { id: "c360.ops.robotics.gcs.drone.003", triples: [] },
          { id: "c360.ops.robotics.gcs.sensor.001", triples: [] },
          { id: "c360.ops.robotics.gcs.sensor.002", triples: [] },
        ],
        count: 5,
        duration_ms: 30,
        community_summaries: [],
        relationships: [],
      },
    }));

    const result = await executeGraphSearch({ query: "drones" }, ctx);

    expect(result.textSummary).toMatch(/5/);
    expect(result.textSummary).toMatch(/drones/i);
  });
});

// ---------------------------------------------------------------------------
// executeGraphSearch — zero results
// ---------------------------------------------------------------------------

describe("executeGraphSearch — zero results", () => {
  it("returns attachment with empty results array", async () => {
    const ctx = makeContext(async () => ({
      globalSearch: {
        entities: [],
        count: 0,
        duration_ms: 8,
        community_summaries: [],
        relationships: [],
      },
    }));

    const result = await executeGraphSearch({ query: "unicorns" }, ctx);
    const attachment = result.attachments[0] as {
      kind: string;
      results: unknown[];
    };

    expect(attachment.results).toHaveLength(0);
  });

  it("textSummary says no results found for the query", async () => {
    const ctx = makeContext(async () => ({
      globalSearch: {
        entities: [],
        count: 0,
        duration_ms: 8,
        community_summaries: [],
        relationships: [],
      },
    }));

    const result = await executeGraphSearch({ query: "unicorns" }, ctx);

    expect(result.textSummary).toMatch(/no results/i);
    expect(result.textSummary).toMatch(/unicorns/i);
  });
});

// ---------------------------------------------------------------------------
// executeGraphSearch — passes depth and limit to GraphQL
// ---------------------------------------------------------------------------

describe("executeGraphSearch — respects depth and limit params", () => {
  it("passes limit to the GraphQL query variables", async () => {
    const capturedVars: Record<string, unknown>[] = [];
    const ctx = makeContext(async (_query, variables) => {
      if (variables) capturedVars.push(variables);
      return {
        globalSearch: {
          entities: [],
          count: 0,
          duration_ms: 0,
          community_summaries: [],
          relationships: [],
        },
      };
    });

    await executeGraphSearch({ query: "sensors", limit: 25 }, ctx);

    expect(capturedVars.length).toBeGreaterThan(0);
    const vars = capturedVars[0];
    // limit should appear somewhere in variables
    expect(vars.limit ?? vars.maxResults ?? vars.size).toBe(25);
  });

  it("uses a sensible default limit when none provided", async () => {
    const capturedVars: Record<string, unknown>[] = [];
    const ctx = makeContext(async (_query, variables) => {
      if (variables) capturedVars.push(variables);
      return {
        globalSearch: {
          entities: [],
          count: 0,
          duration_ms: 0,
          community_summaries: [],
          relationships: [],
        },
      };
    });

    await executeGraphSearch({ query: "sensors" }, ctx);

    expect(capturedVars.length).toBeGreaterThan(0);
    // default limit should be a positive integer
    const vars = capturedVars[0];
    const limit = vars.limit ?? vars.maxResults ?? vars.size;
    expect(typeof limit === "number" && (limit as number) > 0).toBe(true);
  });
});

// ---------------------------------------------------------------------------
// executeGraphSearch — error handling
// ---------------------------------------------------------------------------

describe("executeGraphSearch — handles GraphQL error gracefully", () => {
  it("returns ErrorAttachment when GraphQL throws", async () => {
    const ctx = makeContext(async () => {
      throw new Error("GraphQL server unavailable");
    });

    const result = await executeGraphSearch({ query: "drones" }, ctx);

    expect(result.attachments).toHaveLength(1);
    expect(result.attachments[0].kind).toBe("error");
  });

  it("ErrorAttachment contains the error message", async () => {
    const errorMsg = "GraphQL server unavailable";
    const ctx = makeContext(async () => {
      throw new Error(errorMsg);
    });

    const result = await executeGraphSearch({ query: "drones" }, ctx);
    const attachment = result.attachments[0] as ErrorAttachment;

    expect(attachment.message).toMatch(errorMsg);
  });

  it("ErrorAttachment has a non-empty code field", async () => {
    const ctx = makeContext(async () => {
      throw new Error("search failed");
    });

    const result = await executeGraphSearch({ query: "drones" }, ctx);
    const attachment = result.attachments[0] as ErrorAttachment;

    expect(attachment.code).toBeTruthy();
  });

  it("does not throw — always returns ToolResult", async () => {
    const ctx = makeContext(async () => {
      throw new Error("unexpected");
    });

    await expect(
      executeGraphSearch({ query: "drones" }, ctx),
    ).resolves.toBeDefined();
  });
});

// ---------------------------------------------------------------------------
// executeEntityLookup — success cases
// ---------------------------------------------------------------------------

describe("executeEntityLookup — returns EntityDetailAttachment", () => {
  const entityId = "c360.ops.robotics.gcs.drone.001";

  it("returns attachment with kind='entity-detail'", async () => {
    const ctx = makeContext(async () => ({
      entity: {
        id: entityId,
        triples: [
          {
            subject: entityId,
            predicate: "status.operational.active",
            object: true,
          },
          { subject: entityId, predicate: "location.gps.lat", object: 32.7157 },
        ],
      },
    }));

    const result = await executeEntityLookup({ entityId }, ctx);

    expect(result.attachments).toHaveLength(1);
    expect(result.attachments[0].kind).toBe("entity-detail");
  });

  it("attachment entity.id matches the requested entityId", async () => {
    const ctx = makeContext(async () => ({
      entity: {
        id: entityId,
        triples: [],
      },
    }));

    const result = await executeEntityLookup({ entityId }, ctx);
    const attachment = result.attachments[0] as {
      kind: string;
      entity: { id: string };
    };

    expect(attachment.entity.id).toBe(entityId);
  });

  it("attachment entity includes label, type, and domain derived from entity ID", async () => {
    const ctx = makeContext(async () => ({
      entity: {
        id: entityId,
        triples: [],
      },
    }));

    const result = await executeEntityLookup({ entityId }, ctx);
    const attachment = result.attachments[0] as {
      kind: string;
      entity: { label: string; type: string; domain: string };
    };

    expect(attachment.entity.label).toBeTruthy();
    expect(attachment.entity.type).toBe("drone");
    expect(attachment.entity.domain).toBe("robotics");
  });

  it("attachment entity.properties is an array", async () => {
    const ctx = makeContext(async () => ({
      entity: {
        id: entityId,
        triples: [
          {
            subject: entityId,
            predicate: "status.operational.active",
            object: true,
          },
          { subject: entityId, predicate: "location.gps.lat", object: 32.7157 },
        ],
      },
    }));

    const result = await executeEntityLookup({ entityId }, ctx);
    const attachment = result.attachments[0] as {
      kind: string;
      entity: { properties: unknown[] };
    };

    expect(Array.isArray(attachment.entity.properties)).toBe(true);
    expect(attachment.entity.properties.length).toBeGreaterThan(0);
  });

  it("attachment entity.relationships is an array", async () => {
    const ctx = makeContext(async () => ({
      entity: {
        id: entityId,
        triples: [],
      },
    }));

    const result = await executeEntityLookup({ entityId }, ctx);
    const attachment = result.attachments[0] as {
      kind: string;
      entity: { relationships: unknown[] };
    };

    expect(Array.isArray(attachment.entity.relationships)).toBe(true);
  });

  it("textSummary mentions the entity label or id", async () => {
    const ctx = makeContext(async () => ({
      entity: {
        id: entityId,
        triples: [],
      },
    }));

    const result = await executeEntityLookup({ entityId }, ctx);

    expect(result.textSummary).toMatch(/drone|001|c360/i);
  });
});

// ---------------------------------------------------------------------------
// executeEntityLookup — entity not found
// ---------------------------------------------------------------------------

describe("executeEntityLookup — entity not found", () => {
  it("returns ErrorAttachment when entity is null in response", async () => {
    const ctx = makeContext(async () => ({
      entity: null,
    }));

    const result = await executeEntityLookup(
      {
        entityId: "nonexistent.entity.id.that.does.not.exist.001",
      },
      ctx,
    );

    expect(result.attachments[0].kind).toBe("error");
  });

  it("returns ErrorAttachment when entity is missing from response", async () => {
    const ctx = makeContext(async () => ({}));

    const result = await executeEntityLookup({ entityId: "x.x.x.x.x.x" }, ctx);

    expect(result.attachments[0].kind).toBe("error");
  });

  it("ErrorAttachment message mentions not found or entity id", async () => {
    const missingId = "c360.ops.robotics.gcs.drone.missing";
    const ctx = makeContext(async () => ({ entity: null }));

    const result = await executeEntityLookup({ entityId: missingId }, ctx);
    const attachment = result.attachments[0] as ErrorAttachment;

    expect(attachment.message).toMatch(/not found|missing|c360|drone/i);
  });
});

// ---------------------------------------------------------------------------
// executeEntityLookup — error handling
// ---------------------------------------------------------------------------

describe("executeEntityLookup — handles GraphQL error gracefully", () => {
  it("returns ErrorAttachment when GraphQL throws", async () => {
    const ctx = makeContext(async () => {
      throw new Error("Connection refused");
    });

    const result = await executeEntityLookup(
      {
        entityId: "c360.ops.robotics.gcs.drone.001",
      },
      ctx,
    );

    expect(result.attachments[0].kind).toBe("error");
  });

  it("ErrorAttachment contains the error message on GraphQL failure", async () => {
    const errorMsg = "Connection refused";
    const ctx = makeContext(async () => {
      throw new Error(errorMsg);
    });

    const result = await executeEntityLookup(
      {
        entityId: "c360.ops.robotics.gcs.drone.001",
      },
      ctx,
    );
    const attachment = result.attachments[0] as ErrorAttachment;

    expect(attachment.message).toMatch(errorMsg);
  });

  it("does not throw — always returns ToolResult", async () => {
    const ctx = makeContext(async () => {
      throw new Error("fatal");
    });

    await expect(
      executeEntityLookup({ entityId: "c360.ops.robotics.gcs.drone.001" }, ctx),
    ).resolves.toBeDefined();
  });
});

// ---------------------------------------------------------------------------
// Table-driven: executeGraphSearch result array shape
// ---------------------------------------------------------------------------

describe("executeGraphSearch — table-driven entity ID parsing", () => {
  const entityCases = [
    {
      id: "c360.ops.robotics.gcs.drone.alpha",
      expectedType: "drone",
      expectedDomain: "robotics",
    },
    {
      id: "c360.ops.maritime.coastal.vessel.beta",
      expectedType: "vessel",
      expectedDomain: "maritime",
    },
    {
      id: "c360.sec.border.surveillance.sensor.001",
      expectedType: "sensor",
      expectedDomain: "border",
    },
  ];

  it.each(entityCases)(
    "entity $id parsed to type=$expectedType domain=$expectedDomain",
    async ({ id, expectedType, expectedDomain }) => {
      const ctx = makeContext(async () => ({
        globalSearch: {
          entities: [{ id, triples: [] }],
          count: 1,
          duration_ms: 5,
          community_summaries: [],
          relationships: [],
        },
      }));

      const result = await executeGraphSearch({ query: "test" }, ctx);
      const attachment = result.attachments[0] as {
        kind: string;
        results: Array<{ id: string; type: string; domain: string }>;
      };

      expect(attachment.results[0].type).toBe(expectedType);
      expect(attachment.results[0].domain).toBe(expectedDomain);
    },
  );
});
